// v2_split_merge_redeem：V2 CtfCollateralAdapter 链上行为联调驱动。
//
// 子命令：
//
//	split   <conditionId> <amountPusd>        把 pUSD split 成 YES+NO 位置 token
//	merge   <conditionId> <amountPusd>        把 YES+NO 位置 token 合回 pUSD（随时可做）
//	redeem  <conditionId> <outcomeIndex> [size]  结算后收钱；outcomeIndex=0(YES)/1(NO)，NegRisk 必填 size
//
// 可选参数（全部子命令）：
//
//	--neg-risk    使用 NegRiskCtfCollateralAdapter（本仓 5 分钟 BTC 市场是 Regular，不用加）
//
// 必选 env：poly_sec（或 POLY_PRIVATE_KEY）
// 可选 env：POLY_SIGNATURE_TYPE（默认 2=Safe）、POLY_BUILDER_API_KEY/SECRET/PASSPHRASE
//
// 示例：
//
//	go run ./examples/onchain/v2-split-merge-redeem split  0x45fff1... 2
//	go run ./examples/onchain/v2-split-merge-redeem merge  0x45fff1... 1
//	go run ./examples/onchain/v2-split-merge-redeem redeem 0x45fff1... 1            # Regular: redeem 胜出方=NO
//	go run ./examples/onchain/v2-split-merge-redeem redeem 0x9e54... 1 50 --neg-risk # NegRisk: 胜出 NO，size=50
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
	if len(os.Args) < 3 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	cond := types.Keccak256(os.Args[2])
	if err := cond.Validate(); err != nil {
		log.Fatalf("非法 conditionId %q：%v", os.Args[2], err)
	}

	// 解析余下 args：amount / indexSets + 可选 --neg-risk
	rest := os.Args[3:]
	negRisk := false
	positional := make([]string, 0, len(rest))
	for _, a := range rest {
		if a == "--neg-risk" {
			negRisk = true
			continue
		}
		positional = append(positional, a)
	}

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

	marketKind := "Regular"
	if negRisk {
		marketKind = "NegRisk"
	}
	fmt.Printf("── V2 %s · %s · conditionId=%s ──\n", marketKind, cmd, cond)

	switch cmd {
	case "split":
		if len(positional) < 1 {
			log.Fatalf("split 需要 amountPusd（如 2 表示 2 pUSD）")
		}
		amount := mustFloat(positional[0], "amountPusd")
		fmt.Printf("正在 split %.6f pUSD…\n", amount)
		r, err := gasless.SplitPosition(amount, cond, negRisk)
		report(r, err, "split")

	case "merge":
		if len(positional) < 1 {
			log.Fatalf("merge 需要 amountPusd（全 set 规模）")
		}
		amount := mustFloat(positional[0], "amountPusd")
		fmt.Printf("正在 merge %.6f pUSD 规模 set…\n", amount)
		r, err := gasless.MergePositions(cond, amount, negRisk)
		report(r, err, "merge")

	case "redeem":
		// 用法：redeem <conditionId> <outcomeIndex> <size> [--neg-risk]
		//   outcomeIndex: 0=YES / 1=NO（胜出方）
		//   size: 押在 outcomeIndex 上的位置 token 数量（人类单位，6 decimals）
		if len(positional) < 2 {
			log.Fatalf("redeem 需要 outcomeIndex（0=YES / 1=NO）和 size")
		}
		outcomeIdx, err := strconv.Atoi(positional[0])
		if err != nil || (outcomeIdx != 0 && outcomeIdx != 1) {
			log.Fatalf("非法 outcomeIndex %q：必须是 0 或 1", positional[0])
		}
		size := mustFloat(positional[1], "size")
		fmt.Printf("正在 redeem outcomeIndex=%d size=%.6f…\n", outcomeIdx, size)
		r, err := gasless.RedeemPositions([]web3.RedeemPositionInfo{{
			ConditionID:  cond,
			OutcomeIndex: outcomeIdx,
			Size:         size,
			NegRisk:      negRisk,
		}})
		report(r, err, "redeem")

	default:
		usage()
		os.Exit(1)
	}
}

func report(r *types.TransactionReceipt, err error, label string) {
	if err != nil {
		log.Fatalf("❌ %s 失败: %v", label, err)
	}
	if r == nil {
		log.Fatalf("❌ %s 无回执", label)
	}
	fmt.Printf("✅ %s 成功\n  tx hash = %s\n  block   = %d\n  status  = %d\n",
		label, r.TxHash, r.BlockNumber, r.Status)
}

func mustFloat(s, name string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		log.Fatalf("%s 必须是正数，got %q", name, s)
	}
	return v
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  go run ./examples/onchain/v2-split-merge-redeem split  <conditionId> <amountPusd> [--neg-risk]")
	fmt.Fprintln(os.Stderr, "  go run ./examples/onchain/v2-split-merge-redeem merge  <conditionId> <amountPusd> [--neg-risk]")
	fmt.Fprintln(os.Stderr, "  go run ./examples/onchain/v2-split-merge-redeem redeem <conditionId> <outcomeIndex> [size] [--neg-risk]")
	fmt.Fprintln(os.Stderr, "    outcomeIndex: 0=YES / 1=NO（胜出方）；NegRisk 必填 size（押在该 outcome 的 token 数量）")
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
