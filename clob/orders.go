package clob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/polymas/go-polymarket-sdk/internal"
	http "github.com/polymas/go-polymarket-sdk/internal/transport"
	"github.com/polymas/go-polymarket-sdk/types"
)

// MaxCancelOrdersPerRequest 是官方 DELETE /orders 当前允许的最大订单数。
const MaxCancelOrdersPerRequest = 3000

// GetOrders 获取活跃订单
func (c *orderClientImpl) GetOrders(orderID *types.Keccak256, conditionID *types.Keccak256, tokenID *string) ([]types.OpenOrder, error) {
	return c.GetOrdersContext(context.Background(), orderID, conditionID, tokenID)
}

// GetOrdersContext is GetOrders with caller-controlled cancellation/deadline.
func (c *orderClientImpl) GetOrdersContext(ctx context.Context, orderID *types.Keccak256, conditionID *types.Keccak256, tokenID *string) ([]types.OpenOrder, error) {
	// Validate API credentials
	if c.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.deriveCreds.Key == "" || c.deriveCreds.Secret == "" || c.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.deriveCreds.Key != "", c.deriveCreds.Secret != "", c.deriveCreds.Passphrase != "")
	}

	params := make(map[string]string)
	if orderID != nil {
		params["id"] = string(*orderID)
	}
	if conditionID != nil {
		params["market"] = string(*conditionID)
	}
	if tokenID != nil {
		params["asset_id"] = *tokenID
	}

	// Set up authentication headers (same as Python version - set once, reuse)
	requestArgs := &types.RequestArgs{
		Method:      "GET",
		RequestPath: internal.Orders,
		Body:        nil, // GET request has no body
	}

	headers, err := internal.CreateLevel2Headers(c.web3Client.GetSigner(), c.deriveCreds, requestArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	var allOrders []types.OpenOrder
	nextCursor := "MA=="

	for nextCursor != internal.EndCursor {
		params["next_cursor"] = nextCursor

		response, err := http.GetContext[types.PaginatedResponse[types.OpenOrder]](ctx, c.baseClient.baseURL, internal.Orders, params, http.WithHeaders(headers), http.WithService("clob"))
		if err != nil {
			return nil, fmt.Errorf("failed to get orders: %w", err)
		}

		allOrders = append(allOrders, response.Data...)
		nextCursor = response.NextCursor
	}

	return allOrders, nil
}

// CreateAndPostOrders 创建、签名并提交当前 V2 订单。
// 如果订单数量超过15个，将自动分批提交，每批最多15个订单
// 每笔 OrderArgs 必须显式携带 TickSize；SDK 不在下单热路径请求 /tick-size。
// 同一批订单可以使用不同 tick size，并逐笔完成价格范围校验和金额计算。
// Size 必须不小于默认值 5；SDK 只报错，不会自动改大订单数量。
//
// 返回契约：本地预校验失败时返回 nil + error，且不提交订单。
// 一旦开始提交，results 恒与 orderArgsList 等长同序；即使某个 HTTP
// 子批失败，也会用 market_closed / not_submitted / unknown 明确每个位置。
// HTTP 200 的逐单业务拒绝保留在 results 中，不转换为整批 error。
func (c *orderClientImpl) CreateAndPostOrders(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
) ([]types.OrderPostResponse, error) {
	return c.CreateAndPostOrdersContext(context.Background(), orderArgsList, orderTypes)
}

// CreateAndPostOrdersContext is CreateAndPostOrders with caller-controlled
// cancellation/deadline for both submission and reconciliation.
func (c *orderClientImpl) CreateAndPostOrdersContext(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType) ([]types.OrderPostResponse, error) {
	return c.createAndPostOrdersWith(orderArgsList, orderTypes, func(a []types.OrderArgs, t []types.OrderType, retry ...bool) ([]types.OrderPostResponse, error) {
		return c.postOrdersBatchV2Context(ctx, a, t, retry...)
	})
}

// CreateAndPostOrdersInstant 只等待 CLOB 的 POST /orders 响应，不等待异步
// trade settlement，也不在网络结果不明确时自动轮询订单。返回的 unknown 或
// NeedsSettlement 结果可稍后传给 AwaitOrderResults。
func (c *orderClientImpl) CreateAndPostOrdersInstant(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
) ([]types.OrderPostResponse, error) {
	return c.CreateAndPostOrdersInstantContext(context.Background(), orderArgsList, orderTypes)
}

// CreateAndPostOrdersInstantContext is the immediate-return variant with a
// caller-controlled submission deadline. A timeout is reported as ambiguous
// and is never automatically resent.
func (c *orderClientImpl) CreateAndPostOrdersInstantContext(ctx context.Context, orderArgsList []types.OrderArgs, orderTypes []types.OrderType) ([]types.OrderPostResponse, error) {
	return c.createAndPostOrdersWith(orderArgsList, orderTypes, func(a []types.OrderArgs, t []types.OrderType, retry ...bool) ([]types.OrderPostResponse, error) {
		return c.postOrdersBatchV2InstantContext(ctx, a, t, retry...)
	})
}

// CreateAndPostOrdersAndWait 提交后自动完成 unknown order-hash 对账和异步成交
// 查询。ctx 可以设置更短 deadline；未设置时 SDK 最多等待 30 秒。
func (c *orderClientImpl) CreateAndPostOrdersAndWait(
	ctx context.Context,
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
) ([]types.OrderPostResponse, error) {
	results, submitErr := c.CreateAndPostOrdersInstantContext(ctx, orderArgsList, orderTypes)
	if len(results) == 0 {
		return results, submitErr
	}
	awaited, awaitErr := c.AwaitOrderResults(ctx, results)
	if awaitErr != nil {
		return awaited, errors.Join(submitErr, awaitErr)
	}

	var ambiguousErr *batchPostError
	if submitErr != nil && errors.As(submitErr, &ambiguousErr) && allOrderResponsesAccepted(awaited) {
		// POST 本身报错，但确定性 order hash 已证明整批都被 CLOB 接收。
		submitErr = nil
	}
	return awaited, submitErr
}

type orderBatchPostFunc func(
	[]types.OrderArgs,
	[]types.OrderType,
	...bool,
) ([]types.OrderPostResponse, error)

func (c *orderClientImpl) createAndPostOrdersWith(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	postFn orderBatchPostFunc,
) ([]types.OrderPostResponse, error) {
	if len(orderArgsList) == 0 {
		return []types.OrderPostResponse{}, nil
	}

	if len(orderArgsList) != len(orderTypes) {
		return nil, fmt.Errorf("orderArgsList and orderTypes must have the same length")
	}

	if err := validateOrderTickSizes(orderArgsList); err != nil {
		return nil, err
	}
	if err := validateOrderSizes(orderArgsList); err != nil {
		return nil, err
	}
	if err := validateOrderTokenIDs(orderArgsList); err != nil {
		return nil, err
	}
	if err := validateOrderLifetimes(orderArgsList, orderTypes, time.Now()); err != nil {
		return nil, err
	}

	const maxBatchSize = 15 // 每批最多15个订单
	return runOrderBatches(orderArgsList, orderTypes, maxBatchSize, postFn)
}

func allOrderResponsesAccepted(responses []types.OrderPostResponse) bool {
	if len(responses) == 0 {
		return false
	}
	for _, response := range responses {
		if !response.Accepted() {
			return false
		}
	}
	return true
}

// runOrderBatches 把逻辑批次切成最多 maxBatchSize 的 HTTP 子批。一个子批
// 失败后立即停止：之前子批的结果原样保留，之后订单明确为
// not_submitted。已确认关闭的 token 在后续子批直接回填 market_closed，
// 不会再次请求 CLOB。
func runOrderBatches(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	maxBatchSize int,
	postFn func([]types.OrderArgs, []types.OrderType, ...bool) ([]types.OrderPostResponse, error),
) ([]types.OrderPostResponse, error) {
	allResults := makeBatchStatusResults(len(orderArgsList), OrderNotSubmittedStatus, "order was not attempted")
	totalBatches := (len(orderArgsList) + maxBatchSize - 1) / maxBatchSize
	closedTokens := make(map[string]struct{})

	for i := 0; i < len(orderArgsList); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(orderArgsList) {
			end = len(orderArgsList)
		}

		batchNum := (i / maxBatchSize) + 1
		batchOrderArgs := make([]types.OrderArgs, 0, end-i)
		batchOrderTypes := make([]types.OrderType, 0, end-i)
		batchIndexes := make([]int, 0, end-i)
		for idx := i; idx < end; idx++ {
			if _, closed := closedTokens[orderArgsList[idx].TokenID]; closed {
				allResults[idx] = marketClosedOrderResult(orderArgsList[idx].TokenID)
				continue
			}
			batchOrderArgs = append(batchOrderArgs, orderArgsList[idx])
			batchOrderTypes = append(batchOrderTypes, orderTypes[idx])
			batchIndexes = append(batchIndexes, idx)
		}
		if len(batchOrderArgs) == 0 {
			continue
		}

		internal.LogDebug("提交订单批次 %d/%d (原始订单 %d-%d，实际提交 %d 笔)", batchNum, totalBatches, i+1, end, len(batchOrderArgs))
		batchStart := time.Now()

		batchResults, err := postFn(batchOrderArgs, batchOrderTypes)
		batchDuration := time.Since(batchStart)
		if len(batchResults) != len(batchOrderArgs) {
			var alignErr error
			batchResults, alignErr = normalizeV2BatchResponses(batchResults, len(batchOrderArgs))
			if err == nil {
				err = alignErr
			}
		}
		for j, result := range batchResults {
			if j >= len(batchIndexes) {
				break
			}
			origIdx := batchIndexes[j]
			allResults[origIdx] = result
			if result.Status == OrderMarketClosedStatus {
				closedTokens[orderArgsList[origIdx].TokenID] = struct{}{}
			}
		}

		if err != nil {
			internal.LogError("批次 %d/%d (订单 %d-%d) 提交失败 (耗时: %v): %v", batchNum, totalBatches, i+1, end, batchDuration, err)
			return allResults, fmt.Errorf("批次 %d/%d（原始订单 %d-%d）提交失败: %w", batchNum, totalBatches, i+1, end, err)
		}

		if batchDuration > 5*time.Second {
			internal.LogWarn("批次 %d/%d 耗时过长: %v，可能发生阻塞", batchNum, totalBatches, batchDuration)
		} else {
			internal.LogDebug("批次 %d/%d 完成 (耗时: %v)", batchNum, totalBatches, batchDuration)
		}
	}

	return allResults, nil
}

// PostOrder 提交单个订单
func (c *orderClientImpl) PostOrder(orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error) {
	return c.PostOrderContext(context.Background(), orderArgs, orderType)
}

func (c *orderClientImpl) PostOrderContext(ctx context.Context, orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error) {
	results, err := c.CreateAndPostOrdersContext(ctx, []types.OrderArgs{orderArgs}, []types.OrderType{orderType})
	return firstOrderResponse(results, err)
}

// PostOrderInstant 是单笔立即返回版本。
func (c *orderClientImpl) PostOrderInstant(orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error) {
	return c.PostOrderInstantContext(context.Background(), orderArgs, orderType)
}

func (c *orderClientImpl) PostOrderInstantContext(ctx context.Context, orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error) {
	results, err := c.CreateAndPostOrdersInstantContext(ctx, []types.OrderArgs{orderArgs}, []types.OrderType{orderType})
	return firstOrderResponse(results, err)
}

// PostOrderAndWait 是单笔等待版本。
func (c *orderClientImpl) PostOrderAndWait(
	ctx context.Context,
	orderArgs types.OrderArgs,
	orderType types.OrderType,
) (*types.OrderPostResponse, error) {
	results, err := c.CreateAndPostOrdersAndWait(ctx, []types.OrderArgs{orderArgs}, []types.OrderType{orderType})
	return firstOrderResponse(results, err)
}

func firstOrderResponse(results []types.OrderPostResponse, err error) (*types.OrderPostResponse, error) {
	if len(results) == 0 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("no response from server")
	}
	return &results[0], err
}

// CancelOrders cancels multiple orders
// According to Polymarket API docs: DELETE /orders with body as string[] (orderID array)
func (c *orderClientImpl) CancelOrders(orderIDs []types.Keccak256) (*types.OrderCancelResponse, error) {
	return c.CancelOrdersContext(context.Background(), orderIDs)
}

func (c *orderClientImpl) CancelOrdersContext(ctx context.Context, orderIDs []types.Keccak256) (*types.OrderCancelResponse, error) {
	if len(orderIDs) == 0 {
		return emptyOrderCancelResponse(), nil
	}
	if len(orderIDs) > MaxCancelOrdersPerRequest {
		return nil, fmt.Errorf("cancel orders count %d exceeds per-request limit %d", len(orderIDs), MaxCancelOrdersPerRequest)
	}

	// Convert Keccak256 to string array for request body
	// According to API docs, the body should be a string array directly, not wrapped in an object
	orderIDStrings := make([]string, len(orderIDs))
	for i, orderID := range orderIDs {
		if err := orderID.Validate(); err != nil {
			return nil, fmt.Errorf("invalid order ID at index %d: %w", i, err)
		}
		orderIDStrings[i] = orderID.String()
	}

	// Marshal body to JSON (array of strings)
	// IMPORTANT: For DELETE /orders, body should be formatted JSON string for HMAC signature
	bodyJSON, err := json.Marshal(orderIDStrings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}

	// Convert compact JSON to Python's json.dumps format (with spaces)
	// This matches the format used in HMAC signature calculation
	bodyJSONStr := string(bodyJSON)
	bodyJSONStr = regexp.MustCompile(`":(\S)`).ReplaceAllString(bodyJSONStr, `": $1`)
	bodyJSONStr = regexp.MustCompile(`,(")`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSONStr = regexp.MustCompile(`,(\{|\[)`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSON = []byte(bodyJSONStr)

	// Use RequestBody to pass formatted JSON string to CreateLevel2Headers
	// This matches how CancelAll works (using CreateLevel2Headers)
	requestBody := types.RequestBody(bodyJSON)
	requestArgs := &types.RequestArgs{
		Method:      "DELETE",
		RequestPath: internal.CancelOrders,
		Body:        &requestBody,
	}

	// 使用 CreateLevel2Headers，传入格式化后的 JSON 字符串 body
	// 这样与 CancelAll 的处理方式一致，都使用 CreateLevel2Headers
	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	// 执行请求，使用格式化后的 JSON body
	response, err := http.DeleteRawContext[types.OrderCancelResponse](ctx, c.baseClient.baseURL, internal.CancelOrders, bodyJSON, http.WithHeaders(headers), http.WithService("clob"))
	return normalizeOrderCancelResponse(response), err
}

// CancelOrder 取消单个订单
func (c *orderClientImpl) CancelOrder(orderID types.Keccak256) (*types.OrderCancelResponse, error) {
	return c.CancelOrderContext(context.Background(), orderID)
}

func (c *orderClientImpl) CancelOrderContext(ctx context.Context, orderID types.Keccak256) (*types.OrderCancelResponse, error) {
	return c.CancelOrdersContext(ctx, []types.Keccak256{orderID})
}

// CancelAll cancels all orders
func (c *orderClientImpl) CancelAll() (*types.OrderCancelResponse, error) {
	return c.CancelAllContext(context.Background())
}

func (c *orderClientImpl) CancelAllContext(ctx context.Context) (*types.OrderCancelResponse, error) {
	requestArgs := &types.RequestArgs{
		Method:      "DELETE",
		RequestPath: internal.CancelAll,
	}
	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}
	response, err := http.DeleteContext[types.OrderCancelResponse](ctx, c.baseClient.baseURL, internal.CancelAll, nil, http.WithHeaders(headers), http.WithService("clob"))
	return normalizeOrderCancelResponse(response), err
}

// CancelMarketOrders 取消指定 condition 的所有订单。
// 保留该方法用于源码兼容；需要按 token 或 condition+token 过滤时使用
// CancelMarketOrdersByFilter。
func (c *orderClientImpl) CancelMarketOrders(conditionID types.Keccak256) (*types.OrderCancelResponse, error) {
	return c.CancelMarketOrdersContext(context.Background(), conditionID)
}

func (c *orderClientImpl) CancelMarketOrdersContext(ctx context.Context, conditionID types.Keccak256) (*types.OrderCancelResponse, error) {
	parsed, err := types.ParseConditionID(conditionID.String())
	if err != nil {
		return nil, fmt.Errorf("invalid condition ID: %w", err)
	}
	return c.CancelMarketOrdersByFilterContext(ctx, types.CancelMarketOrdersParams{ConditionID: parsed})
}

type cancelMarketOrdersWire struct {
	Market  string `json:"market,omitempty"`
	AssetID string `json:"asset_id,omitempty"`
}

// CancelMarketOrdersByFilter 按 condition、token 或两者的交集取消订单。
func (c *orderClientImpl) CancelMarketOrdersByFilter(params types.CancelMarketOrdersParams) (*types.OrderCancelResponse, error) {
	return c.CancelMarketOrdersByFilterContext(context.Background(), params)
}

func (c *orderClientImpl) CancelMarketOrdersByFilterContext(ctx context.Context, params types.CancelMarketOrdersParams) (*types.OrderCancelResponse, error) {
	requestBody := cancelMarketOrdersWire{}
	if params.ConditionID != (types.ConditionID{}) {
		requestBody.Market = params.ConditionID.String()
	}
	if params.TokenID != (types.TokenID{}) {
		requestBody.AssetID = params.TokenID.String()
	}
	if requestBody.Market == "" && requestBody.AssetID == "" {
		return nil, fmt.Errorf("cancel market orders requires ConditionID, TokenID, or both")
	}

	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	// Create request args for signing
	requestArgs := &types.RequestArgs{
		Method:      "DELETE",
		RequestPath: internal.CancelMarketOrders,
		Body:        nil, // DELETE request body will be marshaled
	}

	// Marshal body to JSON
	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}

	// Convert compact JSON to Python's json.dumps format (with spaces)
	bodyJSONStr := string(bodyJSON)
	bodyJSONStr = regexp.MustCompile(`":(\S)`).ReplaceAllString(bodyJSONStr, `": $1`)
	bodyJSONStr = regexp.MustCompile(`,(")`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSONStr = regexp.MustCompile(`,(\{|\[)`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSON = []byte(bodyJSONStr)

	// Use RequestBody for signing
	requestBodyForSigning := types.RequestBody(bodyJSON)
	requestArgs.Body = &requestBodyForSigning

	// Create Level 2 headers
	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	// Execute DELETE request with body
	response, err := http.DeleteRawContext[types.OrderCancelResponse](ctx, c.baseClient.baseURL, internal.CancelMarketOrders, bodyJSON, http.WithHeaders(headers), http.WithService("clob"))
	return normalizeOrderCancelResponse(response), err
}

func emptyOrderCancelResponse() *types.OrderCancelResponse {
	return &types.OrderCancelResponse{
		Canceled:    []types.Keccak256{},
		NotCanceled: make(map[types.Keccak256]string),
	}
}

func normalizeOrderCancelResponse(response *types.OrderCancelResponse) *types.OrderCancelResponse {
	if response == nil {
		return nil
	}
	if response.Canceled == nil {
		response.Canceled = []types.Keccak256{}
	}
	if response.NotCanceled == nil {
		response.NotCanceled = make(map[types.Keccak256]string)
	}
	return response
}
