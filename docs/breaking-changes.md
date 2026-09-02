# 不兼容变更迁移说明

本文记录 2026-09-01 目录重排产生的源码级 breaking changes。仓库尚未在本文中声明新的发布版本；正式发布时应在 release notes 中再次列出这些变化。

## 已移除的公共包

### `config`

`config` 从未接入领域客户端，并包含已经失效的 V1/V2 开关和未实现的统一 cache 配置，因此整体移除。

- CLOB、Gamma、Data 分别使用各自构造函数。
- Polygon chain、签名类型和 RPC URL 直接传给 `web3.NewClient`/`web3.NewGaslessClient`。
- SDK 不再提供看似全局、实际不生效的配置对象。

### `middleware`

`middleware` 没有接入 SDK transport，并与真实网络层形成重复的超时和重试策略，因此整体移除。

- 调用方通过 `Context` 设置 deadline 和取消。
- SDK 的幂等重试、`Retry-After`、错误脱敏与结果未知边界由 `internal/transport` 统一实现。
- `internal/transport` 不是公共 API，外部项目不能导入。

### `http`

低层泛型 HTTP 包整体移除。业务代码应调用 `clob`、`gamma`、`data`、`rfq` 或 `web3` 的领域方法，不再绕过 SDK 协议边界直接发送请求。

如果现有业务使用了 `http.GetRaw` 等函数，应先确认所需端点属于哪个领域包；缺失端点应在对应领域 client 中补充带 `Context` 的类型化方法。

### `test`

测试 helper 已迁入 `internal/testutil`，仅供本仓库测试使用。外部项目应维护自己的 fixture 和环境配置，不应依赖 SDK 仓库的集成测试数据。

## 已移除的错误 API

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

## 命令与示例路径

- 维护命令：`cmd/<command>`
- 协议诊断：`cmd/diagnostics/<command>`
- 只读示例：`examples/readonly/<example>`
- 交易示例：`examples/trading/<example>`
- 链上示例：`examples/onchain/<example>`

完整索引见 [`cmd/README.md`](../cmd/README.md) 和 [`examples/README.md`](../examples/README.md)。
