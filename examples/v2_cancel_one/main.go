// v2_cancel_one：按 orderID 撤 V2 CLOB 上的一张订单。
// 必需 env: poly_sec, POLY_V2_CANCEL_ID (orderID)
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/clob"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	privateKey := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if privateKey == "" {
		log.Fatalf("缺少 poly_sec")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.SafeSignatureType)))

	orderID := os.Getenv("POLY_V2_CANCEL_ID")
	if orderID == "" {
		log.Fatalf("缺少 POLY_V2_CANCEL_ID（要撤的订单 ID）")
	}

	fmt.Printf("撤 V2 订单: %s\n", orderID)

	w3, err := web3.NewClient(privateKey, sigType, types.Polygon)
	if err != nil {
		log.Fatalf("web3: %v", err)
	}
	defer w3.Close()

	client, err := clob.NewClient(w3, clob.WithV2())
	if err != nil {
		log.Fatalf("V2 CLOB client: %v", err)
	}

	resp, err := client.CancelOrder(types.Keccak256(orderID))
	if err != nil {
		log.Fatalf("❌ CancelOrder 失败: %v", err)
	}
	fmt.Printf("✅ 响应: %+v\n", resp)
	if len(resp.Canceled) > 0 {
		fmt.Printf("已撤订单数: %d\n", len(resp.Canceled))
	}
	if len(resp.NotCanceled) > 0 {
		fmt.Printf("未撤订单: %+v\n", resp.NotCanceled)
	}
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
