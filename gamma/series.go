package gamma

import (
	"context"
	"fmt"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetSeriesOptions 包含 GetSeries 的所有可选参数
type GetSeriesOptions struct {
	Order          *string
	Ascending      bool
	Closed         *bool
	Slugs          []string
	CategoryIDs    []int
	CategoryLabels []string
	IncludeChat    *bool
	Recurrence     string
	ExcludeEvents  *bool
}

// GetSeriesOption 函数选项类型
type GetSeriesOption func(*GetSeriesOptions)

// WithSeriesOrder 设置排序字段和方向
func WithSeriesOrder(order string, ascending bool) GetSeriesOption {
	return func(opts *GetSeriesOptions) {
		opts.Order = &order
		opts.Ascending = ascending
	}
}

// WithSeriesClosed 设置是否包含已关闭的系列
func WithSeriesClosed(closed bool) GetSeriesOption {
	return func(opts *GetSeriesOptions) {
		opts.Closed = &closed
	}
}

func WithSeriesSlugs(slugs ...string) GetSeriesOption {
	return func(opts *GetSeriesOptions) { opts.Slugs = append([]string(nil), slugs...) }
}
func WithSeriesCategoryIDs(ids ...int) GetSeriesOption {
	return func(opts *GetSeriesOptions) { opts.CategoryIDs = append([]int(nil), ids...) }
}
func WithSeriesCategoryLabels(labels ...string) GetSeriesOption {
	return func(opts *GetSeriesOptions) { opts.CategoryLabels = append([]string(nil), labels...) }
}
func WithSeriesIncludeChat(include bool) GetSeriesOption {
	return func(opts *GetSeriesOptions) { opts.IncludeChat = &include }
}
func WithSeriesRecurrence(recurrence string) GetSeriesOption {
	return func(opts *GetSeriesOptions) { opts.Recurrence = recurrence }
}
func WithSeriesExcludeEvents(exclude bool) GetSeriesOption {
	return func(opts *GetSeriesOptions) { opts.ExcludeEvents = &exclude }
}

// GetSeries 获取系列列表
func (c *polymarketGammaClient) GetSeries(limit int, offset int, options ...GetSeriesOption) ([]types.Series, error) {
	return c.GetSeriesContext(context.Background(), limit, offset, options...)
}

func (c *polymarketGammaClient) GetSeriesContext(ctx context.Context, limit int, offset int, options ...GetSeriesOption) ([]types.Series, error) {
	opts := &GetSeriesOptions{}
	for _, opt := range options {
		opt(opts)
	}

	params := map[string]string{
		"limit":  strconv.Itoa(limit),
		"offset": strconv.Itoa(offset),
	}

	if opts.Order != nil {
		params["order"] = *opts.Order
		params["ascending"] = strconv.FormatBool(opts.Ascending)
	}
	if opts.Closed != nil {
		params["closed"] = strconv.FormatBool(*opts.Closed)
	}
	if opts.IncludeChat != nil {
		params["include_chat"] = strconv.FormatBool(*opts.IncludeChat)
	}
	if opts.Recurrence != "" {
		params["recurrence"] = opts.Recurrence
	}
	if opts.ExcludeEvents != nil {
		params["exclude_events"] = strconv.FormatBool(*opts.ExcludeEvents)
	}
	multi := map[string][]string{"slug": opts.Slugs, "categories_labels": opts.CategoryLabels}
	if len(opts.CategoryIDs) > 0 {
		values := make([]string, len(opts.CategoryIDs))
		for i, id := range opts.CategoryIDs {
			values[i] = strconv.Itoa(id)
		}
		multi["categories_ids"] = values
	}

	result, err := http.GetContext[[]types.Series](ctx, c.baseURL, internal.GetSeries, params, http.WithMultiParams(multi), http.WithService("gamma"))
	if err != nil {
		return nil, fmt.Errorf("failed to get series: %w", err)
	}

	if result == nil {
		return []types.Series{}, nil
	}

	return *result, nil
}

func (c *polymarketGammaClient) GetSeriesByID(id int, includeChat *bool) (*types.Series, error) {
	return c.GetSeriesByIDContext(context.Background(), id, includeChat)
}

func (c *polymarketGammaClient) GetSeriesByIDContext(ctx context.Context, id int, includeChat *bool) (*types.Series, error) {
	params := make(map[string]string)
	if includeChat != nil {
		params["include_chat"] = strconv.FormatBool(*includeChat)
	}
	return http.GetContext[types.Series](ctx, c.baseURL, fmt.Sprintf("/series/%d", id), params, http.WithService("gamma"))
}

func (c *polymarketGammaClient) GetSeriesCommentsCount(id int) (*types.Count, error) {
	return c.GetSeriesCommentsCountContext(context.Background(), id)
}

func (c *polymarketGammaClient) GetSeriesCommentsCountContext(ctx context.Context, id int) (*types.Count, error) {
	return http.GetContext[types.Count](ctx, c.baseURL, fmt.Sprintf("/series/%d/comments/count", id), nil, http.WithService("gamma"))
}

// GetSeriesSummaryByID 通过 ID 获取系列摘要。
func (c *polymarketGammaClient) GetSeriesSummaryByID(id string) (*types.SeriesSummary, error) {
	return c.GetSeriesSummaryByIDContext(context.Background(), id)
}

func (c *polymarketGammaClient) GetSeriesSummaryByIDContext(ctx context.Context, id string) (*types.SeriesSummary, error) {
	return http.GetContext[types.SeriesSummary](ctx, c.baseURL, fmt.Sprintf("%s%s", internal.GetSeriesSummary, id), nil, http.WithService("gamma"))
}

// GetSeriesSummaryBySlug 通过 slug 获取系列摘要。
func (c *polymarketGammaClient) GetSeriesSummaryBySlug(slug string) (*types.SeriesSummary, error) {
	return c.GetSeriesSummaryBySlugContext(context.Background(), slug)
}

func (c *polymarketGammaClient) GetSeriesSummaryBySlugContext(ctx context.Context, slug string) (*types.SeriesSummary, error) {
	return http.GetContext[types.SeriesSummary](ctx, c.baseURL, fmt.Sprintf("%s%s", internal.GetSeriesSummaryBySlug, slug), nil, http.WithService("gamma"))
}
