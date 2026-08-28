package gamma

import (
	"context"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// Client 定义Gamma客户端的接口，供外部包使用
type Client interface {
	GetStatus() (string, error)
	GetStatusContext(ctx context.Context) (string, error)
	GetTeams(limit, offset int, options ...ListTeamsOption) ([]types.Team, error)
	GetTeamsContext(ctx context.Context, limit, offset int, options ...ListTeamsOption) ([]types.Team, error)
	GetTeam(id int) (*types.Team, error)
	GetTeamContext(ctx context.Context, id int) (*types.Team, error)
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
	GetMarketDescription(id int) (*types.MarketDescription, error)
	GetMarketDescriptionContext(ctx context.Context, id int) (*types.MarketDescription, error)
	GetMarketTags(id int) ([]types.Tag, error)
	GetMarketTagsContext(ctx context.Context, id int) ([]types.Tag, error)
	GetMarketsInformation(request MarketsInformationRequest) ([]types.GammaMarket, error)
	GetMarketsInformationContext(ctx context.Context, request MarketsInformationRequest) ([]types.GammaMarket, error)
	GetAbridgedMarkets(request MarketsInformationRequest) ([]types.GammaMarket, error)
	GetAbridgedMarketsContext(ctx context.Context, request MarketsInformationRequest) ([]types.GammaMarket, error)
	GetMarketsPage(limit int, options ...GetMarketsOption) (*types.MarketsPage, error)
	GetMarketsPageContext(ctx context.Context, limit int, options ...GetMarketsOption) (*types.MarketsPage, error)

	// 事件相关方法
	GetEvent(eventID int, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEventContext(ctx context.Context, eventID int, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEventBySlug(slug string, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEventBySlugContext(ctx context.Context, slug string, includeChat *bool, includeTemplate *bool) (*types.Event, error)
	GetEvents(limit int, offset int, options ...GetEventsOption) ([]types.Event, error)
	GetEventsContext(ctx context.Context, limit int, offset int, options ...GetEventsOption) ([]types.Event, error)
	GetEventsPagination(limit, offset int, options ...EventsPaginationOption) (*types.EventsPagination, error)
	GetEventsPaginationContext(ctx context.Context, limit, offset int, options ...EventsPaginationOption) (*types.EventsPagination, error)
	GetSportsEventResults(limit, offset int, order string, ascending *bool) ([]types.Event, error)
	GetSportsEventResultsContext(ctx context.Context, limit, offset int, order string, ascending *bool) ([]types.Event, error)
	GetEventTweetCount(id int) (*types.EventTweetCount, error)
	GetEventTweetCountContext(ctx context.Context, id int) (*types.EventTweetCount, error)
	GetEventCommentsCount(id int) (*types.Count, error)
	GetEventCommentsCountContext(ctx context.Context, id int) (*types.Count, error)
	GetEventTags(id int) ([]types.Tag, error)
	GetEventTagsContext(ctx context.Context, id int) ([]types.Tag, error)
	GetEventCreators(limit, offset int, options ...EventCreatorsOption) ([]types.EventCreator, error)
	GetEventCreatorsContext(ctx context.Context, limit, offset int, options ...EventCreatorsOption) ([]types.EventCreator, error)
	GetEventCreator(id int) (*types.EventCreator, error)
	GetEventCreatorContext(ctx context.Context, id int) (*types.EventCreator, error)
	GetEventsPage(limit int, options EventsKeysetOptions) (*types.EventsPage, error)
	GetEventsPageContext(ctx context.Context, limit int, options EventsKeysetOptions) (*types.EventsPage, error)

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
	GetTagWithTemplate(tagID int, includeTemplate bool) (*types.Tag, error)
	GetTagWithTemplateContext(ctx context.Context, tagID int, includeTemplate bool) (*types.Tag, error)
	GetTagBySlugWithTemplate(slug string, includeTemplate bool) (*types.Tag, error)
	GetTagBySlugWithTemplateContext(ctx context.Context, slug string, includeTemplate bool) (*types.Tag, error)
	GetRelatedTagsByID(id int, options ...RelatedTagsOption) ([]types.RelatedTag, error)
	GetRelatedTagsByIDContext(ctx context.Context, id int, options ...RelatedTagsOption) ([]types.RelatedTag, error)
	GetRelatedTagsBySlug(slug string, options ...RelatedTagsOption) ([]types.RelatedTag, error)
	GetRelatedTagsBySlugContext(ctx context.Context, slug string, options ...RelatedTagsOption) ([]types.RelatedTag, error)
	GetTagsRelatedToTagByID(id int, options ...RelatedTagsOption) ([]types.Tag, error)
	GetTagsRelatedToTagByIDContext(ctx context.Context, id int, options ...RelatedTagsOption) ([]types.Tag, error)
	GetTagsRelatedToTagBySlug(slug string, options ...RelatedTagsOption) ([]types.Tag, error)
	GetTagsRelatedToTagBySlugContext(ctx context.Context, slug string, options ...RelatedTagsOption) ([]types.Tag, error)
	// 系列相关方法
	GetSeries(limit int, offset int, options ...GetSeriesOption) ([]types.Series, error)
	GetSeriesContext(ctx context.Context, limit int, offset int, options ...GetSeriesOption) ([]types.Series, error)
	GetSeriesByID(id int, includeChat *bool) (*types.Series, error)
	GetSeriesByIDContext(ctx context.Context, id int, includeChat *bool) (*types.Series, error)
	GetSeriesCommentsCount(id int) (*types.Count, error)
	GetSeriesCommentsCountContext(ctx context.Context, id int) (*types.Count, error)
	GetSeriesSummaryByID(id string) (*types.SeriesSummary, error)
	GetSeriesSummaryByIDContext(ctx context.Context, id string) (*types.SeriesSummary, error)
	GetSeriesSummaryBySlug(slug string) (*types.SeriesSummary, error)
	GetSeriesSummaryBySlugContext(ctx context.Context, slug string) (*types.SeriesSummary, error)
	// 评论相关方法
	GetComments(parentType types.CommentParentEntityType, parentID int, limit int, offset int, options ...GetCommentsOption) ([]types.Comment, error)
	GetCommentsContext(ctx context.Context, parentType types.CommentParentEntityType, parentID int, limit int, offset int, options ...GetCommentsOption) ([]types.Comment, error)
	GetComment(commentID int, getPositions bool) ([]types.Comment, error)
	GetCommentContext(ctx context.Context, commentID int, getPositions bool) ([]types.Comment, error)
	GetCommentsByUserAddress(user types.EthAddress, limit, offset int, options ...GetCommentsOption) ([]types.Comment, error)
	GetCommentsByUserAddressContext(ctx context.Context, user types.EthAddress, limit, offset int, options ...GetCommentsOption) ([]types.Comment, error)
	// 用户资料相关方法
	GetProfile(address types.EthAddress) (*types.Profile, error)
	GetProfileContext(ctx context.Context, address types.EthAddress) (*types.Profile, error)
	GetProfileByUserAddress(address types.EthAddress) (*types.GammaProfile, error)
	GetProfileByUserAddressContext(ctx context.Context, address types.EthAddress) (*types.GammaProfile, error)
	// 市场扩展方法
	GetSamplingSimplifiedMarkets(limit int) ([]types.SimplifiedMarket, error)
	GetSamplingSimplifiedMarketsContext(ctx context.Context, limit int) ([]types.SimplifiedMarket, error)
	GetSamplingMarkets(limit int) ([]types.GammaMarket, error)
	GetSamplingMarketsContext(ctx context.Context, limit int) ([]types.GammaMarket, error)
	GetSimplifiedMarkets(limit int, offset int, options ...GetMarketsOption) ([]types.SimplifiedMarket, error)
	GetSimplifiedMarketsContext(ctx context.Context, limit int, offset int, options ...GetMarketsOption) ([]types.SimplifiedMarket, error)
	GetSportsMetadata() ([]types.SportsMetadata, error)
	GetSportsMetadataContext(ctx context.Context) ([]types.SportsMetadata, error)
	GetSportsMarketTypes() (*types.SportsMarketTypesResponse, error)
	GetSportsMarketTypesContext(ctx context.Context) (*types.SportsMarketTypesResponse, error)

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
