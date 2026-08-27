package clob

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetOrders 获取活跃订单
func (c *orderClientImpl) GetOrders(orderID *types.Keccak256, conditionID *types.Keccak256, tokenID *string) ([]types.OpenOrder, error) {
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

		response, err := http.Get[types.PaginatedResponse[types.OpenOrder]](c.baseClient.baseURL, internal.Orders, params, http.WithHeaders(headers))
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
// 返回契约：成功路径下返回的切片长度恒等于 len(orderArgsList) 且同序；被
// SDK 自动剥离（如 "orderbook X does not exist"）或批次失败的位置会回填
// Status=OrderStrippedStatus 或 ErrorMsg 非空的合成响应。调用方可安全地
// 通过下标 results[i] 与 orderArgsList[i] 对齐。失败路径仍返回 nil + error。
func (c *orderClientImpl) CreateAndPostOrders(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
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

	const maxBatchSize = 15 // 每批最多15个订单

	// 如果订单数量不超过15个，直接提交
	if len(orderArgsList) <= maxBatchSize {
		return c.postOrdersBatchV2(orderArgsList, orderTypes)
	}

	// 分批提交
	allResults := make([]types.OrderPostResponse, 0, len(orderArgsList))
	totalBatches := (len(orderArgsList) + maxBatchSize - 1) / maxBatchSize
	for i := 0; i < len(orderArgsList); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(orderArgsList) {
			end = len(orderArgsList)
		}

		batchNum := (i / maxBatchSize) + 1
		batchOrderArgs := orderArgsList[i:end]
		batchOrderTypes := orderTypes[i:end]

		internal.LogDebug("提交订单批次 %d/%d (订单 %d-%d，共 %d 个订单)", batchNum, totalBatches, i+1, end, len(batchOrderArgs))
		batchStart := time.Now()

		batchResults, err := c.postOrdersBatchV2(batchOrderArgs, batchOrderTypes)
		batchDuration := time.Since(batchStart)
		if err != nil {
			// 如果某批失败，记录错误但继续处理下一批
			internal.LogError("批次 %d/%d (订单 %d-%d) 提交失败 (耗时: %v): %v", batchNum, totalBatches, i+1, end, batchDuration, err)
			// 为失败的批次创建错误响应
			for j := 0; j < len(batchOrderArgs); j++ {
				allResults = append(allResults, types.OrderPostResponse{
					ErrorMsg: fmt.Sprintf("批次提交失败: %v", err),
				})
			}
			continue
		}

		allResults = append(allResults, batchResults...)
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
	results, err := c.CreateAndPostOrders([]types.OrderArgs{orderArgs}, []types.OrderType{orderType})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no response from server")
	}
	return &results[0], nil
}

// CancelOrders cancels multiple orders
// According to Polymarket API docs: DELETE /orders with body as string[] (orderID array)
func (c *orderClientImpl) CancelOrders(orderIDs []types.Keccak256) (*types.OrderCancelResponse, error) {
	if len(orderIDs) == 0 {
		return &types.OrderCancelResponse{
			Canceled:    []types.Keccak256{},
			NotCanceled: make(map[types.Keccak256]string),
		}, nil
	}

	// Convert Keccak256 to string array for request body
	// According to API docs, the body should be a string array directly, not wrapped in an object
	orderIDStrings := make([]string, len(orderIDs))
	for i, orderID := range orderIDs {
		orderIDStrings[i] = string(orderID)
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
	return http.DeleteRaw[types.OrderCancelResponse](c.baseClient.baseURL, internal.CancelOrders, bodyJSON, http.WithHeaders(headers))
}

// CancelOrder 取消单个订单
func (c *orderClientImpl) CancelOrder(orderID types.Keccak256) (*types.OrderCancelResponse, error) {
	return c.CancelOrders([]types.Keccak256{orderID})
}

// CancelAll cancels all orders
func (c *orderClientImpl) CancelAll() (*types.OrderCancelResponse, error) {
	requestArgs := &types.RequestArgs{
		Method:      "DELETE",
		RequestPath: internal.CancelAll,
	}
	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}
	return http.Delete[types.OrderCancelResponse](c.baseClient.baseURL, internal.CancelAll, nil, http.WithHeaders(headers))
}

// CancelMarketOrders 取消指定市场的所有订单
func (c *orderClientImpl) CancelMarketOrders(conditionID types.Keccak256) (*types.OrderCancelResponse, error) {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	// Build request body with condition_id
	requestBody := map[string]string{
		"condition_id": string(conditionID),
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
	return http.DeleteRaw[types.OrderCancelResponse](c.baseClient.baseURL, internal.CancelMarketOrders, bodyJSON, http.WithHeaders(headers))
}
