package types

import (
	"encoding/json"
	"strings"
	"time"
)

// 默认结束日期常量（用于空字符串或解析失败的情况）
const (
	defaultEndDateYear  = 2099
	defaultEndDateMonth = 12
	defaultEndDateDay   = 31
)

// getDefaultEndDate 返回默认的结束日期
func getDefaultEndDate() time.Time {
	return time.Date(defaultEndDateYear, defaultEndDateMonth, defaultEndDateDay, 0, 0, 0, 0, time.UTC)
}

// EndDate 是用于处理API中各种日期格式的自定义类型
// 可以是空字符串、仅日期字符串（2025-12-24）或完整的RFC3339日期时间
type EndDate struct {
	time.Time
}

// UnmarshalJSON 实现EndDate的自定义JSON反序列化
func (e *EndDate) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	// Handle empty string - set to far future date (like Python does)
	if s == "" {
		e.Time = getDefaultEndDate()
		return nil
	}

	// Try parsing as RFC3339 first (full datetime)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		e.Time = t
		return nil
	}

	// Try parsing as date-only format (2006-01-02)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		e.Time = t
		return nil
	}

	// Try parsing as date with time but no timezone (2006-01-02T15:04:05)
	if strings.Contains(s, "T") && !strings.Contains(s, "Z") && !strings.Contains(s, "+") && !strings.Contains(s, "-") {
		if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
			e.Time = t
			return nil
		}
	}

	// If all parsing fails, set to far future date
	e.Time = getDefaultEndDate()
	return nil
}

// MarshalJSON 实现EndDate的自定义JSON序列化
func (e EndDate) MarshalJSON() ([]byte, error) {
	if e.Time.IsZero() || e.Time.Year() == 2099 {
		return json.Marshal("")
	}
	return json.Marshal(e.Time.Format(time.RFC3339))
}

// Position 表示用户仓位
type Position struct {
	ProxyWallet          EthAddress `json:"proxyWallet"`
	TokenID              string     `json:"asset"`         // alias: asset
	ComplementaryTokenID string     `json:"oppositeAsset"` // alias: oppositeAsset
	ConditionID          Keccak256  `json:"conditionId"`   // 支持 conditionId 和 condition_id
	Outcome              string     `json:"outcome"`
	ComplementaryOutcome string     `json:"oppositeOutcome"` // alias: oppositeOutcome
	OutcomeIndex         int        `json:"outcomeIndex"`    // alias: outcomeIndex
	Size                 float64    `json:"size"`
	AvgPrice             float64    `json:"avgPrice"` // alias: avgPrice
	CurrentPrice         float64    `json:"curPrice"` // alias: curPrice
	Redeemable           bool       `json:"redeemable"`
	InitialValue         float64    `json:"initialValue"`       // alias: initialValue
	CurrentValue         float64    `json:"currentValue"`       // alias: currentValue
	CashPnL              float64    `json:"cashPnl"`            // alias: cashPnl
	PercentPnL           float64    `json:"percentPnl"`         // alias: percentPnl
	TotalBought          float64    `json:"totalBought"`        // alias: totalBought
	RealizedPnL          float64    `json:"realizedPnl"`        // alias: realizedPnl
	PercentRealizedPnL   float64    `json:"percentRealizedPnl"` // alias: percentRealizedPnl
	Title                string     `json:"title"`
	Slug                 string     `json:"slug"`
	Icon                 string     `json:"icon"`
	EventSlug            string     `json:"eventSlug"`    // alias: eventSlug
	EndDate              EndDate    `json:"endDate"`      // alias: endDate - custom type to handle empty string or date-only format
	NegativeRisk         bool       `json:"negativeRisk"` // alias: negativeRisk
	// Legacy fields for backward compatibility
	Price     float64 `json:"price,omitempty"`     // Not in API response, keep for compatibility
	Resolving bool    `json:"resolving,omitempty"` // Not in API response, keep for compatibility
	Mergeable bool    `json:"mergeable,omitempty"` // Not in API response, keep for compatibility
}

// UnmarshalJSON 实现Position的自定义JSON反序列化，支持conditionId和condition_id两种字段名
func (p *Position) UnmarshalJSON(data []byte) error {
	// 先解析到 map 以便处理字段名差异
	var rawData map[string]interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return err
	}

	// 处理 conditionId 和 condition_id 两种字段名
	var conditionIDValue interface{}
	var hasConditionID bool

	// 优先使用 conditionId (驼峰式)
	if val, ok := rawData["conditionId"]; ok {
		conditionIDValue = val
		hasConditionID = true
	} else if val, ok := rawData["condition_id"]; ok {
		// 如果没有 conditionId，尝试 condition_id (下划线式)
		conditionIDValue = val
		hasConditionID = true
	}

	// 如果找到了conditionID，确保它在rawData中
	if hasConditionID {
		rawData["conditionId"] = conditionIDValue
	}

	// 重新序列化并反序列化到结构体
	processedData, err := json.Marshal(rawData)
	if err != nil {
		return err
	}

	// 使用临时结构体进行标准反序列化
	type Alias Position
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(p),
	}

	return json.Unmarshal(processedData, aux)
}

// Trade 表示交易
type Trade struct {
	TradeID     string     `json:"id"`
	ConditionID Keccak256  `json:"market"`
	TokenID     string     `json:"asset_id"`
	Side        OrderSide  `json:"side"`
	Price       float64    `json:"price"`
	Size        float64    `json:"size"`
	CashAmount  float64    `json:"cash_amount"`
	TokenAmount float64    `json:"token_amount"`
	Timestamp   time.Time  `json:"timestamp"`
	User        EthAddress `json:"user"`
	TakerOnly   bool       `json:"taker_only"`
}

// Activity 表示用户活动
type Activity struct {
	ActivityID  string     `json:"id"`
	Type        string     `json:"type"` // TRADE, SPLIT, MERGE, REDEEM, REWARD, CONVERSION
	ConditionID Keccak256  `json:"market"`
	TokenID     string     `json:"asset_id"`
	Side        *OrderSide `json:"side,omitempty"`
	Tokens      float64    `json:"tokens"`
	Cash        float64    `json:"cash"`
	Timestamp   time.Time  `json:"timestamp"`
	User        EthAddress `json:"user"`
}

// HolderResponse 表示持有者信息
type HolderResponse struct {
	TokenID string     `json:"token_id"`
	Address EthAddress `json:"address"`
	Balance float64    `json:"balance"`
}

// ValueResponse 表示仓位价值
type ValueResponse struct {
	User         EthAddress  `json:"user"`
	TotalValue   float64     `json:"total_value"`
	ConditionIDs []Keccak256 `json:"condition_ids,omitempty"`
}

// MarketValue 表示市场价值信息
type MarketValue struct {
	ConditionID Keccak256 `json:"condition_id"`
	Value       float64   `json:"value"`
}

// EventLiveVolume 表示事件的实时交易量
type EventLiveVolume struct {
	EventID int     `json:"event_id"`
	Volume  float64 `json:"volume"`
}

// UserMetric 表示用户指标
type UserMetric struct {
	Address EthAddress `json:"address"`
	Value   float64    `json:"value"`
	Rank    int        `json:"rank"`
}

// UserRank 表示用户排名
type UserRank struct {
	Address EthAddress `json:"address"`
	Rank    int        `json:"rank"`
	Value   float64    `json:"value"`
}

// GQLPosition represents a position from GraphQL
type GQLPosition struct {
	User  EthAddress `json:"user"`
	Asset struct {
		ID        string `json:"id"`
		Condition struct {
			ID Keccak256 `json:"id"`
		} `json:"condition"`
		Complement   string `json:"complement"`
		OutcomeIndex int    `json:"outcomeIndex"`
	} `json:"asset"`
	Balance string `json:"balance"` // BigInt as string
}

// GraphQLResponse 表示 GraphQL 响应
type GraphQLResponse struct {
	Data   interface{}            `json:"data"`
	Errors []GraphQLError         `json:"errors,omitempty"`
}

// GraphQLError 表示 GraphQL 错误
type GraphQLError struct {
	Message   string                 `json:"message"`
	Locations []GraphQLErrorLocation `json:"locations,omitempty"`
	Path      []interface{}          `json:"path,omitempty"`
}

// GraphQLErrorLocation 表示 GraphQL 错误位置
type GraphQLErrorLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// MarketVolume 表示市场交易量
type MarketVolume struct {
	MarketID  string  `json:"marketId"`
	Volume    float64 `json:"volume"`
	TradeCount int    `json:"tradeCount"`
	StartTime int64   `json:"startTime"`
	EndTime   int64   `json:"endTime"`
}

// MarketOpenInterest 表示市场未平仓量
type MarketOpenInterest struct {
	MarketID        string    `json:"marketId"`
	TotalOpenInterest float64 `json:"totalOpenInterest"`
	Timestamp       time.Time `json:"timestamp"`
}

// UserPNL 表示用户盈亏
type UserPNL struct {
	User          EthAddress `json:"user"`
	TotalPNL      float64   `json:"totalPNL"`
	RealizedPNL   float64   `json:"realizedPNL"`
	UnrealizedPNL float64   `json:"unrealizedPNL"`
	Timestamp     time.Time `json:"timestamp"`
}
