// v2_place_cancel_smoke：V2 挂单+撤单一条龙冒烟测试。
// 默认目标：BTC Up/Down - April 29 1PM ET 的 UP token，price=0.01, size=5（成本约 $0.05）。
//
// 流程：PostOrder → GetOrders 验证存在 → CancelOrder → GetOrders 验证消失。
//
// 必选 env：poly_sec
// 可选 env：POLY_SIGNATURE_TYPE（默认 2=Safe）、POLY_TEST_TOKEN_ID、POLY_TEST_PRICE、POLY_TEST_SIZE
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
	defaultTokenID = "46748130113843409601714936950841290785400963426757197063091026836622028104178"
	defaultPrice   = 0.01
	defaultSize    = 5.0
)

func main() {
	priv := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if priv == "" {
		log.Fatalf("缺少 poly_sec / POLY_PRIVATE_KEY")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.SafeSignatureType)))
	tokenID := envOr("POLY_TEST_TOKEN_ID", defaultTokenID)
	price := envFloat("POLY_TEST_PRICE", defaultPrice)
	size := envFloat("POLY_TEST_SIZE", defaultSize)

	fmt.Println("============================================")
	fmt.Println(" V2 place+cancel smoke")
	fmt.Printf(" token=%s\n price=%.4f size=%.2f notional=%.4f\n", tokenID[:18]+"...", price, size, price*size)
	fmt.Println("============================================")

	w3, err := web3.NewClient(priv, sigType, types.Polygon)
	if err != nil {
		log.Fatalf("web3 client: %v", err)
	}
	defer w3.Close()
	fmt.Printf("EOA: %s\n", w3.GetBaseAddress())

	client, err := clob.NewClient(w3)
	if err != nil {
		log.Fatalf("clob V2 client: %v", err)
	}

	t, err := client.GetTime()
	if err != nil {
		log.Fatalf("GetTime: %v", err)
	}
	fmt.Printf("server time: %s (skew %v)\n", t.UTC().Format(time.RFC3339), time.Since(t).Truncate(time.Second))

	// === PostOrder ===
	fmt.Println("\n[1/4] PostOrder…")
	resp, err := client.PostOrder(types.OrderArgs{
		TokenID: tokenID,
		Price:   price,
		Size:    size,
		Side:    types.OrderSideBUY,
	}, types.OrderTypeGTC)
	if err != nil {
		log.Fatalf("❌ PostOrder err: %v", err)
	}
	if resp == nil || resp.OrderID == "" {
		log.Fatalf("❌ PostOrder no orderID: %+v", resp)
	}
	if resp.ErrorMsg != "" {
		log.Fatalf("❌ PostOrder rejected: %s (status=%s)", resp.ErrorMsg, resp.Status)
	}
	orderID := types.Keccak256(resp.OrderID)
	fmt.Printf("✅ orderID=%s status=%s\n", resp.OrderID, resp.Status)

	// === GetOrders to confirm ===
	fmt.Println("\n[2/4] GetOrders 验证…")
	time.Sleep(1500 * time.Millisecond)
	open, err := client.GetOrders(&orderID, nil, nil)
	if err != nil {
		log.Fatalf("❌ GetOrders err: %v", err)
	}
	if len(open) == 0 {
		fmt.Println("⚠️  GetOrders 返回空（订单可能瞬秒成交或同步延迟）")
	} else {
		fmt.Printf("✅ 订单在簿上: id=%s side=%s price=%v size=%v matched=%v\n",
			open[0].OrderID, open[0].Side, open[0].Price, open[0].OriginalSize, open[0].SizeMatched)
	}

	// === CancelOrder ===
	fmt.Println("\n[3/4] CancelOrder…")
	cancelResp, err := client.CancelOrder(orderID)
	if err != nil {
		log.Fatalf("❌ CancelOrder err: %v", err)
	}
	if len(cancelResp.NotCanceled) > 0 {
		fmt.Printf("⚠️  NotCanceled: %v\n", cancelResp.NotCanceled)
	}
	if len(cancelResp.Canceled) > 0 {
		fmt.Printf("✅ Canceled: %v\n", cancelResp.Canceled)
	}

	// === GetOrders again to confirm gone ===
	fmt.Println("\n[4/4] GetOrders 复查…")
	time.Sleep(1500 * time.Millisecond)
	open2, err := client.GetOrders(&orderID, nil, nil)
	if err != nil {
		log.Fatalf("❌ GetOrders2 err: %v", err)
	}
	if len(open2) == 0 {
		fmt.Println("✅ 订单已撤离簿上")
	} else {
		fmt.Printf("ℹ️  按 ID 查得终态: id=%s status=%s matched=%v\n", open2[0].OrderID, open2[0].Status, open2[0].SizeMatched)
	}

	fmt.Println("\n=== Done ===")
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func envFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
func getEnvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
