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
	InitialValue         float64    `json:"initialValue"` // alias: initialValue
	GrossInitialValue    float64    `json:"grossInitialValue"`
	EntryFeesUSDC        float64    `json:"entryFeesUsdc"`
	CurrentValue         float64    `json:"currentValue"`       // alias: currentValue
	CashPnL              float64    `json:"cashPnl"`            // alias: cashPnl
	PercentPnL           float64    `json:"percentPnl"`         // alias: percentPnl
	TotalBought          float64    `json:"totalBought"`        // alias: totalBought
	RealizedPnL          float64    `json:"realizedPnl"`        // alias: realizedPnl
	PercentRealizedPnL   float64    `json:"percentRealizedPnl"` // alias: percentRealizedPnl
	Title                string     `json:"title"`
	Slug                 string     `json:"slug"`
	Icon                 string     `json:"icon"`
	EventSlug            string     `json:"eventSlug"` // alias: eventSlug
	EventID              string     `json:"eventId"`
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

// Trade 表示 Data API /trades 返回的交易。
// Timestamp 是官方响应中的 Unix 秒，不接受字符串或浮点数静默转换。
type Trade struct {
	ProxyWallet           EthAddress `json:"proxyWallet"`
	Side                  OrderSide  `json:"side"`
	TokenID               string     `json:"asset"`       // alias: asset
	ConditionID           Keccak256  `json:"conditionId"` // alias: conditionId
	Size                  float64    `json:"size"`
	Price                 float64    `json:"price"`
	Timestamp             int64      `json:"timestamp"`
	Title                 string     `json:"title"`
	Slug                  string     `json:"slug"`
	Icon                  string     `json:"icon"`
	EventSlug             string     `json:"eventSlug"`
	Outcome               string     `json:"outcome"`
	OutcomeIndex          int        `json:"outcomeIndex"`
	Name                  string     `json:"name"`
	Pseudonym             string     `json:"pseudonym"`
	Bio                   string     `json:"bio"`
	ProfileImage          string     `json:"profileImage"`
	ProfileImageOptimized string     `json:"profileImageOptimized"`
	TransactionHash       string     `json:"transactionHash"`
}

// ActivityType 是 Data API /activity 返回的活动类型。
type ActivityType string

const (
	ActivityTypeTrade          ActivityType = "TRADE"
	ActivityTypeSplit          ActivityType = "SPLIT"
	ActivityTypeMerge          ActivityType = "MERGE"
	ActivityTypeRedeem         ActivityType = "REDEEM"
	ActivityTypeReward         ActivityType = "REWARD"
	ActivityTypeConversion     ActivityType = "CONVERSION"
	ActivityTypeDeposit        ActivityType = "DEPOSIT"
	ActivityTypeWithdrawal     ActivityType = "WITHDRAWAL"
	ActivityTypeYield          ActivityType = "YIELD"
	ActivityTypeMakerRebate    ActivityType = "MAKER_REBATE"
	ActivityTypeTakerRebate    ActivityType = "TAKER_REBATE"
	ActivityTypeReferralReward ActivityType = "REFERRAL_REWARD"
)

// Activity 表示 Data API /activity 返回的用户活动。
// Asset 在非 token 活动中可能为空，因此保留为字符串而不是强制 TokenID。
type Activity struct {
	ProxyWallet           EthAddress   `json:"proxyWallet"`
	Timestamp             int64        `json:"timestamp"`
	ConditionID           Keccak256    `json:"conditionId"`
	Type                  ActivityType `json:"type"`
	Size                  float64      `json:"size"`
	USDCSize              float64      `json:"usdcSize"`
	TransactionHash       string       `json:"transactionHash"`
	Price                 float64      `json:"price"`
	TokenID               string       `json:"asset"` // alias: asset
	Side                  OrderSide    `json:"side,omitempty"`
	OutcomeIndex          int          `json:"outcomeIndex"`
	Title                 string       `json:"title"`
	Slug                  string       `json:"slug"`
	Icon                  string       `json:"icon"`
	EventSlug             string       `json:"eventSlug"`
	Outcome               string       `json:"outcome"`
	Name                  string       `json:"name"`
	Pseudonym             string       `json:"pseudonym"`
	Bio                   string       `json:"bio"`
	ProfileImage          string       `json:"profileImage"`
	ProfileImageOptimized string       `json:"profileImageOptimized"`
	IsCombo               bool         `json:"isCombo,omitempty"`
}

// HolderResponse 表示持有者信息
type HolderResponse struct {
	TokenID string     `json:"token_id"`
	Address EthAddress `json:"address"`
	Balance float64    `json:"balance"`
}

type DataHealth struct {
	Data string `json:"data"`
}

type Holder struct {
	ProxyWallet           EthAddress `json:"proxyWallet"`
	Bio                   string     `json:"bio"`
	TokenID               string     `json:"asset"`
	Pseudonym             string     `json:"pseudonym"`
	Amount                float64    `json:"amount"`
	DisplayUsernamePublic bool       `json:"displayUsernamePublic"`
	OutcomeIndex          int        `json:"outcomeIndex"`
	Name                  string     `json:"name"`
	ProfileImage          string     `json:"profileImage"`
	ProfileImageOptimized string     `json:"profileImageOptimized"`
	Verified              bool       `json:"verified"`
}

type MarketHolders struct {
	TokenID string   `json:"token"`
	Holders []Holder `json:"holders"`
}

type TradedMarkets struct {
	User   EthAddress `json:"user"`
	Traded int64      `json:"traded"`
}

type RevisionEntry struct {
	Revision  string `json:"revision"`
	Timestamp int64  `json:"timestamp"`
}

type RevisionPayload struct {
	QuestionID Keccak256       `json:"questionID"`
	Revisions  []RevisionEntry `json:"revisions"`
}

type OpenInterest struct {
	ConditionID Keccak256 `json:"market"`
	Value       float64   `json:"value"`
}

type MarketVolume struct {
	ConditionID Keccak256 `json:"market"`
	Value       float64   `json:"value"`
}

type LiveVolume struct {
	Total   float64        `json:"total"`
	Markets []MarketVolume `json:"markets"`
}

type ClosedPosition struct {
	ProxyWallet          EthAddress `json:"proxyWallet"`
	TokenID              string     `json:"asset"`
	ConditionID          Keccak256  `json:"conditionId"`
	AvgPrice             float64    `json:"avgPrice"`
	TotalBought          float64    `json:"totalBought"`
	RealizedPnL          float64    `json:"realizedPnl"`
	CurrentPrice         float64    `json:"curPrice"`
	Timestamp            int64      `json:"timestamp"`
	Title                string     `json:"title"`
	Slug                 string     `json:"slug"`
	Icon                 string     `json:"icon"`
	EventSlug            string     `json:"eventSlug"`
	Outcome              string     `json:"outcome"`
	OutcomeIndex         int        `json:"outcomeIndex"`
	ComplementaryOutcome string     `json:"oppositeOutcome"`
	ComplementaryTokenID string     `json:"oppositeAsset"`
	EndDate              EndDate    `json:"endDate"`
}

type OtherPositionSize struct {
	EventID int        `json:"id"`
	User    EthAddress `json:"user"`
	Size    float64    `json:"size"`
}

type MarketPosition struct {
	ProxyWallet  EthAddress `json:"proxyWallet"`
	Name         string     `json:"name"`
	ProfileImage string     `json:"profileImage"`
	Verified     bool       `json:"verified"`
	TokenID      string     `json:"asset"`
	ConditionID  Keccak256  `json:"conditionId"`
	AvgPrice     float64    `json:"avgPrice"`
	Size         float64    `json:"size"`
	CurrentPrice float64    `json:"currPrice"`
	CurrentValue float64    `json:"currentValue"`
	CashPnL      float64    `json:"cashPnl"`
	TotalBought  float64    `json:"totalBought"`
	RealizedPnL  float64    `json:"realizedPnl"`
	TotalPnL     float64    `json:"totalPnl"`
	Outcome      string     `json:"outcome"`
	OutcomeIndex int        `json:"outcomeIndex"`
}

type MarketPositions struct {
	TokenID   string           `json:"token"`
	Positions []MarketPosition `json:"positions"`
}

type BuilderLeaderboardEntry struct {
	Rank        string  `json:"rank"`
	Builder     string  `json:"builder"`
	BuilderCode string  `json:"builderCode"`
	Volume      float64 `json:"volume"`
	ActiveUsers int     `json:"activeUsers"`
	Verified    bool    `json:"verified"`
	BuilderLogo string  `json:"builderLogo"`
}

type BuilderVolumeEntry struct {
	Date        string  `json:"dt"`
	Builder     string  `json:"builder"`
	BuilderCode string  `json:"builderCode"`
	BuilderLogo string  `json:"builderLogo"`
	Verified    bool    `json:"verified"`
	Volume      float64 `json:"volume"`
	ActiveUsers int     `json:"activeUsers"`
	Rank        string  `json:"rank"`
}

type TraderLeaderboardEntry struct {
	Rank          string     `json:"rank"`
	ProxyWallet   EthAddress `json:"proxyWallet"`
	UserName      string     `json:"userName"`
	Volume        float64    `json:"vol"`
	PnL           float64    `json:"pnl"`
	ProfileImage  string     `json:"profileImage"`
	XUsername     string     `json:"xUsername"`
	VerifiedBadge bool       `json:"verifiedBadge"`
}

type DataPagination struct {
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor"`
}

type ComboConditionID string

type ComboEvent struct {
	EventID    string `json:"event_id"`
	EventSlug  string `json:"event_slug"`
	EventTitle string `json:"event_title"`
	EventImage string `json:"event_image"`
}

type ComboMarket struct {
	MarketID    string     `json:"market_id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Outcome     string     `json:"outcome"`
	ImageURL    string     `json:"image_url"`
	IconURL     string     `json:"icon_url"`
	Category    string     `json:"category"`
	Subcategory string     `json:"subcategory"`
	Tags        []string   `json:"tags"`
	EndDate     string     `json:"end_date"`
	Event       ComboEvent `json:"event"`
}

type ComboLeg struct {
	LegIndex     int         `json:"leg_index"`
	PositionID   string      `json:"leg_position_id"`
	ConditionID  string      `json:"leg_condition_id"`
	OutcomeIndex int         `json:"leg_outcome_index"`
	OutcomeLabel string      `json:"leg_outcome_label"`
	Status       string      `json:"leg_status"`
	ResolvedAt   *string     `json:"leg_resolved_at"`
	CurrentPrice string      `json:"leg_current_price"`
	Market       ComboMarket `json:"market"`
}

type ComboPosition struct {
	ConditionID        ComboConditionID `json:"combo_condition_id"`
	PositionID         string           `json:"combo_position_id"`
	ModuleID           int              `json:"module_id"`
	UserAddress        EthAddress       `json:"user_address"`
	SharesBalance      string           `json:"shares_balance"`
	EntryAvgPriceUSDC  string           `json:"entry_avg_price_usdc"`
	EntryCostUSDC      string           `json:"entry_cost_usdc"`
	RealizedPayoutUSDC string           `json:"realized_payout_usdc"`
	TotalCostUSDC      string           `json:"total_cost_usdc"`
	GrossEntryCostUSDC string           `json:"gross_entry_cost_usdc"`
	EntryFeesUSDC      string           `json:"entry_fees_usdc"`
	Status             string           `json:"status"`
	FirstEntryAt       string           `json:"first_entry_at"`
	ResolvedAt         *string          `json:"resolved_at"`
	UpdatedAt          string           `json:"updated_at"`
	LegsTotal          int              `json:"legs_total"`
	LegsResolved       int              `json:"legs_resolved"`
	LegsPending        int              `json:"legs_pending"`
	Legs               []ComboLeg       `json:"legs"`
}

type ComboActivity struct {
	ID          string           `json:"id"`
	EventKind   string           `json:"event_kind"`
	Side        string           `json:"side"`
	Type        string           `json:"type"`
	ModuleKind  string           `json:"module_kind"`
	UserAddress EthAddress       `json:"user_address"`
	ConditionID ComboConditionID `json:"combo_condition_id"`
	PositionID  string           `json:"combo_position_id"`
	ModuleID    int              `json:"module_id"`
	AmountUSDC  *float64         `json:"amount_usdc"`
	PayoutUSDC  *float64         `json:"payout_usdc"`
	Timestamp   int64            `json:"timestamp"`
	TxDateTime  string           `json:"tx_dttm"`
	TxHash      string           `json:"tx_hash"`
	LogIndex    int              `json:"log_index"`
	BlockNumber int64            `json:"block_number"`
	Legs        []ComboLeg       `json:"legs"`
}

type ComboPositionsResponse struct {
	Combos     []ComboPosition `json:"combos"`
	Pagination DataPagination  `json:"pagination"`
}

type ComboActivityResponse struct {
	Activity   []ComboActivity `json:"activity"`
	Pagination DataPagination  `json:"pagination"`
}

type ApprovalContract struct {
	ID       string     `json:"id"`
	Feature  string     `json:"feature"`
	Token    EthAddress `json:"token"`
	Spender  EthAddress `json:"spender"`
	Standard string     `json:"standard"`
	Amount   string     `json:"amount"`
	Approved bool       `json:"approved"`
}

type ApprovalsResponse struct {
	Address   EthAddress         `json:"address"`
	ChainID   int64              `json:"chainId"`
	CheckedAt string             `json:"checkedAt"`
	Contracts []ApprovalContract `json:"contracts"`
}

// ValueResponse 表示仓位价值
type ValueResponse struct {
	User  EthAddress `json:"user"`
	Value float64    `json:"value"`
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
