# 兼容性审计与路线图

> 这是维护者工作台，包含已完成审计项与待办；面向 SDK 使用者的稳定文档从[文档导航](README.md)开始。

> 审计日期：2026-08-26
>
> 审计对象：当前工作区代码（包含尚未提交的本地修改）
>
> 官方基线：[Polymarket 文档索引](https://docs.polymarket.com/llms.txt)、[CLOB OpenAPI](https://docs.polymarket.com/api-spec/clob-openapi.yaml)、[Gamma OpenAPI](https://docs.polymarket.com/api-spec/gamma-openapi.yaml)、[Data OpenAPI](https://docs.polymarket.com/api-spec/data-openapi.yaml)、[Relayer OpenAPI](https://docs.polymarket.com/api-spec/relayer-openapi.yaml)、[Bridge OpenAPI](https://docs.polymarket.com/api-spec/bridge-openapi.yaml)、[Combos RFQ OpenAPI](https://docs.polymarket.com/api-spec/combos-rfq-openapi.yaml)、[Perps OpenAPI](https://docs.polymarket.com/api-spec/perps-openapi.json)
> 实时协议基线：[Market WS](https://docs.polymarket.com/asyncapi.json)、[User WS](https://docs.polymarket.com/asyncapi-user.json)、[Sports WS](https://docs.polymarket.com/asyncapi-sports.json)、[RFQ WS](https://docs.polymarket.com/asyncapi-rfq.json)、[Perps WS](https://docs.polymarket.com/asyncapi-perps.json)

## 如何审核这份清单

- `P0`：已确认与当前官方协议冲突，可能造成错误签名、错误请求、整批拒单、零值反序列化或 WebSocket 根本收不到数据。
- `P1`：主要交易流程缺失或存在高风险的不完整行为。
- `P2`：官方 REST/WS 功能尚未覆盖，按业务需要选做。
- `P3`：新产品线或低频能力，建议独立模块实现，不阻塞现有预测市场 SDK。
- 每个一级复选框可作为是否立项的审核入口；子项是建议验收标准，并不表示已批准实现。

## 审计结论摘要

当前 SDK 不是简单的“少几个接口”，而是同时存在以下三种情况：

1. CLOB 已统一使用 V2，且下单改为由业务层逐笔显式传入 tick；剩余的非十进制幂价格网格、批次错误语义和市场配置失效机制仍会直接影响真实资金交易。
2. Gamma、User WS、Sports WS 和 RFQ 中有多处仍使用旧端点或旧字段，部分方法即使 HTTP 成功也会返回关键字段为零值。
3. Bridge、完整 Rewards、Builder、当前 Combos RFQ、Perps 等官方能力尚未形成 SDK 模块。

建议顺序：先完成全部 P0，再完成 CLOB P1，之后按业务需求选择 P2/P3。

---

## P0：已确认的协议冲突

### [x] P0-1：默认切换到当前 CLOB V2 下单路径，隔离旧协议

完成于 2026-08-27，发布版本 `v1.13.0`。最终采用“仅 V2、无交易回退”方案：官方生产 CLOB 已明确不兼容 V1，因此没有保留 `WithLegacyV1()`。

修复前实现：

- `clob.NewClient()` 的 `useV2` 零值为 `false`，只有显式传 `clob.WithV2()` 才使用当前 V2 订单结构。
- 普通 README/旧调用方只调用 `NewClient()` 时会进入 `clob/orders.go` 的旧签名和旧订单字段。
- 代码仍公开 V1 split/merge/redeem 方法，容易被新钱包误用。

官方基线：当前 [Place Orders](https://docs.polymarket.com/trading/place-orders.md) 和 CLOB OpenAPI 的订单结构为 V2；当前合约地址以 [Contracts](https://docs.polymarket.com/resources/contracts.md) 为准。

修复结果：

- [x] `NewClient()` 只进入 V2 签名/提交路径；删除 `useV2` 分支和 legacy option。
- [x] `WithV2()` 保留为 Deprecated no-op，保证已有显式 V2 调用仍能编译。
- [x] 删除旧 CLOB order struct、签名、批量提交和 NegRisk 重试实现，同时移除不再使用的 `go-order-utils` 依赖。
- [x] 初始化日志记录 V2、签名类型、maker/signer 和 V2 exchange domains，不输出凭证或私钥。
- [x] 删除可写的 `RedeemPositionsV1`、`SplitUSDCV1`、`MergeTokensV1` 及其已过期真实资金测试；保留当前 V2 方法。
- [x] 为 V2 Split/Merge 补齐纯单元测试与显式开关保护的主网往返测试，覆盖 ABI calldata、Regular/NegRisk adapter 路由、金额精度/非法输入、Relayer payload；Regular 与 NegRisk 均完成真实 Split→Merge，并验证 receipt 与最终 pUSD 余额一致性。
- [x] `v1.14.0` 发布前再次用 `.env` Safe 钱包对 Regular 与 NegRisk 各执行 `0.01 pUSD` 的真实 Split→Merge；四笔 receipt 全部成功，两次往返的 pUSD 余额均逐单位一致。
- [x] README 和仓库内已跟踪的 V2 examples 使用默认 `NewClient()`。
- [x] 外部 `polyworker`、`polyworker-weather` 测试命令移除 `-v1` 分支及 `RedeemPositionInfoV1` 转换，统一调用 `RedeemPositions`；全目录已无三个 `*V1` 方法的 Go 调用。
- [x] 现有 V2 body、signer、batch retry 测试继续通过，并增加 `WithV2()` 源码兼容测试。
- [x] 老 Exchange/Factory 地址仅保留给旧 Proxy 地址解析、历史日志解码和诊断，不存在从下单流程回退到 V1 的入口。

### [x] P0-2：由业务层显式传入每笔订单的 tick size

完成于 2026-08-27，发布版本 `v1.14.0`。

完成状态：

- `OrderArgs.TickSize` 是下单必填参数；单笔和批量下单都不再使用 SDK 内置默认值。
- `CreateAndPostOrders` 在任何签名或 HTTP 提交前，逐笔校验 `tick_size` 和对应价格范围。
- 同一批订单可以携带不同的 tick，并逐笔用于价格舍入、maker/taker amount 计算和签名。
- 下单热路径不隐式查询 `/tick-size`，是否使用业务默认值、Gamma/WS 数据或动态查询由业务层决定。
- `GetTickSize` 已加入公开 `MarketDataClient` 接口，业务层需要时可在非热路径显式调用，再把结果写入 `OrderArgs.TickSize`。
- 增加缺失/非法 tick、混合 tick、价格范围和金额计算单元测试；仓库内已跟踪的下单示例均改为显式传值。
- 增加显式开关保护的生产 CLOB 挂单→撤单契约测试：PostOnly、价格等于最小 tick、maker notional 硬限制 `<= 0.05 USDC`，并在失败路径兜底撤单。已使用 `.env` 的 Safe 钱包完成一次真实挂单和撤单。

本项不包含非十进制幂 tick 的网格算法，后者由 P0-3 单独修复。

官方基线：订单必须符合市场的 `tick_size`，官方当前可能返回 `0.1`、`0.01`、`0.005`、`0.0025`、`0.001`、`0.0001`；市场运行中 tick 还可能变化。参见 [Get tick size](https://docs.polymarket.com/api-reference/market-data/get-tick-size.md) 和 [Market WS](https://docs.polymarket.com/api-reference/wss/market.md)。

已做真实请求验证：在一个 BTC 5 分钟市场（最小 tick 为 `0.01`）批量提交一个 `0.01` 单和一个 `0.001` 单，服务端返回顶层 HTTP 400：`price 0.001 breaks minimum tick size rule 0.01`，响应没有逐单结果，随后查询该 token 的开放订单为 0。也就是说，至少对 tick 违规，生产端会拒绝整个 HTTP 批次。

验收结果：

- [x] 调用方必须显式传入 tick，批量签名前不增加市场数据网络请求。
- [x] 同一批多个 token 时逐单应用各自 tick，不使用批次级固定值。
- [x] 把 `GetTickSize` 加入公开市场数据接口，保留显式动态获取能力。
- [x] 缺失、非法或价格范围不符合所传 tick 时，整批在本地提交前失败，不会向服务端发送部分订单。
- [x] `GetTickSize` 每次显式调用都真实查询，不保留隐藏的 SDK tick 缓存；缓存和 `tick_size_change` 策略由业务层管理。
- `0.005` / `0.0025` 的严格网格和精度验收已转入 P0-3，不属于本项未完成工作。

### [x] P0-3：修正 `0.005` / `0.0025` 的价格网格和精度算法

完成于 2026-08-27，发布版本 `v1.15.0`。

完成状态：

- [x] 用官方 V2 order builder 的 rounding config 为六种 tick 建立显式配置，正确表达 `0.005` 和 `0.0025` 的价格精度与金额精度。
- [x] 把价格转为 tick 精度下的整数单位后校验整除关系；容差只吸收 `float64` 二进制尾差，不会改动真实价格。
- [x] 非网格价格直接返回本地校验错误，整批不签名、不发送；按已确认的简洁策略，不增加自动舍入选项。
- [x] size 按官方逻辑向下保留两位，maker/taker amount 用整数与 `big.Int` 精确计算到 `1e6` token units，不增加第三方 decimal 依赖。
- [x] 表驱动单元测试覆盖全部六种 tick、合法/非网格价格、浮点尾差、BUY/SELL 与官方精度配置下的 maker/taker golden amounts。
- [x] 生产 Gamma 市场搜索未发现当前可交易的 `0.005` / `0.0025` 市场，因此不为追求形式上的真实测试而放大交易风险；已用 `.env` Safe 钱包在当前 `0.01` tick 生产市场完成 PostOnly、最大 `0.05 USDC` 名义金额的挂单→撤单回归。

### [x] P0-4：禁止静默把订单 size 改成 5

完成于 2026-08-27，发布版本 `v1.16.0`。

完成状态：

- [x] 删除 V2 签名循环中 `Size < 5` 时静默改成 `5.0` 的逻辑，避免 SDK 擅自放大真实交易数量。
- [x] 按已确认的简洁设计保留默认最小 size `5`，不在下单热路径请求或缓存市场 `min_order_size`。
- [x] 批量中任意订单的 size 小于 5 或为 NaN/Inf 时，整批在 NegRisk 预取、签名和 HTTP 提交前返回包含订单索引、tokenID、requested/minimum 的本地错误。
- [x] size 等于 5 或大于 5 时原样进入后续流程，不增加自动调量选项或新的公开 API。
- [x] 单元测试覆盖边界值、无效浮点值和混合批次在提交前失败。
- [x] 已用 `.env` Safe 钱包完成生产回归：`Size=5`、PostOnly、最大 `0.05 USDC` 名义金额的订单成功挂出并撤销。

官方市场数据可以提供动态 `min_order_size`；本项根据业务取舍有意保留默认值 5。如后续确实存在非 5 的目标市场，再单独评审是否由业务层显式传入最小值，不在 SDK 热路径隐式查询。

### [x] P0-5：保证批量请求逐单对齐，不能静默丢掉签名失败的订单

完成于 2026-08-27，发布版本 `v1.17.0`。

后续补充于 2026-08-27，发布版本 `v1.18.0`：新增底层均为 `[32]byte` 的 `types.ConditionID` 与 `types.TokenID`，用于批量稀疏结果和长期大字典的类型安全 key。两者提供严格解析、官方规范格式的 `String()` 还原以及文本/JSON 编解码；现有公开字符串字段保持兼容，不强制业务一次性迁移。

完成状态：

- [x] 删除 signed order 构建/签名失败时的 `continue`；当前 HTTP 子批不会发送缺单的请求体，并返回带订单索引和 tokenID 的错误。
- [x] 在任何切批/签名前预校验全部 tokenID，与已有 tick/price/size 预校验一起避免确定性输入错误造成部分提交。
- [x] 一旦开始提交，返回切片始终与输入等长同序；缺失或多余的服务端响应会返回对齐结果和非空 error，不再靠下标猜测。
- [x] 保留简洁的 `OrderPostResponse` API，不引入过度设计的 `BatchOrderResult`/`PartialBatchError`；用 `market_closed` / `not_submitted` / `unknown` / `server_rejected` 四个明确状态表达边界。
- [x] `orderbook does not exist` 不再剥离后重试。`market_closed` 仅表示“业务已正确处理、无需重试”，不代表交易所已接单或订单已创建。
- [x] 同一 HTTP 子批全部是已关闭 token 时返回 `market_closed` + `nil` error；混有其他 token 时，其他位置为 `not_submitted` 并返回非空 error。
- [x] 跨 15 单切批时，各子批并发提交、互不阻塞；失败子批按错误语义对齐自己的位置，其它子批正常返回，错误按子批编号 `errors.Join` 汇总。子批之间的服务端接收顺序不保证。
- [x] HTTP 200 的逐单业务拒绝保留在各自结果中，不升级为整批 error；传输超时等无法确认服务端结果的场景标记为 `unknown`，由业务层先对账而非盲目重试。
- [x] 单元测试覆盖关闭订单簿终态、混合关闭 token、顶层 4xx、传输超时、本地签名错误、HTTP 200 逐单拒绝、响应数量错位和跨 15 单并发子批。
- [x] 已用 `.env` Safe 钱包完成生产回归：PostOnly、最大 `0.05 USDC` 名义金额的正常订单成功挂出并撤销，确认本次错误语义修改未破坏成功路径。
- [x] 提供可选的紧凑 `ConditionID` / `TokenID` map key 类型，避免把不同 ID 语义混用；单元测试覆盖 bytes32、uint256 上界、非法输入、规范字符串还原及 JSON map 往返。

### [x] P0-6：修正 Data API 的 Trade、Activity、Value 模型

完成于 2026-08-27，发布版本 `v1.19.0`。

当前实现与官方字段冲突：

- `Trade` 使用 `market`、`asset_id`、`user`、`cash_amount`、`token_amount`；当前 Data API 使用 `conditionId`、`asset`、`proxyWallet`、`size` 等 camelCase 字段，并带市场和用户元数据。
- `Activity` 使用 `market`、`asset_id`、`tokens`、`cash`、`user`；当前 API 使用 `conditionId`、`asset`、`size`、`usdcSize`、`proxyWallet` 等字段。
- `ValueResponse.TotalValue` 标记为 `total_value`；当前响应字段是 `value`。
- `/value` 官方返回数组（按 market 可能有多项），当前 `GetValue` 只返回第一项，调用多个 condition 时会静默丢数据。
- Trade/Activity 的自定义 timestamp 解析失败时写入 `time.Now()`，这会把坏数据伪装成最新事件。

风险：请求可以返回 200，但 condition、token、user 和金额会悄悄变成零值；上层统计和对账结果错误。

建议验收：

- [x] 按 [Data OpenAPI](https://docs.polymarket.com/api-spec/data-openapi.yaml) 重建当前 Trade/Activity/Value 模型；删除错误旧字段，不保留看似可用的 legacy 模型。
- [x] Trade 覆盖 conditionId、asset、proxyWallet、side、size、price、timestamp、title/slug/icon/eventSlug、outcome/outcomeIndex、用户资料、transactionHash。
- [x] Activity 覆盖 size、usdcSize、transactionHash、name/pseudonym、事件元数据和 `isCombo`。
- [x] 增加 `ActivityType` 类型并补齐 `DEPOSIT`、`WITHDRAWAL`、`YIELD`、`MAKER_REBATE`、`TAKER_REBATE`、`REFERRAL_REWARD` 等全部当前枚举；同时支持官方 `excludeDepositsWithdrawals` 参数。
- [x] `ValueResponse` 改为 `{user, value}`；`GetValue` 返回完整 `[]ValueResponse`，新增 `GetTotalValue` 显式求和，不再默认取第一项。
- [x] Trade/Activity timestamp 直接按官方 Unix 秒整数解码；字符串和浮点坏值返回 JSON 错误，不再使用当前时间兜底。
- [x] 用官方 schema 与生产只读响应制作 JSON golden tests，断言关键字段非零且可回序列化；真实请求验证 `/trades`、`/activity`、`/value` 字段与模型一致。
- [x] `GetTrades` 的 limit 上限由错误的 500 修正为官方 10000，保留官方 `takerOnly=true` 默认值，并支持 `start/end` 时间窗口；查询 1000 条不再被 SDK 擅自截断。

### [x] P0-7：重做 User WebSocket 鉴权和订阅报文

修复前：

- `WebSocketAuth` 是 `address/signature/timestamp/nonce`。
- 初始类型发送大写 `USER`。
- 未实现按 `markets` 动态订阅/退订。

官方基线：[User Channel](https://docs.polymarket.com/api-reference/wss/user.md) 要求 auth 为 `apiKey/secret/passphrase`，初始 `type` 为小写 `user`，可选 `markets` 过滤并支持动态订阅。

完成：

- [x] Market 与 User Client 拆分；User 鉴权直接复用已有 `types.ApiCreds`，它严格对应 `apiKey/secret/passphrase`，不再定义重复凭证类型或复用 L1 签名模型。
- [x] 初始订阅和动态订阅使用官方小写 type/operation/markets 结构，markets 使用 `types.ConditionID`。
- [x] 完整解析 order placement/update/cancellation、trade 状态和 maker orders；金额及时间字段保留官方字符串精度。
- [x] 重连后自动恢复 auth 和市场过滤器；回调保留原始 event ID/状态供业务去重。
- [x] 每 10 秒发送纯文本 `PING`，处理服务端纯文本 `PONG`。
- [x] 本地 mock server 覆盖鉴权报文、动态订阅、心跳、事件和重连；`.env` 真实只读监听跨越心跳周期通过。

### [x] P0-8：修正 Sports WebSocket 地址、握手和心跳方向

修复前：

- 连接 `wss://ws-subscriptions-clob.polymarket.com/ws/sports`。
- 连接后发送 `{"type":"SPORTS"}`，客户端主动定时发 JSON `"PING"`。
- 仅在 payload 包含 `event_type == "sports"` 时处理。

官方基线：[Sports Channel](https://docs.polymarket.com/api-reference/wss/sports) 地址为 `wss://sports-api.polymarket.com/ws`，无需订阅报文；服务端每 5 秒发送纯文本 `ping`，客户端须在 10 秒内回复纯文本 `pong`；比赛更新本身没有上述 `event_type` 包装。

完成：

- [x] 更换为 `wss://sports-api.polymarket.com/ws`，删除 SPORTS 订阅包。
- [x] 收到服务端纯文本 `ping` 后立即回复纯文本 `pong`，不再主动发送 JSON heartbeat。
- [x] 新建官方 `SportResult` 模型，覆盖 slug/live/ended/score/period/elapsed/last_update/finished_timestamp/turn。
- [x] mock server 验证无订阅包、ping/pong、重连和事件解析；race 通过。
- [x] 官方生产 Sports Channel 真实只读连接跨越两个 ping 周期保持稳定。

### [x] P0-9：补齐 Market WebSocket 事件，暴露 tick size 变化

修复前：初始订阅发送大写 `MARKET`，只处理 `book`；心跳使用 JSON 编码的 `"PING"`。

官方基线：[Market Channel](https://docs.polymarket.com/api-reference/wss/market) 使用小写 `market`，支持 `initial_dump`、level 和 feature flags，并发布：

- `book`
- `price_change`
- `last_trade_price`
- `tick_size_change`
- `best_bid_ask`
- `new_market`
- `market_resolved`

完成：

- [x] 初始订阅使用小写 `market`；动态更新使用 `subscribe` / `unsubscribe`；每 10 秒发送纯文本 `PING` 并处理 `PONG`。
- [x] 为 7 类官方事件提供 typed model 和统一 `SetOnMarketEvent` 回调，同时保留旧 `SetOnBookUpdate`。
- [x] `MarketTickSizeChangeEvent` 使用 `types.TokenID` 和 `types.TickSize`，业务可直接原子更新 market config cache。
- [x] `NewClientWithOptions` 支持 initial dump、level 1/2/3 和 custom feature；`NewClient` 保留官方默认行为。
- [x] 重连恢复去重后的 asset 订阅与选项；明确事件回调可重放，由业务幂等处理。
- [x] mock 协议、动态订阅、重连、重启和 race 测试通过；官方生产 Market Channel 真实只读订阅跨越心跳周期通过。

### [x] P0-10：废弃当前 RFQ 旧端点，避免暴露看似可用但官方已不存在的方法

当前 `rfq` 包请求 CLOB 主域下的：

- `/rfq/request`
- `/rfq/quotes`
- `/rfq/accept`
- `/rfq/cancel`

这些端点不在当前官方 CLOB 或 Combos RFQ OpenAPI 中，也没有当前 requester/maker 鉴权模型。

官方当前能力是 Combos RFQ：maker REST 端点位于独立服务，使用 CLOB L2 鉴权；quoter 还有独立的 [RFQ WebSocket](https://docs.polymarket.com/api-reference/wss/rfq.md)。

建议验收：

- [x] 直接删除旧 `rfq.Client` 方法与旧模型，不再保留会访问失效端点的兼容壳。
- [x] 按公开的 Combos RFQ maker 模型重建 REST/WS；未虚构公开文档不存在的 requester REST 方法。
- [x] 支持 quote 的六位定点字符串、签名类型、last-look confirmation 和 execution update。
- [x] REST 与 WS 共用身份配置，分别实现 L2 鉴权、WS 首帧认证和只重连不重发写命令。

### [x] P0-11：修正 Gamma 中已失效或返回形状不匹配的端点

已确认冲突：

- `GetProfile` 调用 `/profiles/{address}`；当前公开端点是 `/public-profile?address=...`。
- `GetProfileByUsername` 调用 `/profiles/username/{username}`；当前官方公开 API 没有该端点。
- `GetSeriesBySlug` 调用 `/series/slug/{slug}`；当前官方是 `/series-summary/slug/{slug}`，且返回 `SeriesSummary`，不是完整 `Series`。
- `GetComments` 使用 `market_id`；当前参数是 `parent_entity_type` 与 `parent_entity_id`。
- `GetComment` 按单对象解码；当前 `/comments/{id}` schema 返回数组。
- `GetMarketTradesEvents` 调用 Gamma `/live-activity/events/{marketID}`；当前 Gamma spec 无此路径，CLOB 的 live activity 是另一组端点和模型。
- `GetAllMarkets` 固定以 `limit=500` 请求 `/markets/keyset`；官方该端点最大 `100`，因此会返回 422，而代码不会进入后续分页。

建议验收：

- [x] `GetProfile` 迁移到 `/public-profile?address=`，删除无官方依据的 username 查询。
- [x] 新建 `GetSeriesSummaryBySlug/ID`，使用正确返回类型。
- [x] Comments API 改为通用 parent entity 参数，单评论按官方数组响应解码。
- [x] 删除 Gamma 中无官方路径的 `GetMarketTradesEvents`。
- [x] `GetAllMarkets` keyset page size 为 100，并只依据 `next_cursor` 终止。
- [x] 用本地契约测试锁定当前路径、参数、返回形状和游标分页。

### [x] P0-12：扩展下单响应并实现成交结算对账

当前 `OrderPostResponse` 只有 `orderID/status/errorMsg`，会丢失官方响应中的：

- `success`
- `makingAmount`
- `takingAmount`
- `transactionsHashes`
- `tradeIDs`

官方 2026-07 的行为更新还意味着成功的 FAK/FOK 不一定在下单响应内立即给出交易哈希，需要依据 `tradeIDs` 查询成交并等待 settlement。

建议验收：

- [x] 完整建模 `SendOrderResponse`，金额使用十进制定点字符串；保留 `OrderPostResponse` 名称兼容现有调用方。
- [x] 实现已认证的 `GetTrades` 游标分页和官方过滤器；trade ID 走 `/data/trades?id=`，order ID 走 `/data/order/{orderID}`。
- [x] 提供 `WaitForOrderFillSettlement(ctx, ...)`，区分 matched、mined、confirmed、failed、timeout。
- [x] matched 响应缺少交易哈希时由 SDK 自动等待，批次共享 30 秒窗口并以 4 路并发查询 trade；业务层无需二次调用。
- [x] 网络结果不明确时保留 unknown，并按本地 V2 EIP-712 order hash 自动查询单订单；只在对账仍无法确认时返回 unknown，禁止自动重下。
- [x] 每笔响应暴露 POST、order-hash 对账、settlement 和总耗时，以及轮询/查询错误/超时统计；生产 PostOnly 实测本地 hash 与 CLOB orderID 一致。
- [x] 同时提供 `PostOrder/CreateAndPostOrders` 的 `Instant` 与 `AndWait(ctx, ...)` 版本；旧接口保持自动等待语义兼容，Instant 结果也可稍后交给 `AwaitOrderResult(s)` 补全。
- [x] 为响应提供 `Accepted/NeedsFollowUp/DefinitelyNotSubmitted` 等业务语义辅助方法，并以同一批次共享 deadline，避免按订单累计等待。
- [x] 增加默认跳过的生产批量延迟测试：5 个不同 condition、最低 tick、PostOnly、单笔名义金额不超过 0.05 USDC、逐批精确撤单。20 样本复测中 Instant p95=258.6ms、AndWait p95=259.7ms，等待阶段 p95=0；首轮曾观察到 AndWait p95=1.34s，需按外部 POST 尾延迟预留余量。

### [x] P0-13：迁移 RTDS URL、订阅协议和心跳

当前实现连接 `wss://rtds.polymarket.com/ws`，用 `{type:"subscribe", stream, ids}` 订阅 prices/comments，并每 15 秒 `WriteJSON("PING")`。

官方当前 [Chainlink TWAP Prices](https://docs.polymarket.com/market-data/chainlink-twap.md) 低层协议连接 `wss://ws-live-data.polymarket.com`，订阅报文为 `{action:"subscribe", subscriptions:[{topic,type,filters}]}`；应用层心跳是每 5 秒发送纯文本 frame `PING`。30s/60s TWAP 分别使用 `crypto_prices_twap_thirty` 和 `crypto_prices_twap_sixty`。

风险：当前 RTDS client 即使能建立 TCP/WebSocket，也无法按当前协议订阅或保活；旧 `EncryptedPriceUpdate` 模型也不是当前 TWAP payload。

建议验收：

- [x] 更换当前官方 URL，并支持可注入 URL 便于 mock 测试。
- [x] 改成 action/subscriptions 协议；filters 按官方要求生成紧凑 JSON 字符串。
- [x] 用 `WriteMessage(TextMessage, []byte("PING"))` 每 5 秒发纯文本心跳。
- [x] 新建保留 E18 原值和精确十进制字符串的 TWAP event，区分 publish/observation timestamp。
- [x] 删除当前官方文档无法确认的旧 prices/comments/auth 接口，不保留误导性的 legacy 能力。
- [x] 重连自动恢复全部 topic/window/filter；mock 覆盖 30s/60s、无 symbol filter、心跳和精度。

### [x] P0-14：移除或隔离已经失效的 The Graph Subgraph client

当前 `subgraph` 包硬编码 `https://api.thegraph.com/subgraphs/name/polymarket/clob-v2`；仓库测试已经明确备注该端点被移除，并在请求失败时 Skip。当前官方文档目录也不再提供这组 Subgraph API。

风险：README 仍把它当作 SDK 能力，调用方运行后才发现端点不可用；测试 Skip 又掩盖了长期失效。

- [x] 删除 `subgraph` 包、README 公开能力和仅供旧 schema 使用的 GraphQL 类型。
- [x] positions/PnL/value 使用现有 Data API；旧端点独有的 OI 等能力不伪造替代实现，调用方应使用 indexer/RPC。
- [x] 删除用 Skip 掩盖失效端点的测试。
- [x] 不保留绑定旧 Polymarket schema 的通用 GraphQL client。

### [x] V2-P0-1：修正 `SetV2Allowances` 的 V2 抵押物授权

完成于 2026-08-28，发布版本 `v1.28.0`。

复审发现 `SetV2Allowances()` 虽然以 V2 命名，但前两笔仍向 V2 Exchange / V2 NegRisk Exchange 执行 `USDC.e.approve(MAX)`。官方 CLOB V2 的交易抵押物是 pUSD；USDC.e 只应在进场 wrap 时授权给 CollateralOnramp。

- [x] 前两笔改为 `pUSD.approve(V2 Exchange / V2 NegRisk Exchange, MAX)`；CTF 的两笔 `setApprovalForAll` 保持不变。
- [x] 抽出纯构造函数，单元测试逐笔解码 target、selector、spender/operator 和 amount，并显式断言不得以 USDC.e 为 target。
- [x] `go test -timeout 30s ./web3` 和 `go test -race -timeout 45s ./web3` 通过。
- [x] 使用 `.env` Safe 钱包执行真实 gasless 四笔授权，receipt 成功：tx `0x49dd2e06554093264fccdf13f27e98c483086325354481ac3af9a05484a1861b`，block `92787358`。
- [x] 交易后链上验证两个 pUSD allowance 均为 MAX、两个 CTF approval 均为 true，两个 USDC.e allowance 与交易前逐值一致。
- [x] 更新可执行示例的交易数和抵押物说明，不再引导用户对 V2 Exchange 授权 USDC.e。

### [x] V2-P0-2：统一 V2 余额查询的 pUSD 语义

完成于 2026-08-28，发布版本 `v1.29.0`。

原 `clob.GetUSDCBalance()` 与 `web3.GetUSDCBalance(address)` 固定读取 USDC.e，但 V2 CLOB 的交易抵押物已是 pUSD，业务层用该结果做下单余额判断会得到错误结果。

- [x] `web3.Client` 新增 `GetCollateralBalance(address)`，明确读取 V2 抵押物 pUSD；新增 `GetUSDCEBalance(address)` 供明确的旧 USDC.e 场景使用。
- [x] `clob.AccountClient` 新增无参 `GetCollateralBalance()`，使用已解析的 maker/proxy 地址直读 pUSD。
- [x] 不静默改变旧方法语义：`GetUSDCBalance` 保留为 USDC.e 兼容别名并标记 deprecated，避免存取款/进场业务在升级后情况下无感切换代币。
- [x] 单元测试以 mock JSON-RPC 解析 `eth_call` target：正向确认 pUSD/USDC.e 各返回 6 位精度数值，反向确认 V2 collateral 不得命中 USDC.e，旧别名只命中 USDC.e，非法地址不发 RPC。
- [x] 使用 `.env` 真实 Safe `0xC789151B4dd1F4fd16044742Ab17AAa895ae10FD` 做只读验证：CLOB `COLLATERAL` 返回 `51144933`，链上 `GetCollateralBalance` 返回 `51.144933` pUSD，两者完全一致；旧 USDC.e 返回 `0.000000`。
- [x] 修正 balance probe 的 raw 请求 host 为当前生产 `clob.polymarket.com`，示例和 README 统一使用 pUSD/collateral 语义。
- [x] `go test -timeout 30s ./web3`、`go test -race -timeout 45s ./web3` 以及排除用户未跟踪测试文件后的 clob tracked tests/race 均通过。
- [x] 下游影响核查：`polyworker` 只消费 `web3.Client`/`clob.Client` 构造结果，没有自定义实现这两个接口，本项不需要改它的业务代码。它直接替换到当前 SDK 的全量编译仍被早于本项的 `AssetID/DecimalString/WebSocketAuth` 迁移遗留阻断；本项未修改 `polyworker`。外部若自定义实现 SDK 公开接口，升级时需补充新方法。

---

## P1：核心流程不完整或行为不安全

### [x] P1-1：对齐订单类型和 GTD 规则

当前常量为 `GTC/IOC/FOK`；官方当前枚举为 `GTC/FOK/GTD/FAK`。

- [x] 移除 `IOC`，现有调用迁移到 `FAK`。
- [x] 增加 `GTD`、`FAK`。
- [x] GTD 必须带至少提前 3 分钟的 Unix 秒 expiration，其他类型禁止携带 expiration。
- [x] PostOnly 仅允许 GTC/GTD；使用表驱动测试覆盖 GTC/GTD/FOK/FAK、expiration 和非法类型。

### [x] P1-2：实现官方市价单构造流程

已有 `MarketOrderArgs` 类型，但没有公开的 create/post market order 流程。

完成于 2026-08-28，发布版本 `v1.30.0`。

- [x] 实现 BUY 按 pUSD 金额、SELL 按 outcome token 份额的市价单构造；市价单限制为 FOK/FAK，零值默认 FOK。
- [x] 支持 BUY `MaxPrice`、SELL `MinPrice`。显式保护价不请求订单簿，保留业务层已持有实时盘口时的低延迟路径；保护价为零才由 SDK 获取订单簿估价。
- [x] 未指定保护价时按官方 best-to-worst 规则累计深度：BUY 累计 `price*size`，SELL 累计 shares；FOK 深度不足返回 typed `InsufficientLiquidityError`，FAK 使用最差可成交档位并允许部分成交。
- [x] 市价金额使用整数定点算法：maker 固定向下保留 2 位，taker 根据 tick 使用官方 rounding config；显式保护价向保护方向取整，自动估价路径向下取整。
- [x] 公开 `CalculateMarketPrice`、兼容等待语义、`Instant` 和 `AndWait(ctx)` 三种提交方式；复用 V2 签名、negRisk 重试、歧义结果对账与 settlement 等待，不另建重复下单栈。
- [x] 修正生产响应判定：真实 CLOB 的逐单拒绝可能同时返回 `success=true` 和非空 `errorMsg`，现在 `Accepted()` 以错误字段优先，不会误判为已接单。
- [x] 单元测试覆盖 BUY/SELL、FOK/FAK、空簿/深度不足、side-specific 字段、保护价网格、过期 tick、默认最小 5 shares、零额外盘口请求、真实 EIP-712 签名及 `/orders` wire amounts；clob tracked tests 与 race tests 均通过。
- [x] 从 Git 索引构造的干净副本中 `go test ./clob ./types` 通过；全仓测试的本项相关包及其他包通过，仅既有 `chainws.TestMonitorAddress_USDCAndTokenChanges` 因测试内长时间 sleep 触发 90 秒总超时、`internal` 公共 OnFinality RPC 返回 429，与本项无关。
- [x] 生产正向验证：BTC 5 分钟 token `14557128192760159105557252808339369229830206629064893850242581995445599509282`，以 0.51 BUY 5 shares（2.55 pUSD），再以 0.50 SELL 5 shares，仓位完全关闭，往返价差成本约 0.05 pUSD。BUY tx `0xb19313996c80ecdb6eadbf56de3ba048d83dd8f97e1bcc0d1447d338b81fe2ef`，SELL tx `0xda794ea7f7f54225d074fd7656f119940b7d63ecf6e6288e7c78629a4dab86d3`。
- [x] 生产负向验证：tick `0.001` token 在远离盘口的 0.003 FAK BUY 中，6 位 taker 精度被明确拒绝，官方 5 位精度通过 amount validation 后得到 `no orders found`，两次均无成交和资产损失。由此以生产行为否定旧 issue 中“统一最多 4 位”的过时结论。
- [x] 下游核查：`polyworker` 未调用 SDK 的旧 `MarketOrderArgs`，也没有自定义实现新增的公开接口；本项不修改 `polyworker`。旧类型此前虽未被仓库使用，但字段布局变更对外属于源码级升级点，调用方应迁移到新的 side-specific 字段。

### [x] P1-3：扩展订单簿模型并统一单个/批量返回类型

单个 `OrderBookSummary` 目前只保留 token/bids/asks，丢失 market、timestamp、hash、min_order_size、tick_size、last_trade_price；批量模型字段又不同。

- [x] 单个与批量共用一个完整 `OrderBookSummary`；旧 `OrderBookSummaryResponse` 保留为类型别名。
- [x] 价格和数量使用保留 JSON 原文的 `DecimalString`，需要浮点运算时显式转换。
- [x] 将 `min_order_size/tick_size/neg_risk` 完整暴露给业务层，用于构造显式的下单参数和预校验配置。
- [x] 针对空订单簿、null/空字符串/缺项 last trade 和高精度价格数量建立测试；生产只读请求确认未交易 token 当前会返回 `last_trade_price: ""`。
- [x] SDK 领域模型统一使用 `TokenID/TokenIDs`；官方 `asset_id/assets_ids/winning_asset_id` 仅保留为 JSON wire tag，不向业务层扩散命名差异。

### [x] P1-4：补齐单订单查询、撤单过滤与限制校验

- [x] 实现官方 `GET /data/order/{orderID}`，不要只用 GetOrders 的筛选参数替代。（已随 P0-12 完成）

完成于 2026-08-28，发布版本 `v1.31.0`。

- [x] 修正旧 `CancelMarketOrders(conditionID)` 的 wire body：由错误的 `condition_id` 改为官方 `market`，并保留旧方法作为源码兼容包装。
- [x] 新增 `CancelMarketOrdersByFilter(CancelMarketOrdersParams)`，支持 condition-only、token-only 和 condition+token 交集；领域模型统一使用 `[32]byte` 的 `ConditionID/TokenID`，wire 层才映射为 `market/asset_id`。
- [x] 全空过滤在本地拒绝且不发送请求，避免 `{}` 产生不明确或扩大范围的撤单行为。
- [x] 审计时记录的 1000 上限已过期；2026-08-28 当前官方 `DELETE /orders` 文档明确为单次最多 3000。SDK 导出 `MaxCancelOrdersPerRequest=3000`，超限直接在本地报错，不自动切批引入部分撤单歧义。
- [x] `CancelOrders` 在发送前逐个校验 order ID，并规范化为带 `0x` 的字符串；重复 ID 保持官方服务端自动忽略语义，不额外改变调用方结果。
- [x] `CancelOrders/CancelOrder/CancelAll/CancelMarketOrdersByFilter` 共用稳定的 `OrderCancelResponse`；服务端缺省或返回 null 时，`Canceled` 与 `NotCanceled` 均归一化为非 nil typed 集合。
- [x] mock 测试覆盖三种过滤组合、旧兼容入口、错误 wire 字段缺失、空过滤零请求、非法 ID、3000/3001 边界、ID 规范化和空结果归一化。
- [x] 使用 `.env` Safe 账户在活跃市场做生产无状态验证：先确认 condition/token 均不命中当前开放订单，再发送 condition-only、token-only、condition+token 三种过滤，生产均接受并返回空 typed result；未创建订单、未撤销订单、无资产变化。
- [x] clob tracked tests、race、vet 通过；Git 索引干净副本 `go test -short ./...` 全部通过，普通全仓测试除公共 OnFinality RPC 返回 429 外全部通过，`chainws` 已不再阻塞。
- [x] 下游核查：`polyworker` 只调用签名不变的 `CancelOrders`，未调用 `CancelMarketOrders`，也没有自定义实现 SDK `Client`；本项无需修改 `polyworker`。

### [x] P1-5：重构 HTTP 错误、重试和 context

当前通用 HTTP 层主要返回格式化字符串错误，`errors`/middleware 中的结构没有贯穿公开客户端。

- [x] 通用 HTTP、CLOB、Gamma、Data、RFQ 及 Gasless/Relayer 公开网络操作均提供 `...Context` 入口；旧签名保留为 `context.Background()` 兼容包装。等待型下单、订单/成交对账、Relayer nonce/submit/poll/receipt 与 onboarding 全链路传递同一个 context。
- [x] 定义稳定的 `APIError`：service、method、path、status、requestID、errorCode、message、retryAfter、retryable 与限长脱敏的原始响应；Relayer 兼容错误通过 `Unwrap` 暴露同一个 typed error。
- [x] 按官方状态表处理 425/408/429/500/502/503/504；默认只重试 GET/HEAD/OPTIONS/DELETE，批量 books/prices/midpoints/spreads/last-trades/scoring 等只读 POST 显式声明幂等，普通 POST 不重试。旧 middleware 同步限制为幂等方法并修正 response body 生命周期。
- [x] CLOB order、RFQ mutation 与 Relayer `/submit` 超时返回 `AmbiguousOutcomeError` 且仅提交一次；订单结果仍按输入对齐为 `unknown` 并携带本地 expected order hash。等待型订单用调用方 ctx 对账，Relayer 已取得 transactionID 后用同一 ctx 轮询。
- [x] 统一响应和 URL 脱敏 API secret/key、passphrase、private key、signature、authorization；移除 Relayer 请求体、calldata 前缀和完整响应日志，仅保留长度、状态、transactionID/hash 等诊断字段。
- [x] mock 正负向测试覆盖 typed error、请求 ID/Retry-After、425/503 重试、只读 POST opt-in、写 POST 单次提交、deadline、ambiguous、CLOB expected hash、Relayer 兼容 unwrap、middleware body 与 URL 脱敏；生产只读验证 `GET /time` 成功、非法 `/book` 返回结构化 `APIError`，未鉴权、未下单、无资产变化。
- [x] 官方核实：Error Codes 明确 425 matching engine restarting、429 exponential backoff、500 retry、503 paused/cancel-only；Rate Limits 明确 Cloudflare throttling 与 Relayer `/submit` 25/min；Relayer 文档明确 `/submit` 立即返回 transactionID 后应轮询 `/transaction`。
- [x] clean tracked 副本 tests/race/vet 与示例构建通过；工作区原有未跟踪 live/probe 文件未纳入提交，`polyworker` 未修改。

### [x] P1-6：为 SDK 内部 negRisk 缓存增加并发安全和生命周期

`negRisk` 是市场创建时确定的静态属性；官方 V2 客户端使用进程生命周期缓存，
当前 Market WS 也没有 negRisk 变更事件。因此不做定时/逐单刷新，避免增加下单热路径延迟。

- [x] 裸 map 替换为加锁 typed cache；并发批量签名、显式订单参数更新不再产生 data race。
- [x] cache entry 带 fetchedAt/source/version，区分 API、订单显式参数和业务层预热。
- [x] 同一 token 冷启动请求使用 singleflight；提供 `PrimeNegRisk` 和 `InvalidateNegRisk`。
- [x] 显式 `OrderArgs.NegRisk` 及业务启动预热保持下单零网络；无 TTL 和不存在的 WS 自动刷新。
- [x] mock、生产正负向验证及 `go test -race` 覆盖并发预取、更新、失效和重新获取。

### [x] P1-7：按 V2 设计移除 fee rate

- [x] 删除 `OrderArgs` 入参、公开查询接口、REST endpoint、内部缓存及缓存配置。
- [x] 删除成交模型和 WebSocket 模型里的对应字段；服务端额外字段由 JSON 解码安全忽略。
- [x] 下单热路径不查询、不缓存也不传入该值，V2 签名结构保持官方字段集合。
- [x] README 保留 V2 费用由撮合方在成交时动态处理的说明。

### [x] P1-8：修正 last-trade 的未成交/缺项语义

2026-08-28 官方文档与生产验证：单 token 从未成交时返回占位值
`{"price":"0.5","side":""}`；批量查询直接省略未成交 token，且重复输入只返回一项。

- [x] 单查询保留 pointer 接口，将官方空 side 占位值和兼容性的 `null` 统一返回 `nil, nil`。
- [x] `LastTradePrice` 使用 typed `TokenID`、精确 `DecimalString` price 和 side，不再错误复用 `TokenValue.value`。
- [x] 批量结果返回 `map[TokenID]LastTradePrice`，不假设返回长度、顺序与输入一致。
- [x] mock 覆盖占位值、null、非法 ID 零请求、缺项、重复输入和乱序；生产正负向请求验证通过。

### [x] P1-9：完善新钱包 onboarding

当前 web3 已支持多种签名类型、Deposit Wallet 部署和 V2 approvals，但 CLOB 创建流程仍要求调用方自行拼装多个步骤。

- [x] 新增 `ResolveWalletAccount(ctx)` 和 `WalletAccount`，明确 controlling signer、wallet/funder/maker、V2 order signer、signature type、wallet type 与部署状态；枚举值与当前官方 `AccountIdentity/WalletType` 对齐。
- [x] 新增 `OnboardDepositWallet(ctx, minimumPUSD)`：未部署时走 `WALLET-CREATE`，资金不足返回非错误的 `funding_required` checkpoint，资金到位后自动 wrap USDC.e、只补缺失的四项 V2 approvals 并二次读取链上状态。
- [x] deploy 和 approvals 都以链上状态判定、重复调用为 no-op；SDK 不替业务自动转账，避免 funding 请求超时后重复扣款。任一步失败后重新调用会从 code/balance/allowance/approval 状态恢复。
- [x] README 明确 EOA、legacy Proxy、Safe、Deposit Wallet 语义，并记录 Deposit Wallet 可恢复 onboarding 用法。
- [x] mock 覆盖未部署、部署失败、外部 funding checkpoint、USDC.e wrap 后 ready、授权提交后仍不一致、非法门槛与错误 wallet type；`.env` 当前生产账户 resolver 正向读取通过，公开已部署 Deposit Wallet 和空地址的 `eth_getCode` 正/负向验证通过，未发交易。

### [x] P1-10：更新 `chainws` 的当前合约监听集合和余额模型

当前 `chainws.polymarketContractAddresses()` 仍监听 legacy Exchange、legacy Neg Risk Exchange、USDC.e 和已退役 Neg Risk Adapter，没有加入 V2 exchanges、pUSD、Collateral Onramp/Offramp。

风险：Tracker 会漏掉当前 V2 交易/余额变化，或者继续把已退役合约当作主要数据源；`GetPositions` 结果可能长期不刷新。

- [x] 按 2026-08-28 官方 `ts-sdk`/`py-sdk` production config 建立带版本和用途的 Polygon contract registry；地址集中来自 `internal`，`chainws` 不再维护另一组散落常量。
- [x] 默认监听 V2 exchanges、CTF、pUSD、USDC.e、Onramp/Offramp、当前 CTF adapters 和 AutoRedeem operator；V1 exchanges/retired adapter 仅通过 `WithLegacyContracts()` 显式 opt-in。
- [x] `Tracker` 分开维护 pUSD、USDC.e 和 ERC-1155 position，新增 `PositionKeyPUSD`/`PositionKeyUSDCE`；旧 `PositionKeyUSDC` 仅作为 USDC.e 的 deprecated source-compatible alias。
- [x] 补齐 `TransferBatch` 订阅和严格 ABI 解码；removed log 先反向撤销增量并设置 `NeedsReconcile`，调用方可用 `Reconcile(ctx)` 通过 RPC 精确读取 pUSD、USDC.e、已发现 token 的链上余额并原子替换快照。
- [x] replay tests 使用 Polygon 主网真实 receipt logs：BUY `0xb193…e2ef`、SELL `0xda79…86d3`、split `0x4c6d…3c1d`、merge `0x08c1…4401`、redeem `0x80b7…047d`、wrap `0xde01…906d`、unwrap `0x4b68…4f5c`；另以公开 RPC 重新读取完整 receipts，确认当前 V2 地址、方向和 6 位金额。

---

## P2：官方已有、当前 SDK 尚未覆盖的预测市场功能

以下属于“缺功能”，不一定是现有实现 bug。建议按实际业务勾选。

### [ ] P2-1：CLOB 公共市场数据与市场目录

- [ ] `GET /clob-markets/{condition_id}`：完整 CLOB market config。
- [ ] `GET /markets-by-token/{token_id}`。
- [ ] `POST /markets/live-activity` 与 `GET /markets/live-activity/{condition_id}`。
- [ ] `POST /batch-prices-history`。
- [ ] sampling/simplified market 的官方 CLOB 版本与游标分页。
- [ ] tick-size、neg-risk 的 path 参数版本（若需要完整端点对称性）。

### [ ] P2-2：CLOB 认证交易与 Builder 数据

- [x] `GET /data/trades`：已认证成交查询、游标分页和过滤器。（已随 P0-12 完成）
- [ ] `GET /builder/trades`。
- [ ] Builder API key：创建、列举、删除。
- [ ] closed-only/ban status 查询。
- [ ] `POST /heartbeats` 与 `/v1/heartbeats`，包括 dead-man/连接保活语义。

### [ ] P2-3：CLOB Rewards/Rebates

当前只实现订单是否计分；尚缺：

- [ ] 用户 rewards 明细、total、percentages、markets。
- [ ] 当前 reward markets、按 condition 查询、multi 查询。
- [ ] 当前 rebates。
- [ ] 分页、时间窗口、金额精度和空结果模型。

### [x] P2-4：Gamma 补全

- [x] status（兼容官方纯文本 `OK` 响应）。
- [x] teams 列表与按 ID 查询。
- [x] related tags 四组关系查询。
- [x] events pagination、results、tweet-count、comments count、event tags、creators、creator by ID。
- [x] market description、market tags。
- [x] markets information、markets abridged（只读 POST 标记为幂等请求）。
- [x] events keyset；markets keyset 对外暴露 next cursor，并分别限制官方最大 page size 500/100。
- [x] series by ID、series comments count、series summary by ID/slug，并补齐 series 过滤器与当前响应字段。
- [x] comments by user address。
- [x] sports metadata、sports market types。
- [x] Gamma 新增过滤器与字段：decimalized、rfq_enabled、tag_match、locale 等；同时补齐 search、tags、series 过滤器和按地址 profile 路径。
- [x] 单元契约测试覆盖 query、multi-query、keyset cursor、只读 POST body 和纯文本 status；生产只读联调通过 status、teams、events keyset、markets keyset、sports market types。

### [x] P2-5：Data API 补全

- [x] health。
- [x] accounting snapshot（直接返回官方 ZIP 字节，不错误地按 JSON 解码）。
- [x] approvals。
- [x] combo activity、combo positions，包括 cursor、offset 和当前嵌套模型。
- [x] holders。
- [x] traded / markets traded。
- [x] revisions。
- [x] open interest。
- [x] live volume。
- [x] closed positions。
- [x] other positions。
- [x] market positions。
- [x] builder leaderboard、builder volume。
- [x] user leaderboard。
- [x] positions 的 `includeArchived`、grossInitialValue、entryFeesUsdc、eventId 等当前字段。
- [x] 所有新增网络方法均提供 `...Context` 与兼容的 background 包装；列表上限和官方默认排序在 SDK 边界统一处理。
- [x] 单元契约测试覆盖 ZIP、multi-query、cursor、分页上限和新增字段；生产只读正向联调通过 health、snapshot、combos、positions、traded、OI、holders、live volume、closed/other/market positions、builder/user leaderboard。
- [x] 官方 OpenAPI 已声明但生产暂无法正向验证：`/v1/approvals` 对有效活跃地址返回 500，`/revisions` 返回 404；SDK 保留官方契约实现和 mock 正向测试，未伪造替代端点。

### [ ] P2-6：Bridge API 独立模块

当前没有官方 Bridge REST client：

- [ ] `GET /supported-assets`。
- [ ] `POST /quote`。
- [ ] `POST /deposit`：创建 bridge deposit addresses。
- [ ] `POST /withdraw`：创建 withdrawal addresses。
- [ ] `GET /status/{address}`：cursor 分页和全历史拉取 helper。
- [ ] 金额/费用使用精确十进制，链 ID 与 token address 使用强类型。

### [ ] P2-7：Relayer API 补全

当前 web3 主要覆盖 submit、nonce 和按 transaction ID 查询；尚缺或没有公共统一 client：

- [ ] recent transactions by user。
- [ ] relay payload/address + nonce。
- [ ] wallet deployed 查询。
- [ ] relayer API keys 列举。
- [ ] 将 submit/query/nonce 的响应模型与官方 OpenAPI 对齐，不使用 `map[string]interface{}` 作为公开结果。
- [ ] transaction 状态机、终态、超时和失败原因统一建模。

### [ ] P2-8：当前 Combos RFQ requester/maker

- [ ] requester：combo markets 列表和官方 requester-side RFQ 流程。
- [ ] maker：submit quote。
- [ ] maker：cancel quote。
- [ ] maker：last-look confirmation。
- [ ] RFQ quoter WS：认证、request、quote/ack、cancel/ack、confirmation、execution update、trade、error。
- [ ] 六位定点字段、Unix 毫秒、Exchange V3 order 签名做 golden tests。

### [ ] P2-9：RTDS 在完成 P0-13 迁移后补全高级能力

当前只抽象 prices/comments，建议按官方当前 RTDS topic 再核验和扩展：

- [ ] Chainlink crypto price topic 的 typed payload、symbol/filter 和时间戳。
- [ ] 官方支持的 30s/60s TWAP 窗口（若当前服务开放）。
- [ ] comments topic 的当前 schema 与订阅字段。
- [ ] 原始消息回调、sequence/gap 检测和重连补订阅。

### [ ] P2-10：通知完整模型

- [ ] CLOB 通知使用当前 type/payload schema，不只保留通用字段。
- [ ] 删除通知支持官方请求限制和逐项结果。
- [ ] 与 User WS/order/trade 事件建立可选的去重 ID。

---

## P3：可选的新产品线

### [ ] P3-1：新增独立 `perps` REST client

Perps 是独立交易系统，建议不要塞入现有 `clob` 包。最小可用范围：

- [ ] Public：ping、time、exchange、assets、instruments、tickers、statistics、klines、mark history、BBO、book、index、trades、funding、fees、limit tiers。
- [ ] Trading：create/cancel/modify order、按 client order ID 操作、cancel all、auto-cancel、leverage、batch leverage、isolated margin。
- [ ] Account：credentials/session、orders/open orders、balances、portfolio、fills、equity、PnL、funding、limits、config。
- [ ] Funds：withdraw、internal transfer、deposits/transfers/withdrawals history。
- [ ] Referral/rewards/notifications。
- [ ] BLP enrollment 与 liquidations（仅做市/流动性提供方需要）。
- [ ] 地域限制和限流在客户端错误中显式呈现。

### [ ] P3-2：新增独立 `perps/ws` client

- [ ] 命令：place/modify/cancel/cancel-all/auto-cancel/leverage/margin。
- [ ] 公共流：trades、BBO、book、klines、tickers、statistics。
- [ ] 私有流：fills、backstop、orders、funding、balances、portfolio、deposits、withdrawals、notifications。
- [ ] auth/session refresh、sequence gap、snapshot + delta、重连恢复。

---

## 官方文档之间需要人工确认的差异

这些项目不应在没有实测/官方确认的情况下直接按某一份 schema 硬改：

### [x] D-1：签名类型 3 的文档差异

- Wallet/RFQ 等当前文档已出现签名类型 `3`（POLY_1271 / Deposit Wallet）。
- CLOB OpenAPI 某些枚举位置仍只列 `0/1/2`。
- [x] 以官方名称新增 `Poly1271SignatureType`，以钱包语义提供 `DepositWalletSignatureType`；旧 `CWIASignatureType` 保留为 Deprecated 等值别名，避免破坏下游。
- [x] `SignatureType.String/ValidV2/UsesContractSignature` 明确 0–3 的能力边界；Web3 和签名构建在网络/RPC 前拒绝未知值，不再把未知类型静默当 EOA。
- [x] V2 maker/signer 和嵌套 ERC-1271 签名继续与官方 `py-clob-client-v2` 对齐，Python golden 签名测试保留。
- [x] 生产只读负向验证：同一 EOA 临时使用 `signature_type=3` 查询时，CLOB 返回 `404 no deposit wallet found for owner`，而不是 `Invalid signature_type`，证明生产路由已识别 type 3；当前 `.env` 实际账户是 type 2，因此未伪造“已部署 Deposit Wallet 正向下单”结论。
- [x] README 明确记录 OpenAPI 仍只列 0/1/2 的差异，SDK 不因旧 enum 回退 type 3。

### [x] D-2：批量下单的“逐单失败”与请求级失败边界

- 官方下单说明强调批量结果可逐单成功/失败。
- 真实 tick 违规请求却返回顶层 HTTP 400 且整批未创建订单。
- 客户端应按两层模型处理：先本地完成请求级 schema/tick 校验；发送后同时支持顶层错误和逐单结果，绝不能假设任何错误都只影响一单。
- 已在 P0-5 / `v1.17.0` 落地：顶层错误按请求级处理，HTTP 200 逐单错误保持逐单语义，返回结果全程与输入对齐。

### [x] D-3：订单状态枚举/大小写

- OpenAPI 中部分 schema 使用 `ORDER_STATUS_LIVE` 一类名称。
- 生产查询和历史代码中可见 `LIVE` 等值。
- [x] 新增 `types.OrderStatus` 规范枚举和 `NormalizeOrderStatus`，兼容小写、全大写、`ORDER_STATUS_*`、混合大小写及 `CANCELED/CANCELLED`。
- [x] `OpenOrder`、`OrderPostResponse` 保留 wire 原值到 `RawStatus`，公开 `NormalizedStatus()`；现有 `Status string` 字段不改类型，避免下游源码破坏。
- [x] `Known/IsOpen/IsTerminal` 提供分类；未知未来状态保留并规范化，不因 enum 扩展而 JSON 解码失败。
- [x] Accepted、unknown、settlement、definitely-not-submitted 判断统一使用规范枚举；对账路径不会再丢失原始状态。
- [x] 生产 PostOnly 挂单→查询→撤单实测：提交响应 `raw="live"`，订单查询 `raw="LIVE"`，两者均规范为 `live`；订单成功撤销，最大 maker 名义金额 0.05 USDC，未成交。

### [ ] D-4：Readonly API key 端点未出现在当前公开 OpenAPI 路径表

- SDK 公开了 `/auth/readonly-api-key` 和 `/auth/readonly-api-keys` 的创建、列举、删除方法。
- 当前 CLOB OpenAPI 路径表只明确列出普通 API key 与 Builder API key；schema 描述中仍提到 readonly key。
- 这可能是官方 spec 漏项，也可能是旧私有/已迁移接口。不要仅凭 OpenAPI 直接删除；先用无副作用查询确认、查看官方 SDK，再决定保留、迁移或 Deprecated。

---

## 已核对且当前方向正确的部分

以下不需要因为本次审计而回退：

- [x] 当前 V2 Exchange、Neg Risk Exchange、CTF、pUSD、适配器、Deposit Wallet factory/beacon、Safe/Proxy factory 常量与官方 Contracts 文档的当前地址方向一致。
- [x] 批量下单已知道单个 HTTP 批次最多 15 单；问题在错误语义、tick 和跨批结果，而不是这个上限。
- [x] 已有 V2 split/merge/redeem、approval/readiness、Deposit Wallet 部署和部分 Relayer 状态查询，适合作为后续统一 wallet workflow 的基础。
- [x] 已有 neg-risk 预取思路；需要补的是正确协议默认值、并发安全、缓存生命周期和失败语义。

## 建议实施批次

### [x] 第一批：阻断错误交易请求

P0-1 至 P0-5、P0-12 已完成。业务层必须为每笔订单显式提供 tick size；批量返回区分已关闭、确定未提交、结果未知和逐单业务拒绝。matched 响应由 SDK 自动补全异步交易哈希，网络结果不明确时先按确定性 order hash 对账，不会直接重下。

### [x] 第二批：修复返回数据和实时连接

P0-6 至 P0-14、P1-3、P1-4、P1-8、P1-10 已完成。

### [x] 第三批：稳定性和可恢复性

P1-1 至 P1-10 已完成；资金路径采用 mock、只读生产验证与此前经授权的小额 production contract tests。至此全部 P0/P1 已关闭。

### [ ] 第四批：按业务批准补齐 API

从 P2 中逐模块批准。Bridge、Combos RFQ 和 Rewards 建议各自独立 client，避免继续扩大单个接口。

### [ ] 第五批：决定是否支持 Perps

如果要支持，单独设计 `perps` 模块和版本策略；不建议将其视为现有 CLOB client 的几个新增方法。

## 全局验收门槛

- [ ] 所有官方模型至少有一组来自官方 schema/example 的 JSON golden test。
- [ ] CLOB 签名/金额与至少一个官方 SDK 使用相同输入逐字节比对。
- [ ] 批量下单覆盖：全部成功、逐单业务失败、顶层请求失败、网络超时、未知状态对账、跨 15 单分批。
- [ ] WebSocket 使用 mock server 验证初始订阅、ping/pong、动态订阅、断线重连和 schema。
- [ ] `go test ./...`、`go test -race ./...` 通过；真实资金 integration test 必须显式环境变量 opt-in，默认永不下单。
- [ ] CI 检查官方 OpenAPI/AsyncAPI 变更（至少固定版本 hash 或定期生成 diff），避免下一次协议迁移只能靠线上报错发现。
- [ ] 任何日志、错误和测试快照不得包含私钥、API secret、passphrase、完整 Authorization/HMAC header。
