# 测试指南

测试按副作用分为本地测试、公开网络契约测试、认证测试和真实写入测试。不要仅按文件名判断安全性，运行前应查看测试中的启用条件。

## 日常检查

```bash
go test -short ./...
go vet ./...
```

`-short` 会跳过明确标记的 RPC/WSS 长测试；仓库仍有部分历史契约测试会读取公开 API。完全离线环境应按包或测试名运行已确认的单元测试。

完整测试：

```bash
go test ./...
```

也可以使用 Makefile：

```bash
make test-short
make test-all
make test-coverage
```

`make test` 只查找 `*_readonly_test.go`，它是只读冒烟入口，不代表完整单元测试集。

## 常用测试数据

从 `.env.example` 复制本地配置，按测试需要填写：

```bash
cp .env.example .env
source .env
```

| 变量 | 用途 |
| --- | --- |
| `POLY_TEST_TOKEN_ID` | outcome token，通常是 uint256 十进制字符串 |
| `POLY_TEST_CONDITION_ID` | condition ID |
| `POLY_TEST_MARKET_ID` | Gamma market ID |
| `POLY_TEST_MARKET_SLUG` | Gamma market slug |
| `POLY_PRIVATE_KEY` | 认证或签名测试 |
| `POLY_SIGNATURE_TYPE` | 与测试账户匹配的钱包类型 |

单个测试可能要求额外变量，以测试文件开头和 `t.Skip`/`os.Getenv` 判断为准。

## 联网与真实写入

公开网络契约测试通常由 `POLYMARKET_LIVE_TEST=1` 或专用 `POLY_RUN_*` 开关启用。真实下单、撤单、授权、split/merge 等测试使用更具体的开关，例如：

- `POLY_RUN_MARKET_ORDER_INTEGRATION=1`
- `POLY_RUN_TICK_SIZE_INTEGRATION=1`
- `POLY_RUN_CANCEL_FILTER_INTEGRATION=1`
- `POLY_RUN_V2_ALLOWANCE_INTEGRATION=1`
- `POLY_RUN_SPLIT_MERGE_INTEGRATION=1`

这些开关不是通用承诺；运行前仍需打开对应测试，确认 token、condition、金额、钱包和网络。不要在持有重要资金的钱包上运行集成测试。

建议逐个执行：

```bash
go test ./clob -run '^TestName$' -v -count=1
```

## 排查顺序

1. 先用 `go test -short` 区分本地逻辑错误与外部服务问题。
2. 使用 `-run` 缩小到单个测试，添加 `-count=1` 避免测试缓存。
3. 检查环境变量是否对应同一钱包、链和市场。
4. 对 429、502、504、连接重置等错误单独判断，不要直接当作 SDK 逻辑回归。
5. 写请求超时后先查订单、交易或 receipt，确认没有成功再重试。

历史测试状态清单位于 [`archive/testing-legacy.md`](archive/testing-legacy.md)，它是时点记录，不代表当前通过率。
