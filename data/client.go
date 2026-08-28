package data

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// Client 定义数据客户端的接口，供外部包使用
type Client interface {
	GetPositions(user types.EthAddress, options ...GetPositionsOption) ([]types.Position, error)
	GetPositionsContext(ctx context.Context, user types.EthAddress, options ...GetPositionsOption) ([]types.Position, error)
	GetTrades(limit int, offset int, options ...GetTradesOption) ([]types.Trade, error)
	GetTradesContext(ctx context.Context, limit int, offset int, options ...GetTradesOption) ([]types.Trade, error)
	GetActivity(user types.EthAddress, limit int, offset int, options ...GetActivityOption) ([]types.Activity, error)
	GetActivityContext(ctx context.Context, user types.EthAddress, limit int, offset int, options ...GetActivityOption) ([]types.Activity, error)
	GetValue(user types.EthAddress, conditionIDs interface{}) ([]types.ValueResponse, error)
	GetValueContext(ctx context.Context, user types.EthAddress, conditionIDs interface{}) ([]types.ValueResponse, error)
	GetTotalValue(user types.EthAddress, conditionIDs interface{}) (float64, error)
	GetTotalValueContext(ctx context.Context, user types.EthAddress, conditionIDs interface{}) (float64, error)
}

// polymarketDataClient 处理数据API操作
// 不允许直接导出，只能通过 NewPolymarketDataClient 创建
type polymarketDataClient struct {
	baseURL string // API 基础 URL
}

// NewClient 创建新的数据客户端
// 返回 Client 接口，不允许直接访问实现类型
func NewClient() Client {
	return &polymarketDataClient{
		baseURL: internal.DataAPIDomain,
	}
}

// GetPositionsOptions 包含 GetPositions 的所有可选参数
type GetPositionsOptions struct {
	Limit         int         // limit, 默认值为 500
	Offset        int         // offset, 默认值为 0
	ConditionID   interface{} // string, []string, or nil
	EventID       interface{} // int, []int, or nil
	SizeThreshold float64
	Redeemable    *bool
	Mergeable     *bool
	Title         *string
	SortBy        string
	SortDirection string
}

// GetPositionsOption 函数选项类型
type GetPositionsOption func(*GetPositionsOptions)

// WithPositionsConditionID 设置conditionID过滤
func WithPositionsConditionID(conditionID interface{}) GetPositionsOption {
	return func(opts *GetPositionsOptions) {
		opts.ConditionID = conditionID
	}
}

// WithPositionsEventID 设置eventID过滤
func WithPositionsEventID(eventID interface{}) GetPositionsOption {
	return func(opts *GetPositionsOptions) {
		opts.EventID = eventID
	}
}

// WithPositionsSizeThreshold 设置sizeThreshold
func WithPositionsSizeThreshold(sizeThreshold float64) GetPositionsOption {
	return func(opts *GetPositionsOptions) {
		opts.SizeThreshold = sizeThreshold
	}
}

// WithPositionsRedeemable 设置redeemable过滤
func WithPositionsRedeemable(redeemable bool) GetPositionsOption {
	return func(opts *GetPositionsOptions) {
		opts.Redeemable = &redeemable
	}
}

// WithPositionsMergeable 设置mergeable过滤
func WithPositionsMergeable(mergeable bool) GetPositionsOption {
	return func(opts *GetPositionsOptions) {
		opts.Mergeable = &mergeable
	}
}

// WithPositionsTitle 设置title过滤
func WithPositionsTitle(title string) GetPositionsOption {
	return func(opts *GetPositionsOptions) {
		opts.Title = &title
	}
}

// WithPositionsSort 设置排序
func WithPositionsSort(sortBy string, sortDirection string) GetPositionsOption {
	return func(opts *GetPositionsOptions) {
		opts.SortBy = sortBy
		opts.SortDirection = sortDirection
	}
}

// WithPositionsLimit 设置limit
// func WithPositionsLimit(limit int) GetPositionsOption {
// 	return func(opts *GetPositionsOptions) {
// 		opts.Limit = limit
// 	}
// }

// WithPositionsOffset 设置offset
// func WithPositionsOffset(offset int) GetPositionsOption {
// 	return func(opts *GetPositionsOptions) {
// 		opts.Offset = offset
// 	}
// }

// GetPositions 获取用户仓位
// user 是必要参数，其他参数通过选项函数传入
func (c *polymarketDataClient) GetPositions(user types.EthAddress, options ...GetPositionsOption) ([]types.Position, error) {
	return c.GetPositionsContext(context.Background(), user, options...)
}

func (c *polymarketDataClient) GetPositionsContext(ctx context.Context, user types.EthAddress, options ...GetPositionsOption) ([]types.Position, error) {
	// 初始化默认选项
	opts := &GetPositionsOptions{
		Limit:         500, // 默认值
		Offset:        0,   // 默认值
		SizeThreshold: 0.0,
		SortBy:        "TOKENS",
		SortDirection: "DESC",
	}

	// 应用所有选项
	for _, option := range options {
		option(opts)
	}

	// 使用选项中的 limit 和 offset（默认值已设置为 500 和 0）
	params := map[string]string{
		"user":          string(user),
		"sizeThreshold": formatFloat(opts.SizeThreshold), // Format to match Python (0.0 instead of 0)
		"limit":         strconv.Itoa(min(opts.Limit, 500)),
		"offset":        strconv.Itoa(opts.Offset),
	}

	// Handle condition_id
	if opts.ConditionID != nil {
		switch v := opts.ConditionID.(type) {
		case string:
			params["market"] = v
		case []string:
			params["market"] = strings.Join(v, ",")
		}
	}

	// Handle event_id
	if opts.EventID != nil {
		switch v := opts.EventID.(type) {
		case int:
			params["eventId"] = strconv.Itoa(v)
		case []int:
			eventStrs := make([]string, len(v))
			for i, id := range v {
				eventStrs[i] = strconv.Itoa(id)
			}
			params["eventId"] = strings.Join(eventStrs, ",")
		}
	}

	if opts.Redeemable != nil {
		params["redeemable"] = strconv.FormatBool(*opts.Redeemable)
	}
	if opts.Mergeable != nil {
		params["mergeable"] = strconv.FormatBool(*opts.Mergeable)
	}
	if opts.Title != nil {
		params["title"] = *opts.Title
	}
	if opts.SortBy != "" {
		params["sortBy"] = opts.SortBy
	}
	if opts.SortDirection != "" {
		params["sortDirection"] = opts.SortDirection
	}

	return http.GetSliceContext[types.Position](ctx, c.baseURL, "/positions", params, http.WithService("data"))
}

// GetTradesOptions 包含 GetTrades 的所有可选参数
type GetTradesOptions struct {
	TakerOnly    bool
	FilterType   *string
	FilterAmount *float64
	ConditionID  interface{}
	EventID      interface{}
	User         *string
	Side         *types.OrderSide
	Start        *time.Time
	End          *time.Time
}

// GetTradesOption 函数选项类型
type GetTradesOption func(*GetTradesOptions)

// WithTradesTakerOnly 设置takerOnly
func WithTradesTakerOnly(takerOnly bool) GetTradesOption {
	return func(opts *GetTradesOptions) {
		opts.TakerOnly = takerOnly
	}
}

// WithTradesFilter 设置filterType和filterAmount
func WithTradesFilter(filterType string, filterAmount *float64) GetTradesOption {
	return func(opts *GetTradesOptions) {
		opts.FilterType = &filterType
		opts.FilterAmount = filterAmount
	}
}

// WithTradesConditionID 设置conditionID过滤
func WithTradesConditionID(conditionID interface{}) GetTradesOption {
	return func(opts *GetTradesOptions) {
		opts.ConditionID = conditionID
	}
}

// WithTradesEventID 设置eventID过滤
func WithTradesEventID(eventID interface{}) GetTradesOption {
	return func(opts *GetTradesOptions) {
		opts.EventID = eventID
	}
}

// WithTradesUser 设置user过滤
func WithTradesUser(user string) GetTradesOption {
	return func(opts *GetTradesOptions) {
		opts.User = &user
	}
}

// WithTradesSide 设置side过滤
func WithTradesSide(side types.OrderSide) GetTradesOption {
	return func(opts *GetTradesOptions) {
		opts.Side = &side
	}
}

// WithTradesDateRange 按 Unix 秒时间窗口筛选成交记录。
// nil 表示不发送对应参数，使用 Data API 的默认时间范围。
func WithTradesDateRange(start, end *time.Time) GetTradesOption {
	return func(opts *GetTradesOptions) {
		opts.Start = start
		opts.End = end
	}
}

// GetTrades 获取交易记录
// limit 和 offset 是必要参数，其他参数通过选项函数传入
func (c *polymarketDataClient) GetTrades(limit int, offset int, options ...GetTradesOption) ([]types.Trade, error) {
	return c.GetTradesContext(context.Background(), limit, offset, options...)
}

func (c *polymarketDataClient) GetTradesContext(ctx context.Context, limit int, offset int, options ...GetTradesOption) ([]types.Trade, error) {
	// 初始化默认选项
	opts := &GetTradesOptions{TakerOnly: true}

	// 应用所有选项
	for _, option := range options {
		option(opts)
	}

	params := map[string]string{
		"limit":     strconv.Itoa(min(limit, 10000)),
		"offset":    strconv.Itoa(offset),
		"takerOnly": strconv.FormatBool(opts.TakerOnly),
	}

	if opts.FilterType != nil {
		params["filterType"] = *opts.FilterType
	}
	if opts.FilterAmount != nil {
		params["filterAmount"] = fmt.Sprintf("%f", *opts.FilterAmount)
	}

	// Handle condition_id
	if opts.ConditionID != nil {
		switch v := opts.ConditionID.(type) {
		case string:
			params["market"] = v
		case []string:
			params["market"] = strings.Join(v, ",")
		}
	}

	// Handle event_id
	if opts.EventID != nil {
		switch v := opts.EventID.(type) {
		case int:
			params["eventId"] = strconv.Itoa(v)
		case []int:
			eventStrs := make([]string, len(v))
			for i, id := range v {
				eventStrs[i] = strconv.Itoa(id)
			}
			params["eventId"] = strings.Join(eventStrs, ",")
		}
	}

	if opts.User != nil {
		params["user"] = *opts.User
	}
	if opts.Side != nil {
		params["side"] = string(*opts.Side)
	}
	if opts.Start != nil {
		params["start"] = strconv.FormatInt(opts.Start.Unix(), 10)
	}
	if opts.End != nil {
		params["end"] = strconv.FormatInt(opts.End.Unix(), 10)
	}

	return http.GetSliceContext[types.Trade](ctx, c.baseURL, "/trades", params, http.WithService("data"))
}

// GetActivityOptions 包含 GetActivity 的所有可选参数
type GetActivityOptions struct {
	ConditionID                interface{}
	EventID                    interface{}
	ActivityType               interface{} // string, []string, ActivityType, []ActivityType, or nil
	ExcludeDepositsWithdrawals *bool
	Start                      *time.Time
	End                        *time.Time
	Side                       *types.OrderSide
	SortBy                     string
	SortDirection              string
}

// GetActivityOption 函数选项类型
type GetActivityOption func(*GetActivityOptions)

// WithActivityConditionID 设置conditionID过滤
func WithActivityConditionID(conditionID interface{}) GetActivityOption {
	return func(opts *GetActivityOptions) {
		opts.ConditionID = conditionID
	}
}

// WithActivityEventID 设置eventID过滤
func WithActivityEventID(eventID interface{}) GetActivityOption {
	return func(opts *GetActivityOptions) {
		opts.EventID = eventID
	}
}

// WithActivityType 设置activityType过滤
func WithActivityType(activityType interface{}) GetActivityOption {
	return func(opts *GetActivityOptions) {
		opts.ActivityType = activityType
	}
}

// WithActivityExcludeDepositsWithdrawals 控制是否排除充值和提现活动。
// 官方默认值为 true；查询 DEPOSIT/WITHDRAWAL 时应显式传 false。
func WithActivityExcludeDepositsWithdrawals(exclude bool) GetActivityOption {
	return func(opts *GetActivityOptions) {
		opts.ExcludeDepositsWithdrawals = &exclude
	}
}

// WithActivityDateRange 设置日期范围
func WithActivityDateRange(start, end *time.Time) GetActivityOption {
	return func(opts *GetActivityOptions) {
		opts.Start = start
		opts.End = end
	}
}

// WithActivitySide 设置side过滤
func WithActivitySide(side types.OrderSide) GetActivityOption {
	return func(opts *GetActivityOptions) {
		opts.Side = &side
	}
}

// WithActivitySort 设置排序
func WithActivitySort(sortBy string, sortDirection string) GetActivityOption {
	return func(opts *GetActivityOptions) {
		opts.SortBy = sortBy
		opts.SortDirection = sortDirection
	}
}

// GetActivity 获取用户活动
// user, limit, offset 是必要参数，其他参数通过选项函数传入
func (c *polymarketDataClient) GetActivity(user types.EthAddress, limit int, offset int, options ...GetActivityOption) ([]types.Activity, error) {
	return c.GetActivityContext(context.Background(), user, limit, offset, options...)
}

func (c *polymarketDataClient) GetActivityContext(ctx context.Context, user types.EthAddress, limit int, offset int, options ...GetActivityOption) ([]types.Activity, error) {
	// 初始化默认选项
	opts := &GetActivityOptions{}

	// 应用所有选项
	for _, option := range options {
		option(opts)
	}

	params := map[string]string{
		"user":   string(user),
		"limit":  strconv.Itoa(min(limit, 500)),
		"offset": strconv.Itoa(offset),
	}

	// Handle condition_id
	if opts.ConditionID != nil {
		switch v := opts.ConditionID.(type) {
		case string:
			params["market"] = v
		case []string:
			params["market"] = strings.Join(v, ",")
		}
	}

	// Handle event_id
	if opts.EventID != nil {
		switch v := opts.EventID.(type) {
		case int:
			params["eventId"] = strconv.Itoa(v)
		case []int:
			eventStrs := make([]string, len(v))
			for i, id := range v {
				eventStrs[i] = strconv.Itoa(id)
			}
			params["eventId"] = strings.Join(eventStrs, ",")
		}
	}

	// Handle activityType
	if opts.ActivityType != nil {
		switch v := opts.ActivityType.(type) {
		case string:
			params["type"] = v
		case []string:
			params["type"] = strings.Join(v, ",")
		case types.ActivityType:
			params["type"] = string(v)
		case []types.ActivityType:
			values := make([]string, len(v))
			for i, activityType := range v {
				values[i] = string(activityType)
			}
			params["type"] = strings.Join(values, ",")
		}
	}
	if opts.ExcludeDepositsWithdrawals != nil {
		params["excludeDepositsWithdrawals"] = strconv.FormatBool(*opts.ExcludeDepositsWithdrawals)
	}

	if opts.Start != nil {
		params["start"] = strconv.FormatInt(opts.Start.Unix(), 10)
	}
	if opts.End != nil {
		params["end"] = strconv.FormatInt(opts.End.Unix(), 10)
	}
	if opts.Side != nil {
		params["side"] = string(*opts.Side)
	}
	if opts.SortBy != "" {
		params["sortBy"] = opts.SortBy
	}
	if opts.SortDirection != "" {
		params["sortDirection"] = opts.SortDirection
	}

	return http.GetSliceContext[types.Activity](ctx, c.baseURL, "/activity", params, http.WithService("data"))
}

// GetValue 获取仓位价值
func (c *polymarketDataClient) GetValue(
	user types.EthAddress,
	conditionIDs interface{}, // nil, string, or []string
) ([]types.ValueResponse, error) {
	return c.GetValueContext(context.Background(), user, conditionIDs)
}

func (c *polymarketDataClient) GetValueContext(ctx context.Context, user types.EthAddress, conditionIDs interface{}) ([]types.ValueResponse, error) {
	params := map[string]string{
		"user": string(user),
	}

	if conditionIDs != nil {
		switch v := conditionIDs.(type) {
		case string:
			params["market"] = v
		case []string:
			params["market"] = strings.Join(v, ",")
		}
	}

	responses, err := http.GetSliceContext[types.ValueResponse](ctx, c.baseURL, "/value", params, http.WithService("data"))
	if err != nil {
		return nil, fmt.Errorf("failed to get value: %w", err)
	}
	return responses, nil
}

// GetTotalValue 获取 /value 响应中所有条目的价值合计。
// 当前官方通常返回一条聚合值；求和可完整处理未来或其他筛选下的多条响应。
func (c *polymarketDataClient) GetTotalValue(
	user types.EthAddress,
	conditionIDs interface{},
) (float64, error) {
	return c.GetTotalValueContext(context.Background(), user, conditionIDs)
}

func (c *polymarketDataClient) GetTotalValueContext(ctx context.Context, user types.EthAddress, conditionIDs interface{}) (float64, error) {
	values, err := c.GetValueContext(ctx, user, conditionIDs)
	if err != nil {
		return 0, err
	}
	return sumValueResponses(values), nil
}

func sumValueResponses(values []types.ValueResponse) float64 {
	var total float64
	for _, value := range values {
		total += value.Value
	}
	return total
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatFloat formats a float to match Python's format (0.0 instead of 0)
func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		// If it's a whole number, format with .0
		return fmt.Sprintf("%.1f", f)
	}
	// Otherwise use %g to remove trailing zeros
	return fmt.Sprintf("%g", f)
}
