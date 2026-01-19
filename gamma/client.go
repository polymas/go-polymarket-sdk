package gamma

import (
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// Client 定义Gamma客户端的接口，供外部包使用
type Client interface {
	// 市场相关方法
	GetMarket(marketID string) (*types.GammaMarket, error)
	GetMarketBySlug(slug string, includeTag *bool) (*types.GammaMarket, error)
	GetMarketsByConditionIDs(conditionIDs []string) ([]types.GammaMarket, error)
	GetMarkets(limit int, options ...GetMarketsOption) ([]types.GammaMarket, error) // 获取市场列表（支持分页和过滤）
	GetCertaintyMarkets() ([]types.GammaMarket, error)                              // 获取 Certainty 市场（尾盘市场）
	GetDisputeMarkets() ([]types.GammaMarket, error)                                // 获取争议市场（在 Certainty 市场基础上过滤）
	GetAllMarkets() ([]types.GammaMarket, error)                                    // 获取所有历史市场数据（自动分页）

	// 事件相关方法
	GetEvent(eventID int, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEventBySlug(slug string, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEvents(limit int, offset int, options ...GetEventsOption) ([]types.Event, error)

	// 搜索相关方法
	Search(query string, options ...SearchOption) (*types.SearchResult, error)
	// 标签相关方法
	GetTags(limit int, offset int, options ...GetTagsOption) ([]types.Tag, error)
	GetTag(tagID int) (*types.Tag, error)
	GetTagBySlug(slug string) (*types.Tag, error)
	// 系列相关方法
	GetSeries(limit int, offset int, options ...GetSeriesOption) ([]types.Series, error)
	GetSeriesBySlug(slug string) (*types.Series, error)
	// 评论相关方法
	GetComments(marketID string, limit int, offset int) ([]types.Comment, error)
	GetComment(commentID string) (*types.Comment, error)
	// 用户资料相关方法
	GetProfile(address types.EthAddress) (*types.Profile, error)
	GetProfileByUsername(username string) (*types.Profile, error)
	// 市场扩展方法
	GetSamplingSimplifiedMarkets(limit int) ([]types.SimplifiedMarket, error)
	GetSamplingMarkets(limit int) ([]types.GammaMarket, error)
	GetSimplifiedMarkets(limit int, offset int, options ...GetMarketsOption) ([]types.SimplifiedMarket, error)
	GetMarketTradesEvents(marketID string, limit int, offset int) ([]types.MarketTradesEvent, error)
}

// polymarketGammaClient 处理Gamma API操作
// 不允许直接导出，只能通过 NewPolymarketGammaClient 创建
type polymarketGammaClient struct {
	baseURL string // API 基础 URL
}

// NewClient 创建新的Gamma客户端
// 返回 Client 接口，不允许直接访问实现类型
func NewClient() Client {
	return &polymarketGammaClient{
		baseURL: internal.GammaAPIDomain,
	}
}
