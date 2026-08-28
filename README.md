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

    // 2. 创建 CLOB 客户端（默认使用当前 V2 订单协议）
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

`clob.NewClient` 只使用当前 V2 订单协议。`clob.WithV2()` 仅为旧代码源码兼容
保留，新代码无需传入。生产 CLOB 已不再支持 V1 签名订单，因此 SDK 不提供
V1 回退选项。

> V2 订单不包含 `feeRateBps`。手续费由撮合方在成交时动态处理，SDK 不查询、
> 缓存或接收 fee rate，也不会在下单请求或签名中传入该值。

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
| **RTDS**       | `rtds`       | Chainlink 30/60 秒 TWAP 实时订阅       |
| **RFQ**        | `rfq`        | Combos RFQ maker REST 与 quoter WS     |
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
| `CreateAndPostMarketOrder` | 构造并提交市价单（兼容等待语义） | `marketOrderArgs`                   | `*OrderPostResponse`, `error`         |
| `CreateAndPostMarketOrderInstant` | 构造并立即提交市价单 | `marketOrderArgs`                          | `*OrderPostResponse`, `error`         |
| `CreateAndPostMarketOrderAndWait` | 构造市价单并等待结果 | `ctx`, `marketOrderArgs`                   | `*OrderPostResponse`, `error`         |
| `CancelOrders`           | 取消多个订单（单次最多 3000） | `orderIDs`                            | `*OrderCancelResponse`, `error`       |
| `CancelOrder`            | 取消单个订单           | `orderID`                                  | `*OrderCancelResponse`, `error`       |
| `CancelAll`              | 取消所有订单           | -                                          | `*OrderCancelResponse`, `error`       |
| `CancelMarketOrders`     | 按 condition 取消订单（兼容接口） | `conditionID`                       | `*OrderCancelResponse`, `error`       |
| `CancelMarketOrdersByFilter` | 按 condition/token/交集取消订单 | `params`                           | `*OrderCancelResponse`, `error`       |
| `GetOrderBook`           | 获取订单簿             | `tokenID`                                  | `*OrderBookSummary`, `error`          |
| `GetMultipleOrderBooks`  | 批量获取订单簿         | `requests`                                 | `[]OrderBookSummary`, `error`         |
| `GetMidpoint`            | 获取中间价             | `tokenID`                                  | `*Midpoint`, `error`                  |
| `GetMidpoints`           | 批量获取中间价         | `tokenIDs`                                 | `[]Midpoint`, `error`                 |
| `GetPrice`               | 获取指定方向的价格     | `tokenID`, `side`                          | `*Price`, `error`                     |
| `GetPrices`              | 批量获取价格           | `requests`                                 | `[]Price`, `error`                    |
| `GetSpread`              | 获取价差               | `tokenID`                                  | `*Spread`, `error`                    |
| `GetSpreads`             | 批量获取价差           | `tokenIDs`                                 | `[]Spread`, `error`                   |
| `GetLastTradePrice`      | 获取最后成交价         | `tokenID`                                  | `*LastTradePrice`, `error`            |
| `GetLastTradesPrices`    | 批量获取最后成交价（未成交 token 无 key） | `tokenIDs`                    | `map[TokenID]LastTradePrice`, `error` |
| `CalculateMarketPrice`   | 按订单簿估算市价单最差成交价 | `tokenID`, `side`, `amount`, `orderType` | `float64`, `error`                    |
| `GetNegRisk`            | 获取并缓存市场 negRisk 属性 | `tokenID`                              | `bool`, `error`                       |
| `PrimeNegRisk`          | 用已加载的市场数据预热 negRisk | `tokenID`, `value`                    | `error`                               |
| `InvalidateNegRisk`     | 失效单个 negRisk 缓存项 | `tokenID`                                  | `error`                               |
| `GetTime`                | 获取服务器时间         | -                                          | `time.Time`, `error`                  |
| `GetCollateralBalance`   | 获取 V2 抵押物 pUSD 余额 | -                                      | `float64`, `error`                    |
| `GetUSDCBalance`         | 获取旧 USDC.e 余额（已弃用） | -                                   | `float64`, `error`                    |
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
| `GetMarket`                    | 通过市场ID获取市场（默认包含 tag） | `marketID`                                | `*GammaMarket`, `error`        |
| `GetMarketBySlug`              | 通过slug获取市场（默认包含 tag）   | `slug`                                    | `*GammaMarket`, `error`        |
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
| `GetSeriesSummaryByID`         | 通过 ID 获取系列摘要             | `id`                                        | `*SeriesSummary`, `error`      |
| `GetSeriesSummaryBySlug`       | 通过 slug 获取系列摘要           | `slug`                                      | `*SeriesSummary`, `error`      |
| `GetComments`                  | 按父实体获取评论列表             | `parentType`, `parentID`, `limit`, `offset` | `[]Comment`, `error`           |
| `GetComment`                   | 按 ID 获取评论（官方返回数组）   | `commentID`, `getPositions`                 | `[]Comment`, `error`           |
| `GetProfile`                   | 获取用户资料                     | `address`                                   | `*Profile`, `error`            |
| `GetSamplingSimplifiedMarkets` | 获取采样简化市场                 | `limit`                                     | `[]SimplifiedMarket`, `error`  |
| `GetSamplingMarkets`           | 获取采样市场                     | `limit`                                     | `[]GammaMarket`, `error`       |
| `GetSimplifiedMarkets`         | 获取简化市场列表                 | `limit`, `offset`, `options...`             | `[]SimplifiedMarket`, `error`  |

说明：`GetMarket` 和 `GetMarketBySlug` 会默认发送 `include_tag=true`。

### Data 客户端接口

| 方法            | 描述                     | 参数                                    | 返回值                     |
| --------------- | ------------------------ | --------------------------------------- | -------------------------- |
| `GetPositions`  | 获取用户仓位             | `user`, `options...`                    | `[]Position`, `error`      |
| `GetTrades`     | 获取交易记录             | `limit`, `offset`, `options...`         | `[]Trade`, `error`         |
| `GetActivity`   | 获取用户活动             | `user`, `limit`, `offset`, `options...` | `[]Activity`, `error`      |
| `GetValue`      | 获取完整仓位价值响应数组 | `user`, `conditionIDs`                  | `[]ValueResponse`, `error` |
| `GetTotalValue` | 获取仓位价值合计         | `user`, `conditionIDs`                  | `float64`, `error`         |

`Trade.Timestamp` 和 `Activity.Timestamp` 是官方返回的 Unix 秒 `int64`。Activity 类型应优先使用 `types.ActivityType*` 常量；查询充值或提现时还需传入 `data.WithActivityExcludeDepositsWithdrawals(false)`。
`GetTrades` 默认使用官方的 `takerOnly=true`，可用 `WithTradesTakerOnly(false)` 覆盖；深层历史分页可配合 `WithTradesDateRange(start, end)` 使用独立时间窗口。

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
| `GetCollateralBalance` | 获取 V2 抵押物 pUSD 余额 | `address`       | `float64`, `error`    |
| `GetUSDCEBalance`     | 获取旧 USDC.e 余额 | `address`          | `float64`, `error`    |
| `GetUSDCBalance`      | `GetUSDCEBalance` 的已弃用别名 | `address`       | `float64`, `error`    |
| `GetTokenBalance`     | 获取代币余额   | `tokenID`, `address` | `float64`, `error`    |
| `Close`               | 关闭客户端     | -                    | -                     |

### WebSocket 客户端接口

Market Channel 使用 `websocket.Client`，不需要鉴权：

| 方法                 | 描述               | 参数       | 返回值  |
| -------------------- | ------------------ | ---------- | ------- |
| `SetOnBookUpdate`    | 设置订单簿更新回调 | `callback` | -       |
| `SetOnMarketEvent`   | 设置完整 typed event 回调 | `callback` | -       |
| `Start`              | 启动连接           | `tokenIDs` | `error` |
| `Stop`               | 停止连接           | -          | -       |
| `IsRunning`          | 检查是否运行中     | -          | `bool`  |
| `UpdateSubscription` | 更新订阅           | `tokenIDs` | `error` |
| `SubscribeTokens`    | 订阅 outcome token | `tokenIDs` | `error` |
| `UnsubscribeTokens`  | 取消订阅 token     | `tokenIDs` | `error` |

`NewClient` 使用官方默认值：initial dump 开启、level 2、custom feature 关闭。
需要 `best_bid_ask` / `new_market` / `market_resolved` 时，从
`DefaultMarketSubscriptionOptions` 起步并通过 `NewClientWithOptions` 创建。
`SetOnMarketEvent` 可接收 `MarketBookEvent`、`MarketPriceChangeEvent`、
`MarketLastTradePriceEvent`、`MarketTickSizeChangeEvent`、`MarketBestBidAskEvent`、
`MarketNewMarketEvent` 和 `MarketResolvedEvent`。重连可能重放快照或事件，
业务层更新缓存时应保持幂等。

User Channel 使用独立 `websocket.UserClient`，构造时传入 CLOB `types.ApiCreds`：
动态订阅/退订需要 `Start` 时传入显式 ConditionID；`Start(nil)` 的全市场模式不支持动态改为局部过滤。

| 方法                 | 描述                          | 参数       | 返回值  |
| -------------------- | ----------------------------- | ---------- | ------- |
| `SetOnOrderUpdate`   | 设置订单生命周期事件回调        | `callback` | -       |
| `SetOnTradeUpdate`   | 设置交易生命周期事件回调        | `callback` | -       |
| `Start`              | 启动连接；空 markets 监听全部市场 | `markets`  | `error` |
| `SubscribeMarkets`   | 动态订阅 ConditionID         | `markets`  | `error` |
| `UnsubscribeMarkets` | 动态退订 ConditionID         | `markets`  | `error` |
| `Stop`               | 停止连接                      | -          | -       |
| `IsRunning`          | 检查是否运行中                | -          | `bool`  |

Sports Channel 使用 `websocket.SportsClient`，连接后直接接收全部比赛更新，不需要鉴权或订阅报文：

| 方法                | 描述                     | 参数       | 返回值  |
| ------------------- | ------------------------ | ---------- | ------- |
| `SetOnSportsUpdate` | 设置 `SportResult` 更新回调 | `callback` | -       |
| `Start`             | 启动公开比分连接           | -          | `error` |
| `Stop`              | 停止连接                 | -          | -       |
| `IsRunning`         | 检查是否运行中           | -          | `bool`  |

### RTDS 客户端接口

| 方法               | 描述                          | 参数            | 返回值  |
| ------------------ | ----------------------------- | --------------- | ------- |
| `SetOnTWAPUpdate`  | 设置精确 TWAP 更新回调        | `callback`      | -       |
| `Start`            | 启动连接并恢复订阅            | -               | `error` |
| `Stop`             | 停止连接                      | -               | -       |
| `IsRunning`        | 检查是否运行中                | -               | `bool`  |
| `Subscribe`        | 订阅 30/60 秒窗口及可选 symbol | `subscriptions` | `error` |
| `Unsubscribe`      | 取消订阅                      | `subscriptions` | `error` |

### RFQ 客户端接口

REST `Client` 提供公开的 `GetComboMarkets`，以及使用 CLOB L2 鉴权的
`SubmitQuote`、`CancelQuote`、`Confirm`。`WebSocketClient` 负责 quoter 身份认证、
重连和 `RFQ_REQUEST`/last-look/execution 等事件，并提供 `SendQuote`、
`CancelQuote`、`Confirm`。旧 CLOB `/rfq/*` requester 方法已删除。

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
    // 必填；可来自业务配置、Gamma/WS，或提前调用 clobClient.GetTickSize(tokenID)
    TickSize: types.TickSize0_01,
}

// 提交订单
response, err := clobClient.PostOrder(orderArgs, types.OrderTypeGTC)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("订单ID: %s\n", response.OrderID)
```

市价单在 CLOB 上仍是立即可成交的 FOK/FAK 限价单。BUY 的业务数量是
pUSD 金额，SELL 的业务数量是 outcome token 份额：

```go
// 低延迟 BUY：业务层已有实时盘口时显式传入 MaxPrice，SDK 不再请求订单簿。
buy, err := clobClient.CreateAndPostMarketOrderInstant(types.MarketOrderArgs{
    TokenID:  "token-id",
    Side:     types.OrderSideBUY,
    Amount:   5.00, // 最多花 5.00 pUSD
    MaxPrice: 0.51,
    TickSize: types.TickSize0_01,
    OrderType: types.OrderTypeFOK,
    NegRisk:  &negRisk,
})

// SELL：卖出 5 shares，最低接受 0.50；ctx 控制等待成交/结算结果的时间。
sell, err := clobClient.CreateAndPostMarketOrderAndWait(ctx, types.MarketOrderArgs{
    TokenID:  "token-id",
    Side:     types.OrderSideSELL,
    Shares:   5,
    MinPrice: 0.50,
    TickSize: types.TickSize0_01,
    OrderType: types.OrderTypeFOK,
    NegRisk:  &negRisk,
})
```

`MaxPrice`（BUY）或 `MinPrice`（SELL）为零时，SDK 会先请求当前订单簿：FOK
深度不足会返回可用 `errors.As` 判断的 `*clob.InsufficientLiquidityError`；FAK
会使用当前最差可成交档位，允许部分成交后取消剩余数量。显式保护价是低延迟路径，
SDK 只做 tick、价格范围、金额精度和默认最小 5 shares 校验，不会再查盘口；调用方
应保证价格数据新鲜。`OrderType` 零值默认 FOK，市价单不接受 GTC/GTD。

批量下单时每笔订单都必须显式携带自己的 `TickSize`，同一批可以不同：

```go
orders := []types.OrderArgs{
    {TokenID: "token-a", Price: 0.01, Size: 5, Side: types.OrderSideBUY, TickSize: types.TickSize0_01},
    {TokenID: "token-b", Price: 0.001, Size: 5, Side: types.OrderSideBUY, TickSize: types.TickSize0_001},
}
responses, err := clobClient.CreateAndPostOrders(orders, []types.OrderType{
    types.OrderTypeGTC,
    types.OrderTypeGTC,
})
```

`TickSize` 只用于 SDK 的本地校验、金额计算和签名，不会作为额外字段发给官方 `/orders`；官方请求格式中没有该字段。业务层可用配置、Gamma/WS 数据，或在非热路径显式调用 `GetTickSize(tokenID)` 准备它。`Price` 必须是对应 `TickSize` 的整数倍；非网格价格会在整批签名和提交前报错，SDK 不会自动舍入到其他价格。SDK 保留默认最小 `Size=5`；传入更小数量时整批会在签名前报错，不会静默改成 5，也不会在下单热路径查询市场配置。

`CreateAndPostOrders` 的单个 HTTP 子批最多 15 单；输入更多时 SDK 会切批。一旦开始提交，返回结果始终与输入等长同序：

- `market_closed`：已确认 `orderbook does not exist`，是“业务已正确处理、无需重试”的终态；不代表交易所已接单，也没有创建订单。
- `not_submitted`：可确定未提交，例如本地签名失败、顶层 HTTP 4xx，或同一子批被关闭订单簿拒绝时的其他订单。
- `unknown`：请求可能已发出，但因超时/网络错误无法确认服务端结果；应先查单对账，不要盲目重试。
- `server_rejected`：HTTP 200 中的逐单业务拒绝，例如余额/授权不足；其他订单仍可以成功。

如果一个子批全部是同一个已关闭 token，返回 `market_closed` 且 `error=nil`。如果子批混有其他 token，SDK 不会拆掉关闭 token 后重发；它会返回对齐结果和非空 error。跨 15 单切批时，首个失败子批会终止后续提交，后续位置标记为 `not_submitted`。

撤单接口统一返回 `OrderCancelResponse`：`Canceled` 是成功撤销的订单 ID，
`NotCanceled` 是订单 ID 到失败原因的 typed map；即使服务端省略空字段，SDK 也会
归一化为非 nil 的空集合。`CancelOrders` 单次最多接收 3000 个 ID，超出或包含非法
ID 时会在网络请求前报错，不会自动切批产生部分撤单状态。

按市场撤单时，旧 `CancelMarketOrders(conditionID)` 保留兼容；新代码可显式选择
condition、token 或两者交集：

```go
conditionID, _ := types.ParseConditionID("0x...")
tokenID, _ := types.ParseTokenID("123...")

result, err := clobClient.CancelMarketOrdersByFilter(types.CancelMarketOrdersParams{
    ConditionID: conditionID, // 可单独使用
    TokenID:     tokenID,     // 可单独使用；两者都有时取交集
})
```

两个字段不能同时为零值，以防空请求意外扩大撤单范围。SDK 对外统一使用
`ConditionID/TokenID`，发送给官方接口时分别编码为 `market/asset_id`。

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

批量市场数据响应是稀疏结果：关闭或不存在的 token 可能不出现在返回切片中，不能用响应下标对应请求下标。SDK 提供紧凑的 `types.TokenID`（uint256）和 `types.ConditionID`（bytes32）作为可比较的 `[32]byte` map key：

```go
books, err := clobClient.GetMultipleOrderBooks(requests)
if err != nil {
    log.Fatal(err)
}

booksByToken := make(map[types.TokenID]types.OrderBookSummary, len(books))
for _, book := range books {
    tokenID, err := types.ParseTokenID(book.TokenID)
    if err != nil {
        log.Printf("invalid token ID from server: %v", err)
        continue
    }
    booksByToken[tokenID] = book
}
```

`GetOrderBook` 与 `GetMultipleOrderBooks` 共用完整的 `OrderBookSummary`；旧的
`OrderBookSummaryResponse` 名称保留为类型别名。盘口的 price/size、最小下单量和
最后成交价使用 `DecimalString` 保留原始十进制精度；需要浮点运算时显式调用
`value.Float64()` 并处理错误。`LastTradePrice == nil` 表示该 token 尚无成交价。
SDK 统一把官方 wire 字段 `asset_id/assets_ids` 暴露为 `TokenID/TokenIDs`；JSON
编解码仍使用官方字段名，业务层不需要感知 `asset_id` 这一传输层命名。

`String()` 会恢复官方规范字符串：`ConditionID` 输出小写 `0x` + 64 位 hex，`TokenID` 输出无前导零的 uint256 十进制；二者均支持文本/JSON 值和 JSON map key 编解码。现有请求与响应字段继续使用字符串，业务可只在需要构建字典时转换，不破坏旧代码。

### WebSocket 实时数据订阅

```go
import (
    "time"

    "github.com/polymas/go-polymarket-sdk/websocket"
)

// 公开 Market Channel
wsClient := websocket.NewClient(time.Second)

// 设置回调函数
wsClient.SetOnBookUpdate(func(tokenID string, snapshot *types.BookSnapshot) {
    fmt.Printf("订单簿更新: %s - Bid: %.4f, Ask: %.4f\n", 
        tokenID, snapshot.BestBid.Price, snapshot.BestAsk.Price)
})

// 启动连接
tokenIDs := []string{"token1", "token2"}
err := wsClient.Start(tokenIDs)
if err != nil {
    log.Fatal(err)
}

// 保持运行...
defer wsClient.Stop()

// 鉴权 User Channel；creds 可来自 clobClient.GetAPICreds()
userClient := websocket.NewUserClient(*clobClient.GetAPICreds(), time.Second)
userClient.SetOnOrderUpdate(func(event *websocket.UserOrderEvent) {
    fmt.Printf("订单更新: %s - Status: %s\n", event.ID, event.Status)
})
userClient.SetOnTradeUpdate(func(event *websocket.UserTradeEvent) {
    fmt.Printf("交易更新: %s - Status: %s\n", event.ID, event.Status)
})
if err := userClient.Start(nil); err != nil { // nil 表示监听账户全部市场
    log.Fatal(err)
}
defer userClient.Stop()
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
- `SignatureType`: 签名类型，支持以下四种并存：
  - `EOASignatureType` (0)：普通 EOA，maker == signer == EOA。
  - `ProxySignatureType` (1)：老版 PolyProxy，工厂 `0x4bFb…982E`，派生函数 `getPolyProxyWalletAddress(address)`。
  - `SafeSignatureType` (2)：Gnosis Safe，工厂 `0xaacF…541b`，派生函数 `computeProxyAddress(address)`。
  - `CWIASignatureType` (3)：新版 Polymarket DepositWallet（ERC-7760 / ERC-1967 + immutable args，新注册账号默认走这套）。
    - 工厂：`0x00000000000Fb5C9ADea0298D729A0CB3823Cc07`
    - Legacy UUPS impl：`0x58cA52EbE0dAdFDf531CDe7062E76746de4Db1eB`；新钱包通过工厂 `BEACON()` 获取 Beacon 地址
    - 地址解析：本地分别派生 legacy UUPS 与新 BeaconProxy CREATE2 地址；若旧 UUPS 已部署则始终保留旧地址，只有未部署时才采用工厂 `BEACON()` 对应的新地址。
    - 与老两类型并存，老账号不迁移；签名仍走 EIP-1271（Exchange 调用代理 `isValidSignature`）。
    - **链上 gasless（v1.10.0+）**：`signing.SignCWIABatch` + `web3.BuildCWIABatchCalldata` 已对照真实链上 tx 字节级匹配，可直接广播 `DepositWalletFactory.proxy(Batch[],bytes[])`（需 operator 角色）。
    - **走 Polymarket relayer 提交（v1.10.1+）**：`NewGaslessClient(..., CWIASignatureType, ...)` 已可创建；高层方法（SplitPosition / MergePositions / RedeemPositions / SetAutoClaim 等）会自动走 CWIA 分支。`CWIARelayBody` 已对照官方 [py-builder-relayer-client](https://github.com/Polymarket/py-builder-relayer-client) 中 `DepositWalletBatchRequest.to_dict()` 校准，包含 `depositWalletParams` 嵌套结构（depositWallet/deadline/calls）。
    - **首次部署（v1.10.6+）**：新注册 EOA 的 DepositWallet 是 counter-factual 地址，发批量交易前需先部署。调用 `gaslessClient.DeployDepositWallet(false)` 走 `type=WALLET-CREATE`；force=false 时已部署的钱包会自动跳过。如果在未部署的钱包上直接调 batch 方法，会得到 sentinel `web3.ErrDepositWalletNotDeployed`，业务层可 `errors.Is` 判断并触发 deploy。
    - **POLY_1271 签名（v1.10.4+）**：CLOB v2 订单签名采用 solady ERC-1271 嵌套 TypedDataSign 格式（~317 字节，含 inner_sig + appDomainSep + contents_hash + ORDER_TYPE_STRING + uint16 长度），不是普通 EIP-712 ECDSA。已对照 py-clob-client-v2 字节级一致。
- `OrderSide`: 订单方向（BUY/SELL）
- `OrderType`: 订单类型（GTC/FOK/GTD/FAK）；GTD 至少提前 3 分钟，PostOnly 仅支持 GTC/GTD
- `OrderArgs`: 订单参数
- `MarketOrderArgs`: 市价单参数；BUY 使用 `Amount/MaxPrice`，SELL 使用 `Shares/MinPrice`
- `CancelMarketOrdersParams`: 按 condition、token 或两者交集撤单的过滤参数
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
