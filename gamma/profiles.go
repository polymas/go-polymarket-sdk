package gamma

import (
	"fmt"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetProfile 获取用户资料
func (c *polymarketGammaClient) GetProfile(address types.EthAddress) (*types.Profile, error) {
	return http.Get[types.Profile](c.baseURL, fmt.Sprintf("%s%s", internal.GetProfile, string(address)), nil)
}

// GetProfileByUsername 通过用户名获取资料
func (c *polymarketGammaClient) GetProfileByUsername(username string) (*types.Profile, error) {
	return http.Get[types.Profile](c.baseURL, fmt.Sprintf("%s%s", internal.GetProfileByUsername, username), nil)
}
