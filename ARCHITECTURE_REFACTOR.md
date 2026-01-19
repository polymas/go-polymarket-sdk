# Go Polymarket SDK 架构重构指南

## 概述

本次重构基于 Go 专家架构模式，引入了以下核心改进：

1. **统一配置管理** (`config` 包)
2. **依赖注入容器** (`container` 包)
3. **中间件系统** (`middleware` 包)
4. **统一缓存管理** (`cache` 包)
5. **增强错误处理** (`errors` 包)

## 新架构组件

### 1. 配置管理 (config)

统一管理所有 SDK 配置项，支持函数式选项模式。

```go
import "github.com/polymas/go-polymarket-sdk/config"

// 使用默认配置
cfg := config.DefaultConfig()

// 使用自定义配置
cfg := config.NewConfig(
    config.WithChainID(types.Amoy),
    config.WithSignatureType(types.ProxySignatureType),
    config.WithHTTPTimeout(60 * time.Second),
    config.WithMaxRetries(5),
    config.WithCacheEnabled(true),
    config.WithLogLevel("DEBUG"),
)
```

### 2. 依赖注入容器 (container)

管理所有依赖关系，提供统一的依赖获取接口。

```go
import "github.com/polymas/go-polymarket-sdk/container"

// 创建容器
cfg := config.DefaultConfig()
container := container.NewContainer(cfg)

// 设置 Web3 客户端
web3Client, _ := web3.NewClient(privateKey, sigType, chainID)
container.SetWeb3Client(web3Client)

// 获取依赖
cache := container.GetCache()
web3Client := container.GetWeb3Client()
```

### 3. 中间件系统 (middleware)

提供可组合的 HTTP 请求中间件，支持重试、日志、超时等功能。

#### 重试中间件

自动重试失败的请求，支持指数退避。

```go
import "github.com/polymas/go-polymarket-sdk/middleware"

retryMW := middleware.NewRetryMiddleware(
    3,                      // 最大重试次数
    5 * time.Second,        // 退避基础时间
    30 * time.Second,       // 退避最大时间
)
```

#### 日志中间件

记录请求和响应信息。

```go
loggingMW := middleware.NewLoggingMiddleware(
    true,  // 启用请求日志
    true,  // 启用响应日志
)
```

#### 超时中间件

为请求设置超时时间。

```go
timeoutMW := middleware.NewTimeoutMiddleware(30 * time.Second)
```

#### 组合中间件

使用 `Chain` 函数组合多个中间件。

```go
middlewares := []middleware.Middleware{
    middleware.NewTimeoutMiddleware(30 * time.Second),
    middleware.NewLoggingMiddleware(true, true),
    middleware.NewRetryMiddleware(3, 5*time.Second, 30*time.Second),
}

chain := middleware.Chain(middlewares...)
```

### 4. 缓存管理 (cache)

提供统一的缓存接口，支持内存缓存和禁用缓存。

```go
import "github.com/polymas/go-polymarket-sdk/cache"

// 创建内存缓存
cache := cache.NewMemoryCache()

// 设置缓存值（带 TTL）
cache.Set("key", "value", 5 * time.Minute)

// 获取缓存值
value, ok := cache.Get("key")
if ok {
    // 使用缓存值
}

// 删除缓存值
cache.Delete("key")

// 清空所有缓存
cache.Clear()

// 创建空操作缓存（禁用缓存）
noopCache := cache.NewNoopCache()
```

### 5. 错误处理 (errors)

统一的错误类型和处理机制。

```go
import "github.com/polymas/go-polymarket-sdk/errors"

// 创建不同类型的错误
networkErr := errors.NewNetworkError("连接失败", err)
apiErr := errors.NewAPIError(500, "服务器错误", err)
validationErr := errors.NewValidationError("参数无效", err)
authErr := errors.NewAuthError("认证失败", err)
rateLimitErr := errors.NewRateLimitError("请求过快", err)
timeoutErr := errors.NewTimeoutError("请求超时", err)

// 包装错误
wrappedErr := errors.WrapError(err, errors.ErrorTypeAPI, "API 调用失败")

// 检查错误是否可重试
if errors.IsRetryableError(err) {
    // 执行重试逻辑
}

// 获取错误类型
errorType := errors.GetErrorType(err)
```

## 使用示例

### 基本使用

```go
package main

import (
    "github.com/polymas/go-polymarket-sdk/config"
    "github.com/polymas/go-polymarket-sdk/container"
    "github.com/polymas/go-polymarket-sdk/web3"
    "github.com/polymas/go-polymarket-sdk/types"
)

func main() {
    // 1. 创建配置
    cfg := config.NewConfig(
        config.WithChainID(types.Polygon),
        config.WithSignatureType(types.ProxySignatureType),
        config.WithHTTPTimeout(30 * time.Second),
        config.WithMaxRetries(3),
        config.WithCacheEnabled(true),
        config.WithLogLevel("INFO"),
    )

    // 2. 创建容器
    container := container.NewContainer(cfg)

    // 3. 初始化 Web3 客户端
    web3Client, err := web3.NewClient(
        privateKey,
        cfg.Web3.SignatureType,
        cfg.Web3.ChainID,
    )
    if err != nil {
        panic(err)
    }
    container.SetWeb3Client(web3Client)

    // 4. 使用容器中的依赖
    cache := container.GetCache()
    cache.Set("test", "value", 5 * time.Minute)

    // 5. 清理资源
    defer container.Close()
}
```

### 高级使用：自定义中间件

```go
package main

import (
    "context"
    "net/http"
    "github.com/polymas/go-polymarket-sdk/middleware"
)

// 自定义中间件：添加请求头
func AddHeaderMiddleware(headerKey, headerValue string) middleware.Middleware {
    return func(next middleware.RequestFunc) middleware.RequestFunc {
        return func(ctx context.Context, req *http.Request) (*http.Response, error) {
            req.Header.Set(headerKey, headerValue)
            return next(ctx, req)
        }
    }
}

func main() {
    // 组合自定义中间件和内置中间件
    middlewares := []middleware.Middleware{
        middleware.NewTimeoutMiddleware(30 * time.Second),
        AddHeaderMiddleware("X-Custom-Header", "value"),
        middleware.NewLoggingMiddleware(true, true),
        middleware.NewRetryMiddleware(3, 5*time.Second, 30*time.Second),
    }

    chain := middleware.Chain(middlewares...)
    // 使用 chain...
}
```

## 迁移指南

### 从旧代码迁移

#### 1. 配置迁移

**旧代码：**
```go
// 直接使用内部常量
baseURL := internal.ClobAPIDomain
```

**新代码：**
```go
cfg := config.DefaultConfig()
baseURL := cfg.API.ClobDomain
```

#### 2. 错误处理迁移

**旧代码：**
```go
if err != nil {
    return nil, fmt.Errorf("failed: %w", err)
}
```

**新代码：**
```go
import "github.com/polymas/go-polymarket-sdk/errors"

if err != nil {
    return nil, errors.WrapError(err, errors.ErrorTypeAPI, "failed")
}
```

#### 3. 缓存迁移

**旧代码：**
```go
// 每个客户端自己管理缓存
type client struct {
    tickSizes map[string]types.TickSize
    negRisk   map[string]bool
}
```

**新代码：**
```go
import "github.com/polymas/go-polymarket-sdk/cache"

type client struct {
    cache cache.Cache
}

// 使用统一缓存接口
value, ok := c.cache.Get("tickSize:" + tokenID)
```

## 最佳实践

1. **使用容器管理依赖**：避免全局变量，使用容器统一管理依赖关系。

2. **配置集中管理**：所有配置通过 `config` 包管理，避免硬编码。

3. **错误处理统一**：使用 `errors` 包统一错误类型，便于错误处理和监控。

4. **中间件组合**：使用 `Chain` 函数组合多个中间件，保持代码清晰。

5. **缓存策略**：根据数据特性设置合适的 TTL，平衡性能和一致性。

6. **资源清理**：使用 `defer container.Close()` 确保资源正确释放。

## 性能优化

1. **缓存优化**：
   - 启用缓存可显著减少 API 调用
   - 根据数据更新频率设置合适的 TTL
   - 使用 `cache.NewNoopCache()` 在测试环境禁用缓存

2. **中间件优化**：
   - 重试中间件可自动处理临时性错误
   - 超时中间件防止请求无限等待
   - 日志中间件仅在需要时启用

3. **并发安全**：
   - 所有组件都是并发安全的
   - 容器使用读写锁保护共享状态

## 未来扩展

1. **上下文支持**：为所有客户端方法添加 `context.Context` 参数
2. **指标收集**：添加 Prometheus 指标收集中间件
3. **分布式追踪**：集成 OpenTelemetry 支持
4. **配置热重载**：支持运行时配置更新
5. **更多缓存后端**：支持 Redis 等外部缓存

## 总结

新架构提供了：

- ✅ 统一的配置管理
- ✅ 清晰的依赖关系
- ✅ 可组合的中间件系统
- ✅ 统一的缓存接口
- ✅ 增强的错误处理
- ✅ 更好的可测试性
- ✅ 更好的可扩展性

这些改进使 SDK 更加健壮、易用和可维护。
