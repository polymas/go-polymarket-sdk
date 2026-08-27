package rfq

import (
	"encoding/json"
	"fmt"
	"strings"

	sdkhttp "github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

type Client struct {
	baseURL string
	config  Config
}

func NewClient(config Config) *Client { return newClient(RESTURL, config) }

func newClient(baseURL string, config Config) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), config: config}
}

func (c *Client) GetComboMarkets(limit int, cursor string, exclude []string) (*ComboMarketsResponse, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit must be between 1 and 100")
	}
	params := map[string]string{"limit": fmt.Sprint(limit)}
	if cursor != "" {
		params["cursor"] = cursor
	}
	if len(exclude) > 0 {
		params["exclude"] = strings.Join(exclude, ",")
	}
	return sdkhttp.Get[ComboMarketsResponse](c.baseURL, "/v1/rfq/combo-markets", params)
}

func (c *Client) SubmitQuote(quote Quote) (*RFQSnapshot, error) {
	quote.SignerAddress, quote.MakerAddress, quote.SignatureType = c.identity()
	return postAuthenticated[RFQSnapshot](c, "/v1/maker/quotes", quote)
}

func (c *Client) CancelQuote(request CancelQuoteRequest) (*RFQSnapshot, error) {
	request.SignerAddress, request.MakerAddress, request.SignatureType = c.identity()
	return postAuthenticated[RFQSnapshot](c, "/v1/maker/quotes/cancel", request)
}

func (c *Client) Confirm(request MakerConfirmationRequest) (*MakerConfirmationResult, error) {
	request.SignerAddress, request.MakerAddress, request.SignatureType = c.identity()
	return postAuthenticated[MakerConfirmationResult](c, "/v1/maker/confirmations", request)
}

func (c *Client) identity() (string, string, types.SignatureType) {
	i := c.config.Identity
	return i.SignerAddress, i.MakerAddress, i.SignatureType
}

func postAuthenticated[T any](c *Client, path string, body any) (*T, error) {
	if c.config.Signer == nil || c.config.Credentials == nil {
		return nil, fmt.Errorf("RFQ maker endpoint requires signer and CLOB API credentials")
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	requestBody := types.RequestBody(bodyJSON)
	headers, err := internal.CreateLevel2Headers(c.config.Signer, c.config.Credentials, &types.RequestArgs{
		Method: "POST", RequestPath: path, Body: &requestBody,
	})
	if err != nil {
		return nil, err
	}
	raw, err := sdkhttp.PostRaw(c.baseURL, path, bodyJSON, sdkhttp.WithHeaders(headers))
	if err != nil {
		return nil, err
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode RFQ response: %w", err)
	}
	return &result, nil
}
