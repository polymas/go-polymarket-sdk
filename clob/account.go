package clob

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GetUSDCBalance gets USDC balance
func (c *accountClientImpl) GetUSDCBalance() (float64, error) {
	return c.baseClient.web3Client.GetUSDCBalance(c.baseClient.proxyAddress)
}

// GetBalanceAllowance 获取余额授权信息
func (c *accountClientImpl) GetBalanceAllowance() (*types.BalanceAllowance, error) {
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
		RequestPath: internal.GetBalanceAllowance,
		Body:        nil,
	}

	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	return http.Get[types.BalanceAllowance](c.baseClient.baseURL, internal.GetBalanceAllowance, nil, http.WithHeaders(headers))
}

// UpdateBalanceAllowance 更新余额授权
func (c *accountClientImpl) UpdateBalanceAllowance(amount float64) (*types.BalanceAllowance, error) {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	// Build request body
	requestBody := map[string]float64{
		"amount": amount,
	}

	// Marshal body to JSON
	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}

	// Convert compact JSON to Python's json.dumps format (with spaces)
	bodyJSONStr := string(bodyJSON)
	bodyJSONStr = regexp.MustCompile(`":(\S)`).ReplaceAllString(bodyJSONStr, `": $1`)
	bodyJSONStr = regexp.MustCompile(`,(")`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSONStr = regexp.MustCompile(`,(\{|\[)`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSON = []byte(bodyJSONStr)

	// Create request args for signing
	requestBodyForSigning := types.RequestBody(bodyJSON)
	requestArgs := &types.RequestArgs{
		Method:      "POST",
		RequestPath: internal.UpdateBalanceAllowance,
		Body:        &requestBodyForSigning,
	}

	// Create Level 2 headers
	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	// Execute POST request
	return http.Post[types.BalanceAllowance](c.baseClient.baseURL, internal.UpdateBalanceAllowance, requestBody, http.WithHeaders(headers))
}

// GetNotifications 获取通知列表
func (c *accountClientImpl) GetNotifications(limit int, offset int) ([]types.Notification, error) {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	params := map[string]string{
		"limit":  strconv.Itoa(limit),
		"offset": strconv.Itoa(offset),
	}

	// Set up authentication headers
	requestArgs := &types.RequestArgs{
		Method:      "GET",
		RequestPath: internal.GetNotifications,
		Body:        nil,
	}

	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	result, err := http.Get[[]types.Notification](c.baseClient.baseURL, internal.GetNotifications, params, http.WithHeaders(headers))
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}

	if result == nil {
		return []types.Notification{}, nil
	}

	return *result, nil
}

// DropNotifications 删除通知
func (c *accountClientImpl) DropNotifications(notificationIDs []string) error {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	if len(notificationIDs) == 0 {
		return nil // Nothing to delete
	}

	// Build request body
	requestBody := map[string][]string{
		"notification_ids": notificationIDs,
	}

	// Marshal body to JSON
	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	// Convert compact JSON to Python's json.dumps format (with spaces)
	bodyJSONStr := string(bodyJSON)
	bodyJSONStr = regexp.MustCompile(`":(\S)`).ReplaceAllString(bodyJSONStr, `": $1`)
	bodyJSONStr = regexp.MustCompile(`,(")`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSONStr = regexp.MustCompile(`,(\{|\[)`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSON = []byte(bodyJSONStr)

	// Create request args for signing
	requestBodyForSigning := types.RequestBody(bodyJSON)
	requestArgs := &types.RequestArgs{
		Method:      "DELETE",
		RequestPath: internal.DropNotifications,
		Body:        &requestBodyForSigning,
	}

	// Create Level 2 headers
	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, false)
	if err != nil {
		return fmt.Errorf("failed to create headers: %w", err)
	}

	// Execute DELETE request
	_, err = http.DeleteRaw[map[string]interface{}](c.baseClient.baseURL, internal.DropNotifications, bodyJSON, http.WithHeaders(headers))
	return err
}
