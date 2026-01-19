package clob

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetTickSize 获取代币的tick大小
func (c *marketDataClientImpl) GetTickSize(tokenID string) (types.TickSize, error) {
	if tickSize, ok := c.baseClient.tickSizes[tokenID]; ok {
		return tickSize, nil
	}

	params := map[string]string{"token_id": tokenID}

	// API may return minimum_tick_size as number or string, so we need to handle both
	var rawResponse map[string]interface{}
	resp, err := http.Get[map[string]interface{}](c.baseClient.baseURL, internal.GetTickSize, params)
	if err != nil {
		return "", fmt.Errorf("failed to get tick size: %w", err)
	}
	rawResponse = *resp

	// Extract minimum_tick_size and convert to string
	var tickSizeStr string
	if val, ok := rawResponse["minimum_tick_size"]; ok {
		switch v := val.(type) {
		case string:
			tickSizeStr = v
		case float64:
			// Convert number to string, preserving precision
			tickSizeStr = strconv.FormatFloat(v, 'f', -1, 64)
		case int:
			tickSizeStr = strconv.Itoa(v)
		case int64:
			tickSizeStr = strconv.FormatInt(v, 10)
		default:
			// Try to convert to string
			tickSizeStr = fmt.Sprintf("%v", v)
		}
	} else {
		return "", fmt.Errorf("minimum_tick_size not found in response")
	}

	tickSize := types.TickSize(tickSizeStr)
	c.baseClient.tickSizes[tokenID] = tickSize
	return tickSize, nil
}

// ResolveTickSize 解析并验证 tick size
// 对应 Python 的 __resolve_tick_size 方法
// 逻辑：
// 1. 获取 token 的最小 tick size
// 2. 如果提供了 userTickSize：
//   - 检查 userTickSize 是否小于最小 tick size
//   - 如果小于，返回错误
//   - 否则使用 userTickSize
//
// 3. 如果没有提供 userTickSize，使用从 API 获取的最小 tick size
func (c *marketDataClientImpl) ResolveTickSize(tokenID string, userTickSize *types.TickSize) (types.TickSize, error) {
	// 获取该 token 的最小 tick size
	minTickSize, err := c.GetTickSize(tokenID)
	if err != nil {
		return "", fmt.Errorf("failed to get minimum tick size: %w", err)
	}

	// 如果用户提供了 tick size，验证它是否有效
	if userTickSize != nil {
		// 检查用户提供的 tick size 是否小于最小 tick size
		if isTickSizeSmaller(*userTickSize, minTickSize) {
			return "", fmt.Errorf("invalid tick size (%s), minimum for the market is %s", string(*userTickSize), string(minTickSize))
		}
		// 用户提供的 tick size 有效，使用它
		return *userTickSize, nil
	}

	// 用户没有提供 tick size，使用从 API 获取的最小 tick size
	return minTickSize, nil
}

// GetNegRisk 获取代币的负风险状态
func (c *marketDataClientImpl) GetNegRisk(tokenID string) (bool, error) {
	if negRisk, ok := c.baseClient.negRisk[tokenID]; ok {
		return negRisk, nil
	}

	params := map[string]string{"token_id": tokenID}
	var result struct {
		NegRisk bool `json:"neg_risk"`
	}

	resp, err := http.Get[struct {
		NegRisk bool `json:"neg_risk"`
	}](c.baseClient.baseURL, internal.GetNegRisk, params)
	if err != nil {
		return false, fmt.Errorf("failed to get neg risk: %w", err)
	}
	result = *resp

	c.baseClient.negRisk[tokenID] = result.NegRisk
	return result.NegRisk, nil
}

// GetOrderBook 获取代币的订单簿
func (c *marketDataClientImpl) GetOrderBook(tokenID string) (*types.OrderBookSummary, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.OrderBookSummary](c.baseClient.baseURL, internal.GetOrderBook, params)
}

// GetOrderBook 获取代币的订单簿（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetOrderBook(tokenID string) (*types.OrderBookSummary, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.OrderBookSummary](c.readonlyBaseClient.baseURL, internal.GetOrderBook, params)
}

// GetMultipleOrderBooks 批量获取多个订单簿摘要
// 根据文档: https://docs.polymarket.com/api-reference/orderbook/get-multiple-order-books-summaries-by-request
// requests: 请求数组，每个元素包含 token_id（必需）和可选的 side（BUY/SELL）
// 最大数组长度: 500
// 返回: 订单簿摘要数组
func (c *marketDataClientImpl) GetMultipleOrderBooks(requests []types.BookParams) ([]types.OrderBookSummaryResponse, error) {
	// 验证请求数量
	if len(requests) == 0 {
		return nil, fmt.Errorf("请求数组不能为空")
	}
	if len(requests) > 500 {
		return nil, fmt.Errorf("请求数组长度不能超过500，当前: %d", len(requests))
	}

	// 构建请求体（只包含必需的字段）
	requestBody := make([]map[string]string, len(requests))
	for i, req := range requests {
		requestBody[i] = map[string]string{
			"token_id": req.TokenID,
		}
		// 如果提供了 side，添加到请求中
		if req.Side != "" {
			requestBody[i]["side"] = req.Side
		}
	}

	// 发送 POST 请求
	result, err := http.Post[[]types.OrderBookSummaryResponse](c.baseClient.baseURL, internal.GetOrderBooks, requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取订单簿失败: %w", err)
	}

	return *result, nil
}

// GetMultipleOrderBooks 批量获取多个订单簿摘要（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetMultipleOrderBooks(requests []types.BookParams) ([]types.OrderBookSummaryResponse, error) {
	// 验证请求数量
	if len(requests) == 0 {
		return nil, fmt.Errorf("请求数组不能为空")
	}
	if len(requests) > 500 {
		return nil, fmt.Errorf("请求数组长度不能超过500，当前: %d", len(requests))
	}

	// 构建请求体（只包含必需的字段）
	requestBody := make([]map[string]string, len(requests))
	for i, req := range requests {
		requestBody[i] = map[string]string{
			"token_id": req.TokenID,
		}
		// 如果提供了 side，添加到请求中
		if req.Side != "" {
			requestBody[i]["side"] = req.Side
		}
	}

	// 发送 POST 请求
	result, err := http.Post[[]types.OrderBookSummaryResponse](c.readonlyBaseClient.baseURL, internal.GetOrderBooks, requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取订单簿失败: %w", err)
	}

	return *result, nil
}

// GetMidpoint 获取单个代币的中间价
func (c *marketDataClientImpl) GetMidpoint(tokenID string) (*types.Midpoint, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.Midpoint](c.baseClient.baseURL, internal.MidPoint, params)
}

// GetMidpoints 批量获取多个代币的中间价
func (c *marketDataClientImpl) GetMidpoints(tokenIDs []string) ([]types.Midpoint, error) {
	if len(tokenIDs) == 0 {
		return []types.Midpoint{}, nil
	}
	if len(tokenIDs) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(tokenIDs))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(tokenIDs))
	for i, tokenID := range tokenIDs {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	// API返回的是对象 map[token_id]price_string，而不是数组
	// 需要先获取原始响应，然后转换为数组
	// 先序列化请求体
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: failed to marshal request body: %w", err)
	}
	
	rawBytes, err := http.PostRaw(c.baseClient.baseURL, internal.MidPoints, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: %w", err)
	}

	// 解析为map[string]string (token_id -> price_string)
	var responseMap map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Midpoint, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if priceStr, ok := responseMap[tokenID]; ok {
			price, err := strconv.ParseFloat(priceStr, 64)
			if err != nil {
				return nil, fmt.Errorf("批量获取中间价失败: failed to parse price for token %s: %w", tokenID, err)
			}
			result = append(result, types.Midpoint{
				TokenID: tokenID,
				Value:   price,
			})
		}
	}

	return result, nil
}

// GetPrice 获取指定方向的价格
func (c *marketDataClientImpl) GetPrice(tokenID string, side types.OrderSide) (*types.Price, error) {
	params := map[string]string{
		"token_id": tokenID,
		"side":     string(side),
	}
	return http.Get[types.Price](c.baseClient.baseURL, internal.Price, params)
}

// GetPrices 批量获取多个代币的价格
func (c *marketDataClientImpl) GetPrices(requests []types.BookParams) ([]types.Price, error) {
	if len(requests) == 0 {
		return []types.Price{}, nil
	}
	if len(requests) > 500 {
		return nil, fmt.Errorf("请求数组长度不能超过500，当前: %d", len(requests))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(requests))
	for i, req := range requests {
		requestBody[i] = map[string]string{
			"token_id": req.TokenID,
		}
		if req.Side != "" {
			requestBody[i]["side"] = req.Side
		}
	}

	// API返回的是嵌套对象 map[token_id]map[side]price_string，例如: {"token_id": {"BUY": "0.52"}}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取价格失败: failed to marshal request body: %w", err)
	}
	
	rawBytes, err := http.PostRaw(c.baseClient.baseURL, internal.GetPrices, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取价格失败: %w", err)
	}

	// 先尝试解析为数组（向后兼容）
	var priceArray []types.Price
	if err := json.Unmarshal(rawBytes, &priceArray); err == nil {
		return priceArray, nil
	}

	// 如果解析为数组失败，尝试解析为嵌套对象 map[token_id]map[side]price_string
	var responseMap map[string]map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取价格失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Price, 0, len(requests))
	for _, req := range requests {
		if tokenMap, ok := responseMap[req.TokenID]; ok {
			// 如果指定了side，使用指定的side；否则尝试BUY或SELL
			var priceStr string
			var found bool
			if req.Side != "" {
				if p, ok := tokenMap[req.Side]; ok {
					priceStr = p
					found = true
				}
			} else {
				// 没有指定side，尝试BUY或SELL
				if p, ok := tokenMap["BUY"]; ok {
					priceStr = p
					found = true
					req.Side = "BUY"
				} else if p, ok := tokenMap["SELL"]; ok {
					priceStr = p
					found = true
					req.Side = "SELL"
				}
			}
			
			if found {
				price, err := strconv.ParseFloat(priceStr, 64)
				if err != nil {
					return nil, fmt.Errorf("批量获取价格失败: failed to parse price for token %s: %w", req.TokenID, err)
				}
				result = append(result, types.Price{
					BookParams: req,
					Price:      price,
				})
			}
		}
	}

	return result, nil
}

// GetSpread 获取单个代币的价差
func (c *marketDataClientImpl) GetSpread(tokenID string) (*types.Spread, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.Spread](c.baseClient.baseURL, internal.GetSpread, params)
}

// GetSpreads 批量获取多个代币的价差
func (c *marketDataClientImpl) GetSpreads(tokenIDs []string) ([]types.Spread, error) {
	if len(tokenIDs) == 0 {
		return []types.Spread{}, nil
	}
	if len(tokenIDs) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(tokenIDs))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(tokenIDs))
	for i, tokenID := range tokenIDs {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	// API返回的是对象 map[token_id]spread_string，而不是数组
	// 需要先获取原始响应，然后转换为数组
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取价差失败: failed to marshal request body: %w", err)
	}
	
	rawBytes, err := http.PostRaw(c.baseClient.baseURL, internal.GetSpreads, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取价差失败: %w", err)
	}

	// 解析为map[string]string (token_id -> spread_string)
	var responseMap map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取价差失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Spread, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if spreadStr, ok := responseMap[tokenID]; ok {
			spread, err := strconv.ParseFloat(spreadStr, 64)
			if err != nil {
				return nil, fmt.Errorf("批量获取价差失败: failed to parse spread for token %s: %w", tokenID, err)
			}
			result = append(result, types.Spread{
				TokenID: tokenID,
				Value:   spread,
			})
		}
	}

	return result, nil
}

// GetLastTradePrice 获取单个代币的最后成交价
func (c *marketDataClientImpl) GetLastTradePrice(tokenID string) (*types.LastTradePrice, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.LastTradePrice](c.baseClient.baseURL, internal.GetLastTradePrice, params)
}

// GetLastTradesPrices 批量获取多个代币的最后成交价
func (c *marketDataClientImpl) GetLastTradesPrices(tokenIDs []string) ([]types.LastTradePrice, error) {
	if len(tokenIDs) == 0 {
		return []types.LastTradePrice{}, nil
	}
	if len(tokenIDs) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(tokenIDs))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(tokenIDs))
	for i, tokenID := range tokenIDs {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	result, err := http.Post[[]types.LastTradePrice](c.baseClient.baseURL, internal.GetLastTradesPrices, requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取最后成交价失败: %w", err)
	}

	return *result, nil
}

// GetFeeRate 获取代币的手续费率（以 bps 为单位，1 bps = 0.01%）
func (c *marketDataClientImpl) GetFeeRate(tokenID string) (int, error) {
	// 检查缓存
	if feeRate, ok := c.baseClient.feeRates[tokenID]; ok {
		return feeRate, nil
	}

	params := map[string]string{"token_id": tokenID}

	// API 可能返回数字或字符串格式的 fee_rate
	var rawResponse map[string]interface{}
	resp, err := http.Get[map[string]interface{}](c.baseClient.baseURL, internal.GetFeeRate, params)
	if err != nil {
		return 0, fmt.Errorf("failed to get fee rate: %w", err)
	}
	rawResponse = *resp

	// 提取 fee_rate 并转换为 int
	var feeRate int
	if val, ok := rawResponse["fee_rate"]; ok {
		switch v := val.(type) {
		case float64:
			feeRate = int(v)
		case int:
			feeRate = v
		case int64:
			feeRate = int(v)
		case string:
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return 0, fmt.Errorf("failed to parse fee_rate as int: %w", err)
			}
			feeRate = parsed
		default:
			return 0, fmt.Errorf("unexpected fee_rate type: %T", v)
		}
	} else {
		return 0, fmt.Errorf("fee_rate not found in response")
	}

	// 缓存结果
	c.baseClient.feeRates[tokenID] = feeRate
	return feeRate, nil
}

// GetTime 获取服务器时间
func (c *marketDataClientImpl) GetTime() (time.Time, error) {
	// API返回的是纯数字（Unix时间戳），不是JSON对象
	rawBytes, err := http.GetRaw(c.baseClient.baseURL, "GET", internal.Time, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get server time: %w", err)
	}

	// 尝试解析为数字（Unix时间戳）
	var timestamp int64
	if err := json.Unmarshal(rawBytes, &timestamp); err == nil {
		return time.Unix(timestamp, 0), nil
	}

	// 如果解析为数字失败，尝试解析为JSON对象
	var timeResponse struct {
		Time interface{} `json:"time"`
	}
	if err := json.Unmarshal(rawBytes, &timeResponse); err == nil {
		if timeResponse.Time != nil {
			switch v := timeResponse.Time.(type) {
			case string:
				// 解析时间字符串（通常是 RFC3339 格式）
				serverTime, err := time.Parse(time.RFC3339, v)
				if err != nil {
					// 尝试其他常见格式
					if serverTime, err = time.Parse("2006-01-02T15:04:05Z07:00", v); err != nil {
						return time.Time{}, fmt.Errorf("failed to parse server time: %w", err)
					}
				}
				return serverTime, nil
			case float64:
				return time.Unix(int64(v), 0), nil
			case int64:
				return time.Unix(v, 0), nil
			case int:
				return time.Unix(int64(v), 0), nil
			}
		}
	}

	return time.Time{}, fmt.Errorf("failed to parse server time response")
}

// ========== 只读客户端实现 ==========

// GetMidpoint 获取单个代币的中间价（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetMidpoint(tokenID string) (*types.Midpoint, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.Midpoint](c.readonlyBaseClient.baseURL, internal.MidPoint, params)
}

// GetMidpoints 批量获取多个代币的中间价（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetMidpoints(tokenIDs []string) ([]types.Midpoint, error) {
	if len(tokenIDs) == 0 {
		return []types.Midpoint{}, nil
	}
	if len(tokenIDs) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(tokenIDs))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(tokenIDs))
	for i, tokenID := range tokenIDs {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	// API返回的是对象 map[token_id]price_string，而不是数组
	// 需要先获取原始响应，然后转换为数组
	// 先序列化请求体
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: failed to marshal request body: %w", err)
	}
	
	rawBytes, err := http.PostRaw(c.readonlyBaseClient.baseURL, internal.MidPoints, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: %w", err)
	}

	// 解析为map[string]string (token_id -> price_string)
	var responseMap map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Midpoint, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if priceStr, ok := responseMap[tokenID]; ok {
			price, err := strconv.ParseFloat(priceStr, 64)
			if err != nil {
				return nil, fmt.Errorf("批量获取中间价失败: failed to parse price for token %s: %w", tokenID, err)
			}
			result = append(result, types.Midpoint{
				TokenID: tokenID,
				Value:   price,
			})
		}
	}

	return result, nil
}

// GetPrice 获取指定方向的价格（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetPrice(tokenID string, side types.OrderSide) (*types.Price, error) {
	params := map[string]string{
		"token_id": tokenID,
		"side":     string(side),
	}
	return http.Get[types.Price](c.readonlyBaseClient.baseURL, internal.Price, params)
}

// GetPrices 批量获取多个代币的价格（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetPrices(requests []types.BookParams) ([]types.Price, error) {
	if len(requests) == 0 {
		return []types.Price{}, nil
	}
	if len(requests) > 500 {
		return nil, fmt.Errorf("请求数组长度不能超过500，当前: %d", len(requests))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(requests))
	for i, req := range requests {
		requestBody[i] = map[string]string{
			"token_id": req.TokenID,
		}
		if req.Side != "" {
			requestBody[i]["side"] = req.Side
		}
	}

	// API返回的是嵌套对象 map[token_id]map[side]price_string，例如: {"token_id": {"BUY": "0.52"}}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取价格失败: failed to marshal request body: %w", err)
	}
	
	rawBytes, err := http.PostRaw(c.readonlyBaseClient.baseURL, internal.GetPrices, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取价格失败: %w", err)
	}

	// 先尝试解析为数组（向后兼容）
	var priceArray []types.Price
	if err := json.Unmarshal(rawBytes, &priceArray); err == nil {
		return priceArray, nil
	}

	// 如果解析为数组失败，尝试解析为嵌套对象 map[token_id]map[side]price_string
	var responseMap map[string]map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取价格失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Price, 0, len(requests))
	for _, req := range requests {
		if tokenMap, ok := responseMap[req.TokenID]; ok {
			// 如果指定了side，使用指定的side；否则尝试BUY或SELL
			var priceStr string
			var found bool
			if req.Side != "" {
				if p, ok := tokenMap[req.Side]; ok {
					priceStr = p
					found = true
				}
			} else {
				// 没有指定side，尝试BUY或SELL
				if p, ok := tokenMap["BUY"]; ok {
					priceStr = p
					found = true
					req.Side = "BUY"
				} else if p, ok := tokenMap["SELL"]; ok {
					priceStr = p
					found = true
					req.Side = "SELL"
				}
			}
			
			if found {
				price, err := strconv.ParseFloat(priceStr, 64)
				if err != nil {
					return nil, fmt.Errorf("批量获取价格失败: failed to parse price for token %s: %w", req.TokenID, err)
				}
				result = append(result, types.Price{
					BookParams: req,
					Price:      price,
				})
			}
		}
	}

	return result, nil
}

// GetSpread 获取单个代币的价差（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetSpread(tokenID string) (*types.Spread, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.Spread](c.readonlyBaseClient.baseURL, internal.GetSpread, params)
}

// GetSpreads 批量获取多个代币的价差（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetSpreads(tokenIDs []string) ([]types.Spread, error) {
	if len(tokenIDs) == 0 {
		return []types.Spread{}, nil
	}
	if len(tokenIDs) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(tokenIDs))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(tokenIDs))
	for i, tokenID := range tokenIDs {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	// API返回的是对象 map[token_id]spread_string，而不是数组
	// 需要先获取原始响应，然后转换为数组
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取价差失败: failed to marshal request body: %w", err)
	}
	
	rawBytes, err := http.PostRaw(c.readonlyBaseClient.baseURL, internal.GetSpreads, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取价差失败: %w", err)
	}

	// 解析为map[string]string (token_id -> spread_string)
	var responseMap map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取价差失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Spread, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if spreadStr, ok := responseMap[tokenID]; ok {
			spread, err := strconv.ParseFloat(spreadStr, 64)
			if err != nil {
				return nil, fmt.Errorf("批量获取价差失败: failed to parse spread for token %s: %w", tokenID, err)
			}
			result = append(result, types.Spread{
				TokenID: tokenID,
				Value:   spread,
			})
		}
	}

	return result, nil
}

// GetLastTradePrice 获取单个代币的最后成交价（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetLastTradePrice(tokenID string) (*types.LastTradePrice, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.LastTradePrice](c.readonlyBaseClient.baseURL, internal.GetLastTradePrice, params)
}

// GetLastTradesPrices 批量获取多个代币的最后成交价（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetLastTradesPrices(tokenIDs []string) ([]types.LastTradePrice, error) {
	if len(tokenIDs) == 0 {
		return []types.LastTradePrice{}, nil
	}
	if len(tokenIDs) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(tokenIDs))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(tokenIDs))
	for i, tokenID := range tokenIDs {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	result, err := http.Post[[]types.LastTradePrice](c.readonlyBaseClient.baseURL, internal.GetLastTradesPrices, requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取最后成交价失败: %w", err)
	}

	return *result, nil
}

// GetFeeRate 获取代币的手续费率（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetFeeRate(tokenID string) (int, error) {
	// 检查缓存
	if feeRate, ok := c.readonlyBaseClient.feeRates[tokenID]; ok {
		return feeRate, nil
	}

	params := map[string]string{"token_id": tokenID}

	// API 可能返回数字或字符串格式的 fee_rate
	var rawResponse map[string]interface{}
	resp, err := http.Get[map[string]interface{}](c.readonlyBaseClient.baseURL, internal.GetFeeRate, params)
	if err != nil {
		return 0, fmt.Errorf("failed to get fee rate: %w", err)
	}
	rawResponse = *resp

	// 提取 fee_rate 或 base_fee 并转换为 int
	// API可能返回 fee_rate 或 base_fee 字段
	var feeRate int
	var found bool
	
	// 优先查找 fee_rate
	if val, ok := rawResponse["fee_rate"]; ok {
		found = true
		switch v := val.(type) {
		case float64:
			feeRate = int(v)
		case int:
			feeRate = v
		case int64:
			feeRate = int(v)
		case string:
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return 0, fmt.Errorf("failed to parse fee_rate as int: %w", err)
			}
			feeRate = parsed
		default:
			return 0, fmt.Errorf("unexpected fee_rate type: %T", v)
		}
	} else if val, ok := rawResponse["base_fee"]; ok {
		// 如果没有 fee_rate，尝试 base_fee
		found = true
		switch v := val.(type) {
		case float64:
			feeRate = int(v)
		case int:
			feeRate = v
		case int64:
			feeRate = int(v)
		case string:
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return 0, fmt.Errorf("failed to parse base_fee as int: %w", err)
			}
			feeRate = parsed
		default:
			return 0, fmt.Errorf("unexpected base_fee type: %T", v)
		}
	}
	
	if !found {
		return 0, fmt.Errorf("fee_rate or base_fee not found in response")
	}

	// 缓存结果
	c.readonlyBaseClient.feeRates[tokenID] = feeRate
	return feeRate, nil
}

// GetTime 获取服务器时间（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetTime() (time.Time, error) {
	// API返回的是纯数字（Unix时间戳），不是JSON对象
	rawBytes, err := http.GetRaw(c.readonlyBaseClient.baseURL, "GET", internal.Time, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get server time: %w", err)
	}

	// 尝试解析为数字（Unix时间戳）
	var timestamp int64
	if err := json.Unmarshal(rawBytes, &timestamp); err == nil {
		return time.Unix(timestamp, 0), nil
	}

	// 如果解析为数字失败，尝试解析为JSON对象
	var timeResponse struct {
		Time interface{} `json:"time"`
	}
	if err := json.Unmarshal(rawBytes, &timeResponse); err == nil {
		if timeResponse.Time != nil {
			switch v := timeResponse.Time.(type) {
			case string:
				// 解析时间字符串（通常是 RFC3339 格式）
				serverTime, err := time.Parse(time.RFC3339, v)
				if err != nil {
					// 尝试其他常见格式
					if serverTime, err = time.Parse("2006-01-02T15:04:05Z07:00", v); err != nil {
						return time.Time{}, fmt.Errorf("failed to parse server time: %w", err)
					}
				}
				return serverTime, nil
			case float64:
				return time.Unix(int64(v), 0), nil
			case int64:
				return time.Unix(v, 0), nil
			case int:
				return time.Unix(int64(v), 0), nil
			}
		}
	}

	return time.Time{}, fmt.Errorf("failed to parse server time response")
}
