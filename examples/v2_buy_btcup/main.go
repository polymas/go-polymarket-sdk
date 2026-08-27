// v2_buy_btcup：在 V2 host 上挂一笔 BUY UP 订单，挂完不撤。
// 对应 Bitcoin Up or Down - April 22 2026 3AM ET 市场的 V2 联调。
//
// 必需环境变量：poly_sec（或兼容旧名 POLY_PRIVATE_KEY）。
// 可选环境变量：POLY_SIGNATURE_TYPE（默认 2=Safe）。
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

const (
	upTokenID = "115552491113503015807925003050246593110773789064738805746291252488410967159594"
	buyPrice  = 0.10
	buySize   = 5.0
)

func main() {
	privateKey := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if privateKey == "" {
		log.Fatalf("缺少必选环境变量 poly_sec（或 POLY_PRIVATE_KEY）")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.SafeSignatureType)))

	fmt.Println("=============================================")
	fmt.Println(" V2 BUY UP one-shot (place + leave)")
	fmt.Println(" Host: https://clob-v2.polymarket.com")
	fmt.Printf(" Market: BTC Up/Down 4/22 3AM ET  Side: BUY  Price: %.2f  Size: %.0f (notional ≈ %.2f USDC)\n",
		buyPrice, buySize, buyPrice*buySize)
	fmt.Printf(" SignatureType: %d\n", sigType)
	fmt.Println("=============================================")

	web3Client, err := web3.NewClient(privateKey, sigType, types.Polygon)
	if err != nil {
		log.Fatalf("❌ web3 客户端失败: %v", err)
	}
	defer web3Client.Close()
	fmt.Printf("EOA 地址: %s\n", web3Client.GetBaseAddress())

	client, err := clob.NewClient(web3Client)
	if err != nil {
		log.Fatalf("❌ V2 CLOB 客户端失败（API Key 派生失败）: %v", err)
	}

	serverTime, err := client.GetTime()
	if err != nil {
		log.Fatalf("❌ GetTime 失败: %v", err)
	}
	skew := time.Since(serverTime).Abs()
	fmt.Printf("服务器时间: %s（偏差 %v）\n\n", serverTime.UTC().Format(time.RFC3339), skew.Truncate(time.Millisecond))
	if skew > 5*time.Minute {
		log.Fatalf("❌ 时钟偏差 >5min，签名一定会被拒")
	}

	// 强制让 V2 CLOB 重读链上 allowance（approve 后服务端缓存可能还停在旧值）
	ba, err := client.UpdateBalanceAllowance(0)
	if err != nil {
		fmt.Printf("⚠️  UpdateBalanceAllowance 失败（非致命，继续尝试下单）: %v\n", err)
	} else {
		fmt.Printf("刷新 balance-allowance OK: %+v\n", ba)
	}

	resp, err := client.PostOrder(types.OrderArgs{
		TokenID: upTokenID,
		Price:   buyPrice,
		Size:    buySize,
		Side:    types.OrderSideBUY,
	}, types.OrderTypeGTC)
	if err != nil {
		log.Fatalf("❌ PostOrder 调用失败: %v", err)
	}
	if resp == nil {
		log.Fatalf("❌ PostOrder 返回 nil，可能是 V2 字段名/签名问题")
	}
	if resp.ErrorMsg != "" {
		log.Fatalf("❌ PostOrder 被服务端拒绝: %s（orderID=%q status=%q）", resp.ErrorMsg, resp.OrderID, resp.Status)
	}
	if resp.OrderID == "" {
		log.Fatalf("❌ PostOrder 无 orderID：%+v", resp)
	}

	fmt.Printf("✅ 下单成功\n  orderID = %s\n  status  = %s\n", resp.OrderID, resp.Status)
	fmt.Println("\n订单已挂在 V2 上（未撤）。手动撤：")
	fmt.Printf("  POLY_V2_CANCEL_ID=%s go run ./examples/v2_cancel_one  # 如未写，用官方面板撤\n", resp.OrderID)
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
