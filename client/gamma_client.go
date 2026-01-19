package client

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/polymarket/go-polymarket-sdk/internal"
	"github.com/polymarket/go-polymarket-sdk/types"
)

// GammaClient 定义Gamma客户端的接口，供外部包使用
type GammaClient interface {
	// 市场相关方法
	GetMarket(marketID string) (*types.GammaMarket, error)
	GetMarketBySlug(slug string, includeTag *bool) (*types.GammaMarket, error)
	GetMarketsByConditionIDs(conditionIDs []string) ([]types.GammaMarket, error)
	GetMarkets(limit int, options ...GetMarketsOption) ([]types.GammaMarket, error) // 获取市场列表（支持分页和过滤）
	GetCertaintyMarkets() ([]types.GammaMarket, error)                              // 获取 Certainty 市场（尾盘市场）
	GetDisputeMarkets() ([]types.GammaMarket, error)                                // 获取争议市场（在 Certainty 市场基础上过滤）
	GetAllMarkets() ([]types.GammaMarket, error)                                    // 获取所有历史市场数据（自动分页）

	// 事件相关方法
	GetEvent(eventID int, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEventBySlug(slug string, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEvents(limit int, offset int, options ...GetEventsOption) ([]types.Event, error)

	// 搜索相关方法
	Search(query string, options ...SearchOption) (*types.SearchResult, error)
}

// polymarketGammaClient 处理Gamma API操作
// 不允许直接导出，只能通过 NewPolymarketGammaClient 创建
type polymarketGammaClient struct {
	baseURL string // API 基础 URL
}

// NewPolymarketGammaClient 创建新的Gamma客户端
// 返回 GammaClient 接口，不允许直接访问实现类型
func NewPolymarketGammaClient() GammaClient {
	return &polymarketGammaClient{
		baseURL: internal.GammaAPIDomain,
	}
}

// GetMarket 通过市场ID获取市场
func (c *polymarketGammaClient) GetMarket(marketID string) (*types.GammaMarket, error) {
	return Get[types.GammaMarket](c.baseURL, fmt.Sprintf("/markets/%s", marketID), nil)
}

// GetMarketBySlug 通过slug获取市场
func (c *polymarketGammaClient) GetMarketBySlug(slug string, includeTag *bool) (*types.GammaMarket, error) {
	params := make(map[string]string)
	if includeTag != nil {
		params["include_tag"] = strconv.FormatBool(*includeTag)
	}
	return Get[types.GammaMarket](c.baseURL, fmt.Sprintf("/markets/slug/%s", slug), params)
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
		if certaintyMarkets[i].HasDispute() {
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

	rawJSON, err := GetRaw(c.baseURL, "GET", "/markets", params, WithMultiParams(multiParams))
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

// GetEvent 通过事件ID获取事件
func (c *polymarketGammaClient) GetEvent(eventID int, includeChat *bool, includeTemplate *bool) (*types.Event, error) {
	params := make(map[string]string)
	if includeChat != nil {
		params["include_chat"] = strconv.FormatBool(*includeChat)
	}
	if includeTemplate != nil {
		params["include_template"] = strconv.FormatBool(*includeTemplate)
	}
	return Get[types.Event](c.baseURL, fmt.Sprintf("/events/%d", eventID), params)
}

// GetEventBySlug 通过slug获取事件
func (c *polymarketGammaClient) GetEventBySlug(slug string, includeChat *bool, includeTemplate *bool) (*types.Event, error) {
	params := make(map[string]string)
	if includeChat != nil {
		params["include_chat"] = strconv.FormatBool(*includeChat)
	}
	if includeTemplate != nil {
		params["include_template"] = strconv.FormatBool(*includeTemplate)
	}
	return Get[types.Event](c.baseURL, fmt.Sprintf("/events/slug/%s", slug), params)
}

// GetEventsOptions 包含 GetEvents 的所有可选参数
type GetEventsOptions struct {
	Order        *string
	Ascending    bool
	EventIDs     interface{} // int, []int, or nil
	Slugs        []string
	Archived     *bool
	Active       *bool
	Closed       *bool
	LiquidityMin *float64
	LiquidityMax *float64
	VolumeMin    *float64
	VolumeMax    *float64
	StartDateMin *time.Time
	StartDateMax *time.Time
	EndDateMin   *time.Time
	EndDateMax   *time.Time
	Tag          *string
	TagID        *int
	TagSlug      *string
	RelatedTags  bool
}

// GetEventsOption 函数选项类型
type GetEventsOption func(*GetEventsOptions)

// WithEventsOrder 设置排序字段和方向
func WithEventsOrder(order string, ascending bool) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.Order = &order
		opts.Ascending = ascending
	}
}

// WithEventsEventIDs 设置事件ID过滤
func WithEventsEventIDs(eventIDs interface{}) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.EventIDs = eventIDs
	}
}

// WithEventsSlugs 设置slug过滤
func WithEventsSlugs(slugs []string) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.Slugs = slugs
	}
}

// WithEventsArchived 设置archived过滤
func WithEventsArchived(archived bool) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.Archived = &archived
	}
}

// WithEventsActive 设置active过滤
func WithEventsActive(active bool) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.Active = &active
	}
}

// WithEventsClosed 设置closed过滤
func WithEventsClosed(closed bool) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.Closed = &closed
	}
}

// WithEventsLiquidityRange 设置流动性范围
func WithEventsLiquidityRange(min, max *float64) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.LiquidityMin = min
		opts.LiquidityMax = max
	}
}

// WithEventsVolumeRange 设置交易量范围
func WithEventsVolumeRange(min, max *float64) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.VolumeMin = min
		opts.VolumeMax = max
	}
}

// WithEventsStartDateRange 设置开始日期范围
func WithEventsStartDateRange(min, max *time.Time) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.StartDateMin = min
		opts.StartDateMax = max
	}
}

// WithEventsEndDateRange 设置结束日期范围
func WithEventsEndDateRange(min, max *time.Time) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.EndDateMin = min
		opts.EndDateMax = max
	}
}

// WithEventsTag 设置tag过滤
func WithEventsTag(tag string) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.Tag = &tag
	}
}

// WithEventsTagID 设置tag ID过滤
func WithEventsTagID(tagID int, relatedTags bool) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.TagID = &tagID
		opts.RelatedTags = relatedTags
	}
}

// WithEventsTagSlug 设置tag slug过滤
func WithEventsTagSlug(tagSlug string) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.TagSlug = &tagSlug
	}
}

// GetEvents 使用过滤器获取事件列表
// limit 和 offset 是必要参数，其他参数通过选项函数传入
func (c *polymarketGammaClient) GetEvents(limit int, offset int, options ...GetEventsOption) ([]types.Event, error) {
	// 初始化默认选项
	opts := &GetEventsOptions{}

	// 应用所有选项
	for _, option := range options {
		option(opts)
	}

	params := map[string]string{
		"limit":  strconv.Itoa(limit),
		"offset": strconv.Itoa(offset),
	}

	if opts.Order != nil {
		params["order"] = *opts.Order
		params["ascending"] = strconv.FormatBool(opts.Ascending)
	}

	// Handle eventIDs
	if opts.EventIDs != nil {
		switch v := opts.EventIDs.(type) {
		case int:
			params["event_ids"] = strconv.Itoa(v)
		case []int:
			eventIDStrs := make([]string, len(v))
			for i, id := range v {
				eventIDStrs[i] = strconv.Itoa(id)
			}
			params["event_ids"] = strings.Join(eventIDStrs, ",")
		}
	}

	// Handle slugs
	if len(opts.Slugs) > 0 {
		params["slugs"] = strings.Join(opts.Slugs, ",")
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
	if opts.LiquidityMin != nil {
		params["liquidity_min"] = fmt.Sprintf("%f", *opts.LiquidityMin)
	}
	if opts.LiquidityMax != nil {
		params["liquidity_max"] = fmt.Sprintf("%f", *opts.LiquidityMax)
	}
	if opts.VolumeMin != nil {
		params["volume_min"] = fmt.Sprintf("%f", *opts.VolumeMin)
	}
	if opts.VolumeMax != nil {
		params["volume_max"] = fmt.Sprintf("%f", *opts.VolumeMax)
	}
	if opts.StartDateMin != nil {
		params["start_date_min"] = opts.StartDateMin.Format(time.RFC3339)
	}
	if opts.StartDateMax != nil {
		params["start_date_max"] = opts.StartDateMax.Format(time.RFC3339)
	}
	if opts.EndDateMin != nil {
		params["end_date_min"] = opts.EndDateMin.Format(time.RFC3339)
	}
	if opts.EndDateMax != nil {
		params["end_date_max"] = opts.EndDateMax.Format(time.RFC3339)
	}
	if opts.Tag != nil {
		params["tag"] = *opts.Tag
	} else if opts.TagID != nil {
		params["tag_id"] = strconv.Itoa(*opts.TagID)
		if opts.RelatedTags {
			params["related_tags"] = strconv.FormatBool(opts.RelatedTags)
		}
	} else if opts.TagSlug != nil {
		params["tag_slug"] = *opts.TagSlug
	}

	return GetSlice[types.Event](c.baseURL, "/events", params)
}

// SearchOptions 包含 Search 的所有可选参数
type SearchOptions struct {
	Cache             *bool
	Status            *string
	LimitPerType      *int
	Page              *int
	Tags              []string
	KeepClosedMarkets *bool
	Sort              *string
	Ascending         *bool
}

// SearchOption 函数选项类型
type SearchOption func(*SearchOptions)

// WithSearchCache 设置缓存选项
func WithSearchCache(cache bool) SearchOption {
	return func(opts *SearchOptions) {
		opts.Cache = &cache
	}
}

// WithSearchStatus 设置状态过滤
func WithSearchStatus(status string) SearchOption {
	return func(opts *SearchOptions) {
		opts.Status = &status
	}
}

// WithSearchLimitPerType 设置每种类型的限制数量
func WithSearchLimitPerType(limitPerType int) SearchOption {
	return func(opts *SearchOptions) {
		opts.LimitPerType = &limitPerType
	}
}

// WithSearchPage 设置页码
func WithSearchPage(page int) SearchOption {
	return func(opts *SearchOptions) {
		opts.Page = &page
	}
}

// WithSearchTags 设置标签过滤
func WithSearchTags(tags []string) SearchOption {
	return func(opts *SearchOptions) {
		opts.Tags = tags
	}
}

// WithSearchKeepClosedMarkets 设置是否保留已关闭市场
func WithSearchKeepClosedMarkets(keepClosedMarkets bool) SearchOption {
	return func(opts *SearchOptions) {
		opts.KeepClosedMarkets = &keepClosedMarkets
	}
}

// WithSearchSort 设置排序字段和方向
func WithSearchSort(sort string, ascending bool) SearchOption {
	return func(opts *SearchOptions) {
		opts.Sort = &sort
		opts.Ascending = &ascending
	}
}

// Search performs a search
// query 是必要参数，其他参数通过选项函数传入
func (c *polymarketGammaClient) Search(query string, options ...SearchOption) (*types.SearchResult, error) {
	// 初始化默认选项
	opts := &SearchOptions{}

	// 应用所有选项
	for _, option := range options {
		option(opts)
	}

	params := map[string]string{
		"q": query,
	}

	if opts.Cache != nil {
		params["cache"] = strconv.FormatBool(*opts.Cache)
	}
	if opts.Status != nil {
		params["events_status"] = *opts.Status
	}
	if opts.LimitPerType != nil {
		params["limit_per_type"] = strconv.Itoa(*opts.LimitPerType)
	}
	if opts.Page != nil {
		params["page"] = strconv.Itoa(*opts.Page)
	}
	if len(opts.Tags) > 0 {
		params["tags"] = strings.Join(opts.Tags, ",")
	}
	if opts.KeepClosedMarkets != nil {
		params["keep_closed_markets"] = strconv.FormatBool(*opts.KeepClosedMarkets)
	}
	if opts.Sort != nil {
		params["sort"] = *opts.Sort
	}
	if opts.Ascending != nil {
		params["ascending"] = strconv.FormatBool(*opts.Ascending)
	}

	return Get[types.SearchResult](c.baseURL, "/public-search", params)
}
