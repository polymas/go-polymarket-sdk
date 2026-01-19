package test

import (
	"fmt"
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

// CleanupTestResources 清理测试资源
// 用于在测试后清理创建的资源（如订单、API密钥等）
func CleanupTestResources(t *testing.T, cleanupFunc func() error) {
	if cleanupFunc != nil {
		if err := cleanupFunc(); err != nil {
			t.Logf("Cleanup failed (non-fatal): %v", err)
		}
	}
}

// RetryOperation 重试操作（用于处理网络不稳定等情况）
func RetryOperation(t *testing.T, maxRetries int, operation func() error) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		err := operation()
		if err == nil {
			return nil
		}
		lastErr = err
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * time.Second) // 递增延迟
		}
	}
	return lastErr
}

// AssertNonEmpty 断言值非空
func AssertNonEmpty(t *testing.T, name string, value interface{}) {
	if value == nil {
		t.Fatalf("%s should not be nil", name)
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			t.Fatalf("%s should not be empty", name)
		}
	case []interface{}:
		if len(v) == 0 {
			t.Fatalf("%s should not be empty", name)
		}
	}
}

// AssertPositive 断言值为正数
func AssertPositive(t *testing.T, name string, value float64) {
	if value <= 0 {
		t.Errorf("%s should be positive, got %f", name, value)
	}
}

// AssertNonNegative 断言值为非负数
func AssertNonNegative(t *testing.T, name string, value float64) {
	if value < 0 {
		t.Errorf("%s should be non-negative, got %f", name, value)
	}
}

// WaitForCondition 等待条件满足（带超时）
func WaitForCondition(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		<-ticker.C
	}
	return false
}

// GenerateTestTokenID 生成测试用的tokenID（用于测试）
func GenerateTestTokenID() string {
	// 返回一个格式正确的测试tokenID
	return "0x0000000000000000000000000000000000000000000000000000000000000001"
}

// GenerateTestConditionID 生成测试用的conditionID（用于测试）
func GenerateTestConditionID() types.Keccak256 {
	return types.Keccak256("0x0000000000000000000000000000000000000000000000000000000000000001")
}

// GenerateTestMarketID 生成测试用的marketID（用于测试）
func GenerateTestMarketID() string {
	return "1"
}

// IsIntegrationTest 检查是否为集成测试
func IsIntegrationTest() bool {
	return os.Getenv("INTEGRATION_TEST") == "true"
}

// SkipIfNotIntegration 如果不是集成测试则跳过
func SkipIfNotIntegration(t *testing.T) {
	if !IsIntegrationTest() {
		t.Skip("Skipping test: INTEGRATION_TEST not set")
	}
}

// TestMarketData 测试市场数据，包含从slug获取的市场信息和token IDs
type TestMarketData struct {
	MarketID    string
	Slug        string
	ConditionID types.Keccak256
	TokenIDs    []string
	Market      *types.GammaMarket
}

// GetTestMarketDataBySlug 通过slug获取测试市场数据
// 用于预测试，获取活跃市场的token IDs用于测试
// 默认使用BTC 15分钟涨跌预测市场
func GetTestMarketDataBySlug(slug string) (*TestMarketData, error) {
	// 导入gamma包（避免循环依赖，使用动态导入）
	// 由于Go的限制，我们需要在调用处导入gamma包
	// 这里返回一个函数，让调用者传入gamma客户端
	return nil, fmt.Errorf("请使用 GetTestMarketDataBySlugWithClient 函数，传入gamma客户端")
}

// GetTestMarketDataBySlugWithClient 通过slug获取测试市场数据（需要gamma客户端）
// slug: 市场slug，例如 "btc-15min-up-or-down" 或 "will-bitcoin-price-be-higher-in-15-minutes"
// 返回测试市场数据，包含token IDs等信息
func GetTestMarketDataBySlugWithClient(gammaClient interface{}, slug string) (*TestMarketData, error) {
	// 使用反射或类型断言调用GetMarketBySlug
	// 为了简化，我们要求调用者传入正确的类型
	// 这里使用interface{}，调用者需要确保传入的是gamma.Client类型
	
	// 由于Go的类型系统限制，我们需要在调用处进行类型转换
	// 这里提供一个通用的辅助函数
	return nil, fmt.Errorf("请直接使用gamma客户端调用GetMarketBySlug，然后使用NewTestMarketDataFromMarket创建TestMarketData")
}

// NewTestMarketDataFromMarket 从GammaMarket创建TestMarketData
func NewTestMarketDataFromMarket(market *types.GammaMarket) (*TestMarketData, error) {
	if market == nil {
		return nil, fmt.Errorf("market cannot be nil")
	}

	if len(market.TokenIDs) == 0 {
		return nil, fmt.Errorf("market has no token IDs")
	}

	return &TestMarketData{
		MarketID:    market.MarketID,
		Slug:        market.Slug,
		ConditionID: types.Keccak256(market.ConditionID),
		TokenIDs:    market.TokenIDs,
		Market:      market,
	}, nil
}

// GetBTC15MinMarketData 获取BTC 15分钟涨跌预测市场数据
// 这是一个便捷函数，自动查找BTC 15分钟相关的活跃市场
// 注意：由于Go的类型系统限制，此函数需要调用者传入gamma.Client
// 建议直接使用gamma客户端调用GetMarketBySlug，然后使用NewTestMarketDataFromMarket
func GetBTC15MinMarketData(gammaClient interface{}) (*TestMarketData, error) {
	// 由于Go的类型限制，这个函数需要调用者自己处理
	// 实际使用请参考getTestMarketData函数的实现
	return nil, fmt.Errorf("请使用以下代码获取BTC 15分钟市场数据:\n" +
		"gammaClient := gamma.NewClient()\n" +
		"market, err := gammaClient.GetMarketBySlug(\"btc-15min-up-or-down\", nil)\n" +
		"if err != nil {\n" +
		"    // 尝试其他slug\n" +
		"}\n" +
		"testData, err := test.NewTestMarketDataFromMarket(market)")
}
