# 诊断工具

这里的命令用于协议核对、故障定位、修复或生产冒烟，不属于稳定 SDK API，也不应直接嵌入业务服务。

常用只读诊断：

| 命令 | 用途 |
| --- | --- |
| `v2-allowance-check` | 检查 V2 资金和授权状态 |
| `v2-balance-probe` | 对比 CLOB 与链上余额 |
| `v2-list-orders` | 对比 SDK 与原始订单响应 |
| `relayer-tx-status` | 查询 relayer transaction |
| `negrisk-calldata-check` | 检查 negRisk calldata |
| `poly1271-negrisk-repro` | 复刻并验证 POLY_1271 NegRisk 签名域根因/修复（纯 eth_call，零资金风险） |

其余命令可能创建凭证、修复钱包、提交授权或链上交易。运行前必须阅读对应 `main.go` 文件头，并使用隔离的小额钱包。

```bash
go run ./cmd/diagnostics/<command>
```
