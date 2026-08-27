# go-polymarket-sdk 官方兼容性审计与待办

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
2. Data、Gamma、User WS、Sports WS 和 RFQ 中有多处仍使用旧端点或旧字段，部分方法即使 HTTP 成功也会返回关键字段为零值。
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

### [ ] P0-4：禁止静默把订单 size 改成 5，改为读取市场 `min_order_size`

当前实现：V1/V2 批量循环中，只要 `Size < 5` 就无提示地改成 `5.0`。

官方基线：最小数量是市场配置 `min_order_size`，可从订单簿或 CLOB market info 获得，并非全局常数 5。

风险：调用方意图下 1 份，SDK 可能替其签名并发送 5 份，属于真实资金风险。

建议验收：

- [ ] 删除静默改量逻辑。
- [ ] 按市场配置预校验，返回包含 requested/minimum/tokenID 的 typed error。
- [ ] 如果提供自动调量选项，必须显式 opt-in，并在结果中返回最终签名数量。

### [ ] P0-5：保证批量请求逐单对齐，不能静默丢掉签名失败的订单

当前实现：

- V1/V2 构建某个 signed order 失败时直接 `continue`，该订单从请求体中消失。
- 方法注释承诺返回结果与输入等长、同序，但本地签名失败会破坏请求/响应下标语义。
- 超过 15 单后自动切批；某一子批 HTTP 失败会被转换成合成 `OrderPostResponse`，最终返回 `nil` error，调用方可能误以为函数整体成功。

官方基线：`POST /orders` 单次最多 15 单；正常业务拒绝可在逐单响应中体现，但请求级验证失败可以直接返回非 2xx。参见 [CLOB OpenAPI](https://docs.polymarket.com/api-spec/clob-openapi.yaml) 和 [Error Codes](https://docs.polymarket.com/resources/error-codes.md)。

建议验收：

- [ ] 任一订单在本地构造/签名失败时，默认整个本地调用不发网络请求并返回 indexed error。
- [ ] 如要支持“只发送合法订单”，必须用显式选项，并返回 `inputIndex`，不能靠数组下标猜测。
- [ ] 定义 `BatchOrderResult`：输入索引、本地校验结果、是否已发送、HTTP 状态、orderID、服务端 error、状态是否未知。
- [ ] 跨多个 15 单子批时，返回结构化 `PartialBatchError`；不能用 `nil` error 隐藏 HTTP 失败。
- [ ] 为 tick 违规、余额不足、签名错误、market closed、post-only crossing、网关超时分别做测试。

### [ ] P0-6：修正 Data API 的 Trade、Activity、Value 模型

当前实现与官方字段冲突：

- `Trade` 使用 `market`、`asset_id`、`user`、`cash_amount`、`token_amount`；当前 Data API 使用 `conditionId`、`asset`、`proxyWallet`、`size` 等 camelCase 字段，并带市场和用户元数据。
- `Activity` 使用 `market`、`asset_id`、`tokens`、`cash`、`user`；当前 API 使用 `conditionId`、`asset`、`size`、`usdcSize`、`proxyWallet` 等字段。
- `ValueResponse.TotalValue` 标记为 `total_value`；当前响应字段是 `value`。
- `/value` 官方返回数组（按 market 可能有多项），当前 `GetValue` 只返回第一项，调用多个 condition 时会静默丢数据。
- Trade/Activity 的自定义 timestamp 解析失败时写入 `time.Now()`，这会把坏数据伪装成最新事件。

风险：请求可以返回 200，但 condition、token、user 和金额会悄悄变成零值；上层统计和对账结果错误。

建议验收：

- [ ] 按 [Data OpenAPI](https://docs.polymarket.com/api-spec/data-openapi.yaml) 重建当前模型；如保留旧模型，放入明确的 `legacy` 包。
- [ ] Trade 覆盖 conditionId、asset、proxyWallet、side、size、price、timestamp、title/slug/icon/eventSlug、outcome/outcomeIndex、用户资料、transactionHash。
- [ ] Activity 覆盖 size、usdcSize、transactionHash、name/pseudonym、事件元数据和 `isCombo`。
- [ ] 活动类型补齐 `DEPOSIT`、`WITHDRAWAL`、`YIELD`、`MAKER_REBATE`、`TAKER_REBATE`、`REFERRAL_REWARD` 等当前枚举。
- [ ] `ValueResponse` 改为 `{user, value}`；`GetValue` 返回完整数组，另提供显式的 total helper，而不是默认取第一项。
- [ ] 时间解析失败必须返回错误或保留原始值，不能使用当前时间兜底。
- [ ] 用官方 schema 示例做 JSON golden tests，断言关键字段非零且可回序列化。

### [ ] P0-7：重做 User WebSocket 鉴权和订阅报文

当前实现：

- `WebSocketAuth` 是 `address/signature/timestamp/nonce`。
- 初始类型发送大写 `USER`。
- 未实现按 `markets` 动态订阅/退订。

官方基线：[User Channel](https://docs.polymarket.com/api-reference/wss/user.md) 要求 auth 为 `apiKey/secret/passphrase`，初始 `type` 为小写 `user`，可选 `markets` 过滤并支持动态订阅。

建议验收：

- [ ] 用独立的 `UserWSAuth`，字段严格匹配官方 schema；不要复用 L1 签名模型。
- [ ] 初始订阅和动态订阅使用官方小写 type/operation/markets 结构。
- [ ] 完整解析 order placement/update/cancellation 与 trade 状态事件。
- [ ] 重连后自动恢复 auth 和市场过滤器；回调中提供原始 event ID/状态用于去重。
- [ ] 增加本地 mock server 协议测试和真实只读监听测试。

### [ ] P0-8：修正 Sports WebSocket 地址、握手和心跳方向

当前实现：

- 连接 `wss://ws-subscriptions-clob.polymarket.com/ws/sports`。
- 连接后发送 `{"type":"SPORTS"}`，客户端主动定时发 JSON `"PING"`。
- 仅在 payload 包含 `event_type == "sports"` 时处理。

官方基线：[Sports Channel](https://docs.polymarket.com/api-reference/wss/sports.md) 地址为 `wss://sports-api.polymarket.com/ws`，无需订阅报文；服务端每 5 秒发送纯文本 `ping`，客户端须在 10 秒内回复纯文本 `pong`；比赛更新本身没有上述 `event_type` 包装。

建议验收：

- [ ] 更换 URL，删除 SPORTS 订阅包。
- [ ] 收到服务端纯文本 `ping` 后回复纯文本 `pong`，不要使用 `WriteJSON("PING")`。
- [ ] 按官方 Sports event schema 解码全部字段。
- [ ] 用 mock server 验证 10 秒心跳规则、重连和事件解析。

### [ ] P0-9：补齐 Market WebSocket 事件，暴露 tick size 变化

当前实现：初始订阅发送大写 `MARKET`，只处理 `book`；心跳使用 JSON 编码的 `"PING"`。

官方基线：[Market Channel](https://docs.polymarket.com/api-reference/wss/market.md) 使用小写 `market`，支持 `initial_dump`、level 和 feature flags，并发布：

- `book`
- `price_change`
- `last_trade_price`
- `tick_size_change`
- `best_bid_ask`
- `new_market`
- `market_resolved`

建议验收：

- [ ] 严格按 AsyncAPI 发送初始/动态订阅和纯文本 ping/pong。
- [ ] 为上述每类事件提供 typed model 和回调/统一 event stream。
- [ ] 以 typed event 暴露 `tick_size_change`，由业务层原子更新自己的 market config cache。
- [ ] 支持 initial dump、订阅 level 和 custom feature。
- [ ] 重连时恢复订阅并处理重复快照/增量事件。

### [ ] P0-10：废弃当前 RFQ 旧端点，避免暴露看似可用但官方已不存在的方法

当前 `rfq` 包请求 CLOB 主域下的：

- `/rfq/request`
- `/rfq/quotes`
- `/rfq/accept`
- `/rfq/cancel`

这些端点不在当前官方 CLOB 或 Combos RFQ OpenAPI 中，也没有当前 requester/maker 鉴权模型。

官方当前能力是 Combos RFQ：maker REST 端点位于独立服务，使用 CLOB L2 鉴权；quoter 还有独立的 [RFQ WebSocket](https://docs.polymarket.com/api-reference/wss/rfq.md)。

建议验收：

- [ ] 立即给旧 `rfq.Client` 和全部方法添加 Deprecated/unsupported 标记，默认不要自动调用旧端点。
- [ ] 根据官方 requester 与 maker 模型重新设计包，不在旧类型上硬补字段。
- [ ] 支持 quote 的六位定点字符串、签名类型、last-look confirmation 和 execution update。
- [ ] REST 与 WS 共用身份配置，但分别实现各自鉴权和重连逻辑。

### [ ] P0-11：修正 Gamma 中已失效或返回形状不匹配的端点

已确认冲突：

- `GetProfile` 调用 `/profiles/{address}`；当前公开端点是 `/public-profile?address=...`。
- `GetProfileByUsername` 调用 `/profiles/username/{username}`；当前官方公开 API 没有该端点。
- `GetSeriesBySlug` 调用 `/series/slug/{slug}`；当前官方是 `/series-summary/slug/{slug}`，且返回 `SeriesSummary`，不是完整 `Series`。
- `GetComments` 使用 `market_id`；当前参数是 `parent_entity_type` 与 `parent_entity_id`。
- `GetComment` 按单对象解码；当前 `/comments/{id}` schema 返回数组。
- `GetMarketTradesEvents` 调用 Gamma `/live-activity/events/{marketID}`；当前 Gamma spec 无此路径，CLOB 的 live activity 是另一组端点和模型。
- `GetAllMarkets` 固定以 `limit=500` 请求 `/markets/keyset`；官方该端点最大 `100`，因此会返回 422，而代码不会进入后续分页。

建议验收：

- [ ] 迁移 profile 地址查询；username 查询明确删除、Deprecated 或通过有官方依据的搜索流程实现。
- [ ] 新建 `GetSeriesSummaryBySlug/ID`，保留正确返回类型。
- [ ] Comments API 改为通用 parent entity 参数。
- [ ] 删除或迁移 `GetMarketTradesEvents` 到 CLOB live-activity client。
- [ ] `GetAllMarkets` 的 keyset page size 改为不超过 100，并只依据 `next_cursor` 终止，不用“返回数量少于请求值”替代游标语义。
- [ ] 旧端点测试不能再遇到 404 后直接 `Skip`；必须用契约测试锁定现行路径。

### [ ] P0-12：扩展下单响应并实现成交结算对账

当前 `OrderPostResponse` 只有 `orderID/status/errorMsg`，会丢失官方响应中的：

- `success`
- `makingAmount`
- `takingAmount`
- `transactionsHashes`
- `tradeIDs`

官方 2026-07 的行为更新还意味着成功的 FAK/FOK 不一定在下单响应内立即给出交易哈希，需要依据 `tradeIDs` 查询成交并等待 settlement。

建议验收：

- [ ] 完整建模 `SendOrderResponse`，金额优先使用十进制定点字符串。
- [ ] 实现已认证的 `GetTrades`，并能按 trade/order ID 查询。
- [ ] 提供 `WaitForOrderFillSettlement(ctx, ...)`，区分 matched、mined、confirmed、failed、timeout。
- [ ] 网络超时后将订单状态标记为 unknown，并按确定性 order hash/订单查询做 reconciliation，禁止直接无脑重下。

### [ ] P0-13：迁移 RTDS URL、订阅协议和心跳

当前实现连接 `wss://rtds.polymarket.com/ws`，用 `{type:"subscribe", stream, ids}` 订阅 prices/comments，并每 15 秒 `WriteJSON("PING")`。

官方当前 [Chainlink TWAP Prices](https://docs.polymarket.com/market-data/chainlink-twap.md) 低层协议连接 `wss://ws-live-data.polymarket.com`，订阅报文为 `{action:"subscribe", subscriptions:[{topic,type,filters}]}`；应用层心跳是每 5 秒发送纯文本 frame `PING`。30s/60s TWAP 分别使用 `crypto_prices_twap_thirty` 和 `crypto_prices_twap_sixty`。

风险：当前 RTDS client 即使能建立 TCP/WebSocket，也无法按当前协议订阅或保活；旧 `EncryptedPriceUpdate` 模型也不是当前 TWAP payload。

建议验收：

- [ ] 更换当前官方 URL，并支持可注入 URL 便于 mock 测试。
- [ ] 改成 action/subscriptions 协议；filters 必须按官方要求生成紧凑 JSON 字符串。
- [ ] 用 `WriteMessage(TextMessage, []byte("PING"))` 每 5 秒发纯文本心跳。
- [ ] 新建精确 decimal string 的 TWAP event，区分 outer publish timestamp 与 payload observation timestamp。
- [ ] 旧 prices/comments stream 先确认服务是否仍支持；无法从当前官方文档确认的接口移入 legacy 并 Deprecated。
- [ ] 重连自动恢复全部 topic/window/filter，增加 30s/60s 和无 symbol filter 的协议测试。

### [ ] P0-14：移除或隔离已经失效的 The Graph Subgraph client

当前 `subgraph` 包硬编码 `https://api.thegraph.com/subgraphs/name/polymarket/clob-v2`；仓库测试已经明确备注该端点被移除，并在请求失败时 Skip。当前官方文档目录也不再提供这组 Subgraph API。

风险：README 仍把它当作 SDK 能力，调用方运行后才发现端点不可用；测试 Skip 又掩盖了长期失效。

- [ ] 从默认公开能力/README 中删除，或者整体标记 Deprecated + unsupported。
- [ ] volume、positions、OI、PnL 等功能优先迁移到当前 Data API；链上独有数据再使用可配置的 indexer/RPC。
- [ ] 禁止对已知失效端点用 Skip 维持“绿灯”；测试应明确断言 legacy client 已禁用。
- [ ] 如果保留通用 GraphQL client，URL 必须由调用方注入，且不能再承诺旧 Polymarket schema。

---

## P1：核心流程不完整或行为不安全

### [ ] P1-1：对齐订单类型和 GTD 规则

当前常量为 `GTC/IOC/FOK`；官方当前枚举为 `GTC/FOK/GTD/FAK`。

- [ ] 移除或 Deprecated `IOC`，明确迁移到 `FAK`。
- [ ] 增加 `GTD`、`FAK`。
- [ ] GTD 必须带合法 expiration，并满足官方最小提前时间规则。
- [ ] FOK/FAK/PostOnly 的互斥与响应语义做表驱动测试。

### [ ] P1-2：实现官方市价单构造流程

已有 `MarketOrderArgs` 类型，但没有公开的 create/post market order 流程。

- [ ] 实现 BUY 按金额、SELL 按份额的市价单构造。
- [ ] 支持用户指定 worst price / slippage guard。
- [ ] 提交前基于订单簿估算可成交量；深度不足时在本地返回明确错误。
- [ ] 实现官方建议的 market price estimation，并覆盖 FOK/FAK。

### [ ] P1-3：扩展订单簿模型并统一单个/批量返回类型

单个 `OrderBookSummary` 目前只保留 token/bids/asks，丢失 market、timestamp、hash、min_order_size、tick_size、last_trade_price；批量模型字段又不同。

- [ ] 单个与批量共用一个完整 `OrderBookSummary`。
- [ ] 价格和数量使用精确十进制类型。
- [ ] 将 `min_order_size/tick_size` 完整暴露给业务层，用于构造显式的下单参数和预校验配置。
- [ ] 针对空订单簿、null last trade 和未交易 token 建立测试。

### [ ] P1-4：补齐单订单查询、撤单过滤与限制校验

- [ ] 实现官方 `GET /data/order/{orderID}`，不要只用 GetOrders 的筛选参数替代。
- [ ] `CancelMarketOrders` 同时支持官方的 market/asset 过滤组合，而不是只接 condition ID。
- [ ] `CancelOrders` 本地校验或自动分块遵循官方单次最多 1000 个 order ID。
- [ ] 对 canceled/not_canceled 提供稳定 typed result。

### [ ] P1-5：重构 HTTP 错误、重试和 context

当前通用 HTTP 层主要返回格式化字符串错误，`errors`/middleware 中的结构没有贯穿公开客户端。

- [ ] 所有公开请求接收 `context.Context`，允许调用方取消和设置 deadline。
- [ ] 定义 `APIError`：service、method、path、status、requestID、errorCode、message、retryAfter、原始响应（脱敏）。
- [ ] 处理官方 425、429、503 等状态；只对幂等请求或明确可安全重试的操作自动重试。
- [ ] 下单/Relayer 超时必须返回 ambiguous outcome，先对账再决定是否重发。
- [ ] 日志统一脱敏 API secret、passphrase、私钥、签名和完整鉴权头。

### [ ] P1-6：为 SDK 内部 negRisk/feeRate 缓存增加并发安全和生命周期

当前 negRisk/feeRate map 没有明显的统一锁、TTL 和失效策略；tick 已改为业务层显式传入，SDK 不再隐藏缓存。

- [ ] 使用并发安全缓存；避免批量签名与 WS 更新产生 data race。
- [ ] cache entry 带 fetchedAt/source/version。
- [ ] 支持手动 invalidate、WS invalidate、TTL refresh 和 singleflight。
- [ ] `go test -race` 覆盖并发批量下单与缓存更新。

### [ ] P1-7：使用市场实时 fee rate，而不是依赖调用方或旧默认值

- [ ] 下单前从 market config/cache 获取当前 fee rate。
- [ ] 调用方传入 fee 时与官方值校验，不一致返回明确错误。
- [ ] fee 更新时刷新缓存，并用官方 SDK golden vectors 验证签名。

### [ ] P1-8：修正 last-trade 的 null/缺项语义

官方行为中，单 token 从未成交时可能返回 null；批量查询可能直接省略未成交 token。

- [ ] 单查询返回 `(value, found)`、pointer 或显式 NotTraded 状态。
- [ ] 批量结果按 token ID 映射，不假设返回长度和输入一致。
- [ ] 对 null、缺项、重复输入和乱序响应做测试。

### [ ] P1-9：完善新钱包 onboarding

当前 web3 已支持多种签名类型、Deposit Wallet 部署和 V2 approvals，但 CLOB 创建流程仍要求调用方自行拼装多个步骤。

- [ ] 提供安全的 wallet/account resolver，明确 signer、funder/maker、签名类型和 wallet 类型。
- [ ] 对新 Deposit Wallet 提供 deploy → fund → approvals → ready check 的可恢复流程。
- [ ] 每一步都可重复执行且幂等；失败后可从链上/Relayer 状态恢复。
- [ ] 文档明确 EOA、legacy proxy、Safe、Deposit Wallet 的适用方法。

### [ ] P1-10：更新 `chainws` 的当前合约监听集合和余额模型

当前 `chainws.polymarketContractAddresses()` 仍监听 legacy Exchange、legacy Neg Risk Exchange、USDC.e 和已退役 Neg Risk Adapter，没有加入 V2 exchanges、pUSD、Collateral Onramp/Offramp。

风险：Tracker 会漏掉当前 V2 交易/余额变化，或者继续把已退役合约当作主要数据源；`GetPositions` 结果可能长期不刷新。

- [ ] 按官方 Contracts 文档建立带版本和用途的合约 registry，不再手写一组无版本地址。
- [ ] 默认监听 V2 exchanges、CTF、pUSD 和当前 adapters；legacy 监听必须显式 opt-in。
- [ ] 区分 USDC.e、pUSD 和 ERC-1155 position balances，不能合并成含义不明的“USDC/token”。
- [ ] 处理 removed logs/reorg，并提供从 RPC 快照重新对账的恢复路径。
- [ ] 用真实交易 receipt 的 logs 做 replay test，验证当前买入、卖出、split、merge、redeem、wrap/unwrap。

---

## P2：官方已有、当前 SDK 尚未覆盖的预测市场功能

以下属于“缺功能”，不一定是现有实现 bug。建议按实际业务勾选。

### [ ] P2-1：CLOB 公共市场数据与市场目录

- [ ] `GET /clob-markets/{condition_id}`：完整 CLOB market config。
- [ ] `GET /markets-by-token/{token_id}`。
- [ ] `POST /markets/live-activity` 与 `GET /markets/live-activity/{condition_id}`。
- [ ] `POST /batch-prices-history`。
- [ ] sampling/simplified market 的官方 CLOB 版本与游标分页。
- [ ] tick-size、fee-rate、neg-risk 的 path 参数版本（若需要完整端点对称性）。

### [ ] P2-2：CLOB 认证交易与 Builder 数据

- [ ] `GET /data/trades`：已认证成交查询、游标分页和过滤器。
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

### [ ] P2-4：Gamma 补全

- [ ] status。
- [ ] teams 列表与按 ID 查询。
- [ ] related tags 四组关系查询。
- [ ] events pagination、results、tweet-count、comments count、event tags、creators、creator by ID。
- [ ] market description、market tags。
- [ ] markets information、markets abridged。
- [ ] events keyset；markets keyset 对外暴露 next cursor，并遵守官方 page size 上限。
- [ ] series by ID、series comments count、series summary by ID/slug。
- [ ] comments by user address。
- [ ] sports metadata、sports market types。
- [ ] Gamma 新增过滤器与字段：decimalized、rfq_enabled、tag_match、locale 等。

### [ ] P2-5：Data API 补全

- [ ] health。
- [ ] accounting snapshot。
- [ ] approvals。
- [ ] combo activity、combo positions。
- [ ] holders。
- [ ] traded / markets traded。
- [ ] revisions。
- [ ] open interest。
- [ ] live volume。
- [ ] closed positions。
- [ ] other positions。
- [ ] market positions。
- [ ] builder leaderboard、builder volume。
- [ ] user leaderboard。
- [ ] positions 的 `includeArchived`、grossInitialValue、entryFeesUsdc 等当前字段。

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

### [ ] D-1：签名类型 3 的文档差异

- Wallet/RFQ 等当前文档已出现签名类型 `3`（POLY_1271 / Deposit Wallet）。
- CLOB OpenAPI 某些枚举位置仍只列 `0/1/2`。
- 当前 SDK 已实现 type 3 的大量路径。建议保留，通过当前钱包文档、官方 SDK 和真实请求做 golden/integration test，并给官方文档差异留注释。

### [ ] D-2：批量下单的“逐单失败”与请求级失败边界

- 官方下单说明强调批量结果可逐单成功/失败。
- 真实 tick 违规请求却返回顶层 HTTP 400 且整批未创建订单。
- 客户端应按两层模型处理：先本地完成请求级 schema/tick 校验；发送后同时支持顶层错误和逐单结果，绝不能假设任何错误都只影响一单。

### [ ] D-3：订单状态枚举/大小写

- OpenAPI 中部分 schema 使用 `ORDER_STATUS_LIVE` 一类名称。
- 生产查询和历史代码中可见 `LIVE` 等值。
- 建议保留原始 status，并提供兼容解析/归一化枚举；先录制当前真实响应再收紧校验。

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

### [ ] 第一批：阻断错误交易请求

P0-1 至 P0-3 已完成；继续处理 P0-4、P0-5、P0-12。业务层现在必须为每笔订单显式提供 tick size，非网格价格会在整批提交前失败；min size 策略仍待 P0-4 处理。

### [ ] 第二批：修复返回数据和实时连接

P0-6 至 P0-11、P0-13、P0-14，然后完成 P1-3、P1-4、P1-8、P1-10。

### [ ] 第三批：稳定性和可恢复性

P1-1、P1-2、P1-5 至 P1-9；把真实资金路径纳入 mock + testnet/小额 production contract tests。至此全部 P0/P1 应关闭。

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
