// v2_auto_claim：开关 Polymarket 自动 redeem 服务。
//
// 用法：
//
//	go run ./examples/onchain/v2-auto-claim status     查看当前授权状态
//	go run ./examples/onchain/v2-auto-claim enable     打开（CTF.setApprovalForAll(claimer, true)）
//	go run ./examples/onchain/v2-auto-claim disable    关闭（... false）
//
// 必选 env：poly_sec（或 POLY_PRIVATE_KEY）
// 可选 env：POLY_SIGNATURE_TYPE（默认 2=Safe）、POLY_BUILDER_API_KEY/SECRET/PASSPHRASE
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]

	privateKey := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if privateKey == "" {
		log.Fatalf("缺少 poly_sec 或 POLY_PRIVATE_KEY")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.SafeSignatureType)))

	var creds *types.ApiCreds
	if bk := os.Getenv("POLY_BUILDER_API_KEY"); bk != "" {
		creds = &types.ApiCreds{
			Key:        bk,
			Secret:     os.Getenv("POLY_BUILDER_SECRET"),
			Passphrase: os.Getenv("POLY_BUILDER_PASSPHRASE"),
		}
		if creds.Secret == "" || creds.Passphrase == "" {
			log.Fatalf("POLY_BUILDER_API_KEY 必须和 SECRET/PASSPHRASE 一起设置")
		}
	}

	gasless, err := web3.NewGaslessClient(privateKey, sigType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("Gasless client: %v", err)
	}

	proxy, err := gasless.GetPolyProxyAddress()
	if err != nil {
		log.Fatalf("GetPolyProxyAddress: %v", err)
	}
	fmt.Printf("Safe Proxy: %s\n", proxy)
	fmt.Printf("Claimer   : %s\n", internal.PolymarketAutoClaimer)

	switch cmd {
	case "status":
		enabled, err := gasless.IsAutoClaimEnabled()
		if err != nil {
			log.Fatalf("IsAutoClaimEnabled: %v", err)
		}
		if enabled {
			fmt.Println("当前状态: ✅ 已开启（Polymarket 会自动 redeem）")
		} else {
			fmt.Println("当前状态: ❌ 未开启（需手动 redeem）")
		}

	case "enable":
		r, err := gasless.EnableAutoClaim()
		reportIdempotent(r, err, "enable auto-claim", "已经是开启状态，跳过链上写")

	case "disable":
		r, err := gasless.DisableAutoClaim()
		reportIdempotent(r, err, "disable auto-claim", "已经是关闭状态，跳过链上写")

	default:
		usage()
		os.Exit(1)
	}
}

// reportIdempotent 处理幂等 wrapper 的三态返回：err / 已就位（receipt=nil） / 真发了 tx。
func reportIdempotent(r *types.TransactionReceipt, err error, label, noopMsg string) {
	if err != nil {
		log.Fatalf("❌ %s 失败: %v", label, err)
	}
	if r == nil {
		fmt.Printf("✅ %s：%s\n", label, noopMsg)
		return
	}
	fmt.Printf("✅ %s 成功\n  tx hash = %s\n  block   = %d\n  status  = %d\n",
		label, r.TxHash, r.BlockNumber, r.Status)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  go run ./examples/onchain/v2-auto-claim status")
	fmt.Fprintln(os.Stderr, "  go run ./examples/onchain/v2-auto-claim enable")
	fmt.Fprintln(os.Stderr, "  go run ./examples/onchain/v2-auto-claim disable")
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
