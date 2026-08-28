package gamma

import (
	"context"
	"fmt"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/types"
)

// MarketsInformationRequest is shared by POST /markets/information and
// /markets/abridged. Pointer fields preserve the distinction between false/0
// and a filter that was not supplied.
type MarketsInformationRequest struct {
	IDs                  []int    `json:"id,omitempty"`
	Slugs                []string `json:"slug,omitempty"`
	Closed               *bool    `json:"closed,omitempty"`
	TokenIDs             []string `json:"clobTokenIds,omitempty"`
	ConditionIDs         []string `json:"conditionIds,omitempty"`
	MarketMakerAddresses []string `json:"marketMakerAddress,omitempty"`
	LiquidityNumMin      *float64 `json:"liquidityNumMin,omitempty"`
	LiquidityNumMax      *float64 `json:"liquidityNumMax,omitempty"`
	VolumeNumMin         *float64 `json:"volumeNumMin,omitempty"`
	VolumeNumMax         *float64 `json:"volumeNumMax,omitempty"`
	StartDateMin         *string  `json:"startDateMin,omitempty"`
	StartDateMax         *string  `json:"startDateMax,omitempty"`
	EndDateMin           *string  `json:"endDateMin,omitempty"`
	EndDateMax           *string  `json:"endDateMax,omitempty"`
	RelatedTags          *bool    `json:"relatedTags,omitempty"`
	TagID                *int     `json:"tagId,omitempty"`
	CYOM                 *bool    `json:"cyom,omitempty"`
	UMAResolutionStatus  *string  `json:"umaResolutionStatus,omitempty"`
	GameID               *string  `json:"gameId,omitempty"`
	SportsMarketTypes    []string `json:"sportsMarketTypes,omitempty"`
	RewardsMinSize       *float64 `json:"rewardsMinSize,omitempty"`
	QuestionIDs          []string `json:"questionIds,omitempty"`
	IncludeTags          *bool    `json:"includeTags,omitempty"`
}

func (c *polymarketGammaClient) GetMarketDescription(id int) (*types.MarketDescription, error) {
	return c.GetMarketDescriptionContext(context.Background(), id)
}
func (c *polymarketGammaClient) GetMarketDescriptionContext(ctx context.Context, id int) (*types.MarketDescription, error) {
	return http.GetContext[types.MarketDescription](ctx, c.baseURL, fmt.Sprintf("/markets/%d/description", id), nil, http.WithService("gamma"))
}
func (c *polymarketGammaClient) GetMarketTags(id int) ([]types.Tag, error) {
	return c.GetMarketTagsContext(context.Background(), id)
}
func (c *polymarketGammaClient) GetMarketTagsContext(ctx context.Context, id int) ([]types.Tag, error) {
	return http.GetSliceContext[types.Tag](ctx, c.baseURL, fmt.Sprintf("/markets/%d/tags", id), nil, http.WithService("gamma"))
}

func (c *polymarketGammaClient) GetMarketsInformation(request MarketsInformationRequest) ([]types.GammaMarket, error) {
	return c.GetMarketsInformationContext(context.Background(), request)
}
func (c *polymarketGammaClient) GetMarketsInformationContext(ctx context.Context, request MarketsInformationRequest) ([]types.GammaMarket, error) {
	result, err := http.PostContext[[]types.GammaMarket](ctx, c.baseURL, "/markets/information", request, http.WithIdempotent(), http.WithService("gamma"))
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.GammaMarket{}, nil
	}
	return *result, nil
}
func (c *polymarketGammaClient) GetAbridgedMarkets(request MarketsInformationRequest) ([]types.GammaMarket, error) {
	return c.GetAbridgedMarketsContext(context.Background(), request)
}
func (c *polymarketGammaClient) GetAbridgedMarketsContext(ctx context.Context, request MarketsInformationRequest) ([]types.GammaMarket, error) {
	result, err := http.PostContext[[]types.GammaMarket](ctx, c.baseURL, "/markets/abridged", request, http.WithIdempotent(), http.WithService("gamma"))
	if err != nil {
		return nil, err
	}
	if result == nil {
		return []types.GammaMarket{}, nil
	}
	return *result, nil
}

// GetMarketsPage always uses the keyset endpoint and exposes next_cursor.
func (c *polymarketGammaClient) GetMarketsPage(limit int, options ...GetMarketsOption) (*types.MarketsPage, error) {
	return c.GetMarketsPageContext(context.Background(), limit, options...)
}
func (c *polymarketGammaClient) GetMarketsPageContext(ctx context.Context, limit int, options ...GetMarketsOption) (*types.MarketsPage, error) {
	pageOptions := append([]GetMarketsOption{}, options...)
	pageOptions = append(pageOptions, WithMarketsKeyset())
	markets, nextCursor, err := c.getMarketsPageContext(ctx, limit, pageOptions...)
	if err != nil {
		return nil, err
	}
	return &types.MarketsPage{Markets: markets, NextCursor: nextCursor}, nil
}
