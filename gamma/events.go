package gamma

import (
	"fmt"
	"strconv"
	"time"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetEvent 通过事件ID获取事件
func (c *polymarketGammaClient) GetEvent(eventID int, includeChat *bool, includeTemplate *bool) (*types.Event, error) {
	params := make(map[string]string)
	if includeChat != nil {
		params["include_chat"] = strconv.FormatBool(*includeChat)
	}
	if includeTemplate != nil {
		params["include_template"] = strconv.FormatBool(*includeTemplate)
	}
	return http.Get[types.Event](c.baseURL, fmt.Sprintf("/events/%d", eventID), params)
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
	return http.Get[types.Event](c.baseURL, fmt.Sprintf("/events/slug/%s", slug), params)
}

// GetEventsOptions 包含 GetEvents 的所有可选参数
// 与 List events API 对齐: https://docs.polymarket.com/api-reference/events/list-events
type GetEventsOptions struct {
	Order          *string
	Ascending      bool
	EventIDs       interface{} // int, []int, or nil → 请求参数 id
	Slugs          []string    // → 请求参数 slug (array)
	Archived       *bool
	Active         *bool
	Closed         *bool
	Featured       *bool
	Cyom           *bool
	IncludeChat    *bool
	IncludeTemplate *bool
	Recurrence     string
	ExcludeTagIDs  []int
	LiquidityMin   *float64
	LiquidityMax   *float64
	VolumeMin      *float64
	VolumeMax      *float64
	StartDateMin   *time.Time
	StartDateMax   *time.Time
	EndDateMin     *time.Time
	EndDateMax     *time.Time
	Tag            *string
	TagID          *int
	TagSlug        *string
	RelatedTags    bool
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

// WithEventsFeatured 按 featured 过滤
func WithEventsFeatured(featured bool) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.Featured = &featured
	}
}

// WithEventsCyom 按 cyom 过滤
func WithEventsCyom(cyom bool) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.Cyom = &cyom
	}
}

// WithEventsIncludeChat 设置 include_chat
func WithEventsIncludeChat(include bool) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.IncludeChat = &include
	}
}

// WithEventsIncludeTemplate 设置 include_template
func WithEventsIncludeTemplate(include bool) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.IncludeTemplate = &include
	}
}

// WithEventsRecurrence 按 recurrence 过滤
func WithEventsRecurrence(recurrence string) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.Recurrence = recurrence
	}
}

// WithEventsExcludeTagIDs 排除指定 tag id 列表
func WithEventsExcludeTagIDs(tagIDs []int) GetEventsOption {
	return func(opts *GetEventsOptions) {
		opts.ExcludeTagIDs = tagIDs
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

	// 与 API 对齐：id 为 array of integer，slug 为 array of string
	multiParams := make(map[string][]string)
	if opts.EventIDs != nil {
		switch v := opts.EventIDs.(type) {
		case int:
			multiParams["id"] = []string{strconv.Itoa(v)}
		case []int:
			eventIDStrs := make([]string, len(v))
			for i, id := range v {
				eventIDStrs[i] = strconv.Itoa(id)
			}
			multiParams["id"] = eventIDStrs
		}
	}
	if len(opts.Slugs) > 0 {
		multiParams["slug"] = opts.Slugs
	}
	if len(opts.ExcludeTagIDs) > 0 {
		excludeStrs := make([]string, len(opts.ExcludeTagIDs))
		for i, id := range opts.ExcludeTagIDs {
			excludeStrs[i] = strconv.Itoa(id)
		}
		multiParams["exclude_tag_id"] = excludeStrs
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
	if opts.Featured != nil {
		params["featured"] = strconv.FormatBool(*opts.Featured)
	}
	if opts.Cyom != nil {
		params["cyom"] = strconv.FormatBool(*opts.Cyom)
	}
	if opts.IncludeChat != nil {
		params["include_chat"] = strconv.FormatBool(*opts.IncludeChat)
	}
	if opts.IncludeTemplate != nil {
		params["include_template"] = strconv.FormatBool(*opts.IncludeTemplate)
	}
	if opts.Recurrence != "" {
		params["recurrence"] = opts.Recurrence
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

	return http.GetSlice[types.Event](c.baseURL, internal.Events, params, http.WithMultiParams(multiParams))
}
