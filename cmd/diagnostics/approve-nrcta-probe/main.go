// approve_nrcta_probe：调 SDK 的 ApproveAdaptersV2() 一次性补齐所有缺失的 adapter
// approve（含 NegRiskCtfCollateralAdapter 的 setApprovalForAll）。
//
// 历史 (v1.10.x) 这一笔会被 relayer 白名单封禁；V2 协议下白名单可能已放开 — 实测。
//
// 用法：
//
//	PK=0x... go run ./cmd/diagnostics/approve-nrcta-probe
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	pk := strings.TrimSpace(os.Getenv("PK"))
	if pk == "" {
		log.Fatal("env PK required")
	}
	c, err := web3.NewGaslessClient(pk, types.CWIASignatureType, types.Polygon, nil,
		"https://rpc-mainnet.matic.quiknode.pro")
	if err != nil {
		log.Fatalf("NewGaslessClient: %v", err)
	}

	wallet, _ := c.GetPolyProxyAddress()
	fmt.Printf("CWIA wallet: %s\n", wallet)
	fmt.Println("Calling ApproveAdaptersV2() — 一次提交 5 笔 approve：")
	fmt.Println("  pUSD.approve(CtfCollateralAdapter, MAX)")
	fmt.Println("  pUSD.approve(NegRiskCtfCollateralAdapter, MAX)")
	fmt.Println("  CTF.setApprovalForAll(CtfCollateralAdapter, true)")
	fmt.Println("  CTF.setApprovalForAll(NegRiskCtfCollateralAdapter, true)  ← 关键")
	fmt.Println("  CTF.setApprovalForAll(NegRiskAdapter, true)")
	fmt.Println()

	receipt, err := c.ApproveAdaptersV2()
	if err != nil {
		fmt.Printf("❌ FAILED: %v\n", err)
		fmt.Println()
		fmt.Println("如果错误里有 'call blocked' / 'not in the allowed list':")
		fmt.Println("  → relayer 在 V2 下仍封禁这两个 adapter，需走 EOA 自付 gas 路径")
		fmt.Println("如果是 500 internal error:")
		fmt.Println("  → relayer 后端 bug，需上报")
		fmt.Println("其它错误 → 看具体内容")
		return
	}
	fmt.Printf("✅ OK\n")
	fmt.Printf("   tx hash: %s\n", receipt.TxHash)
	fmt.Printf("   block:   %d\n", receipt.BlockNumber)
	fmt.Printf("   status:  %d\n", receipt.Status)
	fmt.Println()
	fmt.Println("Approval 补完，现在重跑 claim_test 应该 NegRisk redeem 就通了。")
}
