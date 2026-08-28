package gamma

import (
	"context"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// Client 定义Gamma客户端的接口，供外部包使用
type Client interface {
	// 市场相关方法
	GetMarket(marketID string) (*types.GammaMarket, error)
	GetMarketContext(ctx context.Context, marketID string) (*types.GammaMarket, error)
	GetMarketBySlug(slug string) (*types.GammaMarket, error)
	GetMarketBySlugContext(ctx context.Context, slug string) (*types.GammaMarket, error)
	GetMarketsByConditionIDs(conditionIDs []string) ([]types.GammaMarket, error)
	GetMarketsByConditionIDsContext(ctx context.Context, conditionIDs []string) ([]types.GammaMarket, error)
	GetMarkets(limit int, options ...GetMarketsOption) ([]types.GammaMarket, error) // 获取市场列表（支持分页和过滤）
	GetMarketsContext(ctx context.Context, limit int, options ...GetMarketsOption) ([]types.GammaMarket, error)
	GetCertaintyMarkets() ([]types.GammaMarket, error) // 获取 Certainty 市场（尾盘市场）
	GetCertaintyMarketsContext(ctx context.Context) ([]types.GammaMarket, error)
	GetDisputeMarkets() ([]types.GammaMarket, error) // 获取争议市场（在 Certainty 市场基础上过滤）
	GetDisputeMarketsContext(ctx context.Context) ([]types.GammaMarket, error)
	GetAllMarkets() ([]types.GammaMarket, error) // 获取所有历史市场数据（自动分页）
	GetAllMarketsContext(ctx context.Context) ([]types.GammaMarket, error)

	// 事件相关方法
	GetEvent(eventID int, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEventContext(ctx context.Context, eventID int, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEventBySlug(slug string, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEventBySlugContext(ctx context.Context, slug string, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEvents(limit int, offset int, options ...GetEventsOption) ([]types.Event, error)
	GetEventsContext(ctx context.Context, limit int, offset int, options ...GetEventsOption) ([]types.Event, error)

	// 搜索相关方法
	Search(query string, options ...SearchOption) (*types.SearchResult, error)
	SearchContext(ctx context.Context, query string, options ...SearchOption) (*types.SearchResult, error)
	// 标签相关方法
	GetTags(limit int, offset int, options ...GetTagsOption) ([]types.Tag, error)
	GetTagsContext(ctx context.Context, limit int, offset int, options ...GetTagsOption) ([]types.Tag, error)
	GetTag(tagID int) (*types.Tag, error)
	GetTagContext(ctx context.Context, tagID int) (*types.Tag, error)
	GetTagBySlug(slug string) (*types.Tag, error)
	GetTagBySlugContext(ctx context.Context, slug string) (*types.Tag, error)
	// 系列相关方法
	GetSeries(limit int, offset int, options ...GetSeriesOption) ([]types.Series, error)
	GetSeriesContext(ctx context.Context, limit int, offset int, options ...GetSeriesOption) ([]types.Series, error)
	GetSeriesSummaryByID(id string) (*types.SeriesSummary, error)
	GetSeriesSummaryByIDContext(ctx context.Context, id string) (*types.SeriesSummary, error)
	GetSeriesSummaryBySlug(slug string) (*types.SeriesSummary, error)
	GetSeriesSummaryBySlugContext(ctx context.Context, slug string) (*types.SeriesSummary, error)
	// 评论相关方法
	GetComments(parentType types.CommentParentEntityType, parentID int, limit int, offset int, options ...GetCommentsOption) ([]types.Comment, error)
	GetCommentsContext(ctx context.Context, parentType types.CommentParentEntityType, parentID int, limit int, offset int, options ...GetCommentsOption) ([]types.Comment, error)
	GetComment(commentID int, getPositions bool) ([]types.Comment, error)
	GetCommentContext(ctx context.Context, commentID int, getPositions bool) ([]types.Comment, error)
	// 用户资料相关方法
	GetProfile(address types.EthAddress) (*types.Profile, error)
	GetProfileContext(ctx context.Context, address types.EthAddress) (*types.Profile, error)
	// 市场扩展方法
	GetSamplingSimplifiedMarkets(limit int) ([]types.SimplifiedMarket, error)
	GetSamplingSimplifiedMarketsContext(ctx context.Context, limit int) ([]types.SimplifiedMarket, error)
	GetSamplingMarkets(limit int) ([]types.GammaMarket, error)
	GetSamplingMarketsContext(ctx context.Context, limit int) ([]types.GammaMarket, error)
	GetSimplifiedMarkets(limit int, offset int, options ...GetMarketsOption) ([]types.SimplifiedMarket, error)
	GetSimplifiedMarketsContext(ctx context.Context, limit int, offset int, options ...GetMarketsOption) ([]types.SimplifiedMarket, error)

	// URL 构建方法：根据市场标识获取 Polymarket 前端事件页 URL
	// URL 形式: https://polymarket.com/event/{event_slug}/{market_slug}
	// 单市场事件（event_slug == market_slug）简化为: https://polymarket.com/event/{event_slug}
	GetEventURLByMarketID(marketID string) (string, error)
	GetEventURLByMarketIDContext(ctx context.Context, marketID string) (string, error)
	GetEventURLByConditionID(conditionID string) (string, error)
	GetEventURLByConditionIDContext(ctx context.Context, conditionID string) (string, error)
	GetEventURLByTokenID(tokenID string) (string, error)
	GetEventURLByTokenIDContext(ctx context.Context, tokenID string) (string, error)
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
