// v2_list_orders：查 V2 CLOB 上当前账户的所有活跃订单。
// 先用 SDK 自带 GetOrders；再 raw 调 /data/orders 加 signature_type 参，便于对比。
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/clob"
	sdkhttp "github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	privateKey := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if privateKey == "" {
		log.Fatalf("缺少 poly_sec")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.SafeSignatureType)))

	w3, err := web3.NewClient(privateKey, sigType, types.Polygon)
	if err != nil {
		log.Fatalf("web3: %v", err)
	}
	defer w3.Close()

	client, err := clob.NewClient(w3, clob.WithV2())
	if err != nil {
		log.Fatalf("V2 CLOB client: %v", err)
	}
	creds := client.GetAPICreds()

	fmt.Println("── 走 SDK GetOrders（默认参数，无 signature_type）──")
	orders, err := client.GetOrders(nil, nil, nil)
	if err != nil {
		fmt.Printf("err: %v\n", err)
	} else {
		fmt.Printf("共 %d 个活跃订单\n", len(orders))
		for i, o := range orders {
			fmt.Printf("  [%d] id=%s\n      market=%s  asset=%s\n      side=%s price=%s size=%s matched=%s status=%s type=%s\n",
				i+1, o.OrderID, o.ConditionID, truncID(o.TokenID), o.Side, o.Price, o.OriginalSize, o.SizeMatched, o.Status, o.OrderType)
		}
	}

	fmt.Println("\n── raw GET /data/orders?signature_type=2 ──")
	reqArgs := &types.RequestArgs{Method: "GET", RequestPath: internal.Orders, Body: nil}
	headers, err := internal.CreateLevel2Headers(w3.GetSigner(), creds, reqArgs)
	if err != nil {
		log.Fatalf("headers: %v", err)
	}
	for _, params := range []map[string]string{
		{"signature_type": "2"},
		{"signature_type": "2", "next_cursor": "MA=="},
	} {
		raw, err := sdkhttp.GetRaw(internal.ClobAPIV2Domain, "GET", internal.Orders, params, sdkhttp.WithHeaders(headers))
		if err != nil {
			fmt.Printf("  %+v err: %v\n", params, err)
			continue
		}
		fmt.Printf("  %+v -> %s\n", params, string(raw))
	}
}

func truncID(s string) string {
	if len(s) <= 16 {
		return s
	}
	return s[:8] + "…" + s[len(s)-6:]
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
