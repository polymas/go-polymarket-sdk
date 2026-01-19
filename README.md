# Go Polymarket SDK

[![Go Version](https://img.shields.io/badge/go-1.24+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Go 语言编写的 Polymarket SDK，提供完整的 Polymarket 平台 API 访问能力，包括订单交易、市场数据、WebSocket 实时数据等功能。

## 📋 目录

- [功能特性](#功能特性)
- [安装](#安装)
- [快速开始](#快速开始)
- [主要模块](#主要模块)
- [API 接口文档](#api-接口文档)
- [使用示例](#使用示例)
- [配置说明](#配置说明)
- [架构设计](#架构设计)
- [最佳实践](#最佳实践)
- [测试](#测试)
- [贡献](#贡献)
- [许可证](#许可证)

## ✨ 功能特性

- 🚀 **完整的 API 覆盖**：支持 CLOB、Gamma、Data、WebSocket、RTDS 等所有主要 API
- 🔐 **多种钱包支持**：支持 EOA、Proxy Wallet、Safe/Gnosis Wallet
- 📊 **实时数据流**：WebSocket 和 RTDS 实时数据订阅
- 🛡️ **健壮的错误处理**：统一的错误类型和处理机制
- ⚡ **高性能**：内置缓存、重试机制、并发安全
- 🔧 **灵活配置**：统一的配置管理和依赖注入
- 📝 **类型安全**：完整的类型定义和接口抽象

## 📦 安装

```bash
go get github.com/polymas/go-polymarket-sdk
```

## 🚀 快速开始

### 基本使用

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/polymas/go-polymarket-sdk/clob"
    "github.com/polymas/go-polymarket-sdk/gamma"
    "github.com/polymas/go-polymarket-sdk/types"
    "github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
    // 1. 创建 Web3 客户端
    privateKey := "your-private-key"
    web3Client, err := web3.NewClient(
        privateKey,
        types.EOASignatureType,
        types.Polygon,
    )
    if err != nil {
        log.Fatal(err)
    }
    defer web3Client.Close()

    // 2. 创建 CLOB 客户端（需要 Web3 客户端）
    clobClient, err := clob.NewClient(web3Client)
    if err != nil {
        log.Fatal(err)
    }

    // 3. 创建 Gamma 客户端（只读，无需认证）
    gammaClient := gamma.NewClient()

    // 4. 使用客户端
    // 获取订单簿
    orderBook, err := clobClient.GetOrderBook("token-id")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("订单簿: %+v\n", orderBook)

    // 获取市场信息
    market, err := gammaClient.GetMarket("market-id")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("市场: %+v\n", market)
}
```

### 更多示例

查看 `examples/` 目录获取更多使用示例。

## 📚 主要模块

| 模块           | 包路径       | 功能描述                               |
| -------------- | ------------ | -------------------------------------- |
| **CLOB**       | `clob`       | 中央限价订单簿，订单交易、市场数据查询 |
| **Gamma**      | `gamma`      | 市场信息、事件、标签、系列、评论等     |
| **Data**       | `data`       | 用户仓位、交易记录、活动数据           |
| **Web3**       | `web3`       | 区块链交互、余额查询、代理钱包管理     |
| **WebSocket**  | `websocket`  | 实时订单簿、订单、交易数据订阅         |
| **RTDS**       | `rtds`       | 实时价格和评论更新                     |
| **Subgraph**   | `subgraph`   | GraphQL 查询，市场数据、用户数据       |
| **RFQ**        | `rfq`        | 请求报价（Request for Quote）功能      |
| **Cache**      | `cache`      | 统一缓存管理（可选）                   |
| **Middleware** | `middleware` | HTTP 中间件系统（可选）                |
| **Errors**     | `errors`     | 统一错误处理（可选）                   |

## 📖 API 接口文档

### CLOB 客户端接口

| 方法                     | 描述                   | 参数                                       | 返回值                                |
| ------------------------ | ---------------------- | ------------------------------------------ | ------------------------------------- |
| `GetOrders`              | 获取活跃订单           | `orderID`, `conditionID`, `tokenID` (可选) | `[]OpenOrder`, `error`                |
| `CreateAndPostOrders`    | 创建并提交多个订单     | `orderArgsList`, `orderTypes`              | `[]OrderPostResponse`, `error`        |
| `PostOrder`              | 提交单个订单           | `orderArgs`, `orderType`                   | `*OrderPostResponse`, `error`         |
| `CancelOrders`           | 取消多个订单           | `orderIDs`                                 | `*OrderCancelResponse`, `error`       |
| `CancelOrder`            | 取消单个订单           | `orderID`                                  | `*OrderCancelResponse`, `error`       |
| `CancelAll`              | 取消所有订单           | -                                          | `*OrderCancelResponse`, `error`       |
| `CancelMarketOrders`     | 取消指定市场的所有订单 | `conditionID`                              | `*OrderCancelResponse`, `error`       |
| `GetOrderBook`           | 获取订单簿             | `tokenID`                                  | `*OrderBookSummary`, `error`          |
| `GetMultipleOrderBooks`  | 批量获取订单簿         | `requests`                                 | `[]OrderBookSummaryResponse`, `error` |
| `GetMidpoint`            | 获取中间价             | `tokenID`                                  | `*Midpoint`, `error`                  |
| `GetMidpoints`           | 批量获取中间价         | `tokenIDs`                                 | `[]Midpoint`, `error`                 |
| `GetPrice`               | 获取指定方向的价格     | `tokenID`, `side`                          | `*Price`, `error`                     |
| `GetPrices`              | 批量获取价格           | `requests`                                 | `[]Price`, `error`                    |
| `GetSpread`              | 获取价差               | `tokenID`                                  | `*Spread`, `error`                    |
| `GetSpreads`             | 批量获取价差           | `tokenIDs`                                 | `[]Spread`, `error`                   |
| `GetLastTradePrice`      | 获取最后成交价         | `tokenID`                                  | `*LastTradePrice`, `error`            |
| `GetLastTradesPrices`    | 批量获取最后成交价     | `tokenIDs`                                 | `[]LastTradePrice`, `error`           |
| `GetFeeRate`             | 获取手续费率           | `tokenID`                                  | `int`, `error`                        |
| `GetTime`                | 获取服务器时间         | -                                          | `time.Time`, `error`                  |
| `GetUSDCBalance`         | 获取 USDC 余额         | -                                          | `float64`, `error`                    |
| `GetBalanceAllowance`    | 获取余额授权信息       | -                                          | `*BalanceAllowance`, `error`          |
| `UpdateBalanceAllowance` | 更新余额授权           | `amount`                                   | `*BalanceAllowance`, `error`          |
| `GetNotifications`       | 获取通知列表           | `limit`, `offset`                          | `[]Notification`, `error`             |
| `DropNotifications`      | 删除通知               | `notificationIDs`                          | `error`                               |
| `IsOrderScoring`         | 检查订单是否计分       | `orderID`                                  | `bool`, `error`                       |
| `AreOrdersScoring`       | 批量检查订单是否计分   | `orderIDs`                                 | `map[Keccak256]bool`, `error`         |
| `GetAPIKeys`             | 获取所有 API 密钥      | -                                          | `[]APIKey`, `error`                   |
| `DeleteAPIKey`           | 删除 API 密钥          | `keyID`                                    | `error`                               |
| `CreateReadonlyAPIKey`   | 创建只读 API 密钥      | -                                          | `*APIKey`, `error`                    |
| `GetReadonlyAPIKeys`     | 获取只读 API 密钥列表  | -                                          | `[]APIKey`, `error`                   |
| `DeleteReadonlyAPIKey`   | 删除只读 API 密钥      | `keyID`                                    | `error`                               |

### Gamma 客户端接口

| 方法                           | 描述                             | 参数                                        | 返回值                         |
| ------------------------------ | -------------------------------- | ------------------------------------------- | ------------------------------ |
| `GetMarket`                    | 通过市场ID获取市场               | `marketID`                                  | `*GammaMarket`, `error`        |
| `GetMarketBySlug`              | 通过slug获取市场                 | `slug`, `includeTag`                        | `*GammaMarket`, `error`        |
| `GetMarketsByConditionIDs`     | 通过条件ID批量获取市场           | `conditionIDs`                              | `[]GammaMarket`, `error`       |
| `GetMarkets`                   | 获取市场列表（支持分页和过滤）   | `limit`, `options...`                       | `[]GammaMarket`, `error`       |
| `GetCertaintyMarkets`          | 获取 Certainty 市场（尾盘市场）  | -                                           | `[]GammaMarket`, `error`       |
| `GetDisputeMarkets`            | 获取争议市场                     | -                                           | `[]GammaMarket`, `error`       |
| `GetAllMarkets`                | 获取所有历史市场数据（自动分页） | -                                           | `[]GammaMarket`, `error`       |
| `GetEvent`                     | 获取事件                         | `eventID`, `includeChat`, `includeTemplate` | `*Event`, `error`              |
| `GetEventBySlug`               | 通过slug获取事件                 | `slug`, `includeChat`, `includeTemplate`    | `*Event`, `error`              |
| `GetEvents`                    | 获取事件列表                     | `limit`, `offset`, `options...`             | `[]Event`, `error`             |
| `Search`                       | 搜索                             | `query`, `options...`                       | `*SearchResult`, `error`       |
| `GetTags`                      | 获取标签列表                     | `limit`, `offset`, `options...`             | `[]Tag`, `error`               |
| `GetTag`                       | 获取标签                         | `tagID`                                     | `*Tag`, `error`                |
| `GetTagBySlug`                 | 通过slug获取标签                 | `slug`                                      | `*Tag`, `error`                |
| `GetSeries`                    | 获取系列列表                     | `limit`, `offset`, `options...`             | `[]Series`, `error`            |
| `GetSeriesBySlug`              | 通过slug获取系列                 | `slug`                                      | `*Series`, `error`             |
| `GetComments`                  | 获取评论列表                     | `marketID`, `limit`, `offset`               | `[]Comment`, `error`           |
| `GetComment`                   | 获取评论                         | `commentID`                                 | `*Comment`, `error`            |
| `GetProfile`                   | 获取用户资料                     | `address`                                   | `*Profile`, `error`            |
| `GetProfileByUsername`         | 通过用户名获取用户资料           | `username`                                  | `*Profile`, `error`            |
| `GetSamplingSimplifiedMarkets` | 获取采样简化市场                 | `limit`                                     | `[]SimplifiedMarket`, `error`  |
| `GetSamplingMarkets`           | 获取采样市场                     | `limit`                                     | `[]GammaMarket`, `error`       |
| `GetSimplifiedMarkets`         | 获取简化市场列表                 | `limit`, `offset`, `options...`             | `[]SimplifiedMarket`, `error`  |
| `GetMarketTradesEvents`        | 获取市场交易事件                 | `marketID`, `limit`, `offset`               | `[]MarketTradesEvent`, `error` |

### Data 客户端接口

| 方法           | 描述         | 参数                                    | 返回值                    |
| -------------- | ------------ | --------------------------------------- | ------------------------- |
| `GetPositions` | 获取用户仓位 | `user`, `options...`                    | `[]Position`, `error`     |
| `GetTrades`    | 获取交易记录 | `limit`, `offset`, `options...`         | `[]Trade`, `error`        |
| `GetActivity`  | 获取用户活动 | `user`, `limit`, `offset`, `options...` | `[]Activity`, `error`     |
| `GetValue`     | 获取仓位价值 | `user`, `conditionIDs`                  | `*ValueResponse`, `error` |

### Web3 客户端接口

| 方法                  | 描述           | 参数                 | 返回值                |
| --------------------- | -------------- | -------------------- | --------------------- |
| `GetSigner`           | 获取签名器     | -                    | `*Signer`             |
| `GetPrivateKey`       | 获取私钥       | -                    | `*ecdsa.PrivateKey`   |
| `GetBaseAddress`      | 获取基础地址   | -                    | `EthAddress`          |
| `GetPolyProxyAddress` | 获取代理地址   | -                    | `EthAddress`, `error` |
| `GetChainID`          | 获取链ID       | -                    | `ChainID`             |
| `GetSignatureType`    | 获取签名类型   | -                    | `SignatureType`       |
| `GetPOLBalance`       | 获取 POL 余额  | -                    | `float64`, `error`    |
| `GetUSDCBalance`      | 获取 USDC 余额 | `address`            | `float64`, `error`    |
| `GetTokenBalance`     | 获取代币余额   | `tokenID`, `address` | `float64`, `error`    |
| `Close`               | 关闭客户端     | -                    | -                     |

### WebSocket 客户端接口

| 方法                 | 描述               | 参数       | 返回值  |
| -------------------- | ------------------ | ---------- | ------- |
| `SetOnBookUpdate`    | 设置订单簿更新回调 | `callback` | -       |
| `SetOnOrderUpdate`   | 设置订单更新回调   | `callback` | -       |
| `SetOnTradeUpdate`   | 设置交易更新回调   | `callback` | -       |
| `SetAuth`            | 设置认证信息       | `auth`     | -       |
| `Start`              | 启动连接           | `assetIDs` | `error` |
| `Stop`               | 停止连接           | -          | -       |
| `IsRunning`          | 检查是否运行中     | -          | `bool`  |
| `UpdateSubscription` | 更新订阅           | `assetIDs` | `error` |
| `SubscribeAssets`    | 订阅资产           | `assetIDs` | `error` |
| `UnsubscribeAssets`  | 取消订阅资产       | `assetIDs` | `error` |
| `StartUserChannel`   | 启动用户频道       | -          | `error` |
| `StopUserChannel`    | 停止用户频道       | -          | -       |

### RTDS 客户端接口

| 方法                  | 描述             | 参数        | 返回值  |
| --------------------- | ---------------- | ----------- | ------- |
| `SetOnPriceUpdate`    | 设置价格更新回调 | `callback`  | -       |
| `SetOnCommentUpdate`  | 设置评论更新回调 | `callback`  | -       |
| `SetAuth`             | 设置认证信息     | `auth`      | -       |
| `Start`               | 启动连接         | -           | `error` |
| `Stop`                | 停止连接         | -           | -       |
| `IsRunning`           | 检查是否运行中   | -           | `bool`  |
| `SubscribePrices`     | 订阅价格         | `tokenIDs`  | `error` |
| `UnsubscribePrices`   | 取消订阅价格     | `tokenIDs`  | `error` |
| `SubscribeComments`   | 订阅评论         | `marketIDs` | `error` |
| `UnsubscribeComments` | 取消订阅评论     | `marketIDs` | `error` |

### Subgraph 客户端接口

| 方法                    | 描述              | 参数                               | 返回值                         |
| ----------------------- | ----------------- | ---------------------------------- | ------------------------------ |
| `Query`                 | 执行 GraphQL 查询 | `query`, `variables`               | `*GraphQLResponse`, `error`    |
| `GetMarketVolume`       | 获取市场交易量    | `marketID`, `startTime`, `endTime` | `*MarketVolume`, `error`       |
| `GetUserPositions`      | 获取用户仓位      | `userAddress`                      | `[]GQLPosition`, `error`       |
| `GetMarketOpenInterest` | 获取市场未平仓量  | `marketID`                         | `*MarketOpenInterest`, `error` |
| `GetUserPNL`            | 获取用户盈亏      | `userAddress`                      | `*UserPNL`, `error`            |

### RFQ 客户端接口

| 方法            | 描述         | 参数        | 返回值                        |
| --------------- | ------------ | ----------- | ----------------------------- |
| `RequestQuote`  | 请求报价     | `request`   | `*RFQResponse`, `error`       |
| `GetQuotes`     | 获取报价列表 | `requestID` | `[]RFQQuote`, `error`         |
| `AcceptQuote`   | 接受报价     | `quoteID`   | `*RFQAcceptResponse`, `error` |
| `CancelRequest` | 取消请求     | `requestID` | `error`                       |

## 💡 使用示例

### 创建和提交订单

```go
import (
    "github.com/polymas/go-polymarket-sdk/clob"
    "github.com/polymas/go-polymarket-sdk/types"
)

// 创建订单参数
orderArgs := types.OrderArgs{
    TokenID: "token-id",
    Price:   0.5,
    Size:    10.0,
    Side:    types.OrderSideBUY,
}

// 提交订单
response, err := clobClient.PostOrder(orderArgs, types.OrderTypeGTC)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("订单ID: %s\n", response.OrderID)
```

### 批量获取市场数据

```go
// 批量获取中间价
tokenIDs := []string{"token1", "token2", "token3"}
midpoints, err := clobClient.GetMidpoints(tokenIDs)
if err != nil {
    log.Fatal(err)
}

for _, mp := range midpoints {
    fmt.Printf("Token: %s, Midpoint: %.4f\n", mp.TokenID, mp.Midpoint)
}
```

### WebSocket 实时数据订阅

```go
import "github.com/polymas/go-polymarket-sdk/websocket"

// 创建 WebSocket 客户端
wsClient := websocket.NewClient()

// 设置回调函数
wsClient.SetOnBookUpdate(func(assetID string, snapshot *types.BookSnapshot) {
    fmt.Printf("订单簿更新: %s - Bid: %.4f, Ask: %.4f\n", 
        assetID, snapshot.BestBid.Price, snapshot.BestAsk.Price)
})

wsClient.SetOnOrderUpdate(func(order *types.OpenOrder) {
    fmt.Printf("订单更新: %s - Status: %s\n", order.ID, order.Status)
})

// 启动连接
assetIDs := []string{"token1", "token2"}
err := wsClient.Start(assetIDs)
if err != nil {
    log.Fatal(err)
}

// 保持运行...
defer wsClient.Stop()
```

### 获取市场信息

```go
// 获取市场列表
markets, err := gammaClient.GetMarkets(100, 
    gamma.WithMarketsActive(true),
    gamma.WithMarketsOrder("volume"),
    gamma.WithMarketsAscending(false),
)
if err != nil {
    log.Fatal(err)
}

for _, market := range markets {
    fmt.Printf("市场: %s - 交易量: %.2f\n", market.Question, market.Volume)
}
```

### 获取用户仓位

```go
import "github.com/polymas/go-polymarket-sdk/data"

dataClient := data.NewClient()

positions, err := dataClient.GetPositions(
    types.EthAddress("0x..."),
    data.WithPositionsLimit(100),
    data.WithPositionsConditionID("condition-id"),
)
if err != nil {
    log.Fatal(err)
}

for _, pos := range positions {
    fmt.Printf("仓位: %s - 数量: %.2f\n", pos.TokenID, pos.Size)
}
```

## ⚙️ 配置说明

### 使用配置管理

```go
import (
    "github.com/polymas/go-polymarket-sdk/config"
    "github.com/polymas/go-polymarket-sdk/types"
    "time"
)

// 创建自定义配置
cfg := config.NewConfig(
    // 链配置
    config.WithChainID(types.Polygon),           // 或 types.Amoy
    config.WithSignatureType(types.ProxySignatureType),
    
    // HTTP 配置
    config.WithHTTPTimeout(30 * time.Second),
    config.WithMaxRetries(3),
    
    // 缓存配置
    config.WithCacheEnabled(true),
    
    // 日志配置
    config.WithLogLevel("DEBUG"),  // DEBUG, INFO, WARN, ERROR
    
    // API 域名（可选）
    config.WithClobDomain("https://clob.polymarket.com"),
    config.WithGammaDomain("https://gamma-api.polymarket.com"),
)
```

### 环境变量配置

```bash
# 日志级别
export LOG_LEVEL=DEBUG

# 链配置（测试时）
export POLY_CHAIN_ID=80002  # Amoy testnet
export POLY_SIGNATURE_TYPE=1  # Proxy wallet
```

## 🏗️ 架构设计

SDK 采用现代化的 Go 架构模式：

- **配置管理** (`config`): 统一的配置管理，支持函数式选项
- **依赖注入** (`container`): 管理所有依赖关系
- **中间件系统** (`middleware`): 可组合的 HTTP 中间件（重试、日志、超时）
- **缓存管理** (`cache`): 统一的缓存接口和实现
- **错误处理** (`errors`): 统一的错误类型和处理机制

详细架构说明请参考 [ARCHITECTURE_REFACTOR.md](./ARCHITECTURE_REFACTOR.md)。

## 🎯 最佳实践

### 1. 错误处理

```go
import "github.com/polymas/go-polymarket-sdk/errors"

result, err := clobClient.GetOrderBook(tokenID)
if err != nil {
    if errors.IsRetryableError(err) {
        // 可重试的错误
        // 可以在这里实现重试逻辑
    }
    
    switch errors.GetErrorType(err) {
    case errors.ErrorTypeNetwork:
        // 网络错误处理
    case errors.ErrorTypeAPI:
        // API 错误处理
    case errors.ErrorTypeAuth:
        // 认证错误处理
    }
    return err
}
```

### 2. 使用缓存

```go
import "github.com/polymas/go-polymarket-sdk/cache"

cache := cache.NewMemoryCache()

// 检查缓存
if value, ok := cache.Get("key"); ok {
    return value
}

// 设置缓存
cache.Set("key", value, 5 * time.Minute)
```

### 3. 资源清理

```go
// 始终使用 defer 清理资源
defer web3Client.Close()
defer wsClient.Stop()
defer container.Close()
```

### 4. 并发安全

所有客户端都是并发安全的，可以在多个 goroutine 中安全使用。

## 🧪 测试

运行测试：

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./clob/...

# 运行测试并显示覆盖率
go test -cover ./...
```

测试需要配置环境变量，详见 [TEST_README.md](./TEST_README.md)。

## 📝 类型定义

所有类型定义在 `types` 包中，主要类型包括：

- `EthAddress`: 以太坊地址
- `Keccak256`: Keccak256 哈希
- `ChainID`: 链 ID
- `SignatureType`: 签名类型
- `OrderSide`: 订单方向（BUY/SELL）
- `OrderType`: 订单类型（GTC/FOK/FAK/IOC）
- `OrderArgs`: 订单参数
- `OpenOrder`: 开放订单
- `OrderBookSummary`: 订单簿摘要
- `GammaMarket`: Gamma 市场
- `Position`: 仓位
- `Trade`: 交易

## 🤝 贡献

欢迎贡献！请遵循以下步骤：

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 🔗 相关链接

- [Polymarket API 文档](https://docs.polymarket.com)
- [Go 官方文档](https://golang.org/doc/)
- [架构重构指南](./ARCHITECTURE_REFACTOR.md)
- [测试文档](./TEST_README.md)

## 📞 支持

如有问题或建议，请：

- 提交 [Issue](https://github.com/polymas/go-polymarket-sdk/issues)
- 查看 [文档](./ARCHITECTURE_REFACTOR.md)
- 参考 [示例代码](./examples/)

---

**注意**: 使用本 SDK 进行交易时，请确保了解相关风险，并遵守 Polymarket 的使用条款。
