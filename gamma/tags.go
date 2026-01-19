package gamma

import (
	"fmt"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetTagsOptions 包含 GetTags 的所有可选参数
type GetTagsOptions struct {
	Order     *string
	Ascending bool
}

// GetTagsOption 函数选项类型
type GetTagsOption func(*GetTagsOptions)

// WithTagsOrder 设置排序字段和方向
func WithTagsOrder(order string, ascending bool) GetTagsOption {
	return func(opts *GetTagsOptions) {
		opts.Order = &order
		opts.Ascending = ascending
	}
}

// GetTags 获取标签列表
func (c *polymarketGammaClient) GetTags(limit int, offset int, options ...GetTagsOption) ([]types.Tag, error) {
	opts := &GetTagsOptions{}
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

	result, err := http.Get[[]types.Tag](c.baseURL, internal.GetTags, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get tags: %w", err)
	}

	if result == nil {
		return []types.Tag{}, nil
	}

	return *result, nil
}

// GetTag 获取单个标签
func (c *polymarketGammaClient) GetTag(tagID int) (*types.Tag, error) {
	return http.Get[types.Tag](c.baseURL, fmt.Sprintf("%s%d", internal.GetTag, tagID), nil)
}

// GetTagBySlug 通过 slug 获取标签
func (c *polymarketGammaClient) GetTagBySlug(slug string) (*types.Tag, error) {
	return http.Get[types.Tag](c.baseURL, fmt.Sprintf("%s%s", internal.GetTagBySlug, slug), nil)
}
