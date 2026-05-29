// v2_wrap：把 USDC.e wrap 成 pUSD 并批准 V2 相关 spender，一笔 gasless batch。
//
// 必选 env：
//   poly_sec                    钱包私钥
//   POLY_V2_WRAP_AMOUNT         要 wrap 的 USDC.e 数量（人类单位，如 20.0）
// 可选 env：
//   POLY_SIGNATURE_TYPE         默认 2（Safe）
//   POLY_BUILDER_API_KEY/SECRET/PASSPHRASE  已弃用的旧 HMAC 三元组；不设则自动铸 V2 key
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	privateKey := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if privateKey == "" {
		log.Fatalf("缺少 poly_sec")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.SafeSignatureType)))

	amtStr := os.Getenv("POLY_V2_WRAP_AMOUNT")
	if amtStr == "" {
		log.Fatalf("缺少 POLY_V2_WRAP_AMOUNT（要 wrap 的 USDC.e 数量，如 20.0）")
	}
	amount, err := strconv.ParseFloat(amtStr, 64)
	if err != nil {
		log.Fatalf("POLY_V2_WRAP_AMOUNT 格式错: %v", err)
	}

	// POLY_BUILDER_* 三元组已弃用；不设则 GaslessClient 自动铸 V2 RELAYER_API_KEY（仅凭私钥）。
	var creds *types.ApiCreds
	if apiKey := os.Getenv("POLY_BUILDER_API_KEY"); apiKey != "" {
		creds = &types.ApiCreds{
			Key:        apiKey,
			Secret:     os.Getenv("POLY_BUILDER_SECRET"),
			Passphrase: os.Getenv("POLY_BUILDER_PASSPHRASE"),
		}
		if creds.Secret == "" || creds.Passphrase == "" {
			log.Fatalf("设了 POLY_BUILDER_API_KEY 就必须一起设 SECRET 和 PASSPHRASE")
		}
	}

	fmt.Println("=============================================")
	fmt.Println(" V2 Wrap & Approve via Gasless Relayer")
	fmt.Printf(" Amount: %.6f USDC.e → pUSD\n", amount)
	fmt.Printf(" SignatureType: %d\n", sigType)
	fmt.Println(" 5 txs in one relayer batch:")
	fmt.Println("   1) USDC.e.approve(CollateralOnramp, MAX)")
	fmt.Printf("   2) CollateralOnramp.wrap(USDC.e, safeProxy, %.0f)\n", amount*1e6)
	fmt.Println("   3) pUSD.approve(V2 Exchange, MAX)")
	fmt.Println("   4) pUSD.approve(V2 NegRisk Exchange, MAX)")
	fmt.Println("   5) pUSD.approve(NegRisk Adapter, MAX)")
	fmt.Println("=============================================")

	gasless, err := web3.NewGaslessClient(privateKey, sigType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("Gasless client: %v", err)
	}

	receipt, err := gasless.WrapAndApproveV2(amount)
	if err != nil {
		log.Fatalf("❌ WrapAndApproveV2 失败: %v", err)
	}
	if receipt == nil {
		log.Fatalf("❌ 无回执")
	}
	fmt.Printf("✅ 成功\n  tx hash = %s\n  block   = %d\n  status  = %d\n",
		receipt.TxHash, receipt.BlockNumber, receipt.Status)
	fmt.Println("\n下一步：go run ./examples/v2_balance_probe 看 V2 balance 是否变成 pUSD 余额；然后 go run ./examples/v2_buy_btcup 下单")
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
