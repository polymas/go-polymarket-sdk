package rfq

import (
	"fmt"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// Client 定义 RFQ 客户端的接口
type Client interface {
	RequestQuote(request types.RFQRequest) (*types.RFQResponse, error)
	GetQuotes(requestID string) ([]types.RFQQuote, error)
	AcceptQuote(quoteID string) (*types.RFQAcceptResponse, error)
	CancelRequest(requestID string) error
}

// rfqClient 处理 RFQ 操作
type rfqClient struct {
	baseURL string
}

// NewClient 创建新的 RFQ 客户端
func NewClient() Client {
	return &rfqClient{
		baseURL: internal.ClobAPIDomain,
	}
}

// RequestQuote 请求报价
func (r *rfqClient) RequestQuote(request types.RFQRequest) (*types.RFQResponse, error) {
	return http.Post[types.RFQResponse](r.baseURL, "/rfq/request", request)
}

// GetQuotes 获取报价列表
func (r *rfqClient) GetQuotes(requestID string) ([]types.RFQQuote, error) {
	params := map[string]string{
		"request_id": requestID,
	}

	result, err := http.Get[[]types.RFQQuote](r.baseURL, "/rfq/quotes", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get quotes: %w", err)
	}

	if result == nil {
		return []types.RFQQuote{}, nil
	}

	return *result, nil
}

// AcceptQuote 接受报价
func (r *rfqClient) AcceptQuote(quoteID string) (*types.RFQAcceptResponse, error) {
	requestBody := map[string]string{
		"quote_id": quoteID,
	}

	return http.Post[types.RFQAcceptResponse](r.baseURL, "/rfq/accept", requestBody)
}

// CancelRequest 取消请求
func (r *rfqClient) CancelRequest(requestID string) error {
	requestBody := map[string]string{
		"request_id": requestID,
	}

	_, err := http.Post[map[string]interface{}](r.baseURL, "/rfq/cancel", requestBody)
	return err
}
