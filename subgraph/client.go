package subgraph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	subgraphURL = "https://api.thegraph.com/subgraphs/name/polymarket/clob-v2"
)

// Client 定义 Subgraph 客户端的接口
type Client interface {
	Query(query string, variables map[string]interface{}) (*types.GraphQLResponse, error)
	GetMarketVolume(marketID string, startTime, endTime int64) (*types.MarketVolume, error)
	GetUserPositions(userAddress types.EthAddress) ([]types.GQLPosition, error)
	GetMarketOpenInterest(marketID string) (*types.MarketOpenInterest, error)
	GetUserPNL(userAddress types.EthAddress) (*types.UserPNL, error)
}

// subgraphClient 处理 GraphQL 查询
type subgraphClient struct {
	url        string
	httpClient *http.Client
}

// NewClient 创建新的 Subgraph 客户端
func NewClient() Client {
	return &subgraphClient{
		url: subgraphURL,
		httpClient: &http.Client{
			Timeout: internal.HTTPClientTimeout,
		},
	}
}

// GraphQLRequest 表示 GraphQL 请求
type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// Query 执行 GraphQL 查询
func (s *subgraphClient) Query(query string, variables map[string]interface{}) (*types.GraphQLResponse, error) {
	req := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", s.url, bytes.NewBuffer(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var graphQLResp types.GraphQLResponse
	if err := json.Unmarshal(body, &graphQLResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(graphQLResp.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %v", graphQLResp.Errors)
	}

	return &graphQLResp, nil
}

// GetMarketVolume 获取市场交易量
func (s *subgraphClient) GetMarketVolume(marketID string, startTime, endTime int64) (*types.MarketVolume, error) {
	query := `
		query GetMarketVolume($marketId: String!, $startTime: BigInt!, $endTime: BigInt!) {
			marketVolume(id: $marketId, startTime: $startTime, endTime: $endTime) {
				marketId
				volume
				tradeCount
				startTime
				endTime
			}
		}
	`

	variables := map[string]interface{}{
		"marketId":  marketID,
		"startTime": startTime,
		"endTime":   endTime,
	}

	resp, err := s.Query(query, variables)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			MarketVolume *types.MarketVolume `json:"marketVolume"`
		} `json:"data"`
	}

	resultJSON, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, err
	}

	return result.Data.MarketVolume, nil
}

// GetUserPositions 获取用户仓位
func (s *subgraphClient) GetUserPositions(userAddress types.EthAddress) ([]types.GQLPosition, error) {
	query := `
		query GetUserPositions($user: String!) {
			positions(where: { user: $user }) {
				user
				asset {
					id
					condition {
						id
					}
					complement
					outcomeIndex
				}
				balance
			}
		}
	`

	variables := map[string]interface{}{
		"user": string(userAddress),
	}

	resp, err := s.Query(query, variables)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Positions []types.GQLPosition `json:"positions"`
		} `json:"data"`
	}

	resultJSON, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, err
	}

	return result.Data.Positions, nil
}

// GetMarketOpenInterest 获取市场未平仓量
func (s *subgraphClient) GetMarketOpenInterest(marketID string) (*types.MarketOpenInterest, error) {
	query := `
		query GetMarketOpenInterest($marketId: String!) {
			marketOpenInterest(id: $marketId) {
				marketId
				totalOpenInterest
				timestamp
			}
		}
	`

	variables := map[string]interface{}{
		"marketId": marketID,
	}

	resp, err := s.Query(query, variables)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			MarketOpenInterest *types.MarketOpenInterest `json:"marketOpenInterest"`
		} `json:"data"`
	}

	resultJSON, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, err
	}

	return result.Data.MarketOpenInterest, nil
}

// GetUserPNL 获取用户盈亏
func (s *subgraphClient) GetUserPNL(userAddress types.EthAddress) (*types.UserPNL, error) {
	query := `
		query GetUserPNL($user: String!) {
			userPNL(id: $user) {
				user
				totalPNL
				realizedPNL
				unrealizedPNL
				timestamp
			}
		}
	`

	variables := map[string]interface{}{
		"user": string(userAddress),
	}

	resp, err := s.Query(query, variables)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			UserPNL *types.UserPNL `json:"userPNL"`
		} `json:"data"`
	}

	resultJSON, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return nil, err
	}

	return result.Data.UserPNL, nil
}
