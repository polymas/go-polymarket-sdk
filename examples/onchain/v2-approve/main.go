// v2_approve：对 V2 Exchange 和 V2 NegRisk Exchange 做 pUSD approve + CTF setApprovalForAll。
// 通过 Polymarket Relayer 的 gasless 路径一次性打包 4 笔。
//
// 必选 env：poly_sec（或 POLY_PRIVATE_KEY）
// 可选 env：POLY_SIGNATURE_TYPE（默认 2=Safe）
// 可选 env：POLY_BUILDER_API_KEY / POLY_BUILDER_SECRET / POLY_BUILDER_PASSPHRASE
//
//	三个都设 → Relayer 走 POLY_BUILDER_* HMAC 鉴权（前端 "Relayer API Keys" 生成的 key）
//	缺一个 → Relayer 走 L1 EOA-signed 鉴权（POLY_ADDRESS + POLY_SIGNATURE）
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
		log.Fatalf("缺少 poly_sec 或 POLY_PRIVATE_KEY")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.SafeSignatureType)))

	w3, err := web3.NewClient(privateKey, sigType, types.Polygon)
	if err != nil {
		log.Fatalf("web3 client: %v", err)
	}
	defer w3.Close()

	fmt.Println("=============================================")
	fmt.Println(" V2 Allowance Setup via Gasless Relayer")
	fmt.Printf(" EOA           : %s\n", w3.GetBaseAddress())
	proxy, err := w3.GetPolyProxyAddress()
	if err != nil {
		log.Fatalf("GetPolyProxyAddress: %v", err)
	}
	fmt.Printf(" Safe Proxy    : %s\n", proxy)
	fmt.Printf(" SignatureType : %d\n", sigType)
	fmt.Println(" 4 txs in one relayer batch:")
	fmt.Println("   1) pUSD.approve(V2 Exchange,         MAX)")
	fmt.Println("   2) pUSD.approve(V2 NegRisk Exchange, MAX)")
	fmt.Println("   3) CTF.setApprovalForAll(V2 Exchange,         true)")
	fmt.Println("   4) CTF.setApprovalForAll(V2 NegRisk Exchange, true)")
	fmt.Println("=============================================")

	var creds *types.ApiCreds
	if bk := os.Getenv("POLY_BUILDER_API_KEY"); bk != "" {
		creds = &types.ApiCreds{
			Key:        bk,
			Secret:     os.Getenv("POLY_BUILDER_SECRET"),
			Passphrase: os.Getenv("POLY_BUILDER_PASSPHRASE"),
		}
		if creds.Secret == "" || creds.Passphrase == "" {
			log.Fatalf("设了 POLY_BUILDER_API_KEY 就必须一起设 POLY_BUILDER_SECRET 和 POLY_BUILDER_PASSPHRASE")
		}
		fmt.Printf("使用 Relayer API Key (POLY_BUILDER_*): %s…\n\n", truncKey(creds.Key))
	} else {
		// 无 POLY_BUILDER_* → GaslessClient 以 nil creds 运行，
		// LocalSigner 会 fallback 到 L1 EOA-signed 鉴权（CreateLevel1Headers）。
		_ = clob.NewClient // 保留 clob 包导入用于其他例子
		fmt.Println("未设 POLY_BUILDER_*，尝试 L1 EOA-signed 鉴权走 Relayer")
		fmt.Println()
	}

	gasless, err := web3.NewGaslessClient(privateKey, sigType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("Gasless client: %v", err)
	}

	fmt.Println("正在提交 4 笔 approve 到 Relayer（gasless，不花 gas）…")
	receipt, err := gasless.SetV2Allowances()
	if err != nil {
		log.Fatalf("❌ SetV2Allowances 失败: %v", err)
	}
	if receipt == nil {
		log.Fatalf("❌ 无交易回执")
	}
	fmt.Printf("✅ 成功\n  tx hash = %s\n  block   = %d\n  status  = %d (1=成功)\n",
		receipt.TxHash, receipt.BlockNumber, receipt.Status)
	fmt.Println("\n下一步：再跑 `go run ./cmd/diagnostics/v2-allowance-check` 确认四个 ❌ 变 ✅")
}

func truncKey(s string) string {
	if len(s) <= 10 {
		return s
	}
	return s[:8]
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
