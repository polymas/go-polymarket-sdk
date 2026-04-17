package gamma

import (
	"fmt"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetEventURLByMarketID 通过 marketID 获取该市场对应的事件页 URL
// URL 形式:
//   - 多市场事件: {WebDomain}/event/{event_slug}/{market_slug}
//   - 单市场事件（event_slug == market_slug）: {WebDomain}/event/{event_slug}
func (c *polymarketGammaClient) GetEventURLByMarketID(marketID string) (string, error) {
	if marketID == "" {
		return "", fmt.Errorf("marketID is empty")
	}
	market, err := c.findMarketByID(marketID)
	if err != nil {
		return "", fmt.Errorf("get market by id %s: %w", marketID, err)
	}
	if market == nil {
		return "", fmt.Errorf("market not found: %s", marketID)
	}
	return c.buildEventURL(market)
}

// findMarketByID 通过 List markets 端点按 id 过滤，返回包含 events 的完整响应
// /markets/{id} 端点不返回 events 字段，因此改走 list 端点
func (c *polymarketGammaClient) findMarketByID(marketID string) (*types.GammaMarket, error) {
	id, err := strconv.Atoi(marketID)
	if err != nil {
		return nil, fmt.Errorf("invalid marketID %q: %w", marketID, err)
	}
	markets, err := c.getMarkets(1, WithMarketIDs([]int{id}))
	if err != nil {
		return nil, err
	}
	if len(markets) > 0 {
		return &markets[0], nil
	}
	markets, err = c.getMarkets(1, WithMarketIDs([]int{id}), WithClosed(true))
	if err != nil {
		return nil, err
	}
	if len(markets) > 0 {
		return &markets[0], nil
	}
	return nil, nil
}

// GetEventURLByConditionID 通过 conditionID 获取事件页 URL
// 先按活跃市场查询，未命中则回退为包含 closed 的查询，以覆盖已结算市场
func (c *polymarketGammaClient) GetEventURLByConditionID(conditionID string) (string, error) {
	if conditionID == "" {
		return "", fmt.Errorf("conditionID is empty")
	}
	market, err := c.findMarketByConditionID(conditionID)
	if err != nil {
		return "", fmt.Errorf("get markets by condition id %s: %w", conditionID, err)
	}
	if market == nil {
		return "", fmt.Errorf("market not found for conditionID: %s", conditionID)
	}
	return c.buildEventURL(market)
}

// GetEventURLByTokenID 通过 clob tokenID 获取事件页 URL
// 先按活跃市场查询，未命中则回退为包含 closed 的查询
func (c *polymarketGammaClient) GetEventURLByTokenID(tokenID string) (string, error) {
	if tokenID == "" {
		return "", fmt.Errorf("tokenID is empty")
	}
	market, err := c.findMarketByTokenID(tokenID)
	if err != nil {
		return "", fmt.Errorf("get markets by token id %s: %w", tokenID, err)
	}
	if market == nil {
		return "", fmt.Errorf("market not found for tokenID: %s", tokenID)
	}
	return c.buildEventURL(market)
}

// findMarketByConditionID 查询 market，覆盖活跃和已关闭市场
func (c *polymarketGammaClient) findMarketByConditionID(conditionID string) (*types.GammaMarket, error) {
	markets, err := c.GetMarketsByConditionIDs([]string{conditionID})
	if err != nil {
		return nil, err
	}
	if len(markets) > 0 {
		return &markets[0], nil
	}
	markets, err = c.getMarkets(1, WithConditionIDs([]string{conditionID}), WithClosed(true))
	if err != nil {
		return nil, err
	}
	if len(markets) > 0 {
		return &markets[0], nil
	}
	return nil, nil
}

// findMarketByTokenID 查询 market，覆盖活跃和已关闭市场
func (c *polymarketGammaClient) findMarketByTokenID(tokenID string) (*types.GammaMarket, error) {
	markets, err := c.GetMarkets(1, WithTokenIDs([]string{tokenID}))
	if err != nil {
		return nil, err
	}
	if len(markets) > 0 {
		return &markets[0], nil
	}
	markets, err = c.getMarkets(1, WithTokenIDs([]string{tokenID}), WithClosed(true))
	if err != nil {
		return nil, err
	}
	if len(markets) > 0 {
		return &markets[0], nil
	}
	return nil, nil
}

// buildEventURL 根据 market 信息拼接事件页 URL。
// 优先使用 market.Events[0].Slug（单市场端点已返回），否则通过 market.EventID 调用 GetEvent
func (c *polymarketGammaClient) buildEventURL(market *types.GammaMarket) (string, error) {
	if market.Slug == "" {
		return "", fmt.Errorf("market %s has empty slug", market.MarketID)
	}

	eventSlug, err := c.resolveEventSlug(market)
	if err != nil {
		return "", err
	}
	if eventSlug == "" {
		// 没有事件关联：回退为单段 slug
		return fmt.Sprintf("%s/event/%s", internal.PolymarketWebDomain, market.Slug), nil
	}
	if eventSlug == market.Slug {
		return fmt.Sprintf("%s/event/%s", internal.PolymarketWebDomain, eventSlug), nil
	}
	return fmt.Sprintf("%s/event/%s/%s", internal.PolymarketWebDomain, eventSlug, market.Slug), nil
}

// resolveEventSlug 解析事件 slug。优先用内嵌 Events，退化到 GetEvent(EventID)
func (c *polymarketGammaClient) resolveEventSlug(market *types.GammaMarket) (string, error) {
	for _, e := range market.Events {
		if e.Slug != "" {
			return e.Slug, nil
		}
	}
	if market.EventID == "" {
		return "", nil
	}
	eventID, err := strconv.Atoi(market.EventID)
	if err != nil {
		return "", fmt.Errorf("invalid eventID %q on market %s: %w", market.EventID, market.MarketID, err)
	}
	event, err := c.GetEvent(eventID, nil, nil)
	if err != nil {
		return "", fmt.Errorf("get event %d: %w", eventID, err)
	}
	if event == nil {
		return "", nil
	}
	return event.Slug, nil
}
