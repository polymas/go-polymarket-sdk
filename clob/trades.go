package clob

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	polhttp "github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	orderSettlementTimeout      = 30 * time.Second
	orderSettlementPollInterval = 250 * time.Millisecond
	tradeQueryConcurrency       = 4
)

// GetOrder 按官方订单哈希查询单个订单。该端点与只返回活跃订单列表的
// GET /data/orders 语义不同，可用于下单结果不明确时对账。
func (c *orderClientImpl) GetOrder(orderID types.Keccak256) (*types.OpenOrder, error) {
	if err := orderID.Validate(); err != nil {
		return nil, fmt.Errorf("invalid order ID: %w", err)
	}

	path := internal.GetOrder + orderID.String()
	headers, err := c.level2GETHeaders(path)
	if err != nil {
		return nil, err
	}
	order, err := polhttp.Get[types.OpenOrder](c.baseClient.baseURL, path, nil, polhttp.WithHeaders(headers))
	if err != nil {
		return nil, fmt.Errorf("failed to get order %s: %w", orderID, err)
	}
	if order == nil || order.OrderID == "" {
		return nil, fmt.Errorf("order %s was not present in the CLOB response", orderID)
	}
	if order.OrderID != orderID {
		return nil, fmt.Errorf("CLOB returned order %s while querying %s", order.OrderID, orderID)
	}
	return order, nil
}

// GetTrades 获取当前认证用户的成交记录，并按照官方 next_cursor 自动分页。
func (c *orderClientImpl) GetTrades(params *types.ClobTradeParams) ([]types.ClobTrade, error) {
	query := tradeQueryParams(params)
	allTrades := make([]types.ClobTrade, 0)
	nextCursor := "MA=="

	for nextCursor != internal.EndCursor {
		query["next_cursor"] = nextCursor
		page, err := c.getTradesPage(query)
		if err != nil {
			return nil, err
		}
		allTrades = append(allTrades, page.Data...)
		if page.NextCursor == "" || page.NextCursor == nextCursor {
			break
		}
		nextCursor = page.NextCursor
	}
	return allTrades, nil
}

func tradeQueryParams(params *types.ClobTradeParams) map[string]string {
	query := make(map[string]string)
	if params == nil {
		return query
	}
	if params.ID != "" {
		query["id"] = params.ID
	}
	if params.MakerAddress != "" {
		query["maker_address"] = params.MakerAddress.String()
	}
	if params.Market != (types.ConditionID{}) {
		query["market"] = params.Market.String()
	}
	if params.AssetID != (types.TokenID{}) {
		query["asset_id"] = params.AssetID.String()
	}
	if params.Before > 0 {
		query["before"] = strconv.FormatInt(params.Before, 10)
	}
	if params.After > 0 {
		query["after"] = strconv.FormatInt(params.After, 10)
	}
	return query
}

func (c *orderClientImpl) getTradesPage(query map[string]string) (*types.PaginatedResponse[types.ClobTrade], error) {
	headers, err := c.level2GETHeaders(internal.Trades)
	if err != nil {
		return nil, err
	}
	page, err := polhttp.Get[types.PaginatedResponse[types.ClobTrade]](
		c.baseClient.baseURL,
		internal.Trades,
		query,
		polhttp.WithHeaders(headers),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}
	return page, nil
}

func (c *orderClientImpl) level2GETHeaders(path string) (map[string]string, error) {
	if c.deriveCreds == nil || c.deriveCreds.Key == "" || c.deriveCreds.Secret == "" || c.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("complete API credentials are required")
	}
	headers, err := internal.CreateLevel2Headers(c.web3Client.GetSigner(), c.deriveCreds, &types.RequestArgs{
		Method:      "GET",
		RequestPath: path,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create authentication headers: %w", err)
	}
	return headers, nil
}

// WaitForOrderFillSettlement 查询响应中的 tradeIDs，直到每笔成交都取得交易哈希
// 或明确失败。ctx 决定调用方允许等待的最长时间。
func (c *orderClientImpl) WaitForOrderFillSettlement(
	ctx context.Context,
	response types.OrderPostResponse,
) (*types.OrderFillSettlement, error) {
	start := time.Now()
	if len(response.TransactionsHashes) > 0 {
		state := response.SettlementState
		if state == "" {
			state = types.OrderSettlementMatched
		}
		return &types.OrderFillSettlement{
			State:              state,
			Trades:             append([]types.ClobTrade(nil), response.ResolvedTrades...),
			TransactionsHashes: append([]types.Keccak256(nil), response.TransactionsHashes...),
			Timing:             response.Timing,
		}, nil
	}
	if len(response.TradeIDs) == 0 {
		return &types.OrderFillSettlement{
			State:              response.SettlementState,
			Trades:             append([]types.ClobTrade(nil), response.ResolvedTrades...),
			TransactionsHashes: append([]types.Keccak256(nil), response.TransactionsHashes...),
		}, nil
	}

	trades, timing, err := c.waitForTradeIDs(ctx, response.TradeIDs, orderSettlementPollInterval)
	report := settlementForTradeIDs(response.TradeIDs, trades, timing)
	report.Timing.SettlementWaitDuration = time.Since(start)
	if errors.Is(err, context.DeadlineExceeded) {
		report.State = types.OrderSettlementTimeout
		report.Timing.SettlementTimedOut = true
	}
	return report, err
}

// resolveOrderTransactions 按官方 SDK 的兼容语义补全 matched 响应：如果服务端
// 只给出 tradeIDs，则在一个批次级截止时间内等待交易哈希或明确失败。下单已经
// 成功后，查询失败或超时都不会改写为提交失败。
func (c *orderClientImpl) resolveOrderTransactions(responses []types.OrderPostResponse, timeout time.Duration) {
	tradeIDs := make([]string, 0)
	for i := range responses {
		if len(responses[i].TransactionsHashes) == 0 {
			tradeIDs = append(tradeIDs, responses[i].TradeIDs...)
		}
	}
	tradeIDs = uniqueStrings(tradeIDs)
	if len(tradeIDs) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	trades, timing, err := c.waitForTradeIDs(ctx, tradeIDs, orderSettlementPollInterval)
	timing.SettlementWaitDuration = time.Since(start)
	if errors.Is(err, context.DeadlineExceeded) {
		timing.SettlementTimedOut = true
	}

	for i := range responses {
		if len(responses[i].TransactionsHashes) > 0 || len(responses[i].TradeIDs) == 0 {
			continue
		}
		report := settlementForTradeIDs(responses[i].TradeIDs, trades, timing)
		orderTimedOut := timing.SettlementTimedOut && !allTradeIDsResolved(responses[i].TradeIDs, trades)
		if orderTimedOut {
			report.State = types.OrderSettlementTimeout
		}
		responses[i].TransactionsHashes = report.TransactionsHashes
		responses[i].ResolvedTrades = report.Trades
		responses[i].SettlementState = report.State
		responses[i].Timing.SettlementWaitDuration = timing.SettlementWaitDuration
		responses[i].Timing.PollCount = timing.PollCount
		responses[i].Timing.QueryErrors = timing.QueryErrors
		responses[i].Timing.SettlementTimedOut = orderTimedOut
	}

	if timing.SettlementTimedOut {
		internal.LogWarn(
			"异步成交查询超时: trades=%d wait=%v polls=%d query_errors=%d",
			len(tradeIDs), timing.SettlementWaitDuration, timing.PollCount, timing.QueryErrors,
		)
	}
}

func (c *orderClientImpl) waitForTradeIDs(
	ctx context.Context,
	tradeIDs []string,
	pollInterval time.Duration,
) (map[string]types.ClobTrade, types.OrderResponseTiming, error) {
	ids := uniqueStrings(tradeIDs)
	latest := make(map[string]types.ClobTrade, len(ids))
	resolved := make(map[string]struct{}, len(ids))
	timing := types.OrderResponseTiming{}
	if len(ids) == 0 {
		return latest, timing, nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return latest, timing, err
		}
		timing.PollCount++

		pending := make([]string, 0, len(ids)-len(resolved))
		for _, id := range ids {
			if _, ok := resolved[id]; !ok {
				pending = append(pending, id)
			}
		}

		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, tradeQueryConcurrency)
		for _, id := range pending {
			id := id
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}

				trade, found, err := c.getTradeByID(id)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					timing.QueryErrors++
					return
				}
				if !found {
					return
				}
				latest[id] = trade
				if tradeExecutionResolved(trade) {
					resolved[id] = struct{}{}
				}
			}()
		}
		wg.Wait()

		if len(resolved) == len(ids) {
			return latest, timing, nil
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return latest, timing, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *orderClientImpl) reconcileExpectedOrders(
	ctx context.Context,
	expectedOrderIDs []types.Keccak256,
	pollInterval time.Duration,
) (map[types.Keccak256]types.OpenOrder, types.OrderResponseTiming, error) {
	ids := uniqueOrderIDs(expectedOrderIDs)
	found := make(map[types.Keccak256]types.OpenOrder, len(ids))
	timing := types.OrderResponseTiming{}
	if len(ids) == 0 {
		return found, timing, nil
	}

	start := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			timing.ReconciliationWaitDuration = time.Since(start)
			timing.ReconciliationTimedOut = errors.Is(err, context.DeadlineExceeded)
			return found, timing, err
		}
		timing.ReconciliationPollCount++

		pending := make([]types.Keccak256, 0, len(ids)-len(found))
		for _, id := range ids {
			if _, ok := found[id]; !ok {
				pending = append(pending, id)
			}
		}

		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, tradeQueryConcurrency)
		for _, id := range pending {
			id := id
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}

				order, err := c.GetOrder(id)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					timing.ReconciliationQueryErrors++
					return
				}
				if order != nil && order.OrderID == id {
					found[id] = *order
				}
			}()
		}
		wg.Wait()
		if len(found) == len(ids) {
			timing.Reconciled = true
			timing.ReconciliationWaitDuration = time.Since(start)
			return found, timing, nil
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			timing.ReconciliationTimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
			timing.ReconciliationWaitDuration = time.Since(start)
			return found, timing, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *orderClientImpl) getTradeByID(id string) (types.ClobTrade, bool, error) {
	page, err := c.getTradesPage(map[string]string{
		"id":          id,
		"next_cursor": "MA==",
	})
	if err != nil {
		return types.ClobTrade{}, false, err
	}
	for _, trade := range page.Data {
		if trade.ID == id {
			return trade, true, nil
		}
	}
	return types.ClobTrade{}, false, nil
}

func tradeExecutionResolved(trade types.ClobTrade) bool {
	return normalizeTradeStatus(trade.Status) == types.ClobTradeStatusFailed || trade.TransactionHash != ""
}

func allTradeIDsResolved(tradeIDs []string, trades map[string]types.ClobTrade) bool {
	ids := uniqueStrings(tradeIDs)
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		trade, ok := trades[id]
		if !ok || !tradeExecutionResolved(trade) {
			return false
		}
	}
	return true
}

func normalizeTradeStatus(status types.ClobTradeStatus) types.ClobTradeStatus {
	value := strings.ToUpper(string(status))
	value = strings.TrimPrefix(value, "TRADE_STATUS_")
	return types.ClobTradeStatus(value)
}

func settlementForTradeIDs(
	tradeIDs []string,
	trades map[string]types.ClobTrade,
	timing types.OrderResponseTiming,
) *types.OrderFillSettlement {
	report := &types.OrderFillSettlement{State: types.OrderSettlementMatched, Timing: timing}
	seenHashes := make(map[types.Keccak256]struct{})
	allMined := len(tradeIDs) > 0
	allConfirmed := len(tradeIDs) > 0

	for _, id := range uniqueStrings(tradeIDs) {
		trade, ok := trades[id]
		if !ok {
			allMined = false
			allConfirmed = false
			continue
		}
		report.Trades = append(report.Trades, trade)
		status := normalizeTradeStatus(trade.Status)
		if status == types.ClobTradeStatusFailed {
			report.State = types.OrderSettlementFailed
		}
		if status != types.ClobTradeStatusMined && status != types.ClobTradeStatusConfirmed {
			allMined = false
		}
		if status != types.ClobTradeStatusConfirmed {
			allConfirmed = false
		}
		if trade.TransactionHash != "" {
			if _, exists := seenHashes[trade.TransactionHash]; !exists {
				seenHashes[trade.TransactionHash] = struct{}{}
				report.TransactionsHashes = append(report.TransactionsHashes, trade.TransactionHash)
			}
		}
	}

	if report.State != types.OrderSettlementFailed {
		switch {
		case allConfirmed:
			report.State = types.OrderSettlementConfirmed
		case allMined:
			report.State = types.OrderSettlementMined
		}
	}
	return report
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueOrderIDs(values []types.Keccak256) []types.Keccak256 {
	seen := make(map[types.Keccak256]struct{}, len(values))
	result := make([]types.Keccak256, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
