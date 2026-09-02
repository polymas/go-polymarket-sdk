package gamma

import (
	"context"
	"strconv"

	http "github.com/polymas/go-polymarket-sdk/internal/transport"
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
	SearchTags        *bool
	SearchProfiles    *bool
	Recurrence        string
	ExcludeTagIDs     []int
	Optimized         *bool
}

func WithSearchTagsEnabled(enabled bool) SearchOption {
	return func(opts *SearchOptions) { opts.SearchTags = &enabled }
}

func WithSearchProfiles(enabled bool) SearchOption {
	return func(opts *SearchOptions) { opts.SearchProfiles = &enabled }
}

func WithSearchRecurrence(recurrence string) SearchOption {
	return func(opts *SearchOptions) { opts.Recurrence = recurrence }
}

func WithSearchExcludeTagIDs(ids ...int) SearchOption {
	return func(opts *SearchOptions) { opts.ExcludeTagIDs = append([]int(nil), ids...) }
}

func WithSearchOptimized(optimized bool) SearchOption {
	return func(opts *SearchOptions) { opts.Optimized = &optimized }
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
	return c.SearchContext(context.Background(), query, options...)
}

func (c *polymarketGammaClient) SearchContext(ctx context.Context, query string, options ...SearchOption) (*types.SearchResult, error) {
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
	if opts.KeepClosedMarkets != nil {
		if *opts.KeepClosedMarkets {
			params["keep_closed_markets"] = "1"
		} else {
			params["keep_closed_markets"] = "0"
		}
	}
	if opts.Sort != nil {
		params["sort"] = *opts.Sort
	}
	if opts.Ascending != nil {
		params["ascending"] = strconv.FormatBool(*opts.Ascending)
	}
	if opts.SearchTags != nil {
		params["search_tags"] = strconv.FormatBool(*opts.SearchTags)
	}
	if opts.SearchProfiles != nil {
		params["search_profiles"] = strconv.FormatBool(*opts.SearchProfiles)
	}
	if opts.Recurrence != "" {
		params["recurrence"] = opts.Recurrence
	}
	if opts.Optimized != nil {
		params["optimized"] = strconv.FormatBool(*opts.Optimized)
	}
	exclude := make([]string, len(opts.ExcludeTagIDs))
	for i, id := range opts.ExcludeTagIDs {
		exclude[i] = strconv.Itoa(id)
	}

	return http.GetContext[types.SearchResult](ctx, c.baseURL, "/public-search", params, http.WithMultiParams(map[string][]string{
		"events_tag": opts.Tags, "exclude_tag_id": exclude,
	}), http.WithService("gamma"))
}
