package rfq

import (
	"context"
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
	return c.GetComboMarketsContext(context.Background(), limit, cursor, exclude)
}

func (c *Client) GetComboMarketsContext(ctx context.Context, limit int, cursor string, exclude []string) (*ComboMarketsResponse, error) {
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
	return sdkhttp.GetContext[ComboMarketsResponse](ctx, c.baseURL, "/v1/rfq/combo-markets", params, sdkhttp.WithService("rfq"))
}

func (c *Client) SubmitQuote(quote Quote) (*RFQSnapshot, error) {
	return c.SubmitQuoteContext(context.Background(), quote)
}

func (c *Client) SubmitQuoteContext(ctx context.Context, quote Quote) (*RFQSnapshot, error) {
	quote.SignerAddress, quote.MakerAddress, quote.SignatureType = c.identity()
	return postAuthenticatedContext[RFQSnapshot](ctx, c, "/v1/maker/quotes", quote, "submit RFQ quote")
}

func (c *Client) CancelQuote(request CancelQuoteRequest) (*RFQSnapshot, error) {
	return c.CancelQuoteContext(context.Background(), request)
}

func (c *Client) CancelQuoteContext(ctx context.Context, request CancelQuoteRequest) (*RFQSnapshot, error) {
	request.SignerAddress, request.MakerAddress, request.SignatureType = c.identity()
	return postAuthenticatedContext[RFQSnapshot](ctx, c, "/v1/maker/quotes/cancel", request, "cancel RFQ quote")
}

func (c *Client) Confirm(request MakerConfirmationRequest) (*MakerConfirmationResult, error) {
	return c.ConfirmContext(context.Background(), request)
}

func (c *Client) ConfirmContext(ctx context.Context, request MakerConfirmationRequest) (*MakerConfirmationResult, error) {
	request.SignerAddress, request.MakerAddress, request.SignatureType = c.identity()
	return postAuthenticatedContext[MakerConfirmationResult](ctx, c, "/v1/maker/confirmations", request, "confirm RFQ")
}

func (c *Client) identity() (string, string, types.SignatureType) {
	i := c.config.Identity
	return i.SignerAddress, i.MakerAddress, i.SignatureType
}

func postAuthenticated[T any](c *Client, path string, body any) (*T, error) {
	return postAuthenticatedContext[T](context.Background(), c, path, body, "RFQ mutation")
}

func postAuthenticatedContext[T any](ctx context.Context, c *Client, path string, body any, operation string) (*T, error) {
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
	raw, err := sdkhttp.PostRawContext(ctx, c.baseURL, path, bodyJSON, sdkhttp.WithHeaders(headers), sdkhttp.WithService("rfq"), sdkhttp.WithAmbiguousOnTimeout(operation))
	if err != nil {
		return nil, err
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode RFQ response: %w", err)
	}
	return &result, nil
}
