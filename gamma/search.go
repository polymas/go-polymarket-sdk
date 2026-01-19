package gamma

import (
	"strconv"
	"strings"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/types"
)

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

	return http.Get[types.SearchResult](c.baseURL, "/public-search", params)
}
