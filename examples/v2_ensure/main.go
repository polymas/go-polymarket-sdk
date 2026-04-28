// v2_ensure：业务层进场前的"一键自检+自动修复"。
//
// 子命令：
//   status   只读检查 V2 trading 全套前置条件（不发 tx）
//   ensure   缺啥补啥，一笔 gasless batch；全就位则 (nil, nil) 不发 tx
//
// 必选 env：poly_sec（或 POLY_PRIVATE_KEY）
// 可选 env：POLY_SIGNATURE_TYPE（默认 2=Safe）、POLY_BUILDER_API_KEY/SECRET/PASSPHRASE
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
	fmt.Printf("Safe Proxy: %s\n\n", proxy)

	switch cmd {
	case "status":
		ready, missing, err := gasless.IsV2Ready()
		if err != nil {
			log.Fatalf("IsV2Ready: %v", err)
		}
		if ready {
			fmt.Println("✅ V2 trading 全部就位（EnsureV2Ready 当前是 no-op）")
		} else {
			fmt.Printf("⚠️  待修复 %d 项：\n", len(missing))
			for _, m := range missing {
				fmt.Printf("   - %s\n", m)
			}
		}

	case "ensure":
		fmt.Println("自检 V2 trading 前置条件，缺啥补啥…")
		r, err := gasless.EnsureV2Ready()
		if err != nil {
			log.Fatalf("❌ EnsureV2Ready 失败: %v", err)
		}
		if r == nil {
			fmt.Println("✅ 已全部就位，没发 tx")
			return
		}
		fmt.Printf("✅ 已修复\n  tx hash = %s\n  block   = %d\n  status  = %d\n",
			r.TxHash, r.BlockNumber, r.Status)

	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  go run ./examples/v2_ensure status   # 只读检查")
	fmt.Fprintln(os.Stderr, "  go run ./examples/v2_ensure ensure   # 缺啥补啥")
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
