package types

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseTokenIDs parses token IDs from various formats (JSON string, array, etc.)
func parseTokenIDs(value interface{}) []string {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			// Format token ID preserving precision for large numbers
			result = append(result, formatTokenIDString(item))
		}
		return result
	case string:
		// Try to parse as JSON first (API returns JSON string arrays as strings)
		var parsed interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			return parseTokenIDs(parsed)
		}
		// If not JSON, return as single element
		return []string{v}
	default:
		return []string{formatTokenIDString(v)}
	}
}

// formatTokenIDString formats a token ID value to string
// Token IDs should always be treated as strings, never as numbers
func formatTokenIDString(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		// Already a string, return as-is
		return v
	case float64:
		// JSON numbers are parsed as float64, convert to string without scientific notation
		// Use %.0f to preserve full precision for large integers
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		// For any other type, convert to string
		// This ensures Token ID is always a string
		return fmt.Sprintf("%v", v)
	}
}

// parseOutcomes parses outcomes from various formats (JSON string, array, etc.)
// API returns JSON string arrays as strings like "[\"YES\", \"NO\"]"
// Returns []string, converting from any format
func parseOutcomes(value interface{}) []string {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			result = append(result, fmt.Sprintf("%v", item))
		}
		return result
	case string:
		// Try to parse as JSON first (API returns JSON string arrays as strings)
		var parsed interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			return parseOutcomes(parsed)
		}
		// If not JSON, try comma-separated
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			cleaned := strings.TrimSpace(strings.Trim(part, "[]\"'"))
			if cleaned != "" {
				result = append(result, cleaned)
			}
		}
		return result
	default:
		return []string{fmt.Sprintf("%v", v)}
	}
}

// parseOutcomePrices parses outcome prices from various formats (JSON string, array, etc.)
// API returns JSON string arrays as strings like "[0.95, 0.05]"
// Returns []float64, converting from any format
func parseOutcomePrices(value interface{}) []float64 {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case []float64:
		return v
	case []interface{}:
		result := make([]float64, 0, len(v))
		for _, item := range v {
			switch val := item.(type) {
			case float64:
				result = append(result, val)
			case float32:
				result = append(result, float64(val))
			case int:
				result = append(result, float64(val))
			case string:
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					result = append(result, f)
				}
			}
		}
		return result
	case string:
		// Try to parse as JSON first (API returns JSON string arrays as strings)
		var parsed interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			return parseOutcomePrices(parsed)
		}
		// If not JSON, try comma-separated
		parts := strings.Split(v, ",")
		result := make([]float64, 0, len(parts))
		for _, part := range parts {
			cleaned := strings.TrimSpace(strings.Trim(part, "[]\"'"))
			if f, err := strconv.ParseFloat(cleaned, 64); err == nil {
				result = append(result, f)
			}
		}
		return result
	default:
		return nil
	}
}

// Event 表示Polymarket事件
type Event struct {
	// 基础字段
	EventID       string        `json:"id"`
	Slug          string        `json:"slug"`
	Ticker        string        `json:"ticker,omitempty"`
	Title         string        `json:"title"`
	Description   string        `json:"description"`
	Image         string        `json:"image"`
	Icon          string        `json:"icon,omitempty"`
	FeaturedImage string        `json:"featuredImage,omitempty"`
	Active        bool          `json:"active"`
	Closed        bool          `json:"closed"`
	Archived      bool          `json:"archived"`
	StartDate     *time.Time    `json:"startDate,omitempty"`
	EndDate       *time.Time    `json:"endDate,omitempty"`
	CreationDate  *time.Time    `json:"creationDate,omitempty"`
	ClosedTime    *time.Time    `json:"closedTime,omitempty"`
	Liquidity     FloatString   `json:"liquidity"`
	Volume        FloatString   `json:"volume"`
	Markets       []GammaMarket `json:"markets,omitempty"`
	Tags          []Tag         `json:"tags,omitempty"`

	// 事件元数据
	Category    string     `json:"category,omitempty"`
	SortBy      string     `json:"sortBy,omitempty"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`

	// 创建和更新信息
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	UpdatedBy string     `json:"updatedBy,omitempty"`

	// 事件状态标志
	New                 bool `json:"new,omitempty"`
	Featured            bool `json:"featured,omitempty"`
	Restricted          bool `json:"restricted,omitempty"`
	CYOM                bool `json:"cyom,omitempty"`
	ShowAllOutcomes     bool `json:"showAllOutcomes,omitempty"`
	ShowMarketImages    bool `json:"showMarketImages,omitempty"`
	EnableNegRisk       bool `json:"enableNegRisk,omitempty"`
	NegRiskAugmented    bool `json:"negRiskAugmented,omitempty"`
	RequiresTranslation bool `json:"requiresTranslation,omitempty"`
	PendingDeployment   bool `json:"pendingDeployment,omitempty"`
	Deploying           bool `json:"deploying,omitempty"`

	// 数值字段
	OpenInterest  *float64 `json:"openInterest,omitempty"`
	Competitive   *float64 `json:"competitive,omitempty"`
	LiquidityAmm  *float64 `json:"liquidityAmm,omitempty"`
	LiquidityClob *float64 `json:"liquidityClob,omitempty"`
	CommentCount  *int     `json:"commentCount,omitempty"`

	// 交易量统计（24小时、1周、1月、1年）
	Volume24hr *float64 `json:"volume24hr,omitempty"`
	Volume1wk  *float64 `json:"volume1wk,omitempty"`
	Volume1mo  *float64 `json:"volume1mo,omitempty"`
	Volume1yr  *float64 `json:"volume1yr,omitempty"`
}

// GammaMarket 表示Gamma市场
type GammaMarket struct {
	// 基础字段
	MarketID      string      `json:"id"`
	Slug          string      `json:"slug"`
	Question      string      `json:"question"`
	Description   string      `json:"description"`
	Image         string      `json:"image"`
	Icon          string      `json:"icon,omitempty"`
	Active        bool        `json:"active"`
	Closed        bool        `json:"closed"`
	Archived      bool        `json:"archived"`
	StartDate     *time.Time  `json:"startDate,omitempty"`
	EndDate       *time.Time  `json:"endDate,omitempty"`
	ClosedTime    *time.Time  `json:"closedTime,omitempty"`
	Liquidity     FloatString `json:"liquidity"`
	Volume        FloatString `json:"volume"`
	ConditionID   Keccak256   `json:"conditionId"`
	TokenIDs      []string    `json:"clobTokenIds"`
	EventID       string      `json:"eventId,omitempty"`
	Tags          []Tag       `json:"tags,omitempty"`
	Outcomes      []string    `json:"outcomes,omitempty"`      // Parsed from JSON string array if needed
	OutcomePrices []float64   `json:"outcomePrices,omitempty"` // Parsed from JSON string array if needed

	// 市场元数据
	ResolutionSource string `json:"resolutionSource,omitempty"`
	Category         string `json:"category,omitempty"`
	MarketType       string `json:"marketType,omitempty"`
	Fee              string `json:"fee,omitempty"`

	// 创建和更新信息
	CreatedAt                *time.Time `json:"createdAt,omitempty"`
	UpdatedAt                *time.Time `json:"updatedAt,omitempty"`
	UpdatedBy                *int       `json:"updatedBy,omitempty"`
	SubmittedBy              string     `json:"submitted_by,omitempty"`
	ResolvedBy               string     `json:"resolvedBy,omitempty"`
	DeployingTimestamp       *time.Time `json:"deployingTimestamp,omitempty"`
	AcceptingOrdersTimestamp *time.Time `json:"acceptingOrdersTimestamp,omitempty"`

	// 市场状态标志
	New                          bool `json:"new,omitempty"`
	Featured                     bool `json:"featured,omitempty"`
	Restricted                   bool `json:"restricted,omitempty"`
	WideFormat                   bool `json:"wideFormat,omitempty"`
	SentDiscord                  bool `json:"sentDiscord,omitempty"`
	Approved                     bool `json:"approved,omitempty"`
	Ready                        bool `json:"ready,omitempty"`
	Funded                       bool `json:"funded,omitempty"`
	CYOM                         bool `json:"cyom,omitempty"`
	FpmmLive                     bool `json:"fpmmLive,omitempty"`
	ClearBookOnStart             bool `json:"clearBookOnStart,omitempty"`
	ManualActivation             bool `json:"manualActivation,omitempty"`
	NegRisk                      bool `json:"negRisk,omitempty"`
	NegRiskOther                 bool `json:"negRiskOther,omitempty"`
	RequiresTranslation          bool `json:"requiresTranslation,omitempty"`
	PendingDeployment            bool `json:"pendingDeployment,omitempty"`
	Deploying                    bool `json:"deploying,omitempty"`
	RfqEnabled                   bool `json:"rfqEnabled,omitempty"`
	HoldingRewardsEnabled        bool `json:"holdingRewardsEnabled,omitempty"`
	FeesEnabled                  bool `json:"feesEnabled,omitempty"`
	PagerDutyNotificationEnabled bool `json:"pagerDutyNotificationEnabled,omitempty"`
	EnableOrderBook              bool `json:"enableOrderBook,omitempty"`
	AcceptingOrders              bool `json:"acceptingOrders,omitempty"`
	AutomaticallyResolved        bool `json:"automaticallyResolved,omitempty"`
	AutomaticallyActive          bool `json:"automaticallyActive,omitempty"`
	ShowGmpSeries                bool `json:"showGmpSeries,omitempty"`
	ShowGmpOutcome               bool `json:"showGmpOutcome,omitempty"`

	// 市场组信息
	MarketGroup        *int   `json:"marketGroup,omitempty"`
	GroupItemTitle     string `json:"groupItemTitle,omitempty"`
	GroupItemThreshold string `json:"groupItemThreshold,omitempty"`
	QuestionID         string `json:"questionID,omitempty"`

	// UMA 相关
	UmaEndDate            string   `json:"umaEndDate,omitempty"`
	UmaResolutionStatus   string   `json:"umaResolutionStatus,omitempty"`
	UmaResolutionStatuses []string `json:"umaResolutionStatuses,omitempty"`
	UmaBond               string   `json:"umaBond,omitempty"`
	UmaReward             string   `json:"umaReward,omitempty"`

	// 数值字段
	VolumeNum    *float64 `json:"volumeNum,omitempty"`
	LiquidityNum *float64 `json:"liquidityNum,omitempty"`
	LiquidityAmm *float64 `json:"liquidityAmm,omitempty"`
	Competitive  *float64 `json:"competitive,omitempty"`
	Spread       *float64 `json:"spread,omitempty"`

	// 日期 ISO 格式字符串
	EndDateIso    string `json:"endDateIso,omitempty"`
	StartDateIso  string `json:"startDateIso,omitempty"`
	UmaEndDateIso string `json:"umaEndDateIso,omitempty"`

	// 日期相关标志
	HasReviewedDates *bool `json:"hasReviewedDates,omitempty"`
	ReadyForCron     *bool `json:"readyForCron,omitempty"`

	// 交易量统计（24小时、1周、1月、1年）
	Volume24hr *float64 `json:"volume24hr,omitempty"`
	Volume1wk  *float64 `json:"volume1wk,omitempty"`
	Volume1mo  *float64 `json:"volume1mo,omitempty"`
	Volume1yr  *float64 `json:"volume1yr,omitempty"`

	// AMM 交易量统计
	Volume1wkAmm *float64 `json:"volume1wkAmm,omitempty"`
	Volume1moAmm *float64 `json:"volume1moAmm,omitempty"`
	Volume1yrAmm *float64 `json:"volume1yrAmm,omitempty"`

	// CLOB 交易量统计
	Volume1wkClob *float64 `json:"volume1wkClob,omitempty"`
	Volume1moClob *float64 `json:"volume1moClob,omitempty"`
	Volume1yrClob *float64 `json:"volume1yrClob,omitempty"`
	VolumeClob    *float64 `json:"volumeClob,omitempty"`

	// 价格变化
	OneHourPriceChange  *float64 `json:"oneHourPriceChange,omitempty"`
	OneDayPriceChange   *float64 `json:"oneDayPriceChange,omitempty"`
	OneWeekPriceChange  *float64 `json:"oneWeekPriceChange,omitempty"`
	OneMonthPriceChange *float64 `json:"oneMonthPriceChange,omitempty"`
	OneYearPriceChange  *float64 `json:"oneYearPriceChange,omitempty"`

	// 订单簿信息
	LastTradePrice *float64 `json:"lastTradePrice,omitempty"`
	BestBid        *float64 `json:"bestBid,omitempty"`
	BestAsk        *float64 `json:"bestAsk,omitempty"`

	// 奖励相关
	RewardsMinSize   *float64 `json:"rewardsMinSize,omitempty"`
	RewardsMaxSpread *float64 `json:"rewardsMaxSpread,omitempty"`

	// 订单簿配置
	OrderPriceMinTickSize *float64 `json:"orderPriceMinTickSize,omitempty"`
	OrderMinSize          *float64 `json:"orderMinSize,omitempty"`

	// 其他字段
	MarketMakerAddress       string  `json:"marketMakerAddress,omitempty"`
	MailchimpTag             string  `json:"mailchimpTag,omitempty"`
	TwitterCardLocation      string  `json:"twitterCardLocation,omitempty"`
	TwitterCardLastRefreshed string  `json:"twitterCardLastRefreshed,omitempty"`
	TwitterCardLastValidated string  `json:"twitterCardLastValidated,omitempty"`
	Creator                  string  `json:"creator,omitempty"`
	NegRiskRequestID         string  `json:"negRiskRequestID,omitempty"`
	CustomLiveness           *int    `json:"customLiveness,omitempty"`
	SeriesColor              string  `json:"seriesColor,omitempty"`
	Events                   []Event `json:"events,omitempty"`
}

// UnmarshalJSON 实现GammaMarket的自定义JSON反序列化
// 处理 outcomes、outcomePrices、clobTokenIds 和 umaResolutionStatuses 的字符串解析（API返回JSON字符串数组）
// 时间字段使用标准库自动解析 RFC3339 格式
func (m *GammaMarket) UnmarshalJSON(data []byte) error {
	// 先解析到 map 以便预处理字符串数组字段
	var rawData map[string]interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return err
	}

	// 预处理 outcomes（如果是 JSON 字符串）
	if val, ok := rawData["outcomes"]; ok && val != nil {
		if str, ok := val.(string); ok {
			parsed := parseOutcomes(str)
			if parsed != nil {
				rawData["outcomes"] = parsed
			}
		}
	}

	// 预处理 outcomePrices（如果是 JSON 字符串）
	if val, ok := rawData["outcomePrices"]; ok && val != nil {
		if str, ok := val.(string); ok {
			parsed := parseOutcomePrices(str)
			if parsed != nil {
				rawData["outcomePrices"] = parsed
			}
		}
	}

	// 预处理 clobTokenIds（如果是 JSON 字符串）
	if val, ok := rawData["clobTokenIds"]; ok && val != nil {
		if str, ok := val.(string); ok {
			parsed := parseTokenIDs(str)
			if parsed != nil {
				rawData["clobTokenIds"] = parsed
			}
		} else {
			// 如果不是字符串，也尝试解析（可能是数组或其他格式）
			parsed := parseTokenIDs(val)
			if parsed != nil {
				rawData["clobTokenIds"] = parsed
			}
		}
	}

	// 预处理 umaResolutionStatuses（如果是 JSON 字符串）
	if val, ok := rawData["umaResolutionStatuses"]; ok && val != nil {
		if str, ok := val.(string); ok {
			parsed := parseOutcomes(str) // 使用 parseOutcomes 解析字符串数组
			if parsed != nil {
				rawData["umaResolutionStatuses"] = parsed
			}
		} else {
			// 如果不是字符串，也尝试解析（可能是数组或其他格式）
			parsed := parseOutcomes(val)
			if parsed != nil {
				rawData["umaResolutionStatuses"] = parsed
			}
		}
	}

	// 处理 closedTime 字段（API可能返回非标准格式，如 "2025-12-02 23:49:11+00"）
	if val, ok := rawData["closedTime"]; ok && val != nil {
		if str, ok := val.(string); ok && str != "" {
			// 尝试解析多种时间格式
			timeFormats := []string{
				time.RFC3339,
				"2006-01-02 15:04:05-07",
				"2006-01-02 15:04:05+00",
				"2006-01-02T15:04:05Z",
				"2006-01-02T15:04:05-07:00",
			}
			var parsedTime *time.Time
			for _, format := range timeFormats {
				if t, err := time.Parse(format, str); err == nil {
					parsedTime = &t
					break
				}
			}
			if parsedTime != nil {
				rawData["closedTime"] = parsedTime.Format(time.RFC3339)
			} else {
				// 如果无法解析，移除该字段以避免错误
				delete(rawData, "closedTime")
			}
		}
	}

	// 重新序列化并反序列化到结构体
	processedData, err := json.Marshal(rawData)
	if err != nil {
		return err
	}

	// 使用临时结构体进行标准反序列化（时间字段会自动解析 RFC3339 格式）
	type Alias GammaMarket
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	return json.Unmarshal(processedData, aux)
}

// GetOutcomePrices 获取结果价格映射
func GetOutcomePrices(m *GammaMarket) map[string]float64 {
	if m == nil {
		return make(map[string]float64)
	}
	outcomePrices := make(map[string]float64)
	for i, tokenId := range m.TokenIDs {
		outcomePrices[tokenId] = m.OutcomePrices[i]
	}
	return outcomePrices
}

// GetOutcomeNames 获取结果名称映射
func GetOutcomeNames(m *GammaMarket) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	outcomeNames := make(map[string]string)
	for i, tokenId := range m.TokenIDs {
		outcomeNames[tokenId] = m.Outcomes[i]
	}
	return outcomeNames
}

// Tag 表示标签
type Tag struct {
	TagID               string     `json:"id"`
	Label               string     `json:"label"`
	Slug                string     `json:"slug"`
	ForceShow           bool       `json:"forceShow"`
	ForceHide           bool       `json:"forceHide"`
	IsCarousel          bool       `json:"isCarousel,omitempty"`
	PublishedAt         *time.Time `json:"publishedAt,omitempty"`
	UpdatedBy           *int       `json:"updatedBy,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	RequiresTranslation bool       `json:"requiresTranslation,omitempty"`
}

// UnmarshalJSON 实现Tag的自定义JSON反序列化
// 处理publishedAt字段的非标准时间格式
func (t *Tag) UnmarshalJSON(data []byte) error {
	// 先解析到 map 以便预处理时间字段
	var rawData map[string]interface{}
	if err := json.Unmarshal(data, &rawData); err != nil {
		return err
	}

	// 处理 publishedAt 字段（API可能返回非标准格式，如 "2023-10-25 18:55:50.674+00"）
	if val, ok := rawData["publishedAt"]; ok && val != nil {
		if str, ok := val.(string); ok && str != "" {
			// 尝试解析多种时间格式
			timeFormats := []string{
				time.RFC3339,
				"2006-01-02 15:04:05-07",
				"2006-01-02 15:04:05+00",
				"2006-01-02 15:04:05.000-07",
				"2006-01-02 15:04:05.000+00",
				"2006-01-02T15:04:05Z",
				"2006-01-02T15:04:05-07:00",
			}
			var parsedTime *time.Time
			for _, format := range timeFormats {
				if parsed, err := time.Parse(format, str); err == nil {
					parsedTime = &parsed
					break
				}
			}
			if parsedTime != nil {
				rawData["publishedAt"] = parsedTime.Format(time.RFC3339)
			} else {
				// 如果无法解析，移除该字段以避免错误
				delete(rawData, "publishedAt")
			}
		}
	}

	// 重新序列化并反序列化到结构体
	processedData, err := json.Marshal(rawData)
	if err != nil {
		return err
	}

	// 使用临时结构体进行标准反序列化（时间字段会自动解析 RFC3339 格式）
	type Alias Tag
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(t),
	}

	return json.Unmarshal(processedData, aux)
}

// TagRelation 表示标签关系
type TagRelation struct {
	TagID        int `json:"tag_id"`
	RelatedTagID int `json:"related_tag_id"`
	Count        int `json:"count"`
}

// SearchResult 表示搜索结果
type SearchResult struct {
	Events   []Event       `json:"events"`
	Markets  []GammaMarket `json:"markets"`
	Tags     []Tag         `json:"tags"`
	Profiles []interface{} `json:"profiles"` // Profile type not defined in Python version
}

// Series 表示系列
type Series struct {
	SeriesID    int       `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Recurrence  string    `json:"recurrence"` // hourly, daily, weekly, monthly, annual
	Closed      bool      `json:"closed"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UnmarshalJSON 实现Series的自定义JSON反序列化，处理id可能是字符串或数字的情况
func (s *Series) UnmarshalJSON(data []byte) error {
	// 使用临时结构体来解析JSON
	var temp struct {
		ID          interface{} `json:"id"` // 可能是字符串或数字
		Slug        string       `json:"slug"`
		Title       string       `json:"title"`
		Description string       `json:"description"`
		Recurrence  string       `json:"recurrence"`
		Closed      bool         `json:"closed"`
		CreatedAt   time.Time    `json:"created_at"`
		UpdatedAt   time.Time    `json:"updated_at"`
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// 复制字段
	s.Slug = temp.Slug
	s.Title = temp.Title
	s.Description = temp.Description
	s.Recurrence = temp.Recurrence
	s.Closed = temp.Closed
	s.CreatedAt = temp.CreatedAt
	s.UpdatedAt = temp.UpdatedAt

	// 处理id（可能是字符串或数字）
	switch v := temp.ID.(type) {
	case float64:
		s.SeriesID = int(v)
	case int:
		s.SeriesID = v
	case int64:
		s.SeriesID = int(v)
	case string:
		// 尝试解析为整数
		if parsed, err := strconv.Atoi(v); err == nil {
			s.SeriesID = parsed
		} else {
			return fmt.Errorf("failed to parse series id as int: %w", err)
		}
	default:
		return fmt.Errorf("unexpected series id type: %T", v)
	}

	return nil
}

// Team 表示体育团队
type Team struct {
	TeamID       int       `json:"id"`
	Name         string    `json:"name"`
	League       string    `json:"league"`
	Abbreviation string    `json:"abbreviation"`
	Logo         string    `json:"logo"`
	Record       string    `json:"record"`
	Alias        string    `json:"alias"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Sport 表示体育元数据
type Sport struct {
	SportID int      `json:"id"`
	Name    string   `json:"name"`
	Leagues []string `json:"leagues"`
}

// SportsEvent 表示体育事件
type SportsEvent struct {
	EventID   int       `json:"event_id"`
	SportID   int       `json:"sport_id"`
	League    string    `json:"league"`
	HomeTeam  string    `json:"home_team"`
	AwayTeam  string    `json:"away_team"`
	StartTime time.Time `json:"start_time"`
	Status    string    `json:"status"`
	Score     string    `json:"score,omitempty"`
	Markets   []string  `json:"markets,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EncryptedPriceUpdate 表示加密的价格更新
type EncryptedPriceUpdate struct {
	TokenID   string    `json:"token_id"`
	Price     string    `json:"price"` // Encrypted price data
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data,omitempty"` // Additional encrypted data
}

// Comment 表示评论
type Comment struct {
	CommentID        string     `json:"id"`
	ParentEntityType string     `json:"parent_entity_type"` // Event, Series, market
	ParentEntityID   int        `json:"parent_entity_id"`
	UserAddress      EthAddress `json:"user_address"`
	Content          string     `json:"content"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Profile 表示用户资料
type Profile struct {
	Address   EthAddress `json:"address"`
	Username  string     `json:"username,omitempty"`
	Bio       string     `json:"bio,omitempty"`
	Avatar    string     `json:"avatar,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// SimplifiedMarket 表示简化市场（只包含基本字段）
type SimplifiedMarket struct {
	MarketID    string    `json:"id"`
	Slug        string    `json:"slug"`
	Question    string    `json:"question"`
	Image       string    `json:"image"`
	Active      bool      `json:"active"`
	Closed      bool      `json:"closed"`
	ConditionID Keccak256 `json:"conditionId"`
	TokenIDs    []string  `json:"clobTokenIds"`
	Outcomes    []string  `json:"outcomes,omitempty"`
}

// MarketTradesEvent 表示市场交易事件
type MarketTradesEvent struct {
	EventID     string      `json:"event_id"`
	Type        string      `json:"type"` // TRADE, ORDER_PLACED, ORDER_CANCELLED, etc.
	MarketID    string      `json:"market_id"`
	TokenID     string      `json:"token_id,omitempty"`
	Price       *float64    `json:"price,omitempty"`
	Size        *float64    `json:"size,omitempty"`
	Side        *string     `json:"side,omitempty"` // BUY or SELL
	Timestamp   time.Time   `json:"timestamp"`
	UserAddress *EthAddress `json:"user_address,omitempty"`
}

// HasDispute 检查市场是否有dispute状态
// HasDispute 检查市场是否有争议
func HasDispute(m *GammaMarket) bool {
	if m == nil {
		return false
	}

	// 检查 umaResolutionStatus 字段
	if m.UmaResolutionStatus != "" {
		if strings.Contains(strings.ToLower(m.UmaResolutionStatus), "dispute") {
			return true
		}
	}

	// 检查 umaResolutionStatuses 数组
	if len(m.UmaResolutionStatuses) > 0 {
		for _, status := range m.UmaResolutionStatuses {
			if strings.Contains(strings.ToLower(status), "dispute") {
				return true
			}
		}
	}

	return false
}
