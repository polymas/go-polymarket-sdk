package clob

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

// orderedOrderV2 匹配当前 V2 Order 的 JSON 提交格式，字段顺序对齐 V2
// EIP-712 typehash 和官方 V2 SDK 的 order_to_json_v2 输出。
// orderedOrderV2 字段顺序对照 py-clob-client-v2/order_utils/model/order_data_v2.py
// 中的 order_to_json_v2() 输出顺序锁定（HMAC 签名敏感）：
//
//	salt, maker, signer, tokenId, makerAmount, takerAmount, side,
//	expiration, signatureType, timestamp, metadata, builder, signature
type orderedOrderV2 struct {
	Salt          int64  `json:"salt"`
	Maker         string `json:"maker"`
	Signer        string `json:"signer"`
	TokenId       string `json:"tokenId"`
	MakerAmount   string `json:"makerAmount"`
	TakerAmount   string `json:"takerAmount"`
	Side          string `json:"side"`       // "BUY" / "SELL"
	Expiration    string `json:"expiration"` // unix 秒字符串；GTC 订单为 "0"
	SignatureType int    `json:"signatureType"`
	Timestamp     string `json:"timestamp"` // 毫秒整数的字符串形式
	Metadata      string `json:"metadata"`  // 0x + 64 hex
	Builder       string `json:"builder"`   // 0x + 64 hex
	Signature     string `json:"signature"` // 0x + 130 hex
}

// orderRequestV2 是单笔订单的提交载荷。字段顺序与官方 to_dict 保持一致。
type orderRequestV2 struct {
	Order     orderedOrderV2 `json:"order"`
	Owner     string         `json:"owner"`
	OrderType string         `json:"orderType"`
	DeferExec bool           `json:"deferExec"`
	PostOnly  bool           `json:"postOnly"`
}

// dropOrderbookNotFoundRegex 匹配 CLOB v2 批量端点的顶层 400。实测中
// CLOB 只报原始 HTTP 子批的第一个坏 token，并拒绝整批。
var dropOrderbookNotFoundRegex = regexp.MustCompile(`orderbook\s+(\d+)\s+does\s+not\s+exist`)
var topLevelHTTP4xxRegex = regexp.MustCompile(`^HTTP 4\d\d:`)

const (
	// OrderMarketClosedStatus 是已确认的关闭市场终态，不代表交易所已接单。
	OrderMarketClosedStatus = "market_closed"
	// OrderNotSubmittedStatus 表示订单确定未提交。
	OrderNotSubmittedStatus = "not_submitted"
	// OrderUnknownStatus 表示请求可能已发出，但服务端结果不确定。
	OrderUnknownStatus = "unknown"
	// OrderServerRejectedStatus 是 HTTP 200 响应中的逐单业务拒绝。
	OrderServerRejectedStatus = "server_rejected"

	// Deprecated: use OrderMarketClosedStatus. Closed markets are no longer retried.
	OrderStrippedStatus = OrderMarketClosedStatus
)

type batchNotSubmittedError struct{ err error }

func (e *batchNotSubmittedError) Error() string { return e.err.Error() }
func (e *batchNotSubmittedError) Unwrap() error { return e.err }

func newBatchNotSubmittedError(format string, args ...interface{}) error {
	return &batchNotSubmittedError{err: fmt.Errorf(format, args...)}
}

type batchPostError struct {
	err              error
	expectedOrderIDs []types.Keccak256
	makingAmounts    []string
	takingAmounts    []string
	timing           types.OrderResponseTiming
}

type orderAmountOverride struct {
	maker *big.Int
	taker *big.Int
}

func (e *batchPostError) Error() string { return e.err.Error() }
func (e *batchPostError) Unwrap() error { return e.err }

// postOrdersBatchV2 始终返回与输入等长、同序的结果。
// orderbook 不存在时不重试：该 token 标记 market_closed，同一
// 原始 HTTP 子批的其他订单标记 not_submitted。
func (c *orderClientImpl) postOrdersBatchV2(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	isRetry ...bool,
) ([]types.OrderPostResponse, error) {
	return c.postOrdersBatchV2Context(context.Background(), orderArgsList, orderTypes, isRetry...)
}

func (c *orderClientImpl) postOrdersBatchV2Context(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType, isRetry ...bool) ([]types.OrderPostResponse, error) {
	return c.postOrdersBatchV2WithModeContext(ctx, orderArgsList, orderTypes, true, isRetry...)
}

func (c *orderClientImpl) postOrdersBatchV2Instant(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	isRetry ...bool,
) ([]types.OrderPostResponse, error) {
	return c.postOrdersBatchV2InstantContext(context.Background(), orderArgsList, orderTypes, isRetry...)
}

func (c *orderClientImpl) postOrdersBatchV2InstantContext(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType, isRetry ...bool) ([]types.OrderPostResponse, error) {
	return c.postOrdersBatchV2WithModeContext(ctx, orderArgsList, orderTypes, false, isRetry...)
}

func (c *orderClientImpl) postOrdersBatchV2WithMode(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	waitForResult bool,
	isRetry ...bool,
) ([]types.OrderPostResponse, error) {
	return c.postOrdersBatchV2WithModeContext(context.Background(), orderArgsList, orderTypes, waitForResult, isRetry...)
}

func (c *orderClientImpl) postOrdersBatchV2WithModeContext(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType, waitForResult bool, isRetry ...bool) ([]types.OrderPostResponse, error) {
	return resolveV2BatchAttempt(orderArgsList, orderTypes, func(a []types.OrderArgs, t []types.OrderType) ([]types.OrderPostResponse, error) {
		return c.postOrdersBatchV2OnceContext(ctx, a, t, waitForResult, isRetry...)
	})
}

func resolveV2BatchAttempt(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	postFn func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error),
) ([]types.OrderPostResponse, error) {
	n := len(orderArgsList)
	resp, err := postFn(orderArgsList, orderTypes)
	if err == nil {
		return normalizeV2BatchResponses(resp, n)
	}
	var postErr *batchPostError
	errors.As(err, &postErr)
	if len(resp) > 0 {
		aligned, alignErr := normalizeV2BatchResponses(resp, n)
		if alignErr != nil {
			return aligned, fmt.Errorf("%v; %w", alignErr, err)
		}
		return aligned, err
	}

	badTokens := extractBadOrderbookTokensFromErr(err)
	if len(badTokens) > 0 {
		badSet := make(map[string]struct{}, len(badTokens))
		for _, tokenID := range badTokens {
			badSet[tokenID] = struct{}{}
		}
		results := makeBatchStatusResults(n, OrderNotSubmittedStatus, "original HTTP batch was rejected because an orderbook does not exist")
		allClosed := true
		matched := 0
		for i, orderArgs := range orderArgsList {
			if _, closed := badSet[orderArgs.TokenID]; closed {
				results[i] = marketClosedOrderResult(orderArgs.TokenID)
				matched++
				continue
			}
			allClosed = false
		}
		if matched == 0 {
			results = applyBatchPostMetadata(results, postErr)
			return results, fmt.Errorf("CLOB reported closed orderbook(s) %v that are not present in the submitted batch: %w", badTokens, err)
		}
		results = applyBatchPostMetadata(results, postErr)
		if allClosed {
			return results, nil
		}
		return results, fmt.Errorf("CLOB rejected the original HTTP batch because orderbook(s) %v do not exist; other orders were not submitted: %w", badTokens, err)
	}

	status := OrderUnknownStatus
	message := "batch submission outcome is unknown: " + err.Error()
	var notSubmittedErr *batchNotSubmittedError
	if errors.As(err, &notSubmittedErr) || topLevelHTTP4xxRegex.MatchString(err.Error()) {
		status = OrderNotSubmittedStatus
		message = err.Error()
	}
	return applyBatchPostMetadata(makeBatchStatusResults(n, status, message), postErr), err
}

func applyBatchPostMetadata(results []types.OrderPostResponse, postErr *batchPostError) []types.OrderPostResponse {
	if postErr == nil {
		return results
	}
	for i := range results {
		if i < len(postErr.expectedOrderIDs) {
			results[i].ExpectedOrderID = postErr.expectedOrderIDs[i]
		}
		if i < len(postErr.makingAmounts) {
			results[i].MakingAmount = postErr.makingAmounts[i]
		}
		if i < len(postErr.takingAmounts) {
			results[i].TakingAmount = postErr.takingAmounts[i]
		}
		results[i].Timing = postErr.timing
	}
	return results
}

func normalizeV2BatchResponses(resp []types.OrderPostResponse, expected int) ([]types.OrderPostResponse, error) {
	results := makeBatchStatusResults(expected, OrderUnknownStatus, "CLOB response did not include a result for this order")
	limit := len(resp)
	if limit > expected {
		limit = expected
	}
	for i := 0; i < limit; i++ {
		results[i] = resp[i]
		if results[i].ErrorMsg != "" && results[i].Status == "" {
			results[i].Status = OrderServerRejectedStatus
		}
	}
	if len(resp) != expected {
		return results, fmt.Errorf("CLOB batch response count mismatch: got %d, want %d", len(resp), expected)
	}
	return results, nil
}

func makeBatchStatusResults(count int, status, message string) []types.OrderPostResponse {
	results := make([]types.OrderPostResponse, count)
	for i := range results {
		results[i] = types.OrderPostResponse{Status: status, ErrorMsg: message}
	}
	return results
}

func marketClosedOrderResult(tokenID string) types.OrderPostResponse {
	return types.OrderPostResponse{
		Status:   OrderMarketClosedStatus,
		ErrorMsg: fmt.Sprintf("orderbook %s does not exist; market treated as closed", tokenID),
	}
}

// extractBadOrderbookTokensFromErr 从 HTTP 400 错误里抠出 CLOB 报告的所有坏
// token。CLOB 实测一次报一个，但用 FindAllStringSubmatch 兼容未来后端把多个
// token 写在同一条响应里的情况（去重后返回）。
func extractBadOrderbookTokensFromErr(err error) []string {
	if err == nil {
		return nil
	}
	matches := dropOrderbookNotFoundRegex.FindAllStringSubmatch(err.Error(), -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		out = append(out, m[1])
	}
	return out
}

// postOrdersBatchV2Once 是 postOrdersBatchV2 的单次 HTTP 提交逻辑。
// 负责 negRisk 内部重试 + 真正的 HTTP POST。
func (c *orderClientImpl) postOrdersBatchV2Once(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	waitForResult bool,
	isRetry ...bool,
) ([]types.OrderPostResponse, error) {
	return c.postOrdersBatchV2OnceContext(context.Background(), orderArgsList, orderTypes, waitForResult, isRetry...)
}

func (c *orderClientImpl) postOrdersBatchV2OnceContext(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType, waitForResult bool, isRetry ...bool) ([]types.OrderPostResponse, error) {
	return c.postOrdersBatchV2OnceWithAmountsContext(ctx, orderArgsList, orderTypes, waitForResult, nil, isRetry...)
}

func (c *orderClientImpl) postOrdersBatchV2OnceWithAmounts(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	waitForResult bool,
	amountOverrides []orderAmountOverride,
	isRetry ...bool,
) ([]types.OrderPostResponse, error) {
	return c.postOrdersBatchV2OnceWithAmountsContext(context.Background(), orderArgsList, orderTypes, waitForResult, amountOverrides, isRetry...)
}

func (c *orderClientImpl) postOrdersBatchV2OnceWithAmountsContext(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType, waitForResult bool, amountOverrides []orderAmountOverride, isRetry ...bool) ([]types.OrderPostResponse, error) {
	batchStart := time.Now()
	isRetryCall := len(isRetry) > 0 && isRetry[0]
	if len(orderArgsList) == 0 {
		return []types.OrderPostResponse{}, nil
	}
	if len(orderArgsList) > 15 {
		return nil, newBatchNotSubmittedError("postOrdersBatchV2: batch size cannot exceed 15, got %d", len(orderArgsList))
	}
	if amountOverrides != nil && len(amountOverrides) != len(orderArgsList) {
		return nil, newBatchNotSubmittedError("amount override count mismatch: got %d, want %d", len(amountOverrides), len(orderArgsList))
	}

	negRisk := false
	if isRetryCall {
		negRisk = true
	}

	requestBody := make([]orderRequestV2, 0, len(orderArgsList))
	expectedOrderIDs := make([]types.Keccak256, 0, len(orderArgsList))

	// 非重试路径：并发预取本批中未传入、未缓存的 token 的 NegRisk 状态，把签名
	// 循环里逐个串行 GET /neg-risk 压成约 1 次往返的墙钟时间。重试路径强制 true，
	// 无需预取。
	if !isRetryCall {
		c.baseClient.prefetchNegRiskContext(ctx, orderArgsList)
	}

	for i, orderArgs := range orderArgsList {
		// type-3（POLY_1271）签名的 exchange 域取决于市场是否 NegRisk：
		// 非 retry 时按真实状态签对域；retry 时 negRisk 已被强制为 true，作为
		// 整批兜底（首次按真实值仍签错的极端情况下再统一改走 NegRisk Exchange
		// 重签）。真实状态优先取调用方传入的 orderArgs.NegRisk（上游多半已从
		// gamma/clob 市场数据拿到，零网络开销）；未传（nil）才现查 GET /neg-risk。
		perOrderNegRisk := negRisk
		if !isRetryCall {
			if orderArgs.NegRisk != nil {
				perOrderNegRisk = *orderArgs.NegRisk
				// 顺手把权威值喂进缓存，惠及后续 GetNegRisk / 重试路径。
				if canonical, canonicalErr := canonicalNegRiskTokenID(orderArgs.TokenID); canonicalErr == nil {
					c.baseClient.negRisk.set(canonical, perOrderNegRisk, negRiskSourceOrderArg)
				}
			} else {
				perOrderNegRisk, _ = c.baseClient.negRiskForTokenContext(ctx, orderArgs.TokenID)
			}
		}

		var signed *signing.V2SignedOrder
		var err error
		if amountOverrides == nil {
			signed, err = c.createSignedOrderV2(orderArgs, orderArgs.TickSize, perOrderNegRisk, orderTypes[i])
		} else {
			signed, err = c.createSignedOrderV2WithAmounts(
				orderArgs,
				amountOverrides[i].maker,
				amountOverrides[i].taker,
				perOrderNegRisk,
			)
		}
		if err != nil {
			return nil, newBatchNotSubmittedError("订单 %d token=%s 本地构造/签名失败: %w", i+1, orderArgs.TokenID, err)
		}
		expectedOrderID, err := c.v2OrderID(signed, perOrderNegRisk)
		if err != nil {
			return nil, newBatchNotSubmittedError("订单 %d token=%s 本地计算 order hash 失败: %w", i+1, orderArgs.TokenID, err)
		}
		expectedOrderIDs = append(expectedOrderIDs, expectedOrderID)

		requestBody = append(requestBody, orderRequestV2{
			Order:     signedOrderToJSONV2(signed, orderArgs.Side, orderArgs.Expiration),
			Owner:     c.baseClient.deriveCreds.Key,
			OrderType: string(orderTypes[i]),
			DeferExec: orderArgs.DeferExec,
			PostOnly:  orderArgs.PostOnly,
		})
	}

	if len(requestBody) == 0 {
		return nil, newBatchNotSubmittedError("no valid orders to post")
	}

	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, newBatchNotSubmittedError("failed to marshal request body: %w", err)
	}
	// 匹配 Python json.dumps 的空格格式（HMAC 签名要求同一字节序列）
	bodyJSONStr := string(bodyJSON)
	bodyJSONStr = regexp.MustCompile(`":(\S)`).ReplaceAllString(bodyJSONStr, `": $1`)
	bodyJSONStr = regexp.MustCompile(`,(")`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSONStr = regexp.MustCompile(`,(\{|\[)`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSON = []byte(bodyJSONStr)

	requestArgs := &types.RequestArgs{
		Method:      "POST",
		RequestPath: internal.PostOrders,
	}
	headers, err := internal.CreateLevel2HeadersWithBody(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, requestBody)
	if err != nil {
		return nil, newBatchNotSubmittedError("failed to create headers: %w", err)
	}

	postStart := time.Now()
	responseBody, err := http.PostRawContext(ctx, c.baseClient.baseURL, internal.PostOrders, bodyJSON,
		http.WithHeaders(headers), http.WithService("clob"), http.WithAmbiguousOnTimeout("post orders"))
	postDuration := time.Since(postStart)
	if err != nil {
		if waitForResult && !topLevelHTTP4xxRegex.MatchString(err.Error()) {
			if reconciled, complete := c.reconcileAmbiguousPostContext(
				ctx, expectedOrderIDs, requestBody, batchStart, postDuration,
			); len(reconciled) > 0 {
				if complete {
					return reconciled, nil
				}
				return reconciled, newBatchPostError(
					err,
					expectedOrderIDs,
					requestBody,
					reconciled[0].Timing,
				)
			}
		}
		return nil, newBatchPostError(
			err, expectedOrderIDs, requestBody, types.OrderResponseTiming{
				PostDuration:  postDuration,
				TotalDuration: time.Since(batchStart),
			},
		)
	}

	var resp []types.OrderPostResponse
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &resp); err != nil {
			if waitForResult {
				if reconciled, complete := c.reconcileAmbiguousPostContext(
					ctx, expectedOrderIDs, requestBody, batchStart, postDuration,
				); len(reconciled) > 0 {
					if complete {
						return reconciled, nil
					}
					return reconciled, newBatchPostError(
						fmt.Errorf("failed to parse response: %w", err),
						expectedOrderIDs,
						requestBody,
						reconciled[0].Timing,
					)
				}
			}
			return nil, newBatchPostError(
				fmt.Errorf("failed to parse response: %w", err),
				expectedOrderIDs,
				requestBody,
				types.OrderResponseTiming{
					PostDuration:  postDuration,
					TotalDuration: time.Since(batchStart),
				},
			)
		}
	}
	for i := range resp {
		if i < len(expectedOrderIDs) {
			resp[i].ExpectedOrderID = expectedOrderIDs[i]
		}
	}
	// 对可能签错 exchange 域的订单执行一次 NegRisk 重签重试。
	failedOrders := make([]int, 0)
	for i, result := range resp {
		// 签名域选错（含 1271 钱包的 "invalid POLY_1271 signature: ..."）时
		// 改用 NegRisk Exchange 域重签重试，判据见 signatureRetryNeeded。
		if signatureRetryNeeded(result.ErrorMsg) {
			failedOrders = append(failedOrders, i)
		} else if result.ErrorMsg != "" {
			internal.LogError("V2 订单 %d 创建失败: %s", i+1, result.ErrorMsg)
		}
	}
	if len(failedOrders) > 0 && !isRetryCall {
		retryArgs := make([]types.OrderArgs, 0, len(failedOrders))
		retryTypes := make([]types.OrderType, 0, len(failedOrders))
		var retryOverrides []orderAmountOverride
		if amountOverrides != nil {
			retryOverrides = make([]orderAmountOverride, 0, len(failedOrders))
		}
		for _, idx := range failedOrders {
			retryArgs = append(retryArgs, orderArgsList[idx])
			retryTypes = append(retryTypes, orderTypes[idx])
			if amountOverrides != nil {
				retryOverrides = append(retryOverrides, amountOverrides[idx])
			}
		}
		retryResults, err := resolveV2BatchAttempt(retryArgs, retryTypes, func(a []types.OrderArgs, t []types.OrderType) ([]types.OrderPostResponse, error) {
			return c.postOrdersBatchV2OnceWithAmountsContext(ctx, a, t, waitForResult, retryOverrides, true)
		})
		if err != nil {
			internal.LogError("V2 重试订单失败: %v", err)
		} else {
			for j, idx := range failedOrders {
				if j < len(retryResults) && (retryResults[j].ErrorMsg == "" || retryResults[j].Status == OrderMarketClosedStatus) {
					resp[idx] = retryResults[j]
				}
			}
		}
	}

	settlementDuration := time.Duration(0)
	needsSettlement := make([]bool, len(resp))
	if waitForResult {
		for i := range resp {
			needsSettlement[i] = len(resp[i].TradeIDs) > 0 && len(resp[i].TransactionsHashes) == 0
		}
		settlementStart := time.Now()
		settlementCtx, cancel := context.WithTimeout(ctx, orderSettlementTimeout)
		_ = c.resolveOrderTransactionsContext(settlementCtx, resp)
		cancel()
		settlementDuration = time.Since(settlementStart)
	}
	for i := range resp {
		resp[i].Timing.PostDuration = postDuration
		if needsSettlement[i] && resp[i].Timing.SettlementWaitDuration == 0 {
			resp[i].Timing.SettlementWaitDuration = settlementDuration
		}
		resp[i].Timing.TotalDuration = time.Since(batchStart)
	}
	internal.LogDebug(
		"V2 批量下单返回耗时: orders=%d post=%v settlement=%v total=%v",
		len(resp), postDuration, settlementDuration, time.Since(batchStart),
	)

	return resp, nil
}

func newBatchPostError(
	err error,
	expectedOrderIDs []types.Keccak256,
	requestBody []orderRequestV2,
	timing types.OrderResponseTiming,
) *batchPostError {
	postErr := &batchPostError{
		err:              err,
		expectedOrderIDs: append([]types.Keccak256(nil), expectedOrderIDs...),
		makingAmounts:    make([]string, len(requestBody)),
		takingAmounts:    make([]string, len(requestBody)),
		timing:           timing,
	}
	for i := range requestBody {
		postErr.makingAmounts[i] = requestBody[i].Order.MakerAmount
		postErr.takingAmounts[i] = requestBody[i].Order.TakerAmount
	}
	return postErr
}

func (c *orderClientImpl) reconcileAmbiguousPost(
	expectedOrderIDs []types.Keccak256,
	requestBody []orderRequestV2,
	batchStart time.Time,
	postDuration time.Duration,
) ([]types.OrderPostResponse, bool) {
	return c.reconcileAmbiguousPostContext(context.Background(), expectedOrderIDs, requestBody, batchStart, postDuration)
}

func (c *orderClientImpl) reconcileAmbiguousPostContext(
	parent context.Context,
	expectedOrderIDs []types.Keccak256,
	requestBody []orderRequestV2,
	batchStart time.Time,
	postDuration time.Duration,
) ([]types.OrderPostResponse, bool) {
	ctx, cancel := context.WithTimeout(parent, orderSettlementTimeout)
	defer cancel()
	deadline, _ := ctx.Deadline()
	orders, timing, _ := c.reconcileExpectedOrders(ctx, expectedOrderIDs, orderSettlementPollInterval)
	if len(orders) == 0 {
		return nil, false
	}

	results := makeBatchStatusResults(
		len(expectedOrderIDs),
		OrderUnknownStatus,
		"submission outcome remains unknown after order-hash reconciliation",
	)
	for i, expectedOrderID := range expectedOrderIDs {
		results[i].ExpectedOrderID = expectedOrderID
		results[i].Timing = timing
		results[i].Timing.PostDuration = postDuration
		results[i].Timing.TotalDuration = time.Since(batchStart)
		order, found := orders[expectedOrderID]
		if !found {
			continue
		}
		results[i].Success = true
		results[i].OrderID = order.OrderID
		results[i].RawStatus = order.RawStatus
		if results[i].RawStatus == "" {
			results[i].RawStatus = order.Status
		}
		results[i].Status = normalizeOrderStatus(order.Status)
		results[i].ErrorMsg = ""
		results[i].TradeIDs = append([]string(nil), order.AssociateTrades...)
		results[i].Timing.Reconciled = true
		if i < len(requestBody) {
			results[i].MakingAmount = requestBody[i].Order.MakerAmount
			results[i].TakingAmount = requestBody[i].Order.TakerAmount
		}
	}

	settlementStart := time.Now()
	remaining := time.Until(deadline)
	if remaining < 0 {
		remaining = 0
	}
	c.resolveOrderTransactions(results, remaining)
	settlementDuration := time.Since(settlementStart)
	for i := range results {
		if len(results[i].TradeIDs) > 0 && results[i].Timing.SettlementWaitDuration == 0 {
			results[i].Timing.SettlementWaitDuration = settlementDuration
		}
		results[i].Timing.TotalDuration = time.Since(batchStart)
	}
	internal.LogWarn(
		"下单响应不明确后按本地 order hash 对账: found=%d total=%d wait=%v polls=%d query_errors=%d",
		len(orders), len(expectedOrderIDs), timing.ReconciliationWaitDuration,
		timing.ReconciliationPollCount, timing.ReconciliationQueryErrors,
	)
	return results, len(orders) == len(uniqueOrderIDs(expectedOrderIDs))
}

func normalizeOrderStatus(status string) string {
	return string(types.NormalizeOrderStatus(status))
}

func (c *orderClientImpl) v2OrderID(signed *signing.V2SignedOrder, negRisk bool) (types.Keccak256, error) {
	exchangeAddrHex := internal.PolygonExchangeV2
	if negRisk {
		exchangeAddrHex = internal.PolygonNegRiskExchangeV2
	}
	digest, err := signing.V2OrderDigest(
		&signed.V2Order,
		big.NewInt(int64(c.baseClient.web3Client.GetChainID())),
		common.HexToAddress(exchangeAddrHex),
	)
	if err != nil {
		return "", err
	}
	return types.Keccak256(digest.Hex()), nil
}

// createSignedOrderV2 构建并签名一笔 V2 订单，返回 signing.V2SignedOrder。
func (c *orderClientImpl) createSignedOrderV2(
	orderArgs types.OrderArgs,
	tickSize types.TickSize,
	negRisk bool,
	_ types.OrderType, // V2 没有 expiration 字段，orderType 只影响提交时的 orderType 字段
) (*signing.V2SignedOrder, error) {
	tickSizeFloat, err := parseRequiredTickSize(tickSize)
	if err != nil {
		return nil, fmt.Errorf("invalid tick size: %w", err)
	}
	if orderArgs.Price < tickSizeFloat || orderArgs.Price > 1.0-tickSizeFloat {
		return nil, fmt.Errorf("price (%.6f) out of range [%.6f, %.6f]", orderArgs.Price, tickSizeFloat, 1.0-tickSizeFloat)
	}

	makerAmount, takerAmount, err := c.calculateOrderAmounts(orderArgs.Side, orderArgs.Size, orderArgs.Price, tickSize)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate amounts: %w", err)
	}
	return c.createSignedOrderV2WithAmounts(orderArgs, makerAmount, takerAmount, negRisk)
}

func (c *orderClientImpl) createSignedOrderV2WithAmounts(
	orderArgs types.OrderArgs,
	makerAmount *big.Int,
	takerAmount *big.Int,
	negRisk bool,
) (*signing.V2SignedOrder, error) {
	if makerAmount == nil || makerAmount.Sign() <= 0 || takerAmount == nil || takerAmount.Sign() <= 0 {
		return nil, fmt.Errorf("maker and taker amounts must be positive")
	}

	baseAddr := string(c.baseClient.web3Client.GetBaseAddress())
	makerAddr := baseAddr
	signerAddr := baseAddr
	switch c.baseClient.signatureType {
	case types.ProxySignatureType, types.SafeSignatureType:
		// Proxy/Safe：maker = proxy（资金来源），signer = EOA（API key 持有者）
		makerAddr = string(c.baseClient.proxyAddress)
	case types.Poly1271SignatureType:
		// V2 POLY_1271 (=3) 语义：合约钱包通过 EIP-1271 验证签名，
		// 因此 signer 字段必须 = funder/proxy 地址，不能是 EOA。
		// 对照 py-clob-client-v2/order_builder/builder.py 的 _v2_order_signer：
		//   if signature_type == POLY_1271: return funder
		// 不这么做 CLOB 会报 "the order signer address has to be the
		// address of the API KEY"（v2 后端对 1271 类型走另一套校验）。
		makerAddr = string(c.baseClient.proxyAddress)
		signerAddr = string(c.baseClient.proxyAddress)
	}

	var side signing.V2OrderSide
	if orderArgs.Side == types.OrderSideBUY {
		side = signing.V2SideBUY
	} else {
		side = signing.V2SideSELL
	}

	exchangeAddrHex := internal.PolygonExchangeV2
	if negRisk {
		exchangeAddrHex = internal.PolygonNegRiskExchangeV2
	}

	chainID := big.NewInt(int64(c.baseClient.web3Client.GetChainID()))
	return signing.BuildSignedV2OrderWithSigner(
		c.baseClient.web3Client.GetSigner(),
		&signing.V2OrderData{
			Maker:         makerAddr,
			Signer:        signerAddr,
			TokenID:       orderArgs.TokenID,
			MakerAmount:   makerAmount.String(),
			TakerAmount:   takerAmount.String(),
			Side:          side,
			SignatureType: c.baseClient.signatureType,
			// TimestampMS=0 → BuildSignedV2Order 自动填当前时间
		},
		chainID,
		common.HexToAddress(exchangeAddrHex),
	)
}

// signedOrderToJSONV2 把 V2SignedOrder 转换成请求体 JSON 结构。
//
// expiration 取自 OrderArgs.Expiration（unix 秒）：0 表示 GTC（不过期），
// 非 0 表示 GTD 订单的截止时间。注意：expiration 仅出现在 wire body 里，
// 不是 V2 EIP-712 签名结构的一部分（见 docs.polymarket.com 的 post-multiple-orders
// 说明："expiration remains in the POST /order wire body for GTD/order-expiry
// handling, but it is not part of the CLOB V2 EIP-712 signed order struct"）。
func signedOrderToJSONV2(s *signing.V2SignedOrder, side types.OrderSide, expiration int64) orderedOrderV2 {
	return orderedOrderV2{
		Salt:          s.Salt.Int64(),
		Maker:         s.Maker.Hex(),
		Signer:        s.Signer.Hex(),
		TokenId:       s.TokenID.String(),
		MakerAmount:   s.MakerAmount.String(),
		TakerAmount:   s.TakerAmount.String(),
		Side:          string(side),
		Expiration:    strconv.FormatInt(expiration, 10),
		SignatureType: int(s.SignatureType),
		Timestamp:     s.TimestampMS.String(),
		Metadata:      "0x" + hex.EncodeToString(s.Metadata[:]),
		Builder:       "0x" + hex.EncodeToString(s.Builder[:]),
		Signature:     "0x" + hex.EncodeToString(s.Signature),
	}
}
