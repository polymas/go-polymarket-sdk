package gamma

import (
	"context"
	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetProfile 获取用户资料
func (c *polymarketGammaClient) GetProfile(address types.EthAddress) (*types.Profile, error) {
	return c.GetProfileContext(context.Background(), address)
}

func (c *polymarketGammaClient) GetProfileContext(ctx context.Context, address types.EthAddress) (*types.Profile, error) {
	return http.GetContext[types.Profile](ctx, c.baseURL, internal.GetProfile, map[string]string{"address": string(address)}, http.WithService("gamma"))
}
