# 快速开始与配置

## 前置条件

- Go 1.24+
- 只读查询：无需钱包或 API 凭证
- CLOB 认证交易：Polygon 钱包私钥
- 链上/gasless：可用的 Polygon RPC；部分流程还需要 builder/API 凭证

安装依赖：

```bash
go get github.com/polymas/go-polymarket-sdk
```

## 只读客户端

公共数据按领域拆分，无需先创建一个全局 SDK 对象：

```go
clobClient := clob.NewReadonlyClient()
gammaClient := gamma.NewClient()
dataClient := data.NewClient()
```

- `clob` 查询订单簿、价格、价差、成交价等交易数据。
- `gamma` 查询市场、事件、标签、系列和搜索结果。
- `data` 查询地址的仓位、交易和活动。

网络调用优先选择 `...Context` 版本：

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

book, err := clobClient.GetOrderBookContext(ctx, tokenID)
```

## 认证 CLOB 客户端

SDK 不自动读取 `.env`。应用需要自己读取环境变量或接入 secrets manager，然后把值传给构造函数。

```go
privateKey := os.Getenv("POLY_PRIVATE_KEY")
if privateKey == "" {
	return errors.New("POLY_PRIVATE_KEY is required")
}

w3, err := web3.NewClient(
	privateKey,
	types.SafeSignatureType,
	types.Polygon,
)
if err != nil {
	return err
}
defer w3.Close()

client, err := clob.NewClient(w3)
if err != nil {
	return err
}
```

`clob.NewClient` 使用当前 CLOB V2 协议，并在初始化时派生/获取 CLOB API 凭证。`clob.WithV2()` 已是无操作兼容选项，新代码不需要传入。

### 签名类型

| 值 | 常量 | 钱包语义 |
| --- | --- | --- |
| `0` | `types.EOASignatureType` | maker、signer 均为 EOA |
| `1` | `types.ProxySignatureType` | 旧版 PolyProxy |
| `2` | `types.SafeSignatureType` | Gnosis Safe |
| `3` | `types.DepositWalletSignatureType` | POLY_1271 / Deposit Wallet |

签名类型必须匹配账户在 Polymarket 的实际钱包结构，不能仅凭偏好选择。新 Deposit Wallet 的部署与恢复式 onboarding 见 [V2 业务接入](v2-guide.md)。

## RPC 配置

`web3.NewClient` 的最后一个参数是可选 RPC URL 列表。省略时使用 SDK 内置列表；生产服务建议显式传入自己的节点：

```go
w3, err := web3.NewClient(privateKey, sigType, types.Polygon,
	"https://polygon-rpc.example.com",
	"https://polygon-rpc-backup.example.com",
)
```

不要把空字符串作为 RPC 参数传入；未配置时直接省略可变参数。

## 环境变量模板

仓库的 [`.env.example`](../.env.example) 是示例清单，可复制后按需填写：

```bash
cp .env.example .env
source .env
```

常用变量：

| 变量 | 说明 |
| --- | --- |
| `POLY_PRIVATE_KEY` | 私钥，仅认证/签名流程需要 |
| `POLY_SIGNATURE_TYPE` | `0`/`1`/`2`/`3`，必须与钱包一致 |
| `POLY_RPC_URL` | 示例程序使用的自定义 Polygon RPC |
| `POLY_TEST_TOKEN_ID` | 测试/示例使用的十进制 outcome token ID |
| `POLY_TEST_CONDITION_ID` | 测试市场 condition ID |
| `POLY_API_KEY/SECRET/PASSPHRASE` | 部分 relayer/提现示例使用的 HMAC 凭证 |
| `POLY_BUILDER_API_KEY/SECRET/PASSPHRASE` | builder/gasless 测试所需凭证 |

不要在 shell 历史、日志、错误信息或截图中暴露私钥和 secret。

## 下一步

- 查看[示例索引](../examples/README.md)，选择与你的钱包类型一致的流程。
- 做资金与授权自检前阅读 [V2 业务接入](v2-guide.md)。
- 查方法签名时使用 [API 与包概览](api-overview.md)中的 `go doc` 命令。
