package container

import (
	"sync"

	"github.com/polymas/go-polymarket-sdk/cache"
	"github.com/polymas/go-polymarket-sdk/config"
	"github.com/polymas/go-polymarket-sdk/middleware"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// Container 依赖注入容器
type Container struct {
	mu sync.RWMutex

	// 配置
	config *config.Config

	// 缓存
	cache cache.Cache

	// Web3 客户端
	web3Client web3.Client

	// HTTP 客户端中间件
	httpMiddleware middleware.Middleware

	// 已初始化的服务
	services map[string]interface{}
}

// NewContainer 创建新容器
func NewContainer(cfg *config.Config) *Container {
	// 初始化缓存
	var cacheInstance cache.Cache
	if cfg.Cache.Enabled {
		cacheInstance = cache.NewMemoryCache()
	} else {
		cacheInstance = cache.NewNoopCache()
	}

	// 构建 HTTP 中间件链
	middlewares := []middleware.Middleware{
		middleware.NewTimeoutMiddleware(cfg.HTTP.Timeout),
		middleware.NewLoggingMiddleware(
			cfg.HTTP.EnableRequestLog,
			cfg.HTTP.EnableResponseLog,
		),
		middleware.NewRetryMiddleware(
			cfg.Retry.MaxRetries,
			cfg.Retry.BackoffBase,
			cfg.Retry.BackoffMax,
		),
	}

	httpMiddleware := middleware.Chain(middlewares...)

	return &Container{
		config:         cfg,
		cache:          cacheInstance,
		httpMiddleware: httpMiddleware,
		services:       make(map[string]interface{}),
	}
}

// GetConfig 获取配置
func (c *Container) GetConfig() *config.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// GetCache 获取缓存实例
func (c *Container) GetCache() cache.Cache {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache
}

// SetWeb3Client 设置 Web3 客户端
func (c *Container) SetWeb3Client(client web3.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.web3Client = client
}

// GetWeb3Client 获取 Web3 客户端
func (c *Container) GetWeb3Client() web3.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.web3Client
}

// GetHTTPMiddleware 获取 HTTP 中间件
func (c *Container) GetHTTPMiddleware() middleware.Middleware {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.httpMiddleware
}

// SetService 设置服务实例
func (c *Container) SetService(name string, service interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services[name] = service
}

// GetService 获取服务实例
func (c *Container) GetService(name string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	service, ok := c.services[name]
	return service, ok
}

// Close 关闭容器，清理资源
func (c *Container) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 关闭 Web3 客户端
	if c.web3Client != nil {
		c.web3Client.Close()
	}

	// 清空缓存
	if c.cache != nil {
		c.cache.Clear()
	}

	// 清空服务
	c.services = make(map[string]interface{})

	return nil
}
