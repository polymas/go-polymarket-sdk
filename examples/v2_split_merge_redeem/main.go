// v2_split_merge_redeem：V2 CtfCollateralAdapter 链上行为联调驱动。
//
// 子命令：
//   split   <conditionId> <amountPusd>        把 pUSD split 成 YES+NO 位置 token
//   merge   <conditionId> <amountPusd>        把 YES+NO 位置 token 合回 pUSD（随时可做）
//   redeem  <conditionId> [indexSets]         结算后收钱；indexSets 默认 1,2，也可传 1 或 2
//
// 可选参数（全部子命令）：
//   --neg-risk    使用 NegRiskCtfCollateralAdapter（本仓 5 分钟 BTC 市场是 Regular，不用加）
//
// 必选 env：poly_sec（或 POLY_PRIVATE_KEY）
// 可选 env：POLY_SIGNATURE_TYPE（默认 2=Safe）、POLY_BUILDER_API_KEY/SECRET/PASSPHRASE
//
// 示例：
//   go run ./examples/v2_split_merge_redeem split  0x45fff1... 2
//   go run ./examples/v2_split_merge_redeem merge  0x45fff1... 1
//   go run ./examples/v2_split_merge_redeem redeem 0x45fff1... 1,2   # 合并 redeem
//   go run ./examples/v2_split_merge_redeem redeem 0x45fff1... 1     # 单独 redeem YES
package main

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"

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
		indexSets := []*big.Int{big.NewInt(1), big.NewInt(2)}
		if len(positional) >= 1 {
			parsed, err := parseIndexSets(positional[0])
			if err != nil {
				log.Fatalf("解析 indexSets 失败: %v", err)
			}
			indexSets = parsed
		}
		label := indexSetsLabel(indexSets)
		fmt.Printf("正在 redeem indexSets=%s…\n", label)
		r, err := gasless.RedeemPositions([]web3.RedeemPositionInfo{{
			ConditionID: cond,
			IndexSets:   indexSets,
			NegRisk:     negRisk,
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

func parseIndexSets(s string) ([]*big.Int, error) {
	parts := strings.Split(s, ",")
	out := make([]*big.Int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, ok := new(big.Int).SetString(p, 10)
		if !ok || v.Sign() <= 0 {
			return nil, fmt.Errorf("非法 indexSet %q", p)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("indexSets 为空")
	}
	return out, nil
}

func indexSetsLabel(s []*big.Int) string {
	parts := make([]string, len(s))
	for i, v := range s {
		parts[i] = v.String()
	}
	return "[" + strings.Join(parts, ",") + "]"
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
	fmt.Fprintln(os.Stderr, "  go run ./examples/v2_split_merge_redeem split  <conditionId> <amountPusd> [--neg-risk]")
	fmt.Fprintln(os.Stderr, "  go run ./examples/v2_split_merge_redeem merge  <conditionId> <amountPusd> [--neg-risk]")
	fmt.Fprintln(os.Stderr, "  go run ./examples/v2_split_merge_redeem redeem <conditionId> [indexSets]  [--neg-risk]")
	fmt.Fprintln(os.Stderr, "    indexSets 默认 1,2（合并 redeem）；传 1 或 2 做单独 redeem")
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
