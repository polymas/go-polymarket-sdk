package clob

import (
	"fmt"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetAPIKeys 获取所有 API 密钥
func (c *apiKeyClientImpl) GetAPIKeys() ([]types.APIKey, error) {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	// Set up authentication headers
	requestArgs := &types.RequestArgs{
		Method:      "GET",
		RequestPath: internal.GetAPIKeys,
		Body:        nil,
	}

	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	result, err := http.Get[[]types.APIKey](c.baseClient.baseURL, internal.GetAPIKeys, nil, http.WithHeaders(headers))
	if err != nil {
		return nil, fmt.Errorf("failed to get API keys: %w", err)
	}

	if result == nil {
		return []types.APIKey{}, nil
	}

	return *result, nil
}

// DeleteAPIKey 删除 API 密钥
func (c *apiKeyClientImpl) DeleteAPIKey(keyID string) error {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	// Set up authentication headers
	requestArgs := &types.RequestArgs{
		Method:      "DELETE",
		RequestPath: fmt.Sprintf("%s/%s", internal.DeleteAPIKey, keyID),
		Body:        nil,
	}

	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, false)
	if err != nil {
		return fmt.Errorf("failed to create headers: %w", err)
	}

	_, err = http.Delete[map[string]interface{}](c.baseClient.baseURL, fmt.Sprintf("%s/%s", internal.DeleteAPIKey, keyID), nil, http.WithHeaders(headers))
	return err
}

// CreateReadonlyAPIKey 创建只读 API 密钥
func (c *apiKeyClientImpl) CreateReadonlyAPIKey() (*types.APIKey, error) {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	// Set up authentication headers
	requestArgs := &types.RequestArgs{
		Method:      "POST",
		RequestPath: internal.CreateReadonlyAPIKey,
		Body:        nil,
	}

	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	return http.Post[types.APIKey](c.baseClient.baseURL, internal.CreateReadonlyAPIKey, nil, http.WithHeaders(headers))
}

// GetReadonlyAPIKeys 获取只读 API 密钥列表
func (c *apiKeyClientImpl) GetReadonlyAPIKeys() ([]types.APIKey, error) {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	// Set up authentication headers
	requestArgs := &types.RequestArgs{
		Method:      "GET",
		RequestPath: internal.GetReadonlyAPIKeys,
		Body:        nil,
	}

	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	result, err := http.Get[[]types.APIKey](c.baseClient.baseURL, internal.GetReadonlyAPIKeys, nil, http.WithHeaders(headers))
	if err != nil {
		return nil, fmt.Errorf("failed to get readonly API keys: %w", err)
	}

	if result == nil {
		return []types.APIKey{}, nil
	}

	return *result, nil
}

// DeleteReadonlyAPIKey 删除只读 API 密钥
func (c *apiKeyClientImpl) DeleteReadonlyAPIKey(keyID string) error {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	// Set up authentication headers
	requestArgs := &types.RequestArgs{
		Method:      "DELETE",
		RequestPath: fmt.Sprintf("%s/%s", internal.DeleteReadonlyAPIKey, keyID),
		Body:        nil,
	}

	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, false)
	if err != nil {
		return fmt.Errorf("failed to create headers: %w", err)
	}

	_, err = http.Delete[map[string]interface{}](c.baseClient.baseURL, fmt.Sprintf("%s/%s", internal.DeleteReadonlyAPIKey, keyID), nil, http.WithHeaders(headers))
	return err
}
