package internal

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/polymas/go-polymarket-sdk/internal/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	PolyAddress           = "POLY_ADDRESS"
	PolySignature         = "POLY_SIGNATURE"
	PolyTimestamp         = "POLY_TIMESTAMP"
	PolyNonce             = "POLY_NONCE"
	PolyAPIKey            = "POLY_API_KEY"
	PolyPassphrase        = "POLY_PASSPHRASE"
	PolyBuilderAPIKey     = "POLY_BUILDER_API_KEY"
	PolyBuilderPassphrase = "POLY_BUILDER_PASSPHRASE"
	PolyBuilderSignature  = "POLY_BUILDER_SIGNATURE"
	PolyBuilderTimestamp  = "POLY_BUILDER_TIMESTAMP"
)

// CreateLevel1Headers creates Level 1 Poly headers for a request
func CreateLevel1Headers(signer *signing.Signer, nonce *int) (map[string]string, error) {
	timestamp := time.Now().UTC().Unix()

	n := 0
	if nonce != nil {
		n = *nonce
	}

	signature, err := signing.SignClobAuthMessage(signer, int64(timestamp), n)
	if err != nil {
		return nil, fmt.Errorf("failed to sign CLOB auth message: %w", err)
	}

	headers := map[string]string{
		PolyAddress:   signer.Address().String(),
		PolySignature: signature,
		PolyTimestamp: strconv.FormatInt(timestamp, 10),
		PolyNonce:     strconv.Itoa(n),
	}

	return headers, nil
}

// CreateLevel2Headers creates Level 2 Poly headers for a request
func CreateLevel2Headers(
	signer *signing.Signer,
	creds *types.ApiCreds,
	requestArgs *types.RequestArgs,
	builder bool,
) (map[string]string, error) {
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)

	// Convert RequestBody to interface{} for BuildHMACSignature
	// Python version passes body directly (dict/list), then build_hmac_signature does str(body).replace("'", '"')
	// Go version: Body is already JSON string (from RequestBody), pass it directly
	var body interface{}
	if requestArgs.Body != nil {
		// Body is already a JSON string (from json.Marshal), pass it as string
		body = string(*requestArgs.Body)
	}

	hmacSig, err := signing.BuildHMACSignature(
		creds.Secret,
		timestamp,
		requestArgs.Method,
		requestArgs.RequestPath,
		body,
	)
	if err != nil {
		return nil, err
	}

	if builder {
		return map[string]string{
			PolyBuilderSignature:  hmacSig,
			PolyBuilderTimestamp:  timestamp,
			PolyBuilderAPIKey:     creds.Key,
			PolyBuilderPassphrase: creds.Passphrase,
		}, nil
	}

	headers := map[string]string{
		PolyAddress:    signer.Address().String(),
		PolySignature:  hmacSig,
		PolyTimestamp:  timestamp,
		PolyAPIKey:     creds.Key,
		PolyPassphrase: creds.Passphrase,
	}

	return headers, nil
}

// CreateLevel2HeadersWithBody creates Level 2 Poly headers with body passed directly (for POST /orders)
// This matches Python behavior where body is passed as list/dict, not JSON string
func CreateLevel2HeadersWithBody(
	signer *signing.Signer,
	creds *types.ApiCreds,
	requestArgs *types.RequestArgs,
	body interface{},
	builder bool,
) (map[string]string, error) {
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)

	// Pass body directly to BuildHMACSignature (matches Python: body is list/dict, not JSON string)
	hmacSig, err := signing.BuildHMACSignature(
		creds.Secret,
		timestamp,
		requestArgs.Method,
		requestArgs.RequestPath,
		body,
	)
	if err != nil {
		return nil, err
	}

	if builder {
		return map[string]string{
			PolyBuilderSignature:  hmacSig,
			PolyBuilderTimestamp:  timestamp,
			PolyBuilderAPIKey:     creds.Key,
			PolyBuilderPassphrase: creds.Passphrase,
		}, nil
	}

	headers := map[string]string{
		PolyAddress:    signer.Address().String(),
		PolySignature:  hmacSig,
		PolyTimestamp:  timestamp,
		PolyAPIKey:     creds.Key,
		PolyPassphrase: creds.Passphrase,
	}

	return headers, nil
}

// ValidateEthAddress 验证以太坊地址格式
// 返回错误如果地址格式无效
func ValidateEthAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("address cannot be empty")
	}

	// 检查前缀
	if !strings.HasPrefix(addr, "0x") {
		return fmt.Errorf("address must start with 0x")
	}

	// 检查长度（0x + 40个十六进制字符）
	if len(addr) != 42 {
		return fmt.Errorf("invalid address length: expected 42 characters (0x + 40 hex), got %d", len(addr))
	}

	// 验证十六进制字符
	matched, err := regexp.MatchString("^0x[0-9a-fA-F]{40}$", addr)
	if err != nil {
		return fmt.Errorf("failed to validate address format: %w", err)
	}
	if !matched {
		return fmt.Errorf("address contains invalid characters: only hexadecimal characters (0-9, a-f, A-F) are allowed")
	}

	return nil
}

// ValidateKeccak256 验证Keccak256哈希格式（64个十六进制字符，可选0x前缀）
func ValidateKeccak256(hash string) error {
	if hash == "" {
		return fmt.Errorf("hash cannot be empty")
	}

	// 移除0x前缀（如果存在）
	hashStr := hash
	if strings.HasPrefix(hash, "0x") {
		hashStr = hash[2:]
	}

	// 检查长度（64个十六进制字符）
	if len(hashStr) != 64 {
		return fmt.Errorf("invalid hash length: expected 64 hex characters, got %d", len(hashStr))
	}

	// 验证十六进制字符
	matched, err := regexp.MatchString("^[0-9a-fA-F]{64}$", hashStr)
	if err != nil {
		return fmt.Errorf("failed to validate hash format: %w", err)
	}
	if !matched {
		return fmt.Errorf("hash contains invalid characters: only hexadecimal characters (0-9, a-f, A-F) are allowed")
	}

	return nil
}

// ValidateLimitOffset 验证limit和offset参数
// maxLimit: 最大limit值（默认10000）
func ValidateLimitOffset(limit, offset int, maxLimit int) error {
	if maxLimit <= 0 {
		maxLimit = 10000 // 默认最大值
	}

	if limit < 0 {
		return fmt.Errorf("limit cannot be negative, got %d", limit)
	}
	if limit > maxLimit {
		return fmt.Errorf("limit exceeds maximum allowed value: %d > %d", limit, maxLimit)
	}

	if offset < 0 {
		return fmt.Errorf("offset cannot be negative, got %d", offset)
	}

	return nil
}

// ValidatePriceRange 验证价格范围
// price: 价格值
// minPrice: 最小价格（默认0）
// maxPrice: 最大价格（默认1）
func ValidatePriceRange(price, minPrice, maxPrice float64) error {
	if minPrice < 0 {
		minPrice = 0
	}
	if maxPrice <= 0 {
		maxPrice = 1.0
	}

	if price < minPrice {
		return fmt.Errorf("price below minimum: %.6f < %.6f", price, minPrice)
	}
	if price > maxPrice {
		return fmt.Errorf("price above maximum: %.6f > %.6f", price, maxPrice)
	}

	return nil
}
