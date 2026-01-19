package test

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
)

// TestConfig 测试配置
type TestConfig struct {
	PrivateKey          string
	ChainID             types.ChainID
	SignatureType       types.SignatureType
	BuilderAPIKey       string
	BuilderSecret       string
	BuilderPassphrase   string
	TestUserAddress     types.EthAddress
	TestTokenID          string
	TestConditionID     types.Keccak256
	TestMarketID        string
	TestMarketSlug      string
	TestEventID         int
	RTDSToken           string
}

// LoadTestConfig 从环境变量加载测试配置
func LoadTestConfig() *TestConfig {
	config := &TestConfig{
		PrivateKey:        getEnv("POLY_PRIVATE_KEY", ""),
		ChainID:           types.ChainID(getEnvInt("POLY_CHAIN_ID", 137)),
		SignatureType:     types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", 0)),
		BuilderAPIKey:     getEnv("POLY_BUILDER_API_KEY", ""),
		BuilderSecret:     getEnv("POLY_BUILDER_SECRET", ""),
		BuilderPassphrase: getEnv("POLY_BUILDER_PASSPHRASE", ""),
		TestUserAddress:   types.EthAddress(getEnv("POLY_TEST_USER_ADDRESS", "0x0000000000000000000000000000000000000000")),
		TestTokenID:       getEnv("POLY_TEST_TOKEN_ID", ""),
		TestConditionID:   types.Keccak256(getEnv("POLY_TEST_CONDITION_ID", "")),
		TestMarketID:      getEnv("POLY_TEST_MARKET_ID", ""),
		TestMarketSlug:    getEnv("POLY_TEST_MARKET_SLUG", ""),
		TestEventID:       getEnvInt("POLY_TEST_EVENT_ID", 0),
		RTDSToken:         getEnv("POLY_RTDS_TOKEN", ""),
	}
	return config
}

// getEnv 获取环境变量，如果不存在则返回默认值
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt 获取整数环境变量，如果不存在则返回默认值
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}


// SkipIfShort 如果是短测试则跳过
func SkipIfShort(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}
}

// SkipIfNoAuth 如果没有认证信息则跳过
func SkipIfNoAuth(t *testing.T, config *TestConfig) {
	if config.PrivateKey == "" {
		t.Skip("Skipping test: POLY_PRIVATE_KEY not set")
	}
}

// SkipIfNoBuilderCreds 如果没有Builder凭证则跳过
func SkipIfNoBuilderCreds(t *testing.T, config *TestConfig) {
	if config.BuilderAPIKey == "" || config.BuilderSecret == "" || config.BuilderPassphrase == "" {
		t.Skip("Skipping test: Builder credentials not set")
	}
}

// WaitForWebSocket 等待WebSocket消息（带超时）
func WaitForWebSocket(timeout time.Duration) <-chan time.Time {
	return time.After(timeout)
}

// ValidateEthAddress 验证以太坊地址格式
func ValidateEthAddress(t *testing.T, addr types.EthAddress) {
	if err := addr.Validate(); err != nil {
		t.Fatalf("Invalid Ethereum address: %v", err)
	}
}

// ValidateKeccak256 验证Keccak256哈希格式
func ValidateKeccak256(t *testing.T, hash types.Keccak256) {
	if err := hash.Validate(); err != nil {
		t.Fatalf("Invalid Keccak256 hash: %v", err)
	}
}

// GetTestUserAddress 获取测试用户地址（如果未设置则使用默认值）
func GetTestUserAddress(config *TestConfig) types.EthAddress {
	if config.TestUserAddress != "" && config.TestUserAddress != "0x0000000000000000000000000000000000000000" {
		return config.TestUserAddress
	}
	// 使用一个已知的测试地址
	return types.EthAddress("0x1234567890123456789012345678901234567890")
}

// IsMainnet 检查是否为主网
func IsMainnet(chainID types.ChainID) bool {
	return chainID == types.Polygon
}

// IsTestnet 检查是否为测试网
func IsTestnet(chainID types.ChainID) bool {
	return chainID == types.Amoy
}

// DefaultReconnectDelay 默认重连延迟
const DefaultReconnectDelay = 5 * time.Second
