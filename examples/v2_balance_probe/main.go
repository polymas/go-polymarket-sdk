// v2_balance_probe：查 V2 CLOB 服务端对当前钱包 balance/allowance 的认知，帮助定位 "not enough balance / allowance" 根因。
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

	fmt.Println("── V2 CLOB ──")
	v2, err := clob.NewClient(w3)
	if err != nil {
		log.Fatalf("V2 client: %v", err)
	}
	baV2, err := v2.GetBalanceAllowance()
	if err != nil {
		fmt.Printf("GetBalanceAllowance err: %v\n", err)
	} else {
		fmt.Printf("%+v\n", baV2)
	}
	usdcV2, _ := v2.GetUSDCBalance()
	fmt.Printf("GetUSDCBalance: %.6f\n", usdcV2)

	// Raw V2 response dump for debugging
	fmt.Println("\n── V2 CLOB raw /balance-allowance response ──")
	creds := v2.GetAPICreds()
	reqArgs := &types.RequestArgs{Method: "GET", RequestPath: internal.GetBalanceAllowance, Body: nil}
	headers, err := internal.CreateLevel2Headers(w3.GetSigner(), creds, reqArgs)
	if err != nil {
		fmt.Printf("headers err: %v\n", err)
		return
	}
	proxy, _ := w3.GetPolyProxyAddress()
	for _, params := range []map[string]string{
		{"asset_type": "COLLATERAL"},
		{"asset_type": "COLLATERAL", "signature_type": "2"},
		{"asset_type": "COLLATERAL", "address": string(proxy)},
		{"asset_type": "COLLATERAL", "signature_type": "2", "address": string(proxy)},
	} {
		label := fmt.Sprintf("%+v", params)
		raw, err := sdkhttp.GetRaw(internal.ClobAPIV2Domain, "GET", internal.GetBalanceAllowance,
			params, sdkhttp.WithHeaders(headers))
		if err != nil {
			fmt.Printf("  %s err: %v\n", label, err)
			continue
		}
		fmt.Printf("  %s -> %s\n", label, string(raw))
	}

	// GET /balance-allowance/update (V2 强制刷新索引)
	fmt.Println("\n── GET /balance-allowance/update ──")
	updArgs := &types.RequestArgs{Method: "GET", RequestPath: internal.UpdateBalanceAllowance, Body: nil}
	updHeaders, err := internal.CreateLevel2Headers(w3.GetSigner(), creds, updArgs)
	if err != nil {
		fmt.Printf("headers err: %v\n", err)
		return
	}
	rawUpd, err := sdkhttp.GetRaw(internal.ClobAPIV2Domain, "GET", internal.UpdateBalanceAllowance,
		map[string]string{"asset_type": "COLLATERAL"}, sdkhttp.WithHeaders(updHeaders))
	if err != nil {
		fmt.Printf("  err: %v\n", err)
	} else {
		fmt.Printf("  response: %q\n", string(rawUpd))
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
