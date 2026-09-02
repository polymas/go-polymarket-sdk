# 不兼容变更迁移说明

本文按日期记录源码级 breaking changes（最新在后）。仓库尚未在本文中声明新的发布版本；正式发布时应在 release notes 中再次列出这些变化。

## 2026-09-01 — 目录重排与公共包移除

### 已移除的公共包

#### `config`

`config` 从未接入领域客户端，并包含已经失效的 V1/V2 开关和未实现的统一 cache 配置，因此整体移除。

- CLOB、Gamma、Data 分别使用各自构造函数。
- Polygon chain、签名类型和 RPC URL 直接传给 `web3.NewClient`/`relayer.NewGaslessClient`。
- SDK 不再提供看似全局、实际不生效的配置对象。

#### `middleware`

`middleware` 没有接入 SDK transport，并与真实网络层形成重复的超时和重试策略，因此整体移除。

- 调用方通过 `Context` 设置 deadline 和取消。
- SDK 的幂等重试、`Retry-After`、错误脱敏与结果未知边界由 `internal/transport` 统一实现。
- `internal/transport` 不是公共 API，外部项目不能导入。

#### `http`

低层泛型 HTTP 包整体移除。业务代码应调用 `clob`、`gamma`、`data`、`rfq` 或 `web3` 的领域方法，不再绕过 SDK 协议边界直接发送请求。

如果现有业务使用了 `http.GetRaw` 等函数，应先确认所需端点属于哪个领域包；缺失端点应在对应领域 client 中补充带 `Context` 的类型化方法。

#### `test`

测试 helper 已迁入 `internal/testutil`，仅供本仓库测试使用。外部项目应维护自己的 fixture 和环境配置，不应依赖 SDK 仓库的集成测试数据。

### 已移除的错误 API

以下只服务旧 middleware 的 API 已移除：

- `errors.ErrorType`
- `errors.SDKError`
- `NewNetworkError`、`NewAPIError`、`NewValidationError`、`NewAuthError`
- `NewRateLimitError`、`NewTimeoutError`、`WrapError`
- `IsRetryableError`、`GetErrorType`

当前错误处理方式：

```go
var apiErr *sdkerrors.APIError
var ambiguous *sdkerrors.AmbiguousOutcomeError

switch {
case errors.As(err, &ambiguous):
	// 写请求结果未知，先对账再决定是否重试。
case errors.As(err, &apiErr):
	// 使用 Status、RequestID、RetryAfter 和 Retryable。
case errors.Is(err, context.DeadlineExceeded):
	// 调用方 deadline 到期。
}
```

### 命令与示例路径

- 维护命令：`cmd/<command>`
- 协议诊断：`cmd/diagnostics/<command>`
- 只读示例：`examples/readonly/<example>`
- 交易示例：`examples/trading/<example>`
- 链上示例：`examples/onchain/<example>`

完整索引见 [`cmd/README.md`](../cmd/README.md) 和 [`examples/README.md`](../examples/README.md)。

## 2026-09-02 — 拆分 `web3`：新增 `web3/relayer` 子包

### 改了什么

原来 6700 行的 `web3` 包同时承担两件互不相干的事：只读链上查询，以及
gasless（Polymarket relayer）交易的构造/签名/提交。现在按职责物理拆开：

| 原位置 | 新位置 | 说明 |
| ------ | ------ | ---- |
| `web3` | `web3` | 只读链上查询：RPC 池与故障转移、余额/授权查询、Poly Proxy / Gnosis Safe / Deposit Wallet(CWIA) 地址派生 |
| `web3` | `web3/relayer` | Gasless 中继路径：`GaslessClient` 及其全部方法、PROXY/SAFE/CWIA relay body、V2 授权、split/merge/redeem、auto-claim、pUSD 提现、Deposit Wallet onboarding |

**标识符名称一律不变，只有包路径（前缀）变了。**

搬到 `web3/relayer`（`github.com/polymas/go-polymarket-sdk/web3/relayer`，包名
`relayer`）的导出符号：

- `GaslessClient`、`NewGaslessClient`
- `RedeemPositionInfo`、`RelayerTransactionInfo`
- `SignatureParams`、`ProxyRelayBody`、`SafeSignatureParams`、`SafeRelayBody`
- `CWIADeployBody`、`CWIADepositWalletParams`、`CWIARelayCallPayload`、`CWIARelayBody`
- `DepositWalletOnboardingStage`、`DepositWalletOnboardingResult`、
  `OnboardingStageFundingRequired` / `OnboardingStageApprovalsPending` / `OnboardingStageReady`
- `LocalSigner`、`NewLocalSigner`、`CreateLocalSigner`

留在 `web3` 的导出符号（**无需改动**）：`Client`、`NewClient`、`WalletAccount`、
`WalletType` 及其常量、`CWIACallInput`、`ErrDepositWalletNotDeployed`、
`BuildCWIABatchCalldata`，以及所有 `Client` 接口方法。

### 为什么

`GaslessClient` 直接嵌入了实现类型（`type GaslessClient struct { *baseClient; ... }`），
而 Go 不允许跨包嵌入非导出类型。要把 `GaslessClient` 挪出 `web3`，就必须先把
`baseClient` 导出。

因此本次同时引入：

- `web3.baseClient` → **`web3.BaseClient`**（导出）。它仍然只能通过
  `web3.NewClient` 构造（返回 `web3.Client` 接口），字段依然私有；导出类型名
  纯粹是为了让 `relayer.GaslessClient` 能嵌入 `*web3.BaseClient`。
- `*BaseClient` 上若干重试型 RPC 方法随之导出，供 relayer 复用同一个 RPC 池：
  `CallContractWithRetry`、`EstimateGasWithRetry`、`BalanceAtWithRetry`、
  `TransactionReceiptWithRetry`、`TransactionByHashWithRetry`、`CodeAtWithRetry`、
  `GetNextClientIndex`、`DepositWalletFactoryBeacon`。
- 新增 `web3.GetSafeProxyFactoryABI()`（原 `getSafeProxyFactoryABI`），
  relayer 侧的 Safe 代理地址计算需要它。

relayer 代码不再触碰 `BaseClient` 的私有字段，全部改走已有的导出访问器
（`GetSigner()` / `GetBaseAddress()` / `GetChainID()` / `GetSignatureType()` /
`GetPolyProxyAddress()`）。

### 迁移方法

只需加一个 import 并给受影响的符号换前缀：

```go
import (
    "github.com/polymas/go-polymarket-sdk/web3"
    "github.com/polymas/go-polymarket-sdk/web3/relayer"  // 新增
)

// 之前
client, err := web3.NewGaslessClient(pk, types.SafeSignatureType, types.Polygon, creds)
positions := []web3.RedeemPositionInfo{...}

// 之后
client, err := relayer.NewGaslessClient(pk, types.SafeSignatureType, types.Polygon, creds)
positions := []relayer.RedeemPositionInfo{...}
```

`NewGaslessClient` 的签名没有变化：它内部仍然自己调 `web3.NewClient` 建底层
客户端，调用方**不需要**先建 `web3.Client` 再传进来。只用只读能力的文件
（`web3.Client` / `web3.NewClient` / `web3.WalletAccount` 等）完全不受影响，
`clob.NewClient(web3Client)` 也照旧。

`GaslessClient` 依然嵌入 `*web3.BaseClient`，所以 `web3.Client` 接口上的全部
查询方法仍可直接在 `GaslessClient` 上调用，无需额外改写。

### 已知偏差

`LocalSigner` 原计划迁到 `signing` 包，但会形成 import 环
（`signing` → `internal` → `signing`：`LocalSigner` 需要 `internal.V2RelayerKey`
与 `internal.CreateRelayerHeaders`，而 `internal` 已经依赖 `signing`）。
由于 `LocalSigner` 只被 gasless 路径使用，最终落在 `web3/relayer`。
它此前未被 SDK 外部引用，因此实际不影响任何调用方。
