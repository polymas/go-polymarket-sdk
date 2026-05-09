// relayer_tx_status：用 transactionID 查 relayer 那笔的最终状态，主要给 STATE_FAILED 诊断用。
//
// 用法：
//
//	go run ./examples/relayer_tx_status <transactionID>
//
// env：
//
//	poly_sec / POLY_PRIVATE_KEY              owner 私钥（HMAC 签名要用）
//	POLY_BUILDER_API_KEY/SECRET/PASSPHRASE   Builder Key 三件套
//	POLY_SIGNATURE_TYPE                      默认 2（Safe）
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("用法: relayer_tx_status <transactionID>")
	}
	txID := os.Args[1]

	pk := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if pk == "" {
		log.Fatalf("缺少 poly_sec 或 POLY_PRIVATE_KEY")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.SafeSignatureType)))

	creds := &types.ApiCreds{
		Key:        os.Getenv("POLY_BUILDER_API_KEY"),
		Secret:     os.Getenv("POLY_BUILDER_SECRET"),
		Passphrase: os.Getenv("POLY_BUILDER_PASSPHRASE"),
	}
	if creds.Key == "" || creds.Secret == "" || creds.Passphrase == "" {
		log.Fatalf("Builder API key 三件套不全：POLY_BUILDER_API_KEY/SECRET/PASSPHRASE")
	}

	gasless, err := web3.NewGaslessClient(pk, sigType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("Gasless client: %v", err)
	}

	infos, err := gasless.GetRelayerTransaction(txID)
	if err != nil {
		log.Fatalf("查询失败: %v", err)
	}
	if len(infos) == 0 {
		fmt.Printf("未找到 transactionID=%s 的记录\n", txID)
		return
	}

	out, _ := json.MarshalIndent(infos, "", "  ")
	fmt.Println(string(out))

	for _, info := range infos {
		fmt.Println("──────")
		fmt.Printf("state           = %s\n", info.State)
		fmt.Printf("transactionHash = %s\n", info.TransactionHash)
		fmt.Printf("to              = %s\n", info.To)
		fmt.Printf("metadata        = %q\n", info.Metadata)
		if info.TransactionHash != "" {
			fmt.Printf("📍 Polygonscan: https://polygonscan.com/tx/%s\n", info.TransactionHash)
		} else {
			fmt.Printf("⚠️  无 transactionHash —— relayer 预校验阶段拒绝（STATE_INVALID 或同等失败），未上链\n")
		}
	}
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
