package clob

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	ordermodel "github.com/polymarket/go-order-utils/pkg/model"
	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetOrders 获取活跃订单
func (c *polymarketClobClient) GetOrders(orderID *types.Keccak256, conditionID *types.Keccak256, tokenID *string) ([]types.OpenOrder, error) {
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

	headers, err := internal.CreateLevel2Headers(c.web3Client.GetSigner(), c.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	var allOrders []types.OpenOrder
	nextCursor := "MA=="

	for nextCursor != internal.EndCursor {
		params["next_cursor"] = nextCursor

		response, err := http.Get[types.PaginatedResponse[types.OpenOrder]](c.baseURL, internal.Orders, params, http.WithHeaders(headers))
		if err != nil {
			return nil, fmt.Errorf("failed to get orders: %w", err)
		}

		allOrders = append(allOrders, response.Data...)
		nextCursor = response.NextCursor
	}

	return allOrders, nil
}

// CreateAndPostOrders 使用go-order-utils创建并提交多个订单
// 如果订单数量超过15个，将自动分批提交，每批最多15个订单
// 内部统一逻辑：
//   - tickSize 默认使用 0.001
//   - negRisk 默认使用 false，如果出现签名错误则使用 true 重试
//   - 统一检查所有订单的 price 是否符合条件
func (c *polymarketClobClient) CreateAndPostOrders(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
) ([]types.OrderPostResponse, error) {
	if len(orderArgsList) == 0 {
		return []types.OrderPostResponse{}, nil
	}

	if len(orderArgsList) != len(orderTypes) {
		return nil, fmt.Errorf("orderArgsList and orderTypes must have the same length")
	}

	// 统一检查所有订单的 price 是否符合条件（使用 tickSize=0.001）
	const defaultTickSize = 0.001
	for i, orderArgs := range orderArgsList {
		if orderArgs.Price < defaultTickSize || orderArgs.Price > 1.0-defaultTickSize {
			return nil, fmt.Errorf("订单 %d 价格无效: price=%.3f 必须在范围 [%.3f, %.3f] 内",
				i+1, orderArgs.Price, defaultTickSize, 1.0-defaultTickSize)
		}
	}

	const maxBatchSize = 15 // 每批最多15个订单

	// 如果订单数量不超过15个，直接提交
	if len(orderArgsList) <= maxBatchSize {
		return c.postOrdersBatch(orderArgsList, orderTypes)
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

		batchResults, err := c.postOrdersBatch(batchOrderArgs, batchOrderTypes)
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

// postOrdersBatch 提交一批订单（内部方法，最多15个订单）
// 内部统一逻辑：
//   - tickSize 默认使用 0.001
//   - negRisk 默认使用 false，如果是重试调用则使用 true
//
// isRetry: 是否为重试调用，如果是则使用 negRisk=true，且不再进行重试（避免无限递归）
func (c *polymarketClobClient) postOrdersBatch(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	isRetry ...bool,
) ([]types.OrderPostResponse, error) {
	// 检查是否为重试调用
	isRetryCall := len(isRetry) > 0 && isRetry[0]
	if len(orderArgsList) == 0 {
		return []types.OrderPostResponse{}, nil
	}

	if len(orderArgsList) > 15 {
		return nil, fmt.Errorf("postOrdersBatch: batch size cannot exceed 15, got %d", len(orderArgsList))
	}

	// 统一使用默认值
	const defaultTickSize = "0.001"
	defaultNegRisk := false
	if isRetryCall {
		defaultNegRisk = true
	}

	// Build request body: array of {order: {...}, owner: api_key, orderType: "GTC"}
	// IMPORTANT: Field order must match Python: order, owner, orderType
	// Python's order_to_json returns: {"order": order.dict(), "owner": owner, "orderType": order_type.value}

	// Define OrderedOrder struct to preserve field order (matching Python's order.dict())
	// IMPORTANT: According to Polymarket API docs:
	//   - salt: integer (not string)
	//   - signatureType: integer (not string)
	//   - All other numeric fields are strings
	type OrderedOrder struct {
		Salt          int64  `json:"salt"` // integer per API docs
		TokenId       string `json:"tokenId"`
		MakerAmount   string `json:"makerAmount"`
		TakerAmount   string `json:"takerAmount"`
		Side          string `json:"side"`
		Expiration    string `json:"expiration"`
		Nonce         string `json:"nonce"`
		FeeRateBps    string `json:"feeRateBps"`
		SignatureType int    `json:"signatureType"` // integer per API docs
		Maker         string `json:"maker"`
		Taker         string `json:"taker"`
		Signer        string `json:"signer"`
		Signature     string `json:"signature"`
	}

	type OrderRequest struct {
		Order     OrderedOrder `json:"order"`     // Use struct to preserve field order
		Owner     string       `json:"owner"`     // Second field
		OrderType string       `json:"orderType"` // Third field
	}

	// 所有token统一使用默认值
	internal.LogDebug("所有token使用默认值: TickSize=0.001, NegRisk=%v (不请求API)", defaultNegRisk)

	// Use append instead of fixed-size slice to avoid empty orders
	requestBody := make([]OrderRequest, 0, len(orderArgsList))

	for i, orderArgs := range orderArgsList {
		// 如果订单share小于5，则设置为5
		if orderArgs.Size < 5.0 {
			orderArgs.Size = 5.0
		}

		// 统一使用默认值
		tickSize := types.TickSize(defaultTickSize)
		negRisk := defaultNegRisk

		// 记录使用的tickSize和negRisk值（用于调试签名问题）
		// 注意：不记录完整的订单参数，避免泄露敏感信息
		internal.LogDebug("订单签名参数: token=%s, tickSize=%s, negRisk=%v",
			orderArgs.TokenID, tickSize, negRisk)

		// Get fee rate (default to 0 if not specified)
		feeRateBps := 0
		if orderArgs.FeeRateBps != nil {
			feeRateBps = *orderArgs.FeeRateBps
		}

		// Create signed order using order builder
		signedOrder, err := c.createSignedOrder(orderArgs, tickSize, negRisk, feeRateBps, orderTypes[i])
		if err != nil {
			// Skip this order (can't create empty OrderedOrder, so we'll skip it)
			// We'll handle this by reducing the slice size later
			continue
		}

		// Convert SignedOrder to OrderedOrder struct (matching Python's order.dict() field order)
		// Python: order_to_json returns {"order": order.dict(), "owner": owner, "orderType": order_type.value}
		// Field order: salt, tokenId, makerAmount, takerAmount, side, expiration, nonce, feeRateBps, signatureType, maker, taker, signer, signature
		// IMPORTANT: Per Polymarket API docs, salt and signatureType must be integers, not strings
		orderedOrder := OrderedOrder{
			Salt:          signedOrder.Order.Salt.Int64(), // integer per API docs
			TokenId:       signedOrder.Order.TokenId.String(),
			MakerAmount:   signedOrder.Order.MakerAmount.String(),
			TakerAmount:   signedOrder.Order.TakerAmount.String(),
			Side:          string(orderArgs.Side),
			Expiration:    signedOrder.Order.Expiration.String(),
			Nonce:         signedOrder.Order.Nonce.String(),
			FeeRateBps:    signedOrder.Order.FeeRateBps.String(),
			SignatureType: int(signedOrder.Order.SignatureType.Int64()), // integer per API docs
			Maker:         signedOrder.Order.Maker.Hex(),
			Taker:         signedOrder.Order.Taker.Hex(),
			Signer:        signedOrder.Order.Signer.Hex(),
			Signature:     "0x" + fmt.Sprintf("%x", signedOrder.Signature),
		}

		requestBody = append(requestBody, OrderRequest{
			Order:     orderedOrder,
			Owner:     c.deriveCreds.Key,
			OrderType: string(orderTypes[i]),
		})
	}

	if len(requestBody) == 0 {
		return []types.OrderPostResponse{}, fmt.Errorf("no valid orders to post")
	}

	// Marshal body to JSON for logging and actual request
	// IMPORTANT: Python uses json.dumps(body) which adds spaces after colons and commas
	// We need to match this format exactly for Cloudflare validation
	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Convert compact JSON to Python's json.dumps format (with spaces)
	// Python: json.dumps produces {"key": "value", "key2": "value2"}
	// Go: json.Marshal produces {"key":"value","key2":"value2"}
	// We need to add spaces to match Python format (same as HMAC signature)
	bodyJSONStr := string(bodyJSON)
	// Add space after colon: "key":"value" -> "key": "value"
	bodyJSONStr = regexp.MustCompile(`":(\S)`).ReplaceAllString(bodyJSONStr, `": $1`)
	// Add space after comma: "a","b" -> "a", "b"
	// Also handle comma followed by { or [ (for nested structures)
	bodyJSONStr = regexp.MustCompile(`,(")`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSONStr = regexp.MustCompile(`,(\{|\[)`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSON = []byte(bodyJSONStr)

	// Create request args for signing
	// Python: body = [order_to_json(...) for ...] (list of dicts)
	// Then: create_level_2_headers(..., body=body) - body is list, not JSON string
	requestArgs := &types.RequestArgs{
		Method:      "POST",
		RequestPath: internal.PostOrders,
		Body:        nil, // Not used when passing body directly
	}

	// Create Level 2 headers (HMAC signature)
	// Pass requestBody directly (struct/slice) to match Python behavior
	headers, err := internal.CreateLevel2HeadersWithBody(c.web3Client.GetSigner(), c.deriveCreds, requestArgs, requestBody, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	// Make POST request using PostRaw to send pre-formatted JSON (with spaces matching Python's json.dumps)
	responseBody, err := http.PostRaw(c.baseURL, internal.PostOrders, bodyJSON, http.WithHeaders(headers))
	if err != nil {
		return nil, err
	}

	// Parse response
	var resp []types.OrderPostResponse
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
	}

	if len(resp) == 0 {
		return []types.OrderPostResponse{}, nil
	}

	// 检查失败的订单，特别是invalid signature错误
	// 对于这些订单，使用negRisk=true重试
	failedOrders := make([]int, 0) // 存储失败订单的索引
	orderbookNotExistCount := 0    // 统计订单簿不存在的错误（token进入结算过期，正常情况）
	for i, result := range resp {
		if result.ErrorMsg != "" {
			// 如果是签名错误，尝试使用negRisk=true重试（正常业务流程，不记录日志）
			if strings.Contains(result.ErrorMsg, "invalid signature") {
				failedOrders = append(failedOrders, i)
			} else if strings.Contains(result.ErrorMsg, "the orderbook") && strings.Contains(result.ErrorMsg, "does not exist") {
				// 订单簿不存在（token进入结算过期），正常情况，不打印详细日志，只统计
				orderbookNotExistCount++
			} else {
				internal.LogError("订单 %d 创建失败: %s, order: %+v", i+1, result.ErrorMsg, orderArgsList[i])
			}
		}
	}

	// 统计订单簿不存在的错误（token进入结算过期，正常情况）
	if orderbookNotExistCount > 0 {
		internal.LogInfo("订单簿不存在（token已进入结算）: %d 个订单", orderbookNotExistCount)
	}

	// 如果有失败的订单（invalid signature），且不是重试调用，使用negRisk=true重试
	if len(failedOrders) > 0 && !isRetryCall {
		retryOrderArgs := make([]types.OrderArgs, 0, len(failedOrders))
		retryOrderTypes := make([]types.OrderType, 0, len(failedOrders))
		retryIndices := make([]int, 0, len(failedOrders)) // 记录原始索引，用于更新结果

		for _, idx := range failedOrders {
			retryOrderArgs = append(retryOrderArgs, orderArgsList[idx])
			retryOrderTypes = append(retryOrderTypes, orderTypes[idx])
			retryIndices = append(retryIndices, idx)
		}

		// 调用内部方法重试（只重试失败的订单，标记为重试调用避免无限递归，使用 negRisk=true）
		retryResults, err := c.postOrdersBatch(retryOrderArgs, retryOrderTypes, true)
		if err != nil {
			internal.LogError("重试订单失败: %v", err)
		} else {
			// 统计重试结果
			retrySuccessCount := 0
			retryFailCount := 0
			retryFailReasons := make(map[string]int) // 统计失败原因

			// 更新原始结果
			for j, retryIdx := range retryIndices {
				if j < len(retryResults) {
					// 如果重试成功，更新结果
					if retryResults[j].ErrorMsg == "" {
						retrySuccessCount++
						resp[retryIdx] = retryResults[j]
					} else {
						retryFailCount++
						errorMsg := retryResults[j].ErrorMsg
						if errorMsg == "" {
							errorMsg = "未知错误"
						}
						retryFailReasons[errorMsg]++
						// 保留原始错误信息，但标记为重试失败
						resp[retryIdx].ErrorMsg = fmt.Sprintf("重试失败: %s (原始错误: %s)", retryResults[j].ErrorMsg, resp[retryIdx].ErrorMsg)
					}
				}
			}

			// 重试是正常业务流程，不记录日志
			// 只有重试调用本身失败时才记录错误
		}
	}

	return resp, nil
}

// createSignedOrder creates a signed order using go-order-utils
func (c *polymarketClobClient) createSignedOrder(
	orderArgs types.OrderArgs,
	tickSize types.TickSize,
	negRisk bool,
	feeRateBps int,
	orderType types.OrderType,
) (*ordermodel.SignedOrder, error) {
	// Get private key from signer

	// Parse tick size
	tickSizeFloat, err := strconv.ParseFloat(string(tickSize), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid tick size: %w", err)
	}

	// Validate price range: tick_size <= price <= 1 - tick_size (per Python price_valid function)
	if orderArgs.Price < tickSizeFloat || orderArgs.Price > 1.0-tickSizeFloat {
		return nil, fmt.Errorf("price (%.6f) must be in range [%.6f, %.6f], min: %s - max: %.6f",
			orderArgs.Price, tickSizeFloat, 1.0-tickSizeFloat, tickSize, 1.0-tickSizeFloat)
	}

	// Calculate maker and taker amounts based on side
	makerAmount, takerAmount, err := c.calculateOrderAmounts(orderArgs.Side, orderArgs.Size, orderArgs.Price, tickSize)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate order amounts: %w", err)
	}

	// Determine verifying contract
	var verifyingContract ordermodel.VerifyingContract
	if negRisk {
		verifyingContract = ordermodel.NegRiskCTFExchange
	} else {
		verifyingContract = ordermodel.CTFExchange
	}

	// Get nonce
	// For proxy wallets, nonce should typically be 0 (per Python default)
	// Nonce is used for onchain cancellations, and 0 is the default value
	// Using 0 matches Python's OrderArgs.nonce default value
	nonce := "0"

	// Get expiration based on order type
	// GTC: expiration = "0" (per API requirement: "it should be equal to '0' as the order is not a GTD order")
	// FOK/FAK/IOC: also use "0" (they are immediate execution orders)
	var expirationStr string
	if orderType == types.OrderTypeGTC {
		expirationStr = "0"
	} else {
		// For FOK/FAK/IOC, also use "0" (they are immediate execution orders)
		expirationStr = "0"
	}

	// Determine side
	var side ordermodel.Side
	if orderArgs.Side == types.OrderSideBUY {
		side = ordermodel.BUY
	} else {
		side = ordermodel.SELL
	}

	// Determine signature type
	var sigType ordermodel.SignatureType
	switch c.signatureType {
	case 0:
		sigType = ordermodel.EOA
	case 1:
		sigType = ordermodel.POLY_PROXY
	case 2:
		sigType = ordermodel.POLY_GNOSIS_SAFE
	default:
		sigType = ordermodel.EOA
	}

	// Create OrderData (Maker and Taker are strings, not Address)
	// IMPORTANT: For proxy wallets (signatureType=1), maker should be proxy address (funder),
	// while signer should be base address. For EOA (signatureType=0), both are base address.
	// Python: maker=self.funder (proxy address for proxy wallet), signer=self.signer.address() (base address)
	baseAddr := string(c.web3Client.GetBaseAddress())
	makerAddr := baseAddr
	signerAddr := baseAddr

	// For proxy wallet, maker should be proxy address (funder)
	if c.signatureType == types.ProxySignatureType || c.signatureType == types.SafeSignatureType {
		// Use proxy address as maker (funder) and base address as signer
		// proxyAddress is already set during initialization
		makerAddr = string(c.proxyAddress)
		signerAddr = baseAddr
	} else {
		// For EOA, both are base address
		makerAddr = baseAddr
		signerAddr = baseAddr
	}

	orderData := &ordermodel.OrderData{
		Maker:         makerAddr,
		Taker:         "0x0000000000000000000000000000000000000000", // Zero address for public orders
		TokenId:       orderArgs.TokenID,
		MakerAmount:   makerAmount.String(),
		TakerAmount:   takerAmount.String(),
		Side:          side,
		FeeRateBps:    strconv.Itoa(feeRateBps),
		Nonce:         nonce,
		Signer:        signerAddr,
		Expiration:    expirationStr,
		SignatureType: sigType,
	}

	// Build signed order
	signedOrder, err := c.orderBuilder.BuildSignedOrder(c.web3Client.GetPrivateKey(), orderData, verifyingContract)
	if err != nil {
		return nil, fmt.Errorf("failed to build signed order: %w", err)
	}

	return signedOrder, nil
}

// PostOrder 提交单个订单
func (c *polymarketClobClient) PostOrder(orderArgs types.OrderArgs, orderType types.OrderType) (*types.OrderPostResponse, error) {
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
func (c *polymarketClobClient) CancelOrders(orderIDs []types.Keccak256) (*types.OrderCancelResponse, error) {
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
	headers, err := internal.CreateLevel2Headers(c.web3Client.GetSigner(), c.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	// 执行请求，使用格式化后的 JSON body
	return http.DeleteRaw[types.OrderCancelResponse](c.baseURL, internal.CancelOrders, bodyJSON, http.WithHeaders(headers))
}

// CancelOrder 取消单个订单
func (c *polymarketClobClient) CancelOrder(orderID types.Keccak256) (*types.OrderCancelResponse, error) {
	return c.CancelOrders([]types.Keccak256{orderID})
}

// CancelAll cancels all orders
func (c *polymarketClobClient) CancelAll() (*types.OrderCancelResponse, error) {
	requestArgs := &types.RequestArgs{
		Method:      "DELETE",
		RequestPath: internal.CancelAll,
	}
	headers, err := internal.CreateLevel2Headers(c.web3Client.GetSigner(), c.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}
	return http.Delete[types.OrderCancelResponse](c.baseURL, internal.CancelAll, nil, http.WithHeaders(headers))
}

// CancelMarketOrders 取消指定市场的所有订单
func (c *polymarketClobClient) CancelMarketOrders(conditionID types.Keccak256) (*types.OrderCancelResponse, error) {
	// Validate API credentials
	if c.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.deriveCreds.Key == "" || c.deriveCreds.Secret == "" || c.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.deriveCreds.Key != "", c.deriveCreds.Secret != "", c.deriveCreds.Passphrase != "")
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
	headers, err := internal.CreateLevel2Headers(c.web3Client.GetSigner(), c.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	// Execute DELETE request with body
	return http.DeleteRaw[types.OrderCancelResponse](c.baseURL, internal.CancelMarketOrders, bodyJSON, http.WithHeaders(headers))
}
