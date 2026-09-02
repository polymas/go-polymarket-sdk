# 示例索引

示例按副作用分为只读、交易和链上三组。协议探针、修复脚本和冒烟工具不再与教学示例混放，统一位于 [`cmd/diagnostics`](../cmd/diagnostics/README.md)。

SDK 不会自动加载 `.env`。运行前先注入所需环境变量，并阅读目标目录的 `main.go` 文件头。

## 只读示例

| 示例 | 用途 |
| --- | --- |
| [`readonly/basic-usage`](readonly/basic-usage/main.go) | CLOB、Gamma 与认证客户端的最小入口 |
| [`readonly/eoa-query`](readonly/eoa-query/main.go) | 查询 EOA 钱包与链上余额 |

```bash
go run ./examples/readonly/basic-usage
```

## 交易示例

| 示例 | 用途 |
| --- | --- |
| [`trading/v2-buy-btcup`](trading/v2-buy-btcup/main.go) | V2 下单 |
| [`trading/v2-cancel-one`](trading/v2-cancel-one/main.go) | 查询并撤销单个订单 |
| [`trading/v2-place-cancel-smoke`](trading/v2-place-cancel-smoke/main.go) | 下单后撤单的完整冒烟流程 |
| [`trading/eoa-btc5m-double-quote`](trading/eoa-btc5m-double-quote/main.go) | EOA 双边报价场景 |

这些程序会创建或撤销真实订单，并可能成交。只使用隔离的小额钱包。

## 链上示例

| 示例 | 用途 |
| --- | --- |
| [`onchain/v2-ensure`](onchain/v2-ensure/main.go) | V2 readiness 检查与补齐 |
| [`onchain/v2-approve`](onchain/v2-approve/main.go) | V2 抵押物授权 |
| [`onchain/v2-approve-adapters`](onchain/v2-approve-adapters/main.go) | adapter 授权 |
| [`onchain/v2-eoa-approve`](onchain/v2-eoa-approve/main.go) | EOA 路径授权 |
| [`onchain/v2-wrap`](onchain/v2-wrap/main.go) | USDC.e → pUSD |
| [`onchain/v2-auto-claim`](onchain/v2-auto-claim/main.go) | auto-claim 偏好 |
| [`onchain/v2-split-merge-redeem`](onchain/v2-split-merge-redeem/main.go) | position split/merge/redeem |
| [`onchain/redeem-eoa-direct`](onchain/redeem-eoa-direct/main.go) | EOA 直接 redeem |
| `onchain/withdraw-pusd*` | 不同钱包路径的 pUSD 提现 |

链上示例可能签名授权、转账或提交交易。运行前先阅读 [V2 业务接入](../docs/v2-guide.md)，核对钱包类型、合约、condition ID 和金额。

## 目录约定

每个叶子目录都是独立的 `package main`：

```text
examples/
├── readonly/
├── trading/
└── onchain/
```

新增示例时按副作用选择目录；诊断性命令放入 `cmd/diagnostics`，不要使用 `extended`、`misc` 等模糊分类。
