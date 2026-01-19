package gamma

import (
	"fmt"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetSeriesOptions 包含 GetSeries 的所有可选参数
type GetSeriesOptions struct {
	Order     *string
	Ascending bool
	Closed    *bool
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

// GetSeries 获取系列列表
func (c *polymarketGammaClient) GetSeries(limit int, offset int, options ...GetSeriesOption) ([]types.Series, error) {
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

	result, err := http.Get[[]types.Series](c.baseURL, internal.GetSeries, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get series: %w", err)
	}

	if result == nil {
		return []types.Series{}, nil
	}

	return *result, nil
}

// GetSeriesBySlug 通过 slug 获取系列
func (c *polymarketGammaClient) GetSeriesBySlug(slug string) (*types.Series, error) {
	return http.Get[types.Series](c.baseURL, fmt.Sprintf("%s%s", internal.GetSeriesBySlug, slug), nil)
}
