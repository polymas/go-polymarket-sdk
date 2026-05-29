// legacy_proxy_repair：给指定的老 PolyProxy 邮箱钱包补 V2 缺失授权。
//
// 强制 funder 地址校验：私钥派生出的 PolyProxy 必须等于 -funder 才会执行，
// 防止私钥/sig type 配错把别的钱包改掉。
//
// env：
//   poly_sec / POLY_PRIVATE_KEY            owner EOA 私钥（必须）
//   POLY_BUILDER_API_KEY/SECRET/PASSPHRASE 可选；已弃用的旧 HMAC creds，不设则自动铸 V2 key
//
// flags：
//   -funder <0xaddr>   预期的 PolyProxy 地址，guard
//   -apply             实际提交 batch（缺省 dry-run，只跑 IsV2Ready）
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	expectedFunder := flag.String("funder", "0xcBdB56d5cf93eE8583D3473A8Ba6fF022c61E24d",
		"预期 PolyProxy 地址，派生不一致直接退出")
	apply := flag.Bool("apply", false, "实际提交 EnsureV2Ready batch（缺省仅 dry-run）")
	split := flag.Bool("split", false, "把 missing 拆成 N 笔单 op batch 各发一次（诊断用）")
	flag.Parse()

	pk := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if pk == "" {
		log.Fatal("缺少环境变量：poly_sec 或 POLY_PRIVATE_KEY")
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
			log.Fatal("设了 POLY_BUILDER_API_KEY 就必须一起设 SECRET 和 PASSPHRASE")
		}
	}

	cli, err := web3.NewClient(pk, types.ProxySignatureType, types.Polygon)
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	defer cli.Close()

	eoa := cli.GetBaseAddress()
	derived, err := cli.GetPolyProxyAddress()
	if err != nil {
		log.Fatalf("GetPolyProxyAddress: %v", err)
	}

	fmt.Printf("EOA       : %s\n", eoa)
	fmt.Printf("Derived   : %s\n", derived)
	fmt.Printf("Expected  : %s\n", *expectedFunder)
	if !strings.EqualFold(string(derived), *expectedFunder) {
		log.Fatalf("❌ 派生的 PolyProxy 地址与 -funder 不一致；私钥不是这个 PolyProxy 的 owner，已中止")
	}
	fmt.Println("✅ funder 校验通过")
	fmt.Println()

	gasless, err := web3.NewGaslessClient(pk, types.ProxySignatureType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("NewGaslessClient: %v", err)
	}

	ready, missing, err := gasless.IsV2Ready()
	if err != nil {
		log.Fatalf("IsV2Ready: %v", err)
	}
	if ready {
		fmt.Println("✅ V2 trading 全部就位，无需修复")
		return
	}
	fmt.Printf("⚠️  待修复 %d 项：\n", len(missing))
	for _, m := range missing {
		fmt.Printf("   - %s\n", m)
	}

	if !*apply {
		fmt.Println("\n(dry-run；加 -apply 实际提交)")
		return
	}

	if *split {
		fmt.Println("\n→ -split 模式：每项 missing 单独发一笔 batch …")
		receipts, errs, err := gasless.EnsureV2ReadyOneByOne()
		if err != nil {
			log.Fatalf("❌ EnsureV2ReadyOneByOne: %v", err)
		}
		for i, r := range receipts {
			if errs[i] != nil {
				fmt.Printf("  #%d ❌ %v\n", i+1, errs[i])
				continue
			}
			if r == nil {
				fmt.Printf("  #%d (no tx)\n", i+1)
				continue
			}
			fmt.Printf("  #%d ✅ tx=%s block=%d status=%d\n",
				i+1, r.TxHash, r.BlockNumber, r.Status)
		}
	} else {
		fmt.Println("\n→ 调用 EnsureV2Ready，gasless 一次性补齐…")
		r, err := gasless.EnsureV2Ready()
		if err != nil {
			log.Fatalf("❌ EnsureV2Ready: %v", err)
		}
		if r == nil {
			fmt.Println("已就位，没发 tx")
			return
		}
		fmt.Printf("✅ 提交成功\n  tx     = %s\n  block  = %d\n  status = %d\n",
			r.TxHash, r.BlockNumber, r.Status)
	}

	fmt.Println("\n复核 IsV2Ready…")
	ready2, missing2, err := gasless.IsV2Ready()
	if err != nil {
		log.Fatalf("复核 IsV2Ready: %v", err)
	}
	if ready2 {
		fmt.Println("✅ 全部修复完成")
		return
	}
	fmt.Printf("⚠️  仍有 %d 项未修复：\n", len(missing2))
	for _, m := range missing2 {
		fmt.Printf("   - %s\n", m)
	}
	os.Exit(2)
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
