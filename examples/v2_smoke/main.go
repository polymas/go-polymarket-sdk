// v2_smoke 在 clob-v2.polymarket.com 上跑最小联调验证。
//
// 必选环境变量：
//
//	poly_sec                  钱包私钥（0x 前缀可选；兼容旧名 POLY_PRIVATE_KEY）
//
// 可选环境变量：
//
//	POLY_SIGNATURE_TYPE       0=EOA, 1=Proxy (默认), 2=Safe
//	POLY_V2_TEST_TOKEN        用于 order book / midpoint / 下单的 tokenID（十进制 uint256 字符串）
//	POLY_V2_SMOKE_POST=1      打开后执行 Step B（post 小额订单并立刻 cancel，会消耗 gas / 风险）
//
// 运行方式：
//
//	go run ./examples/v2_smoke
//
// 验证成功的标志：
//
//	Step A 全部 ✅
//	Step B（如果启用）最后一行 "✅ POST + CANCEL round-trip 成功"
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/polymas/go-polymarket-sdk/clob"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	privateKey := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if privateKey == "" {
		log.Fatalf("缺少必选环境变量 poly_sec（或兼容旧名 POLY_PRIVATE_KEY）")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.ProxySignatureType)))
	testToken := os.Getenv("POLY_V2_TEST_TOKEN")
	runPost := os.Getenv("POLY_V2_SMOKE_POST") == "1"

	fmt.Println("=============================================")
	fmt.Println(" Polymarket V2 Smoke Test")
	fmt.Println(" Host: https://clob-v2.polymarket.com")
	fmt.Printf(" SignatureType: %d   TestToken: %s   PostStep: %v\n", sigType, truncate(testToken), runPost)
	fmt.Println("=============================================")

	web3Client, err := web3.NewClient(privateKey, sigType, types.Polygon)
	if err != nil {
		log.Fatalf("❌ 创建 web3 客户端失败: %v", err)
	}
	defer web3Client.Close()
	fmt.Printf("钱包地址: %s\n\n", web3Client.GetBaseAddress())

	client, err := clob.NewClient(web3Client)
	if err != nil {
		log.Fatalf("❌ 创建 V2 CLOB 客户端失败（API Key 派生可能失败）: %v", err)
	}

	stepA(client, testToken)

	if runPost {
		if testToken == "" {
			log.Fatalf("❌ Step B 需要 POLY_V2_TEST_TOKEN")
		}
		stepB(client, testToken)
	} else {
		fmt.Println("\nℹ️  Step B 未执行（设置 POLY_V2_SMOKE_POST=1 启用）")
	}
}

// Step A：不花钱的只读验证。任何一步失败都意味着 V2 host / 认证有问题。
func stepA(client clob.Client, testToken string) {
	fmt.Println("── Step A: 只读验证 ──")

	// A1：GET /time —— 最纯粹的连通性测试，不需要任何签名
	serverTime, err := client.GetTime()
	if err != nil {
		log.Fatalf("❌ A1 GetTime 失败（V2 host 或网络不通）: %v", err)
	}
	skew := time.Since(serverTime).Abs()
	if skew > 5*time.Minute {
		log.Fatalf("❌ A1 本地时钟与服务器相差 %v，签名会因为 timestamp 超窗被拒", skew)
	}
	fmt.Printf("✅ A1 GetTime: %s（本地偏差 %v）\n", serverTime.UTC().Format(time.RFC3339), skew.Truncate(time.Millisecond))

	// A2：GET /balance-allowance —— 验 L2 HMAC 鉴权能跑通；USDC 余额未必准（pUSD 风险）
	balance, err := client.GetUSDCBalance()
	if err != nil {
		log.Fatalf("❌ A2 GetUSDCBalance 失败（L2 鉴权 / pUSD 合约问题）: %v", err)
	}
	fmt.Printf("✅ A2 GetUSDCBalance: %.6f（若 V2 已切 pUSD 这个数可能错，只要不报错就算过）\n", balance)

	if testToken == "" {
		fmt.Println("ℹ️  A3/A4 跳过（未设置 POLY_V2_TEST_TOKEN）")
		return
	}

	// A3：GET /book —— 读 V2 独立订单簿
	book, err := client.GetOrderBook(testToken)
	if err != nil {
		log.Fatalf("❌ A3 GetOrderBook 失败: %v", err)
	}
	fmt.Printf("✅ A3 GetOrderBook: %d bids / %d asks\n", len(book.Bids), len(book.Asks))

	// A4：GET /midpoint —— 后面 Step B 要用
	mid, err := client.GetMidpoint(testToken)
	if err != nil {
		log.Fatalf("❌ A4 GetMidpoint 失败: %v", err)
	}
	fmt.Printf("✅ A4 GetMidpoint: %v\n", mid)
}

// Step B：post 一笔远离中间价的 BUY，立刻 cancel。验证 V2 签名 + 请求体字段名。
// 价格离中间价 30 分，size=5，下完立刻 cancel，理论上不会成交。
func stepB(client clob.Client, testToken string) {
	fmt.Println("\n── Step B: POST /order + 立即 cancel ──")

	mid, err := client.GetMidpoint(testToken)
	if err != nil {
		log.Fatalf("❌ B0 GetMidpoint 失败: %v", err)
	}
	midPrice := mid.Value
	// 取 (midPrice - 0.30) 做 BUY，压到 0.01 以下就强制为 0.01（避免低于 tick）
	price := midPrice - 0.30
	if price < 0.01 {
		price = 0.01
	}
	fmt.Printf("   中间价 %.4f → 下单价 %.4f（远离避免成交）\n", midPrice, price)
	tickSize, err := client.GetTickSize(testToken)
	if err != nil {
		log.Fatalf("❌ B0 GetTickSize 失败: %v", err)
	}

	resp, err := client.PostOrder(types.OrderArgs{
		TokenID:  testToken,
		Price:    price,
		Size:     5.0,
		Side:     types.OrderSideBUY,
		TickSize: tickSize,
	}, types.OrderTypeGTC)
	if err != nil {
		log.Fatalf("❌ B1 PostOrder 调用失败: %v", err)
	}
	if resp == nil || resp.OrderID == "" {
		log.Fatalf("❌ B1 PostOrder 返回无 orderID：ErrorMsg=%q —— 很可能是签名或字段名错", errOrEmpty(resp))
	}
	fmt.Printf("✅ B1 PostOrder: orderID=%s\n", resp.OrderID)

	cancelResp, err := client.CancelOrder(resp.OrderID)
	if err != nil {
		log.Fatalf("❌ B2 CancelOrder 失败: %v（单已挂出，请手动撤）", err)
	}
	if len(cancelResp.Canceled) == 0 {
		log.Fatalf("❌ B2 CancelOrder 未成功撤单（单已挂出，请手动撤）: %+v", cancelResp)
	}
	fmt.Printf("✅ B2 CancelOrder: 已撤 %d 单\n", len(cancelResp.Canceled))

	fmt.Println("\n✅ POST + CANCEL round-trip 成功 —— V2 签名和请求体字段名都对得上")
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func truncate(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:6] + "…" + s[len(s)-4:]
}

func errOrEmpty(r *types.OrderPostResponse) string {
	if r == nil {
		return "<nil response>"
	}
	return r.ErrorMsg
}
