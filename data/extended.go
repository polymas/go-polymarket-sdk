package data

import (
	"context"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/types"
)

type LeaderboardTimePeriod string

const (
	LeaderboardDay   LeaderboardTimePeriod = "DAY"
	LeaderboardWeek  LeaderboardTimePeriod = "WEEK"
	LeaderboardMonth LeaderboardTimePeriod = "MONTH"
	LeaderboardAll   LeaderboardTimePeriod = "ALL"
)

type ComboPositionsOptions struct {
	Statuses      []string
	Sort          string
	MarketIDs     []string
	Limit         int
	Offset        int
	UpdatedAfter  *int64
	UpdatedBefore *int64
	Cursor        string
}

type ComboActivityOptions struct {
	MarketIDs []string
	Limit     int
	Offset    int
	Cursor    string
}

type ClosedPositionsOptions struct {
	ConditionIDs  []string
	Title         string
	EventIDs      []int
	Limit         int
	Offset        int
	SortBy        string
	SortDirection string
}

type MarketPositionsOptions struct {
	User          *types.EthAddress
	Status        string
	SortBy        string
	SortDirection string
	Limit         int
	Offset        int
}

type BuilderLeaderboardOptions struct {
	TimePeriod LeaderboardTimePeriod
	Limit      int
	Offset     int
}

type TraderLeaderboardOptions struct {
	Category   string
	TimePeriod LeaderboardTimePeriod
	OrderBy    string
	Limit      int
	Offset     int
	User       *types.EthAddress
	UserName   string
}

func (c *polymarketDataClient) GetHealth() (*types.DataHealth, error) {
	return c.GetHealthContext(context.Background())
}
func (c *polymarketDataClient) GetHealthContext(ctx context.Context) (*types.DataHealth, error) {
	return http.GetContext[types.DataHealth](ctx, c.baseURL, "/", nil, http.WithService("data"))
}

func (c *polymarketDataClient) GetAccountingSnapshot(user types.EthAddress) ([]byte, error) {
	return c.GetAccountingSnapshotContext(context.Background(), user)
}
func (c *polymarketDataClient) GetAccountingSnapshotContext(ctx context.Context, user types.EthAddress) ([]byte, error) {
	return http.GetRawContext(ctx, c.baseURL, "GET", "/v1/accounting/snapshot", map[string]string{"user": string(user)}, http.WithService("data"))
}

func (c *polymarketDataClient) GetApprovals(user types.EthAddress) (*types.ApprovalsResponse, error) {
	return c.GetApprovalsContext(context.Background(), user)
}
func (c *polymarketDataClient) GetApprovalsContext(ctx context.Context, user types.EthAddress) (*types.ApprovalsResponse, error) {
	return http.GetContext[types.ApprovalsResponse](ctx, c.baseURL, "/v1/approvals", map[string]string{"user": string(user)}, http.WithService("data"))
}

func (c *polymarketDataClient) GetComboActivity(user types.EthAddress, options ComboActivityOptions) (*types.ComboActivityResponse, error) {
	return c.GetComboActivityContext(context.Background(), user, options)
}
func (c *polymarketDataClient) GetComboActivityContext(ctx context.Context, user types.EthAddress, o ComboActivityOptions) (*types.ComboActivityResponse, error) {
	limit := defaultInt(o.Limit, 50, 500)
	params := map[string]string{"user": string(user), "limit": strconv.Itoa(limit), "offset": strconv.Itoa(clamp(o.Offset, 0, 10000))}
	if o.Cursor != "" {
		params["cursor"] = o.Cursor
	}
	return http.GetContext[types.ComboActivityResponse](ctx, c.baseURL, "/v1/activity/combos", params, http.WithMultiParams(map[string][]string{"market_id": o.MarketIDs}), http.WithService("data"))
}

func (c *polymarketDataClient) GetComboPositions(user types.EthAddress, options ComboPositionsOptions) (*types.ComboPositionsResponse, error) {
	return c.GetComboPositionsContext(context.Background(), user, options)
}
func (c *polymarketDataClient) GetComboPositionsContext(ctx context.Context, user types.EthAddress, o ComboPositionsOptions) (*types.ComboPositionsResponse, error) {
	limit := defaultInt(o.Limit, 20, 1000)
	sort := o.Sort
	if sort == "" {
		sort = "current_value_desc"
	}
	params := map[string]string{"user": string(user), "limit": strconv.Itoa(limit), "offset": strconv.Itoa(clamp(o.Offset, 0, 100000)), "sort": sort}
	if o.UpdatedAfter != nil {
		params["updatedAfter"] = strconv.FormatInt(*o.UpdatedAfter, 10)
	}
	if o.UpdatedBefore != nil {
		params["updatedBefore"] = strconv.FormatInt(*o.UpdatedBefore, 10)
	}
	if o.Cursor != "" {
		params["cursor"] = o.Cursor
	}
	return http.GetContext[types.ComboPositionsResponse](ctx, c.baseURL, "/v1/positions/combos", params, http.WithMultiParams(map[string][]string{"status": o.Statuses, "market_id": o.MarketIDs}), http.WithService("data"))
}

func (c *polymarketDataClient) GetHolders(conditionIDs []string, limit, minBalance int) ([]types.MarketHolders, error) {
	return c.GetHoldersContext(context.Background(), conditionIDs, limit, minBalance)
}
func (c *polymarketDataClient) GetHoldersContext(ctx context.Context, conditionIDs []string, limit, minBalance int) ([]types.MarketHolders, error) {
	params := map[string]string{"limit": strconv.Itoa(defaultInt(limit, 20, 20)), "minBalance": strconv.Itoa(clamp(minBalance, 0, 999999))}
	return http.GetSliceContext[types.MarketHolders](ctx, c.baseURL, "/holders", params, http.WithMultiParams(map[string][]string{"market": conditionIDs}), http.WithService("data"))
}

func (c *polymarketDataClient) GetTradedMarkets(user types.EthAddress) (*types.TradedMarkets, error) {
	return c.GetTradedMarketsContext(context.Background(), user)
}
func (c *polymarketDataClient) GetTradedMarketsContext(ctx context.Context, user types.EthAddress) (*types.TradedMarkets, error) {
	return http.GetContext[types.TradedMarkets](ctx, c.baseURL, "/traded", map[string]string{"user": string(user)}, http.WithService("data"))
}

func (c *polymarketDataClient) GetRevisions(questionID types.Keccak256, limit int) ([]types.RevisionPayload, error) {
	return c.GetRevisionsContext(context.Background(), questionID, limit)
}
func (c *polymarketDataClient) GetRevisionsContext(ctx context.Context, questionID types.Keccak256, limit int) ([]types.RevisionPayload, error) {
	return http.GetSliceContext[types.RevisionPayload](ctx, c.baseURL, "/revisions", map[string]string{"questionID": questionID.String(), "limit": strconv.Itoa(defaultInt(limit, 100, 500))}, http.WithService("data"))
}

func (c *polymarketDataClient) GetOpenInterest(conditionIDs []string) ([]types.OpenInterest, error) {
	return c.GetOpenInterestContext(context.Background(), conditionIDs)
}
func (c *polymarketDataClient) GetOpenInterestContext(ctx context.Context, conditionIDs []string) ([]types.OpenInterest, error) {
	return http.GetSliceContext[types.OpenInterest](ctx, c.baseURL, "/oi", nil, http.WithMultiParams(map[string][]string{"market": conditionIDs}), http.WithService("data"))
}

func (c *polymarketDataClient) GetLiveVolume(eventID int) ([]types.LiveVolume, error) {
	return c.GetLiveVolumeContext(context.Background(), eventID)
}
func (c *polymarketDataClient) GetLiveVolumeContext(ctx context.Context, eventID int) ([]types.LiveVolume, error) {
	return http.GetSliceContext[types.LiveVolume](ctx, c.baseURL, "/live-volume", map[string]string{"id": strconv.Itoa(eventID)}, http.WithService("data"))
}

func (c *polymarketDataClient) GetClosedPositions(user types.EthAddress, options ClosedPositionsOptions) ([]types.ClosedPosition, error) {
	return c.GetClosedPositionsContext(context.Background(), user, options)
}
func (c *polymarketDataClient) GetClosedPositionsContext(ctx context.Context, user types.EthAddress, o ClosedPositionsOptions) ([]types.ClosedPosition, error) {
	limit := defaultInt(o.Limit, 10, 50)
	sortBy := o.SortBy
	if sortBy == "" {
		sortBy = "REALIZEDPNL"
	}
	direction := o.SortDirection
	if direction == "" {
		direction = "DESC"
	}
	params := map[string]string{"user": string(user), "limit": strconv.Itoa(limit), "offset": strconv.Itoa(clamp(o.Offset, 0, 100000)), "sortBy": sortBy, "sortDirection": direction}
	if o.Title != "" {
		params["title"] = o.Title
	}
	eventIDs := make([]string, len(o.EventIDs))
	for i, id := range o.EventIDs {
		eventIDs[i] = strconv.Itoa(id)
	}
	return http.GetSliceContext[types.ClosedPosition](ctx, c.baseURL, "/closed-positions", params, http.WithMultiParams(map[string][]string{"market": o.ConditionIDs, "eventId": eventIDs}), http.WithService("data"))
}

func (c *polymarketDataClient) GetOtherPositions(eventID int, user types.EthAddress) ([]types.OtherPositionSize, error) {
	return c.GetOtherPositionsContext(context.Background(), eventID, user)
}
func (c *polymarketDataClient) GetOtherPositionsContext(ctx context.Context, eventID int, user types.EthAddress) ([]types.OtherPositionSize, error) {
	return http.GetSliceContext[types.OtherPositionSize](ctx, c.baseURL, "/other", map[string]string{"id": strconv.Itoa(eventID), "user": string(user)}, http.WithService("data"))
}

func (c *polymarketDataClient) GetMarketPositions(market types.Keccak256, options MarketPositionsOptions) ([]types.MarketPositions, error) {
	return c.GetMarketPositionsContext(context.Background(), market, options)
}
func (c *polymarketDataClient) GetMarketPositionsContext(ctx context.Context, market types.Keccak256, o MarketPositionsOptions) ([]types.MarketPositions, error) {
	status := o.Status
	if status == "" {
		status = "ALL"
	}
	sortBy := o.SortBy
	if sortBy == "" {
		sortBy = "TOTAL_PNL"
	}
	direction := o.SortDirection
	if direction == "" {
		direction = "DESC"
	}
	params := map[string]string{"market": market.String(), "status": status, "sortBy": sortBy, "sortDirection": direction, "limit": strconv.Itoa(defaultInt(o.Limit, 50, 500)), "offset": strconv.Itoa(clamp(o.Offset, 0, 10000))}
	if o.User != nil {
		params["user"] = string(*o.User)
	}
	return http.GetSliceContext[types.MarketPositions](ctx, c.baseURL, "/v1/market-positions", params, http.WithService("data"))
}

func (c *polymarketDataClient) GetBuilderLeaderboard(options BuilderLeaderboardOptions) ([]types.BuilderLeaderboardEntry, error) {
	return c.GetBuilderLeaderboardContext(context.Background(), options)
}
func (c *polymarketDataClient) GetBuilderLeaderboardContext(ctx context.Context, o BuilderLeaderboardOptions) ([]types.BuilderLeaderboardEntry, error) {
	period := o.TimePeriod
	if period == "" {
		period = LeaderboardDay
	}
	params := map[string]string{"timePeriod": string(period), "limit": strconv.Itoa(defaultInt(o.Limit, 25, 50)), "offset": strconv.Itoa(clamp(o.Offset, 0, 1000))}
	return http.GetSliceContext[types.BuilderLeaderboardEntry](ctx, c.baseURL, "/v1/builders/leaderboard", params, http.WithService("data"))
}

func (c *polymarketDataClient) GetBuilderVolume(period LeaderboardTimePeriod) ([]types.BuilderVolumeEntry, error) {
	return c.GetBuilderVolumeContext(context.Background(), period)
}
func (c *polymarketDataClient) GetBuilderVolumeContext(ctx context.Context, period LeaderboardTimePeriod) ([]types.BuilderVolumeEntry, error) {
	if period == "" {
		period = LeaderboardDay
	}
	return http.GetSliceContext[types.BuilderVolumeEntry](ctx, c.baseURL, "/v1/builders/volume", map[string]string{"timePeriod": string(period)}, http.WithService("data"))
}

func (c *polymarketDataClient) GetTraderLeaderboard(options TraderLeaderboardOptions) ([]types.TraderLeaderboardEntry, error) {
	return c.GetTraderLeaderboardContext(context.Background(), options)
}
func (c *polymarketDataClient) GetTraderLeaderboardContext(ctx context.Context, o TraderLeaderboardOptions) ([]types.TraderLeaderboardEntry, error) {
	category := o.Category
	if category == "" {
		category = "OVERALL"
	}
	period := o.TimePeriod
	if period == "" {
		period = LeaderboardDay
	}
	orderBy := o.OrderBy
	if orderBy == "" {
		orderBy = "PNL"
	}
	params := map[string]string{"category": category, "timePeriod": string(period), "orderBy": orderBy, "limit": strconv.Itoa(defaultInt(o.Limit, 25, 50)), "offset": strconv.Itoa(clamp(o.Offset, 0, 1000))}
	if o.User != nil {
		params["user"] = string(*o.User)
	}
	if o.UserName != "" {
		params["userName"] = o.UserName
	}
	return http.GetSliceContext[types.TraderLeaderboardEntry](ctx, c.baseURL, "/v1/leaderboard", params, http.WithService("data"))
}

func defaultInt(value, fallback, maximum int) int {
	if value <= 0 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}
func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
