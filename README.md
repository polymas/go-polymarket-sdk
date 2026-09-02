# Go Polymarket SDK

[![Go Version](https://img.shields.io/badge/go-1.24+-blue.svg)](https://go.dev/)

面向 Polymarket 的 Go SDK，覆盖 CLOB V2 交易、Gamma/Data 查询、WebSocket/RTDS 实时数据以及 Polygon 链上与 gasless 操作。

> 本项目包含真实交易和链上写操作。请先在只读模式验证 token、钱包类型和市场参数；不要把私钥或 API 凭证提交到仓库。

## 安装

```bash
go get github.com/polymas/go-polymarket-sdk
```

要求 Go 1.24 或更高版本。

## 快速开始

只读查询不需要私钥：

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/polymas/go-polymarket-sdk/clob"
)

func main() {
	tokenID := os.Getenv("POLY_TEST_TOKEN_ID")
	if tokenID == "" {
		log.Fatal("POLY_TEST_TOKEN_ID is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	book, err := clob.NewReadonlyClient().GetOrderBookContext(ctx, tokenID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("market=%s bids=%d asks=%d\n", book.Market, len(book.Bids), len(book.Asks))
}
```

运行：

```bash
POLY_TEST_TOKEN_ID='<decimal-token-id>' go run main.go
```

需要认证的 CLOB 客户端由 Web3 客户端创建：

```go
web3Client, err := web3.NewClient(
	os.Getenv("POLY_PRIVATE_KEY"),
	types.SafeSignatureType,
	types.Polygon,
)
if err != nil {
	return err
}
defer web3Client.Close()

clobClient, err := clob.NewClient(web3Client)
```

完整初始化、钱包类型和配置方式见[快速开始](docs/getting-started.md)。当前 CLOB 客户端只使用 V2 订单协议；`clob.WithV2()` 仅为旧代码兼容保留。

## 能力概览

| 包 | 用途 | 是否需要认证 |
| --- | --- | --- |
| `clob` | 订单簿、价格、订单、成交、撤单与账户接口 | 市场数据否；交易是 |
| `gamma` | 市场、事件、标签、系列、评论与搜索 | 否 |
| `data` | 仓位、交易、活动、持有人与排行榜 | 否 |
| `websocket` | CLOB market/user channel 和 sports feed | user channel 是 |
| `rtds` | RTDS 实时数据 | 否 |
| `rfq` | Combos RFQ maker REST 与 quoter WebSocket | 视接口而定 |
| `web3` | 余额、钱包解析、授权、split/merge/redeem 与 gasless | 是 |
| `chainws` | Polygon 日志订阅与钱包余额跟踪 | RPC/WSS |
| `signing` | HMAC、EIP-712 与 POLY_1271 签名 | 本地私钥/凭证 |
| `types` | 跨模块共享类型和值对象 | 否 |

完整包边界与入口见 [API 概览](docs/api-overview.md)。方法签名以源码和 `go doc` 为准：

```bash
go doc github.com/polymas/go-polymarket-sdk/clob
go doc github.com/polymas/go-polymarket-sdk/web3.GaslessClient
```

## 关键约定

- CLOB V2 抵押物是 pUSD；`GetUSDCBalance` 是兼容旧代码的 USDC.e 查询，新代码应按意图使用 `GetCollateralBalance` 或 `GetUSDCEBalance`。
- V2 订单不接收 `feeRateBps`；手续费由撮合方动态处理。
- 限价订单必须显式传入每笔订单的 tick size，SDK 不在下单路径中隐式请求。
- 优先调用带 `Context` 的方法，为网络请求设置 deadline。
- 写请求超时可能是“结果未知”，应先查询订单或链上状态再决定是否重试。
- `web3.Client`、`web3.GaslessClient`、WebSocket 和链上订阅客户端使用完后都要关闭。

## 文档

- [文档导航](docs/README.md)
- [快速开始与配置](docs/getting-started.md)
- [API 与包概览](docs/api-overview.md)
- [V2 业务接入](docs/v2-guide.md)
- [示例索引](examples/README.md)
- [测试指南](docs/testing.md)
- [架构说明](docs/architecture.md)
- [不兼容变更](docs/breaking-changes.md)
- [兼容性审计与路线图](docs/roadmap.md)

历史迁移、重构和时点性审计记录位于 [`docs/archive/`](docs/archive/README.md)，不作为当前 API 使用说明。

## 开发

```bash
go test -short ./...
go vet ./...
```

部分契约测试会访问公开网络，真实交易测试必须通过专用环境变量显式开启。详见[测试指南](docs/testing.md)。

提交代码前请确保没有把 `.env`、私钥、API secret 或测试钱包数据加入版本控制。

## 相关链接

- [Polymarket 开发者文档](https://docs.polymarket.com/)
- [Go package documentation](https://pkg.go.dev/github.com/polymas/go-polymarket-sdk)
- [问题反馈](https://github.com/polymas/go-polymarket-sdk/issues)
