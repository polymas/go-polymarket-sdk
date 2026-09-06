# API 与包概览

SDK 按 Polymarket 服务边界拆包，不提供需要全量初始化的顶层 client。应用只需依赖实际使用的包。

## 公共 API 包

| 包 | 主要入口 | 职责 |
| --- | --- | --- |
| `clob` | `NewReadonlyClient`, `NewClient` | CLOB 市场数据与 V2 交易 |
| `gamma` | `NewClient` | 市场目录、事件与元数据 |
| `data` | `NewClient` | 用户仓位、成交与统计数据 |
| `websocket` | `NewClient`, `NewUserClient`, `NewSportsClient` | market/user/sports 实时订阅 |
| `rtds` | `NewClient` | RTDS 订阅 |
| `rfq` | `NewClient`, `NewWebSocketClient` | Combos RFQ |
| `web3` | `NewClient` | Polygon 只读查询、钱包地址解析 |
| `web3/relayer` | `NewGaslessClient` | Gasless 中继交易、授权与 onboarding |
| `chainws` | `NewClient`, `NewTracker` | 链上事件和本地余额状态 |

## 支撑包

| 包 | 职责 |
| --- | --- |
| `types` | ID、订单、市场、仓位、交易和链上共享类型 |
| `signing` | 私钥 signer、HMAC、EIP-712 和 POLY_1271 |
| `errors` | 可判定的 API/结果未知错误类型 |

`internal/transport` 是 SDK 当前使用的网络实现，`internal/testutil` 只服务仓库测试。所有 `internal` 路径都不是公共 API，外部项目不应导入。

旧 `config`、`middleware`、`http` 和 `test` 公共包已移除，迁移方式见[不兼容变更说明](breaking-changes.md)。

## 查找方法

完整方法表不在 Markdown 中重复维护。请直接从当前版本生成文档：

```bash
# 包级入口
go doc github.com/polymas/go-polymarket-sdk/clob

# 接口的完整方法集
go doc github.com/polymas/go-polymarket-sdk/clob.Client
go doc github.com/polymas/go-polymarket-sdk/clob.ReadonlyClient

# 具体类型
go doc github.com/polymas/go-polymarket-sdk/web3/relayer.GaslessClient
go doc github.com/polymas/go-polymarket-sdk/types.OrderArgs
```

在线版本见 [pkg.go.dev](https://pkg.go.dev/github.com/polymas/go-polymarket-sdk)。在线页面对应已发布版本，本地 `go doc` 对应当前 checkout；排查版本差异时优先确认 `go.mod` 中实际解析的版本。

## API 约定

### Context

主要网络方法同时提供无 `Context` 和 `...Context` 版本。业务代码优先传入带 deadline 的 context；便捷方法适合脚本和向后兼容。

### ID 与 wire 格式

`types.ConditionID`、`types.TokenID`、`types.Keccak256` 和 `types.EthAddress` 用于减少跨领域 ID 混用。官方 wire 字段的 `asset_id/assets_ids` 在业务类型中统一暴露为 token ID，JSON 编解码仍保持官方字段名。

### CLOB V2

- V2 是唯一订单路径，没有 V1 回退。
- 订单不包含 `feeRateBps`。
- 每笔限价订单必须有合法 tick size；批量请求逐单对齐。
- 市价 BUY 的 amount 是 pUSD，SELL 的 amount 是 shares。
- 写请求超时或断连后，不应盲目重发；先用订单 ID、成交或链上 receipt 对账。

### 下单热路径（低延迟建议）

SDK 内部已经是 HTTP/2 多路复用、请求体只序列化一次、超过 15 单的子批并发提交。
剩下能省掉整段网络往返的都在调用方参数上：

- **限价单传 `OrderArgs.NegRisk`。** 为 nil 时 SDK 先查 GET /neg-risk 再签名；猜错
  exchange 域还会多一次重签重发。gamma 市场数据里就有 negRisk，直接透传；也可以
  启动时用 `PrimeNegRisk` 灌缓存。
- **市价单传保护价（BUY 传 `MaxPrice`，SELL 传 `MinPrice`）。** 不传时 SDK 会先 GET
  /book 算价，传了则本地算金额，整笔只剩一次 POST。有 WebSocket 盘口的业务层应
  始终传。
- **用 `...Context` 变体并设短 deadline。** SDK 默认 HTTP 超时 8 秒，对狙击型策略
  太长；传 1 到 2 秒的 deadline 即可。超时被视为结果未知，SDK 按本地确定性
  order hash 对账，不会重复下单。
- **结算确认喂 WebSocket 事件。** `AndWait` / `AwaitOrderResults` 默认每 250ms 逐个
  trade ID 轮询 GET /data/trades。把 user channel 的 trade 事件通过
  `OrderClient.RecordTradeUpdate(event.ToClobTrade())` 喂给 SDK 后，等待改为事件
  唤醒，HTTP 退化为 2 秒一次的兜底：

  ```go
  userWS.SetOnTradeUpdate(func(ev *websocket.UserTradeEvent) {
      clobClient.RecordTradeUpdate(ev.ToClobTrade())
  })
  ```

### 余额

- `GetCollateralBalance`：当前 V2 抵押物 pUSD。
- `GetUSDCEBalance`：旧 bridged USDC.e。
- `GetUSDCBalance`：兼容别名，已弃用，不应在新业务中表达模糊余额语义。

更完整的资金与授权流程见 [V2 业务接入](v2-guide.md)。
