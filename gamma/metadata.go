package gamma

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/types"
)

type ListTeamsOptions struct {
	Order         string
	Ascending     *bool
	Leagues       []string
	Names         []string
	Abbreviations []string
}

type ListTeamsOption func(*ListTeamsOptions)

func WithTeamsOrder(order string, ascending bool) ListTeamsOption {
	return func(o *ListTeamsOptions) { o.Order, o.Ascending = order, &ascending }
}
func WithTeamLeagues(values ...string) ListTeamsOption {
	return func(o *ListTeamsOptions) { o.Leagues = append([]string(nil), values...) }
}
func WithTeamNames(values ...string) ListTeamsOption {
	return func(o *ListTeamsOptions) { o.Names = append([]string(nil), values...) }
}
func WithTeamAbbreviations(values ...string) ListTeamsOption {
	return func(o *ListTeamsOptions) { o.Abbreviations = append([]string(nil), values...) }
}

func (c *polymarketGammaClient) GetStatus() (string, error) {
	return c.GetStatusContext(context.Background())
}

func (c *polymarketGammaClient) GetStatusContext(ctx context.Context) (string, error) {
	raw, err := http.GetRawContext(ctx, c.baseURL, "GET", "/status", nil, http.WithService("gamma"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func (c *polymarketGammaClient) GetTeams(limit, offset int, options ...ListTeamsOption) ([]types.Team, error) {
	return c.GetTeamsContext(context.Background(), limit, offset, options...)
}

func (c *polymarketGammaClient) GetTeamsContext(ctx context.Context, limit, offset int, options ...ListTeamsOption) ([]types.Team, error) {
	opts := &ListTeamsOptions{}
	for _, option := range options {
		option(opts)
	}
	params := map[string]string{"limit": strconv.Itoa(limit), "offset": strconv.Itoa(offset)}
	if opts.Order != "" {
		params["order"] = opts.Order
	}
	if opts.Ascending != nil {
		params["ascending"] = strconv.FormatBool(*opts.Ascending)
	}
	return http.GetSliceContext[types.Team](ctx, c.baseURL, "/teams", params, http.WithMultiParams(map[string][]string{
		"league": opts.Leagues, "name": opts.Names, "abbreviation": opts.Abbreviations,
	}), http.WithService("gamma"))
}

func (c *polymarketGammaClient) GetTeam(id int) (*types.Team, error) {
	return c.GetTeamContext(context.Background(), id)
}

func (c *polymarketGammaClient) GetTeamContext(ctx context.Context, id int) (*types.Team, error) {
	return http.GetContext[types.Team](ctx, c.baseURL, fmt.Sprintf("/teams/%d", id), nil, http.WithService("gamma"))
}

type RelatedTagsOptions struct {
	OmitEmpty *bool
	Status    string
}

type RelatedTagsOption func(*RelatedTagsOptions)

func WithRelatedTagsOmitEmpty(value bool) RelatedTagsOption {
	return func(o *RelatedTagsOptions) { o.OmitEmpty = &value }
}
func WithRelatedTagsStatus(status string) RelatedTagsOption {
	return func(o *RelatedTagsOptions) { o.Status = status }
}

func relatedTagsParams(options []RelatedTagsOption) map[string]string {
	opts := &RelatedTagsOptions{}
	for _, option := range options {
		option(opts)
	}
	params := make(map[string]string)
	if opts.OmitEmpty != nil {
		params["omit_empty"] = strconv.FormatBool(*opts.OmitEmpty)
	}
	if opts.Status != "" {
		params["status"] = opts.Status
	}
	return params
}

func (c *polymarketGammaClient) GetRelatedTagsByID(id int, options ...RelatedTagsOption) ([]types.RelatedTag, error) {
	return c.GetRelatedTagsByIDContext(context.Background(), id, options...)
}
func (c *polymarketGammaClient) GetRelatedTagsByIDContext(ctx context.Context, id int, options ...RelatedTagsOption) ([]types.RelatedTag, error) {
	return http.GetSliceContext[types.RelatedTag](ctx, c.baseURL, fmt.Sprintf("/tags/%d/related-tags", id), relatedTagsParams(options), http.WithService("gamma"))
}
func (c *polymarketGammaClient) GetRelatedTagsBySlug(slug string, options ...RelatedTagsOption) ([]types.RelatedTag, error) {
	return c.GetRelatedTagsBySlugContext(context.Background(), slug, options...)
}
func (c *polymarketGammaClient) GetRelatedTagsBySlugContext(ctx context.Context, slug string, options ...RelatedTagsOption) ([]types.RelatedTag, error) {
	return http.GetSliceContext[types.RelatedTag](ctx, c.baseURL, "/tags/slug/"+slug+"/related-tags", relatedTagsParams(options), http.WithService("gamma"))
}
func (c *polymarketGammaClient) GetTagsRelatedToTagByID(id int, options ...RelatedTagsOption) ([]types.Tag, error) {
	return c.GetTagsRelatedToTagByIDContext(context.Background(), id, options...)
}
func (c *polymarketGammaClient) GetTagsRelatedToTagByIDContext(ctx context.Context, id int, options ...RelatedTagsOption) ([]types.Tag, error) {
	return http.GetSliceContext[types.Tag](ctx, c.baseURL, fmt.Sprintf("/tags/%d/related-tags/tags", id), relatedTagsParams(options), http.WithService("gamma"))
}
func (c *polymarketGammaClient) GetTagsRelatedToTagBySlug(slug string, options ...RelatedTagsOption) ([]types.Tag, error) {
	return c.GetTagsRelatedToTagBySlugContext(context.Background(), slug, options...)
}
func (c *polymarketGammaClient) GetTagsRelatedToTagBySlugContext(ctx context.Context, slug string, options ...RelatedTagsOption) ([]types.Tag, error) {
	return http.GetSliceContext[types.Tag](ctx, c.baseURL, "/tags/slug/"+slug+"/related-tags/tags", relatedTagsParams(options), http.WithService("gamma"))
}

func (c *polymarketGammaClient) GetSportsMetadata() ([]types.SportsMetadata, error) {
	return c.GetSportsMetadataContext(context.Background())
}
func (c *polymarketGammaClient) GetSportsMetadataContext(ctx context.Context) ([]types.SportsMetadata, error) {
	return http.GetSliceContext[types.SportsMetadata](ctx, c.baseURL, "/sports", nil, http.WithService("gamma"))
}
func (c *polymarketGammaClient) GetSportsMarketTypes() (*types.SportsMarketTypesResponse, error) {
	return c.GetSportsMarketTypesContext(context.Background())
}
func (c *polymarketGammaClient) GetSportsMarketTypesContext(ctx context.Context) (*types.SportsMarketTypesResponse, error) {
	return http.GetContext[types.SportsMarketTypesResponse](ctx, c.baseURL, "/sports/market-types", nil, http.WithService("gamma"))
}
