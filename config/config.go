package config

import (
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
)

// Config 表示 SDK 的全局配置
type Config struct {
	// API 配置
	API APIConfig

	// Web3 配置
	Web3 Web3Config

	// HTTP 配置
	HTTP HTTPConfig

	// 缓存配置
	Cache CacheConfig

	// 日志配置
	Log LogConfig

	// 重试配置
	Retry RetryConfig
}

// APIConfig API 服务配置
type APIConfig struct {
	// CLOB API 域名
	ClobDomain string

	// Gamma API 域名
	GammaDomain string

	// Data API 域名
	DataDomain string

	// Relayer 域名
	RelayerDomain string
}

// Web3Config Web3 相关配置
type Web3Config struct {
	// RPC URL（主网）
	RPCMainnet string

	// RPC URL（测试网）
	RPCAmoy string

	// 链 ID
	ChainID types.ChainID

	// 签名类型
	SignatureType types.SignatureType

	// 私钥（不导出，仅用于初始化）
	privateKey string
}

// HTTPConfig HTTP 客户端配置
type HTTPConfig struct {
	// 请求超时时间
	Timeout time.Duration

	// 长请求超时时间
	LongTimeout time.Duration

	// 最大重试次数
	MaxRetries int

	// 是否启用请求日志
	EnableRequestLog bool

	// 是否启用响应日志
	EnableResponseLog bool
}

// CacheConfig 缓存配置
type CacheConfig struct {
	// 是否启用缓存
	Enabled bool

	// 默认 TTL
	DefaultTTL time.Duration

	// TickSize 缓存 TTL
	TickSizeTTL time.Duration

	// NegRisk 缓存 TTL
	NegRiskTTL time.Duration

	// FeeRate 缓存 TTL
	FeeRateTTL time.Duration
}

// LogConfig 日志配置
type LogConfig struct {
	// 日志级别: DEBUG, INFO, WARN, ERROR
	Level string

	// 是否启用文件日志
	EnableFileLog bool

	// 日志文件路径
	FilePath string
}

// RetryConfig 重试配置
type RetryConfig struct {
	// 最大重试次数
	MaxRetries int

	// 退避基础时间
	BackoffBase time.Duration

	// 退避最大时间
	BackoffMax time.Duration

	// 可重试的状态码
	RetryableStatusCodes []int
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		API: APIConfig{
			ClobDomain:    "https://clob.polymarket.com",
			GammaDomain:   "https://gamma-api.polymarket.com",
			DataDomain:    "https://data-api.polymarket.com",
			RelayerDomain: "https://relayer-v2.polymarket.com",
		},
		Web3: Web3Config{
			RPCMainnet:    "https://rpc-mainnet.matic.quiknode.pro",
			RPCAmoy:       "https://rpc-amoy.polygon.technology",
			ChainID:       types.Polygon,
			SignatureType: types.EOASignatureType,
		},
		HTTP: HTTPConfig{
			Timeout:         30 * time.Second,
			LongTimeout:     60 * time.Second,
			MaxRetries:      3,
			EnableRequestLog: false,
			EnableResponseLog: false,
		},
		Cache: CacheConfig{
			Enabled:     true,
			DefaultTTL:  5 * time.Minute,
			TickSizeTTL: 10 * time.Minute,
			NegRiskTTL:  10 * time.Minute,
			FeeRateTTL:  10 * time.Minute,
		},
		Log: LogConfig{
			Level:         "INFO",
			EnableFileLog: false,
		},
		Retry: RetryConfig{
			MaxRetries:         3,
			BackoffBase:        5 * time.Second,
			BackoffMax:         30 * time.Second,
			RetryableStatusCodes: []int{500, 502, 503, 504, 408, 429},
		},
	}
}

// Option 配置选项函数
type Option func(*Config)

// WithAPIDomain 设置 API 域名
func WithAPIDomain(domain string) Option {
	return func(c *Config) {
		c.API.ClobDomain = domain
	}
}

// WithClobDomain 设置 CLOB API 域名
func WithClobDomain(domain string) Option {
	return func(c *Config) {
		c.API.ClobDomain = domain
	}
}

// WithGammaDomain 设置 Gamma API 域名
func WithGammaDomain(domain string) Option {
	return func(c *Config) {
		c.API.GammaDomain = domain
	}
}

// WithDataDomain 设置 Data API 域名
func WithDataDomain(domain string) Option {
	return func(c *Config) {
		c.API.DataDomain = domain
	}
}

// WithChainID 设置链 ID
func WithChainID(chainID types.ChainID) Option {
	return func(c *Config) {
		c.Web3.ChainID = chainID
	}
}

// WithSignatureType 设置签名类型
func WithSignatureType(sigType types.SignatureType) Option {
	return func(c *Config) {
		c.Web3.SignatureType = sigType
	}
}

// WithHTTPTimeout 设置 HTTP 超时时间
func WithHTTPTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.HTTP.Timeout = timeout
	}
}

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(maxRetries int) Option {
	return func(c *Config) {
		c.HTTP.MaxRetries = maxRetries
		c.Retry.MaxRetries = maxRetries
	}
}

// WithCacheEnabled 设置缓存是否启用
func WithCacheEnabled(enabled bool) Option {
	return func(c *Config) {
		c.Cache.Enabled = enabled
	}
}

// WithLogLevel 设置日志级别
func WithLogLevel(level string) Option {
	return func(c *Config) {
		c.Log.Level = level
	}
}

// NewConfig 创建新配置
func NewConfig(options ...Option) *Config {
	cfg := DefaultConfig()
	for _, opt := range options {
		opt(cfg)
	}
	return cfg
}
