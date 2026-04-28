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

// GetBalanceAllowance 获取余额授权信息。
// V2 要求 asset_type + signature_type 两个 query 参数：
//   - asset_type：COLLATERAL 查 pUSD，CONDITIONAL 查 CTF 份额
//   - signature_type：默认 0（按 EOA 查），Safe/Proxy 用户必须传 2/1 才查 proxy 的余额和授权
func (c *accountClientImpl) GetBalanceAllowance() (*types.BalanceAllowance, error) {
	// Validate API credentials
	if c.baseClient.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	// Polymarket HMAC 只签 path，不含 query string
	requestArgs := &types.RequestArgs{
		Method:      "GET",
		RequestPath: internal.GetBalanceAllowance,
		Body:        nil,
	}

	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	params := map[string]string{
		"asset_type":     "COLLATERAL",
		"signature_type": strconv.Itoa(int(c.baseClient.signatureType)),
	}
	return http.Get[types.BalanceAllowance](c.baseClient.baseURL, internal.GetBalanceAllowance, params, http.WithHeaders(headers))
}

// UpdateBalanceAllowance 触发服务端重查链上 balance/allowance（强制索引刷新）。
// V2 改版：原 POST /balance-allowance/update 改成 GET，且用 ?asset_type= 区分 COLLATERAL / CONDITIONAL。
// amount 参数在 V2 里已无意义（body 不再发），保留签名兼容 V1。
func (c *accountClientImpl) UpdateBalanceAllowance(amount float64) (*types.BalanceAllowance, error) {
	_ = amount
	if c.baseClient.deriveCreds == nil {
		return nil, fmt.Errorf("API credentials not set")
	}
	if c.baseClient.deriveCreds.Key == "" || c.baseClient.deriveCreds.Secret == "" || c.baseClient.deriveCreds.Passphrase == "" {
		return nil, fmt.Errorf("API credentials incomplete: key=%v, secret=%v, passphrase=%v",
			c.baseClient.deriveCreds.Key != "", c.baseClient.deriveCreds.Secret != "", c.baseClient.deriveCreds.Passphrase != "")
	}

	requestArgs := &types.RequestArgs{
		Method:      "GET",
		RequestPath: internal.UpdateBalanceAllowance,
		Body:        nil,
	}
	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	params := map[string]string{
		"asset_type":     "COLLATERAL",
		"signature_type": strconv.Itoa(int(c.baseClient.signatureType)),
	}
	return http.Get[types.BalanceAllowance](c.baseClient.baseURL, internal.UpdateBalanceAllowance, params, http.WithHeaders(headers))
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

	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs)
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
	headers, err := internal.CreateLevel2Headers(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs)
	if err != nil {
		return fmt.Errorf("failed to create headers: %w", err)
	}

	// Execute DELETE request
	_, err = http.DeleteRaw[map[string]interface{}](c.baseClient.baseURL, internal.DropNotifications, bodyJSON, http.WithHeaders(headers))
	return err
}
