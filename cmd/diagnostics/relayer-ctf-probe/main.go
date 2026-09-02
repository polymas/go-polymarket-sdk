// relayer_ctf_probe：用同一套 gasless 提交机制打一笔到 CTF（allowlisted `to`），
// 验证 relayer 是否放行 —— 用来区分 NegRisk redeem 被拒是 (i) `to` 硬策略 还是
// (ii) 提交格式缺陷。
//
// 发的是幂等的 CTF.setApprovalForAll(PolymarketAutoClaimer, true)（已是 true → 链上 no-op），
// 但是一次真实的 V2 relayer 提交。
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3/relayer"
)

func main() {
	pk := os.Getenv("PK")
	if pk == "" {
		pk = os.Getenv("POLY_PRIVATE_KEY")
	}
	if pk == "" {
		log.Fatal("PK / POLY_PRIVATE_KEY required")
	}
	c, err := relayer.NewGaslessClient(pk, types.SafeSignatureType, types.Polygon, nil)
	if err != nil {
		log.Fatalf("client: %v", err)
	}
	fmt.Println("提交 gasless CTF.setApprovalForAll(autoClaimer, true) —— to=CTF（allowlisted）…")
	r, err := c.SetAutoClaim(true)
	if err != nil {
		fmt.Printf("❌ 被 relayer 拒: %v\n", err)
		fmt.Println("→ CTF `to` 也走不通：偏向 (ii) 提交机制问题")
		return
	}
	if r == nil {
		fmt.Println("（短路了，没发链上——不该发生）")
		return
	}
	fmt.Printf("✅ relayer 放行并上链：tx=%s block=%d status=%d\n", r.TxHash, r.BlockNumber, r.Status)
	fmt.Println("→ 同一套提交机制打到 CTF 成功；NegRisk adapter 被秒拒 ⟹ 偏向 (i) `to` 硬策略")
}
