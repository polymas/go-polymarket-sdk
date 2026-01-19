package gamma

import (
	"fmt"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetComments 获取市场评论
func (c *polymarketGammaClient) GetComments(marketID string, limit int, offset int) ([]types.Comment, error) {
	params := map[string]string{
		"market_id": marketID,
		"limit":     strconv.Itoa(limit),
		"offset":    strconv.Itoa(offset),
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

// GetComment 获取单个评论
func (c *polymarketGammaClient) GetComment(commentID string) (*types.Comment, error) {
	return http.Get[types.Comment](c.baseURL, fmt.Sprintf("%s%s", internal.GetComment, commentID), nil)
}
