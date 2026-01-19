package clob

import (
	"fmt"
	"strconv"
	"time"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetTickSize 获取代币的tick大小
func (c *polymarketClobClient) GetTickSize(tokenID string) (types.TickSize, error) {
	if tickSize, ok := c.tickSizes[tokenID]; ok {
		return tickSize, nil
	}

	params := map[string]string{"token_id": tokenID}

	// API may return minimum_tick_size as number or string, so we need to handle both
	var rawResponse map[string]interface{}
	resp, err := http.Get[map[string]interface{}](c.baseURL, internal.GetTickSize, params)
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
	c.tickSizes[tokenID] = tickSize
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
func (c *polymarketClobClient) ResolveTickSize(tokenID string, userTickSize *types.TickSize) (types.TickSize, error) {
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
func (c *polymarketClobClient) GetNegRisk(tokenID string) (bool, error) {
	if negRisk, ok := c.negRisk[tokenID]; ok {
		return negRisk, nil
	}

	params := map[string]string{"token_id": tokenID}
	var result struct {
		NegRisk bool `json:"neg_risk"`
	}

	resp, err := http.Get[struct {
		NegRisk bool `json:"neg_risk"`
	}](c.baseURL, internal.GetNegRisk, params)
	if err != nil {
		return false, fmt.Errorf("failed to get neg risk: %w", err)
	}
	result = *resp

	c.negRisk[tokenID] = result.NegRisk
	return result.NegRisk, nil
}

// GetOrderBook 获取代币的订单簿
func (c *polymarketClobClient) GetOrderBook(tokenID string) (*types.OrderBookSummary, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.OrderBookSummary](c.baseURL, internal.GetOrderBook, params)
}

// GetMultipleOrderBooks 批量获取多个订单簿摘要
// 根据文档: https://docs.polymarket.com/api-reference/orderbook/get-multiple-order-books-summaries-by-request
// requests: 请求数组，每个元素包含 token_id（必需）和可选的 side（BUY/SELL）
// 最大数组长度: 500
// 返回: 订单簿摘要数组
func (c *polymarketClobClient) GetMultipleOrderBooks(requests []types.BookParams) ([]types.OrderBookSummaryResponse, error) {
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
	result, err := http.Post[[]types.OrderBookSummaryResponse](c.baseURL, internal.GetOrderBooks, requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取订单簿失败: %w", err)
	}

	return *result, nil
}

// GetMidpoint 获取单个代币的中间价
func (c *polymarketClobClient) GetMidpoint(tokenID string) (*types.Midpoint, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.Midpoint](c.baseURL, internal.MidPoint, params)
}

// GetMidpoints 批量获取多个代币的中间价
func (c *polymarketClobClient) GetMidpoints(tokenIDs []string) ([]types.Midpoint, error) {
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

	result, err := http.Post[[]types.Midpoint](c.baseURL, internal.MidPoints, requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: %w", err)
	}

	return *result, nil
}

// GetPrice 获取指定方向的价格
func (c *polymarketClobClient) GetPrice(tokenID string, side types.OrderSide) (*types.Price, error) {
	params := map[string]string{
		"token_id": tokenID,
		"side":     string(side),
	}
	return http.Get[types.Price](c.baseURL, internal.Price, params)
}

// GetPrices 批量获取多个代币的价格
func (c *polymarketClobClient) GetPrices(requests []types.BookParams) ([]types.Price, error) {
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

	result, err := http.Post[[]types.Price](c.baseURL, internal.GetPrices, requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取价格失败: %w", err)
	}

	return *result, nil
}

// GetSpread 获取单个代币的价差
func (c *polymarketClobClient) GetSpread(tokenID string) (*types.Spread, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.Spread](c.baseURL, internal.GetSpread, params)
}

// GetSpreads 批量获取多个代币的价差
func (c *polymarketClobClient) GetSpreads(tokenIDs []string) ([]types.Spread, error) {
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

	result, err := http.Post[[]types.Spread](c.baseURL, internal.GetSpreads, requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取价差失败: %w", err)
	}

	return *result, nil
}

// GetLastTradePrice 获取单个代币的最后成交价
func (c *polymarketClobClient) GetLastTradePrice(tokenID string) (*types.LastTradePrice, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.LastTradePrice](c.baseURL, internal.GetLastTradePrice, params)
}

// GetLastTradesPrices 批量获取多个代币的最后成交价
func (c *polymarketClobClient) GetLastTradesPrices(tokenIDs []string) ([]types.LastTradePrice, error) {
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

	result, err := http.Post[[]types.LastTradePrice](c.baseURL, internal.GetLastTradesPrices, requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取最后成交价失败: %w", err)
	}

	return *result, nil
}

// GetFeeRate 获取代币的手续费率（以 bps 为单位，1 bps = 0.01%）
func (c *polymarketClobClient) GetFeeRate(tokenID string) (int, error) {
	// 检查缓存
	if feeRate, ok := c.feeRates[tokenID]; ok {
		return feeRate, nil
	}

	params := map[string]string{"token_id": tokenID}

	// API 可能返回数字或字符串格式的 fee_rate
	var rawResponse map[string]interface{}
	resp, err := http.Get[map[string]interface{}](c.baseURL, internal.GetFeeRate, params)
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
	c.feeRates[tokenID] = feeRate
	return feeRate, nil
}

// GetTime 获取服务器时间
func (c *polymarketClobClient) GetTime() (time.Time, error) {
	var timeResponse struct {
		Time string `json:"time"`
	}

	resp, err := http.Get[struct {
		Time string `json:"time"`
	}](c.baseURL, internal.Time, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get server time: %w", err)
	}
	timeResponse = *resp

	// 解析时间字符串（通常是 RFC3339 格式）
	serverTime, err := time.Parse(time.RFC3339, timeResponse.Time)
	if err != nil {
		// 尝试其他常见格式
		if serverTime, err = time.Parse("2006-01-02T15:04:05Z07:00", timeResponse.Time); err != nil {
			return time.Time{}, fmt.Errorf("failed to parse server time: %w", err)
		}
	}

	return serverTime, nil
}
