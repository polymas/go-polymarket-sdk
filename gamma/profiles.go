package gamma

import (
	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetProfile 获取用户资料
func (c *polymarketGammaClient) GetProfile(address types.EthAddress) (*types.Profile, error) {
	return http.Get[types.Profile](c.baseURL, internal.GetProfile, map[string]string{"address": string(address)})
}
