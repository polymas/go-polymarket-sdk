package gamma

import (
	"context"
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
	return c.GetTagsContext(context.Background(), limit, offset, options...)
}

func (c *polymarketGammaClient) GetTagsContext(ctx context.Context, limit int, offset int, options ...GetTagsOption) ([]types.Tag, error) {
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

	result, err := http.GetContext[[]types.Tag](ctx, c.baseURL, internal.GetTags, params, http.WithService("gamma"))
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
	return c.GetTagContext(context.Background(), tagID)
}

func (c *polymarketGammaClient) GetTagContext(ctx context.Context, tagID int) (*types.Tag, error) {
	return http.GetContext[types.Tag](ctx, c.baseURL, fmt.Sprintf("%s%d", internal.GetTag, tagID), nil, http.WithService("gamma"))
}

// GetTagBySlug 通过 slug 获取标签
func (c *polymarketGammaClient) GetTagBySlug(slug string) (*types.Tag, error) {
	return c.GetTagBySlugContext(context.Background(), slug)
}

func (c *polymarketGammaClient) GetTagBySlugContext(ctx context.Context, slug string) (*types.Tag, error) {
	return http.GetContext[types.Tag](ctx, c.baseURL, fmt.Sprintf("%s%s", internal.GetTagBySlug, slug), nil, http.WithService("gamma"))
}
