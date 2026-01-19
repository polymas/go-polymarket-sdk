package gamma

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetMarket 通过市场ID获取市场
func (c *polymarketGammaClient) GetMarket(marketID string) (*types.GammaMarket, error) {
	return http.Get[types.GammaMarket](c.baseURL, fmt.Sprintf("/markets/%s", marketID), nil)
}

// GetMarketBySlug 通过slug获取市场
func (c *polymarketGammaClient) GetMarketBySlug(slug string, includeTag *bool) (*types.GammaMarket, error) {
	params := make(map[string]string)
	if includeTag != nil {
		params["include_tag"] = strconv.FormatBool(*includeTag)
	}
	return http.Get[types.GammaMarket](c.baseURL, fmt.Sprintf("/markets/slug/%s", slug), params)
}

// GetMarketsOptions 包含 GetMarkets 的所有可选参数
type GetMarketsOptions struct {
	Offset              int
	Order               *string
	Ascending           bool
	Archived            *bool
	Active              *bool
	Closed              *bool
	Slugs               []string
	MarketIDs           []int
	TokenIDs            []string
	ConditionIDs        []string
	TagID               *int
	RelatedTags         *bool
	UmaResolutionStatus *string
}

// GetMarketsOption 函数选项类型
type GetMarketsOption func(*GetMarketsOptions)

// WithOffset 设置偏移量
func WithOffset(offset int) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.Offset = offset
	}
}

// WithOrder 设置排序字段和方向
func WithOrder(order string, ascending bool) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.Order = &order
		opts.Ascending = ascending
	}
}

// WithArchived 设置是否包含已归档市场
func WithArchived(archived bool) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.Archived = &archived
	}
}

// WithActive 设置是否只获取活跃市场
func WithActive(active bool) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.Active = &active
	}
}

// WithClosed 设置是否只获取已关闭市场
func WithClosed(closed bool) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.Closed = &closed
	}
}

// WithSlugs 设置slug列表
func WithSlugs(slugs []string) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.Slugs = slugs
	}
}

// WithMarketIDs 设置市场ID列表
func WithMarketIDs(marketIDs []int) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.MarketIDs = marketIDs
	}
}

// WithTokenIDs 设置token ID列表
func WithTokenIDs(tokenIDs []string) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.TokenIDs = tokenIDs
	}
}

// WithConditionIDs 设置条件ID列表
func WithConditionIDs(conditionIDs []string) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.ConditionIDs = conditionIDs
	}
}

// WithTagID 设置标签ID
func WithTagID(tagID int, relatedTags *bool) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.TagID = &tagID
		opts.RelatedTags = relatedTags
	}
}

// WithUmaResolutionStatus 设置UMA解析状态
func WithUmaResolutionStatus(status string) GetMarketsOption {
	return func(opts *GetMarketsOptions) {
		opts.UmaResolutionStatus = &status
	}
}

// GetDisputeMarkets 获取争议市场
// 在 Certainty 市场基础上，过滤出有 dispute 状态的市场
func (c *polymarketGammaClient) GetDisputeMarkets() ([]types.GammaMarket, error) {
	// 先获取所有 Certainty 市场
	certaintyMarkets, err := c.GetCertaintyMarkets()
	if err != nil {
		return nil, err
	}

	// 过滤出有 dispute 状态的市场
	disputeMarkets := make([]types.GammaMarket, 0)
	for i := range certaintyMarkets {
		if types.HasDispute(&certaintyMarkets[i]) {
			disputeMarkets = append(disputeMarkets, certaintyMarkets[i])
		}
	}

	return disputeMarkets, nil
}

// GetCertaintyMarkets 获取尾盘数据（Certainty 市场）
// 过滤条件：active=true, closed=false, uma_resolution_status=proposed, 按 endDate 升序排序
func (c *polymarketGammaClient) GetCertaintyMarkets() ([]types.GammaMarket, error) {
	return c.getMarkets(500,
		WithOffset(0),
		WithOrder("endDate", true),
		WithActive(true),
		WithClosed(false),
		WithUmaResolutionStatus("proposed"),
	)
}

// GetMarketsByConditionIDs 根据条件ID列表获取市场
func (c *polymarketGammaClient) GetMarketsByConditionIDs(conditionIDs []string) ([]types.GammaMarket, error) {
	if len(conditionIDs) == 0 {
		return []types.GammaMarket{}, nil
	}
	return c.getMarkets(500, WithConditionIDs(conditionIDs))
}

// GetMarkets 获取市场列表（支持分页和过滤）
// limit 是每页的数量，options 是过滤选项
func (c *polymarketGammaClient) GetMarkets(limit int, options ...GetMarketsOption) ([]types.GammaMarket, error) {
	return c.getMarkets(limit, options...)
}

// GetAllMarkets 获取所有历史市场数据（自动分页）
// 自动处理分页，返回所有市场数据，不限制状态（包括活跃、关闭、归档等所有市场）
func (c *polymarketGammaClient) GetAllMarkets() ([]types.GammaMarket, error) {
	const pageSize = 500 // 每页500条，减少请求次数
	allMarkets := make([]types.GammaMarket, 0)
	page := 0

	for {
		offset := page * pageSize
		markets, err := c.getMarkets(pageSize, WithOffset(offset), WithOrder("endDate", true))
		if err != nil {
			return nil, err
		}

		// 如果返回的数据为空，说明已经获取完所有数据
		if len(markets) == 0 {
			break
		}

		allMarkets = append(allMarkets, markets...)

		// 如果返回的数据少于 pageSize，说明已经是最后一页
		if len(markets) < pageSize {
			break
		}

		page++
	}

	return allMarkets, nil
}

// getMarkets 使用过滤器获取市场列表（内部方法）
// limit 是必要参数，其他参数通过选项函数传入
// 内部使用 raw 数据解析，确保所有字段都被正确解析
func (c *polymarketGammaClient) getMarkets(limit int, options ...GetMarketsOption) ([]types.GammaMarket, error) {
	// 初始化默认选项
	opts := &GetMarketsOptions{}

	// 应用所有选项
	for _, option := range options {
		option(opts)
	}

	// 单个值参数
	params := make(map[string]string)
	params["include_tag"] = "true"
	params["limit"] = strconv.Itoa(limit)
	params["offset"] = strconv.Itoa(opts.Offset)

	if opts.Order != nil {
		params["order"] = *opts.Order
		params["ascending"] = strconv.FormatBool(opts.Ascending)
	}
	if opts.Archived != nil {
		params["archived"] = strconv.FormatBool(*opts.Archived)
	}
	if opts.Active != nil {
		params["active"] = strconv.FormatBool(*opts.Active)
	}
	if opts.Closed != nil {
		params["closed"] = strconv.FormatBool(*opts.Closed)
	}
	if opts.TagID != nil {
		params["tag_id"] = strconv.Itoa(*opts.TagID)
		if opts.RelatedTags != nil {
			params["related_tags"] = strconv.FormatBool(*opts.RelatedTags)
		}
	}
	if opts.UmaResolutionStatus != nil {
		params["uma_resolution_status"] = *opts.UmaResolutionStatus
	}

	// 多个值参数（同名参数）
	multiParams := make(map[string][]string)
	if len(opts.ConditionIDs) > 0 {
		multiParams["condition_ids"] = opts.ConditionIDs
	}
	if len(opts.Slugs) > 0 {
		multiParams["slugs"] = opts.Slugs
	}
	if len(opts.MarketIDs) > 0 {
		marketIDStrs := make([]string, len(opts.MarketIDs))
		for i, id := range opts.MarketIDs {
			marketIDStrs[i] = strconv.Itoa(id)
		}
		multiParams["market_ids"] = marketIDStrs
	}
	if len(opts.TokenIDs) > 0 {
		multiParams["clob_token_ids"] = opts.TokenIDs
	}

	rawJSON, err := http.GetRaw(c.baseURL, "GET", "/markets", params, http.WithMultiParams(multiParams))
	if err != nil {
		return nil, err
	}

	// 使用 types.GammaMarket 的自定义 UnmarshalJSON 解析
	var markets1 []types.GammaMarket
	if err := json.Unmarshal(rawJSON, &markets1); err != nil {
		return nil, fmt.Errorf("解析市场JSON失败: %w", err)
	}

	return markets1, nil
}

// GetSamplingSimplifiedMarkets 获取采样简化市场
func (c *polymarketGammaClient) GetSamplingSimplifiedMarkets(limit int) ([]types.SimplifiedMarket, error) {
	params := map[string]string{
		"limit": strconv.Itoa(limit),
	}

	result, err := http.Get[[]types.SimplifiedMarket](c.baseURL, internal.GetSamplingSimplifiedMarkets, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get sampling simplified markets: %w", err)
	}

	if result == nil {
		return []types.SimplifiedMarket{}, nil
	}

	return *result, nil
}

// GetSamplingMarkets 获取采样市场
func (c *polymarketGammaClient) GetSamplingMarkets(limit int) ([]types.GammaMarket, error) {
	params := map[string]string{
		"limit": strconv.Itoa(limit),
	}

	result, err := http.Get[[]types.GammaMarket](c.baseURL, internal.GetSamplingMarkets, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get sampling markets: %w", err)
	}

	if result == nil {
		return []types.GammaMarket{}, nil
	}

	return *result, nil
}

// GetSimplifiedMarkets 获取简化市场列表
func (c *polymarketGammaClient) GetSimplifiedMarkets(limit int, offset int, options ...GetMarketsOption) ([]types.SimplifiedMarket, error) {
	// 初始化默认选项
	opts := &GetMarketsOptions{}

	// 应用所有选项
	for _, option := range options {
		option(opts)
	}

	// 构建参数
	params := make(map[string]string)
	params["limit"] = strconv.Itoa(limit)
	params["offset"] = strconv.Itoa(offset)

	if opts.Order != nil {
		params["order"] = *opts.Order
		params["ascending"] = strconv.FormatBool(opts.Ascending)
	}
	if opts.Archived != nil {
		params["archived"] = strconv.FormatBool(*opts.Archived)
	}
	if opts.Active != nil {
		params["active"] = strconv.FormatBool(*opts.Active)
	}
	if opts.Closed != nil {
		params["closed"] = strconv.FormatBool(*opts.Closed)
	}
	if opts.TagID != nil {
		params["tag_id"] = strconv.Itoa(*opts.TagID)
		if opts.RelatedTags != nil {
			params["related_tags"] = strconv.FormatBool(*opts.RelatedTags)
		}
	}
	if opts.UmaResolutionStatus != nil {
		params["uma_resolution_status"] = *opts.UmaResolutionStatus
	}

	// 多个值参数（同名参数）
	multiParams := make(map[string][]string)
	if len(opts.ConditionIDs) > 0 {
		multiParams["condition_ids"] = opts.ConditionIDs
	}
	if len(opts.Slugs) > 0 {
		multiParams["slugs"] = opts.Slugs
	}
	if len(opts.MarketIDs) > 0 {
		marketIDStrs := make([]string, len(opts.MarketIDs))
		for i, id := range opts.MarketIDs {
			marketIDStrs[i] = strconv.Itoa(id)
		}
		multiParams["market_ids"] = marketIDStrs
	}
	if len(opts.TokenIDs) > 0 {
		multiParams["clob_token_ids"] = opts.TokenIDs
	}

	result, err := http.Get[[]types.SimplifiedMarket](c.baseURL, internal.GetSimplifiedMarkets, params, http.WithMultiParams(multiParams))
	if err != nil {
		return nil, fmt.Errorf("failed to get simplified markets: %w", err)
	}

	if result == nil {
		return []types.SimplifiedMarket{}, nil
	}

	return *result, nil
}

// GetMarketTradesEvents 获取市场交易事件
func (c *polymarketGammaClient) GetMarketTradesEvents(marketID string, limit int, offset int) ([]types.MarketTradesEvent, error) {
	params := map[string]string{
		"limit":  strconv.Itoa(limit),
		"offset": strconv.Itoa(offset),
	}

	result, err := http.Get[[]types.MarketTradesEvent](c.baseURL, fmt.Sprintf("%s%s", internal.GetMarketTradesEvents, marketID), params)
	if err != nil {
		return nil, fmt.Errorf("failed to get market trades events: %w", err)
	}

	if result == nil {
		return []types.MarketTradesEvent{}, nil
	}

	return *result, nil
}
