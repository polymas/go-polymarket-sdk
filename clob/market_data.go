package clob

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// tokenIDPattern: ERC1155 tokenId 是 uint256 的十进制字符串，1~78 位数字
// （uint256 max = 2^256-1 共 78 位）。SDK 在调用批量端点前用此模式预过滤，
// 把明显非法的输入（hex / 字母 / 过长 / 全 0）直接挡掉，避免无意义的网关
// 往返。
var tokenIDPattern = regexp.MustCompile(`^[0-9]{1,78}$`)

// isValidTokenIDFormat 判断字符串是否是合法格式的 ERC1155 tokenId。
// 注意：仅做格式校验，不保证该 token 在链上存在或市场仍然有效——这两类
// "格式合法但不存在/已结算" 的情况由网关返回缺失项，业务层视 len(result)
// 短于输入长度即可识别。
func isValidTokenIDFormat(id string) bool {
	if !tokenIDPattern.MatchString(id) {
		return false
	}
	// 拒绝全 0（"0"、"00" 等），它们格式合法但语义无效
	if strings.Trim(id, "0") == "" {
		return false
	}
	return true
}

// sanitizeTokenIDs 对批量端点的输入做三重清洗：
//  1. trim 前后空白（实测 V2 网关不做 trim，"  V1  " 会被当作未知 token）
//  2. 丢弃空串和格式非法（非数字 / 超过 78 位 / 全 0）的项
//  3. 按原顺序去重（V2 网关也会自动 dedupe，但提前去重能减少请求体大小）
//
// 实测（2026-05-07，V2 数据面 = clob.polymarket.com）：批量端点对绝大多数
// 坏 token 是容错的——空串、null、不存在的 77 位数、字母串都会被静默丢弃，
// 整批仍 200。唯二会触发 400 Invalid payload 的是：① token_id 不是 JSON
// 字符串（数字/数组/bool）；② 顶层不是数组。SDK 编码层用 map[string]string
// 已规避 ①，本函数主要为节省往返 + 让业务层透明处理坏数据。
func sanitizeTokenIDs(tokenIDs []string) []string {
	seen := make(map[string]struct{}, len(tokenIDs))
	cleaned := make([]string, 0, len(tokenIDs))
	for _, id := range tokenIDs {
		id = strings.TrimSpace(id)
		if !isValidTokenIDFormat(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, id)
	}
	return cleaned
}

// sanitizeBookParams 与 sanitizeTokenIDs 同语义，但对 BookParams 按
// (TokenID, Side) 去重，保留 BUY/SELL 同 token 的独立请求。
func sanitizeBookParams(requests []types.BookParams) []types.BookParams {
	type key struct{ tokenID, side string }
	seen := make(map[key]struct{}, len(requests))
	cleaned := make([]types.BookParams, 0, len(requests))
	for _, req := range requests {
		req.TokenID = strings.TrimSpace(req.TokenID)
		if !isValidTokenIDFormat(req.TokenID) {
			continue
		}
		k := key{req.TokenID, req.Side}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		cleaned = append(cleaned, req)
	}
	return cleaned
}

// payloadSnippet 截取请求体首 200 字符，用于在批量端点失败时把发出去的
// JSON 摘要带进 error message，避免线上只能看到 "HTTP 400: Invalid payload"。
func payloadSnippet(body []byte) string {
	const max = 200
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...(truncated)"
}

// PricesHistoryOptions 为 GET /prices-history 提供的可选参数
// 参考: https://docs.polymarket.com/developers/CLOB/timeseries
type PricesHistoryOptions struct {
	StartTs  *int64 // 仅返回此 Unix 时间戳之后的数据
	EndTs    *int64 // 仅返回此 Unix 时间戳之前的数据
	Interval string // 聚合间隔: max, all, 1m, 1w, 1d, 6h, 1h
	Fidelity *int   // 精度（分钟），默认 1
}

// PricesHistoryOption 函数选项类型
type PricesHistoryOption func(*PricesHistoryOptions)

// WithPricesHistoryStartTs 设置起始时间戳
func WithPricesHistoryStartTs(ts int64) PricesHistoryOption {
	return func(o *PricesHistoryOptions) {
		o.StartTs = &ts
	}
}

// WithPricesHistoryEndTs 设置结束时间戳
func WithPricesHistoryEndTs(ts int64) PricesHistoryOption {
	return func(o *PricesHistoryOptions) {
		o.EndTs = &ts
	}
}

// WithPricesHistoryInterval 设置时间间隔（max, all, 1m, 1w, 1d, 6h, 1h）
func WithPricesHistoryInterval(interval string) PricesHistoryOption {
	return func(o *PricesHistoryOptions) {
		o.Interval = interval
	}
}

// WithPricesHistoryFidelity 设置精度（分钟）
func WithPricesHistoryFidelity(minutes int) PricesHistoryOption {
	return func(o *PricesHistoryOptions) {
		o.Fidelity = &minutes
	}
}

// getTickSize 显式查询代币的 tick 大小，供认证和只读客户端共用。
// 本方法不隐藏缓存；是否缓存、何时刷新由业务层决定。
func getTickSize(baseURL string, tokenID string) (types.TickSize, error) {
	return getTickSizeWithFetcher(tokenID, func(tokenID string) (map[string]interface{}, error) {
		params := map[string]string{"token_id": tokenID}
		resp, err := http.Get[map[string]interface{}](baseURL, internal.GetTickSize, params)
		if err != nil {
			return nil, err
		}
		return *resp, nil
	})
}

// getTickSizeWithFetcher 保持查询与解析逻辑可测试；每次调用都执行 fetch。
func getTickSizeWithFetcher(tokenID string, fetch func(string) (map[string]interface{}, error)) (types.TickSize, error) {
	rawResponse, err := fetch(tokenID)
	if err != nil {
		return "", fmt.Errorf("failed to get tick size: %w", err)
	}

	// API may return minimum_tick_size as number or string, so we need to handle both.
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

	return types.TickSize(tickSizeStr), nil
}

// GetTickSize 显式获取代币的 tick 大小。下单路径不会隐式调用本方法。
func (c *marketDataClientImpl) GetTickSize(tokenID string) (types.TickSize, error) {
	return getTickSize(c.baseClient.baseURL, tokenID)
}

// GetTickSize 显式获取代币的 tick 大小。
func (c *readonlyMarketDataClientImpl) GetTickSize(tokenID string) (types.TickSize, error) {
	return getTickSize(c.readonlyBaseClient.baseURL, tokenID)
}

// GetNegRisk 获取代币的负风险状态。
//
// 缓存命中时直接返回；未命中时走 baseClient.negRiskForToken 查询（共用
// baseClient.negRisk 缓存）。仅当查询失败且无缓存时才返回 error——
// negRiskForToken 内部把失败吞成 false，这里复查缓存以区分 "查到 = false"
// 与 "查询失败"，保留原有的 error 契约。
func (c *marketDataClientImpl) GetNegRisk(tokenID string) (bool, error) {
	if negRisk, ok := c.baseClient.negRisk[tokenID]; ok {
		return negRisk, nil
	}
	v := c.baseClient.negRiskForToken(tokenID)
	if _, ok := c.baseClient.negRisk[tokenID]; !ok {
		return false, fmt.Errorf("failed to get neg risk for token %s", tokenID)
	}
	return v, nil
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
func (c *marketDataClientImpl) GetMultipleOrderBooks(requests []types.BookParams) ([]types.OrderBookSummary, error) {
	cleaned := sanitizeBookParams(requests)
	// 与 GetMidpoints/GetSpreads/GetLastTradesPrices 行为对齐：sanitize 后为空
	// 直接返回空切片，不发请求也不报错，让业务层透明处理坏 tokenId。
	if len(cleaned) == 0 {
		return []types.OrderBookSummary{}, nil
	}
	if len(cleaned) > 500 {
		return nil, fmt.Errorf("请求数组长度不能超过500，当前: %d", len(cleaned))
	}

	// 构建请求体（只包含必需的字段）
	requestBody := make([]map[string]string, len(cleaned))
	for i, req := range cleaned {
		requestBody[i] = map[string]string{
			"token_id": req.TokenID,
		}
		// 如果提供了 side，添加到请求中
		if req.Side != "" {
			requestBody[i]["side"] = req.Side
		}
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取订单簿失败: failed to marshal request body: %w", err)
	}

	rawBytes, err := http.PostRaw(c.baseClient.baseURL, internal.GetOrderBooks, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取订单簿失败 (n=%d, payload=%s): %w", len(cleaned), payloadSnippet(bodyBytes), err)
	}

	var result []types.OrderBookSummary
	if err := json.Unmarshal(rawBytes, &result); err != nil {
		return nil, fmt.Errorf("批量获取订单簿失败: failed to decode response: %w", err)
	}
	return result, nil
}

// GetMultipleOrderBooks 批量获取多个订单簿摘要（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetMultipleOrderBooks(requests []types.BookParams) ([]types.OrderBookSummary, error) {
	cleaned := sanitizeBookParams(requests)
	if len(cleaned) == 0 {
		return []types.OrderBookSummary{}, nil
	}
	if len(cleaned) > 500 {
		return nil, fmt.Errorf("请求数组长度不能超过500，当前: %d", len(cleaned))
	}

	// 构建请求体（只包含必需的字段）
	requestBody := make([]map[string]string, len(cleaned))
	for i, req := range cleaned {
		requestBody[i] = map[string]string{
			"token_id": req.TokenID,
		}
		// 如果提供了 side，添加到请求中
		if req.Side != "" {
			requestBody[i]["side"] = req.Side
		}
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取订单簿失败: failed to marshal request body: %w", err)
	}

	rawBytes, err := http.PostRaw(c.readonlyBaseClient.baseURL, internal.GetOrderBooks, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取订单簿失败 (n=%d, payload=%s): %w", len(cleaned), payloadSnippet(bodyBytes), err)
	}

	var result []types.OrderBookSummary
	if err := json.Unmarshal(rawBytes, &result); err != nil {
		return nil, fmt.Errorf("批量获取订单簿失败: failed to decode response: %w", err)
	}
	return result, nil
}

// GetMidpoint 获取单个代币的中间价
func (c *marketDataClientImpl) GetMidpoint(tokenID string) (*types.Midpoint, error) {
	params := map[string]string{"token_id": tokenID}
	return http.Get[types.Midpoint](c.baseClient.baseURL, internal.MidPoint, params)
}

// GetMidpoints 批量获取多个代币的中间价
func (c *marketDataClientImpl) GetMidpoints(tokenIDs []string) ([]types.Midpoint, error) {
	cleaned := sanitizeTokenIDs(tokenIDs)
	if len(cleaned) == 0 {
		return []types.Midpoint{}, nil
	}
	if len(cleaned) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(cleaned))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(cleaned))
	for i, tokenID := range cleaned {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	// API返回的是对象 map[token_id]price_string，而不是数组
	// 需要先获取原始响应，然后转换为数组
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: failed to marshal request body: %w", err)
	}

	rawBytes, err := http.PostRaw(c.baseClient.baseURL, internal.MidPoints, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取中间价失败 (n=%d, payload=%s): %w", len(cleaned), payloadSnippet(bodyBytes), err)
	}

	// 解析为map[string]string (token_id -> price_string)
	var responseMap map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Midpoint, 0, len(cleaned))
	for _, tokenID := range cleaned {
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
	cleaned := sanitizeBookParams(requests)
	if len(cleaned) == 0 {
		return []types.Price{}, nil
	}
	if len(cleaned) > 500 {
		return nil, fmt.Errorf("请求数组长度不能超过500，当前: %d", len(cleaned))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(cleaned))
	for i, req := range cleaned {
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
		return nil, fmt.Errorf("批量获取价格失败 (n=%d, payload=%s): %w", len(cleaned), payloadSnippet(bodyBytes), err)
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
	result := make([]types.Price, 0, len(cleaned))
	for _, req := range cleaned {
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
	cleaned := sanitizeTokenIDs(tokenIDs)
	if len(cleaned) == 0 {
		return []types.Spread{}, nil
	}
	if len(cleaned) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(cleaned))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(cleaned))
	for i, tokenID := range cleaned {
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
		return nil, fmt.Errorf("批量获取价差失败 (n=%d, payload=%s): %w", len(cleaned), payloadSnippet(bodyBytes), err)
	}

	// 解析为map[string]string (token_id -> spread_string)
	var responseMap map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取价差失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Spread, 0, len(cleaned))
	for _, tokenID := range cleaned {
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
	cleaned := sanitizeTokenIDs(tokenIDs)
	if len(cleaned) == 0 {
		return []types.LastTradePrice{}, nil
	}
	if len(cleaned) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(cleaned))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(cleaned))
	for i, tokenID := range cleaned {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取最后成交价失败: failed to marshal request body: %w", err)
	}

	rawBytes, err := http.PostRaw(c.baseClient.baseURL, internal.GetLastTradesPrices, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取最后成交价失败 (n=%d, payload=%s): %w", len(cleaned), payloadSnippet(bodyBytes), err)
	}

	var result []types.LastTradePrice
	if err := json.Unmarshal(rawBytes, &result); err != nil {
		return nil, fmt.Errorf("批量获取最后成交价失败: failed to decode response: %w", err)
	}
	return result, nil
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

// GetPricesHistory 获取市场的历史价格时序（GET /prices-history）
// market 为必填的 asset id；可选参数见 PricesHistoryOption
func (c *marketDataClientImpl) GetPricesHistory(market string, opts ...PricesHistoryOption) (*types.PricesHistoryResponse, error) {
	if market == "" {
		return nil, fmt.Errorf("market is required")
	}
	options := &PricesHistoryOptions{}
	for _, f := range opts {
		f(options)
	}
	params := map[string]string{"market": market}
	if options.StartTs != nil {
		params["startTs"] = strconv.FormatInt(*options.StartTs, 10)
	}
	if options.EndTs != nil {
		params["endTs"] = strconv.FormatInt(*options.EndTs, 10)
	}
	if options.Interval != "" {
		params["interval"] = options.Interval
	}
	if options.Fidelity != nil {
		params["fidelity"] = strconv.Itoa(*options.Fidelity)
	}
	return http.Get[types.PricesHistoryResponse](c.baseClient.baseURL, internal.GetPricesHistory, params)
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
	cleaned := sanitizeTokenIDs(tokenIDs)
	if len(cleaned) == 0 {
		return []types.Midpoint{}, nil
	}
	if len(cleaned) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(cleaned))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(cleaned))
	for i, tokenID := range cleaned {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	// API返回的是对象 map[token_id]price_string，而不是数组
	// 需要先获取原始响应，然后转换为数组
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: failed to marshal request body: %w", err)
	}

	rawBytes, err := http.PostRaw(c.readonlyBaseClient.baseURL, internal.MidPoints, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取中间价失败 (n=%d, payload=%s): %w", len(cleaned), payloadSnippet(bodyBytes), err)
	}

	// 解析为map[string]string (token_id -> price_string)
	var responseMap map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取中间价失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Midpoint, 0, len(cleaned))
	for _, tokenID := range cleaned {
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
	cleaned := sanitizeTokenIDs(tokenIDs)
	if len(cleaned) == 0 {
		return []types.Spread{}, nil
	}
	if len(cleaned) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(cleaned))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(cleaned))
	for i, tokenID := range cleaned {
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
		return nil, fmt.Errorf("批量获取价差失败 (n=%d, payload=%s): %w", len(cleaned), payloadSnippet(bodyBytes), err)
	}

	// 解析为map[string]string (token_id -> spread_string)
	var responseMap map[string]string
	if err := json.Unmarshal(rawBytes, &responseMap); err != nil {
		return nil, fmt.Errorf("批量获取价差失败: failed to decode response: %w", err)
	}

	// 转换为数组格式
	result := make([]types.Spread, 0, len(cleaned))
	for _, tokenID := range cleaned {
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
	cleaned := sanitizeTokenIDs(tokenIDs)
	if len(cleaned) == 0 {
		return []types.LastTradePrice{}, nil
	}
	if len(cleaned) > 500 {
		return nil, fmt.Errorf("tokenIDs数组长度不能超过500，当前: %d", len(cleaned))
	}

	// 构建请求体
	requestBody := make([]map[string]string, len(cleaned))
	for i, tokenID := range cleaned {
		requestBody[i] = map[string]string{
			"token_id": tokenID,
		}
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("批量获取最后成交价失败: failed to marshal request body: %w", err)
	}

	rawBytes, err := http.PostRaw(c.readonlyBaseClient.baseURL, internal.GetLastTradesPrices, bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("批量获取最后成交价失败 (n=%d, payload=%s): %w", len(cleaned), payloadSnippet(bodyBytes), err)
	}

	var result []types.LastTradePrice
	if err := json.Unmarshal(rawBytes, &result); err != nil {
		return nil, fmt.Errorf("批量获取最后成交价失败: failed to decode response: %w", err)
	}
	return result, nil
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

// GetPricesHistory 获取市场的历史价格时序（只读客户端实现）
func (c *readonlyMarketDataClientImpl) GetPricesHistory(market string, opts ...PricesHistoryOption) (*types.PricesHistoryResponse, error) {
	if market == "" {
		return nil, fmt.Errorf("market is required")
	}
	options := &PricesHistoryOptions{}
	for _, f := range opts {
		f(options)
	}
	params := map[string]string{"market": market}
	if options.StartTs != nil {
		params["startTs"] = strconv.FormatInt(*options.StartTs, 10)
	}
	if options.EndTs != nil {
		params["endTs"] = strconv.FormatInt(*options.EndTs, 10)
	}
	if options.Interval != "" {
		params["interval"] = options.Interval
	}
	if options.Fidelity != nil {
		params["fidelity"] = strconv.Itoa(*options.Fidelity)
	}
	return http.Get[types.PricesHistoryResponse](c.readonlyBaseClient.baseURL, internal.GetPricesHistory, params)
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
