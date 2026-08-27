package gamma

import (
	"fmt"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetCommentsOptions 是评论列表的可选过滤参数。
type GetCommentsOptions struct {
	Order        string
	Ascending    *bool
	GetPositions *bool
	HoldersOnly  *bool
}

type GetCommentsOption func(*GetCommentsOptions)

func WithCommentsOrder(order string, ascending bool) GetCommentsOption {
	return func(opts *GetCommentsOptions) { opts.Order, opts.Ascending = order, &ascending }
}

func WithCommentPositions(include bool) GetCommentsOption {
	return func(opts *GetCommentsOptions) { opts.GetPositions = &include }
}

func WithCommentHoldersOnly(holdersOnly bool) GetCommentsOption {
	return func(opts *GetCommentsOptions) { opts.HoldersOnly = &holdersOnly }
}

// GetComments 按父实体获取评论。
func (c *polymarketGammaClient) GetComments(parentType types.CommentParentEntityType, parentID int, limit int, offset int, options ...GetCommentsOption) ([]types.Comment, error) {
	opts := &GetCommentsOptions{}
	for _, option := range options {
		option(opts)
	}
	params := map[string]string{
		"parent_entity_type": string(parentType),
		"parent_entity_id":   strconv.Itoa(parentID),
		"limit":              strconv.Itoa(limit),
		"offset":             strconv.Itoa(offset),
	}
	if opts.Order != "" {
		params["order"] = opts.Order
	}
	if opts.Ascending != nil {
		params["ascending"] = strconv.FormatBool(*opts.Ascending)
	}
	if opts.GetPositions != nil {
		params["get_positions"] = strconv.FormatBool(*opts.GetPositions)
	}
	if opts.HoldersOnly != nil {
		params["holders_only"] = strconv.FormatBool(*opts.HoldersOnly)
	}

	result, err := http.Get[[]types.Comment](c.baseURL, internal.GetComments, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments: %w", err)
	}

	if result == nil {
		return []types.Comment{}, nil
	}

	return *result, nil
}

// GetComment 获取指定 ID 的评论。官方响应形状为数组。
func (c *polymarketGammaClient) GetComment(commentID int, getPositions bool) ([]types.Comment, error) {
	result, err := http.Get[[]types.Comment](c.baseURL, fmt.Sprintf("%s%d", internal.GetComment, commentID), map[string]string{
		"get_positions": strconv.FormatBool(getPositions),
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.Comment{}, nil
	}
	return *result, nil
}
