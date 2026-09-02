// claim_diag：用 transactionID 查 relayer 那笔的最终状态（自动铸 V2 key，不需 builder creds）。
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3/relayer"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("用法: claim_diag <transactionID>")
	}
	txID := os.Args[1]
	pk := os.Getenv("PK")
	if pk == "" {
		pk = os.Getenv("POLY_PRIVATE_KEY")
	}
	if pk == "" {
		log.Fatalf("缺少 PK / POLY_PRIVATE_KEY")
	}
	gasless, err := relayer.NewGaslessClient(pk, types.SafeSignatureType, types.Polygon, nil)
	if err != nil {
		log.Fatalf("Gasless client: %v", err)
	}
	infos, err := gasless.GetRelayerTransaction(txID)
	if err != nil {
		log.Fatalf("查询失败: %v", err)
	}
	out, _ := json.MarshalIndent(infos, "", "  ")
	fmt.Println(string(out))
	for _, info := range infos {
		fmt.Println("──────")
		fmt.Printf("state           = %s\n", info.State)
		fmt.Printf("transactionHash = %s\n", info.TransactionHash)
		fmt.Printf("to              = %s\n", info.To)
		if info.TransactionHash != "" {
			fmt.Printf("📍 https://polygonscan.com/tx/%s\n", info.TransactionHash)
		} else {
			fmt.Printf("⚠️  无 hash —— relayer 预校验阶段拒绝，未上链\n")
		}
	}
}
