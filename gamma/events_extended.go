package gamma

import (
	"context"
	"fmt"
	"strconv"
	"time"

	http "github.com/polymas/go-polymarket-sdk/internal/transport"
	"github.com/polymas/go-polymarket-sdk/types"
)

type EventsPaginationOptions struct {
	Order           string
	Ascending       *bool
	IncludeChat     *bool
	IncludeTemplate *bool
	Recurrence      string
}

type EventsPaginationOption func(*EventsPaginationOptions)

func WithEventsPaginationOrder(order string, ascending bool) EventsPaginationOption {
	return func(o *EventsPaginationOptions) { o.Order, o.Ascending = order, &ascending }
}
func WithEventsPaginationChat(include bool) EventsPaginationOption {
	return func(o *EventsPaginationOptions) { o.IncludeChat = &include }
}
func WithEventsPaginationTemplate(include bool) EventsPaginationOption {
	return func(o *EventsPaginationOptions) { o.IncludeTemplate = &include }
}
func WithEventsPaginationRecurrence(recurrence string) EventsPaginationOption {
	return func(o *EventsPaginationOptions) { o.Recurrence = recurrence }
}

func (c *polymarketGammaClient) GetEventsPagination(limit, offset int, options ...EventsPaginationOption) (*types.EventsPagination, error) {
	return c.GetEventsPaginationContext(context.Background(), limit, offset, options...)
}
func (c *polymarketGammaClient) GetEventsPaginationContext(ctx context.Context, limit, offset int, options ...EventsPaginationOption) (*types.EventsPagination, error) {
	o := &EventsPaginationOptions{}
	for _, option := range options {
		option(o)
	}
	params := map[string]string{"limit": strconv.Itoa(limit), "offset": strconv.Itoa(offset)}
	if o.Order != "" {
		params["order"] = o.Order
	}
	if o.Ascending != nil {
		params["ascending"] = strconv.FormatBool(*o.Ascending)
	}
	if o.IncludeChat != nil {
		params["include_chat"] = strconv.FormatBool(*o.IncludeChat)
	}
	if o.IncludeTemplate != nil {
		params["include_template"] = strconv.FormatBool(*o.IncludeTemplate)
	}
	if o.Recurrence != "" {
		params["recurrence"] = o.Recurrence
	}
	return http.GetContext[types.EventsPagination](ctx, c.baseURL, "/events/pagination", params, http.WithService("gamma"))
}

func (c *polymarketGammaClient) GetSportsEventResults(limit, offset int, order string, ascending *bool) ([]types.Event, error) {
	return c.GetSportsEventResultsContext(context.Background(), limit, offset, order, ascending)
}
func (c *polymarketGammaClient) GetSportsEventResultsContext(ctx context.Context, limit, offset int, order string, ascending *bool) ([]types.Event, error) {
	params := map[string]string{"limit": strconv.Itoa(limit), "offset": strconv.Itoa(offset)}
	if order != "" {
		params["order"] = order
	}
	if ascending != nil {
		params["ascending"] = strconv.FormatBool(*ascending)
	}
	return http.GetSliceContext[types.Event](ctx, c.baseURL, "/events/results", params, http.WithService("gamma"))
}

func (c *polymarketGammaClient) GetEventTweetCount(id int) (*types.EventTweetCount, error) {
	return c.GetEventTweetCountContext(context.Background(), id)
}
func (c *polymarketGammaClient) GetEventTweetCountContext(ctx context.Context, id int) (*types.EventTweetCount, error) {
	return http.GetContext[types.EventTweetCount](ctx, c.baseURL, fmt.Sprintf("/events/%d/tweet-count", id), nil, http.WithService("gamma"))
}
func (c *polymarketGammaClient) GetEventCommentsCount(id int) (*types.Count, error) {
	return c.GetEventCommentsCountContext(context.Background(), id)
}
func (c *polymarketGammaClient) GetEventCommentsCountContext(ctx context.Context, id int) (*types.Count, error) {
	return http.GetContext[types.Count](ctx, c.baseURL, fmt.Sprintf("/events/%d/comments/count", id), nil, http.WithService("gamma"))
}
func (c *polymarketGammaClient) GetEventTags(id int) ([]types.Tag, error) {
	return c.GetEventTagsContext(context.Background(), id)
}
func (c *polymarketGammaClient) GetEventTagsContext(ctx context.Context, id int) ([]types.Tag, error) {
	return http.GetSliceContext[types.Tag](ctx, c.baseURL, fmt.Sprintf("/events/%d/tags", id), nil, http.WithService("gamma"))
}

type EventCreatorsOptions struct {
	Order         string
	Ascending     *bool
	CreatorName   string
	CreatorHandle string
}
type EventCreatorsOption func(*EventCreatorsOptions)

func WithEventCreatorsOrder(order string, ascending bool) EventCreatorsOption {
	return func(o *EventCreatorsOptions) { o.Order, o.Ascending = order, &ascending }
}
func WithEventCreatorName(name string) EventCreatorsOption {
	return func(o *EventCreatorsOptions) { o.CreatorName = name }
}
func WithEventCreatorHandle(handle string) EventCreatorsOption {
	return func(o *EventCreatorsOptions) { o.CreatorHandle = handle }
}
func (c *polymarketGammaClient) GetEventCreators(limit, offset int, options ...EventCreatorsOption) ([]types.EventCreator, error) {
	return c.GetEventCreatorsContext(context.Background(), limit, offset, options...)
}
func (c *polymarketGammaClient) GetEventCreatorsContext(ctx context.Context, limit, offset int, options ...EventCreatorsOption) ([]types.EventCreator, error) {
	o := &EventCreatorsOptions{}
	for _, option := range options {
		option(o)
	}
	params := map[string]string{"limit": strconv.Itoa(limit), "offset": strconv.Itoa(offset)}
	if o.Order != "" {
		params["order"] = o.Order
	}
	if o.Ascending != nil {
		params["ascending"] = strconv.FormatBool(*o.Ascending)
	}
	if o.CreatorName != "" {
		params["creator_name"] = o.CreatorName
	}
	if o.CreatorHandle != "" {
		params["creator_handle"] = o.CreatorHandle
	}
	return http.GetSliceContext[types.EventCreator](ctx, c.baseURL, "/events/creators", params, http.WithService("gamma"))
}
func (c *polymarketGammaClient) GetEventCreator(id int) (*types.EventCreator, error) {
	return c.GetEventCreatorContext(context.Background(), id)
}
func (c *polymarketGammaClient) GetEventCreatorContext(ctx context.Context, id int) (*types.EventCreator, error) {
	return http.GetContext[types.EventCreator](ctx, c.baseURL, fmt.Sprintf("/events/creators/%d", id), nil, http.WithService("gamma"))
}

// EventsKeysetOptions exposes every filter in GET /events/keyset. Zero values
// are omitted; pointer booleans distinguish false from an omitted parameter.
type EventsKeysetOptions struct {
	AfterCursor                                        string
	Offset                                             *int
	Order                                              string
	Ascending                                          *bool
	IDs                                                []int
	Slugs                                              []string
	Closed, Live, Featured, CYOM                       *bool
	TitleSearch                                        string
	LiquidityMin, LiquidityMax, VolumeMin, VolumeMax   *float64
	StartDateMin, StartDateMax, EndDateMin, EndDateMax *time.Time
	StartTimeMin, StartTimeMax                         *time.Time
	TagIDs                                             []int
	TagSlug                                            string
	ExcludeTagIDs                                      []int
	RelatedTags                                        *bool
	TagMatch                                           string
	SeriesIDs                                          []int
	GameIDs                                            []string
	EventDate                                          string
	EventWeek                                          *int
	FeaturedOrder                                      *bool
	Recurrence                                         string
	CreatedBy                                          []string
	ParentEventID                                      *int
	IncludeChildren                                    *bool
	PartnerSlug                                        string
	IncludeChat, IncludeTemplate, IncludeBestLines     *bool
	Locale                                             string
}

func (c *polymarketGammaClient) GetEventsPage(limit int, options EventsKeysetOptions) (*types.EventsPage, error) {
	return c.GetEventsPageContext(context.Background(), limit, options)
}
func (c *polymarketGammaClient) GetEventsPageContext(ctx context.Context, limit int, o EventsKeysetOptions) (*types.EventsPage, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	params := map[string]string{"limit": strconv.Itoa(limit)}
	putString := func(k, v string) {
		if v != "" {
			params[k] = v
		}
	}
	putBool := func(k string, v *bool) {
		if v != nil {
			params[k] = strconv.FormatBool(*v)
		}
	}
	putFloat := func(k string, v *float64) {
		if v != nil {
			params[k] = strconv.FormatFloat(*v, 'f', -1, 64)
		}
	}
	putTime := func(k string, v *time.Time) {
		if v != nil {
			params[k] = v.Format(time.RFC3339)
		}
	}
	putString("after_cursor", o.AfterCursor)
	putString("order", o.Order)
	putString("title_search", o.TitleSearch)
	putString("tag_slug", o.TagSlug)
	putString("tag_match", o.TagMatch)
	putString("event_date", o.EventDate)
	putString("recurrence", o.Recurrence)
	putString("partner_slug", o.PartnerSlug)
	putString("locale", o.Locale)
	if o.Offset != nil {
		params["offset"] = strconv.Itoa(*o.Offset)
	}
	if o.EventWeek != nil {
		params["event_week"] = strconv.Itoa(*o.EventWeek)
	}
	if o.ParentEventID != nil {
		params["parent_event_id"] = strconv.Itoa(*o.ParentEventID)
	}
	putBool("ascending", o.Ascending)
	putBool("closed", o.Closed)
	putBool("live", o.Live)
	putBool("featured", o.Featured)
	putBool("cyom", o.CYOM)
	putBool("related_tags", o.RelatedTags)
	putBool("featured_order", o.FeaturedOrder)
	putBool("include_children", o.IncludeChildren)
	putBool("include_chat", o.IncludeChat)
	putBool("include_template", o.IncludeTemplate)
	putBool("include_best_lines", o.IncludeBestLines)
	putFloat("liquidity_min", o.LiquidityMin)
	putFloat("liquidity_max", o.LiquidityMax)
	putFloat("volume_min", o.VolumeMin)
	putFloat("volume_max", o.VolumeMax)
	putTime("start_date_min", o.StartDateMin)
	putTime("start_date_max", o.StartDateMax)
	putTime("end_date_min", o.EndDateMin)
	putTime("end_date_max", o.EndDateMax)
	putTime("start_time_min", o.StartTimeMin)
	putTime("start_time_max", o.StartTimeMax)
	ints := func(values []int) []string {
		out := make([]string, len(values))
		for i, value := range values {
			out[i] = strconv.Itoa(value)
		}
		return out
	}
	multi := map[string][]string{
		"id": ints(o.IDs), "slug": o.Slugs, "tag_id": ints(o.TagIDs), "exclude_tag_id": ints(o.ExcludeTagIDs),
		"series_id": ints(o.SeriesIDs), "game_id": o.GameIDs, "created_by": o.CreatedBy,
	}
	return http.GetContext[types.EventsPage](ctx, c.baseURL, "/events/keyset", params, http.WithMultiParams(multi), http.WithService("gamma"))
}
