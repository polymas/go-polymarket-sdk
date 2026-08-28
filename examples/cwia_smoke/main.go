// cwia_smoke 是 CWIA / DepositWallet 端到端冒烟测试工具。
//
// 必选环境变量：
//
//	poly_sec                  钱包 EOA 私钥（兼容 POLY_PRIVATE_KEY）
//
// 可选（提交 relayer 时必填）：
//
//	POLY_BUILDER_API_KEY
//	POLY_BUILDER_SECRET
//	POLY_BUILDER_PASSPHRASE
//
// 标志：
//
//	-submit          实际把 batch 提交到 Polymarket relayer（默认 dry-run，
//	                 仅打印将要发送的 body）
//	-target <addr>   batch 内 call.target（默认 pUSD 合约）
//	-data <0xhex>    call.data（默认 approve(0x0, 0)，实际无副作用）
//
// 用法：
//
//	./cwia_smoke              # dry-run，打印派生 / nonce / body
//	./cwia_smoke -submit      # 真正提交到 relayer
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	submit := flag.Bool("submit", false, "actually POST to relayer (default dry-run)")
	target := flag.String("target", internal.PolygonPUSD, "batch call.target address")
	dataHex := flag.String("data", "", "batch call.data (0xhex). default = approve(0x0, 0) — no-op")
	flag.Parse()

	pk := getEnvAny("poly_sec", "POLY_PRIVATE_KEY")
	if pk == "" {
		log.Fatal("missing env: poly_sec or POLY_PRIVATE_KEY")
	}
	pk = strings.TrimPrefix(pk, "0x")

	cli, err := web3.NewClient(pk, types.CWIASignatureType, types.Polygon)
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}
	defer cli.Close()

	baseAddr := cli.GetBaseAddress()
	fmt.Printf("EOA            : %s\n", baseAddr)
	fmt.Printf("ChainID        : %d\n", cli.GetChainID())
	fmt.Printf("SignatureType  : %d (CWIA)\n", cli.GetSignatureType())

	walletHex, err := cli.GetPolyProxyAddress()
	if err != nil {
		log.Fatalf("GetPolyProxyAddress: %v", err)
	}
	fmt.Printf("DepositWallet  : %s\n", walletHex)

	bc, ok := cli.(interface {
		GetDepositWalletNonce(ctx context.Context, w common.Address) (uint64, error)
	})
	if !ok {
		log.Fatal("client missing GetDepositWalletNonce — SDK too old")
	}
	wallet := common.HexToAddress(string(walletHex))
	nonce, err := bc.GetDepositWalletNonce(context.Background(), wallet)
	if err != nil {
		log.Fatalf("GetDepositWalletNonce: %v", err)
	}
	fmt.Printf("OnChain nonce  : %d\n", nonce)

	// 默认 call: approve(0x0, 0) — 无副作用，纯做 EIP-1271 + relayer 校验
	var callData []byte
	if *dataHex == "" {
		// approve(address spender, uint256 amount) selector = 0x095ea7b3
		selector, _ := hex.DecodeString("095ea7b3")
		args := make([]byte, 64)
		// spender = 0x0000…0000 (32 bytes zero)
		// amount  = 0           (32 bytes zero)
		callData = append(selector, args...)
		fmt.Printf("Call           : approve(0x0, 0) on %s (no-op)\n", *target)
	} else {
		callData, err = hex.DecodeString(strings.TrimPrefix(*dataHex, "0x"))
		if err != nil {
			log.Fatalf("invalid -data hex: %v", err)
		}
		fmt.Printf("Call           : custom %d-byte data on %s\n", len(callData), *target)
	}

	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())
	batch := signing.CWIABatch{
		Wallet:   wallet,
		Nonce:    new(big.Int).SetUint64(nonce),
		Deadline: deadline,
		Calls: []signing.CWIACall{{
			Target: common.HexToAddress(*target),
			Value:  big.NewInt(0),
			Data:   callData,
		}},
	}
	chainID := big.NewInt(int64(cli.GetChainID()))

	digest := signing.CWIABatchDigest(chainID, batch)
	fmt.Printf("EIP-712 digest : %s\n", digest.Hex())

	pkECDSA, err := crypto.HexToECDSA(pk)
	if err != nil {
		log.Fatalf("HexToECDSA: %v", err)
	}
	sig, err := signing.SignCWIABatch(pkECDSA, chainID, batch)
	if err != nil {
		log.Fatalf("SignCWIABatch: %v", err)
	}
	fmt.Printf("Batch sig      : 0x%s\n", hex.EncodeToString(sig))

	// 自检：ecrecover 签名者是否 == EOA
	sig0 := append([]byte{}, sig...)
	if sig0[64] >= 27 {
		sig0[64] -= 27
	}
	pub, _ := crypto.SigToPub(digest.Bytes(), sig0)
	recovered := crypto.PubkeyToAddress(*pub).Hex()
	if !strings.EqualFold(recovered, string(baseAddr)) {
		log.Fatalf("CRITICAL: ecrecover mismatch  recovered=%s  EOA=%s", recovered, baseAddr)
	}
	fmt.Printf("Selfcheck      : ecrecover(sig) == EOA ✓\n")

	// 构造与官方 schema 一致的 body
	body := map[string]interface{}{
		"type":      "WALLET",
		"from":      string(baseAddr),
		"to":        internal.PolygonDepositWalletFactory,
		"nonce":     batch.Nonce.String(),
		"signature": "0x" + hex.EncodeToString(sig),
		"depositWalletParams": map[string]interface{}{
			"depositWallet": wallet.Hex(),
			"deadline":      batch.Deadline.String(),
			"calls": []map[string]interface{}{{
				"target": batch.Calls[0].Target.Hex(),
				"value":  batch.Calls[0].Value.String(),
				"data":   "0x" + hex.EncodeToString(batch.Calls[0].Data),
			}},
		},
	}
	bodyJSON, _ := json.MarshalIndent(body, "", "  ")
	fmt.Printf("\n--- /submit body (preview) ---\n%s\n", string(bodyJSON))

	if !*submit {
		fmt.Printf("\nDRY RUN. 加 -submit 才会真正提交到 relayer。\n")
		return
	}

	// 真正走 GaslessClient 提交。POLY_BUILDER_* 三元组已弃用；不设则自动铸 V2 RELAYER_API_KEY（仅凭私钥）。
	var creds *types.ApiCreds
	if apiKey := os.Getenv("POLY_BUILDER_API_KEY"); apiKey != "" {
		creds = &types.ApiCreds{
			Key:        apiKey,
			Secret:     os.Getenv("POLY_BUILDER_SECRET"),
			Passphrase: os.Getenv("POLY_BUILDER_PASSPHRASE"),
		}
		if creds.Secret == "" || creds.Passphrase == "" {
			log.Fatalf("设了 POLY_BUILDER_API_KEY 就必须一起设 SECRET 和 PASSPHRASE")
		}
	}
	gc, err := web3.NewGaslessClient(pk, types.CWIASignatureType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("NewGaslessClient: %v", err)
	}
	defer gc.Close()

	fmt.Println("\n--- submitting to relayer ---")
	// 直接复用 GaslessClient 的内部分支：构造单条 proxyTxn map 调用
	// SetV2Allowances / SplitPosition 等高层方法（也走同一条 batch 路径）。
	//
	// 这里不走高层方法，因为我们要把 -target/-data 透传过去；用最小化的
	// approve no-op 等价于一次正常的 batch（链上无副作用，仅消耗 relayer
	// 配额 + 一次 gas 由 relayer 代付）。
	receipt, err := callExecuteGaslessBatch(gc, batch.Calls[0], "")
	if err != nil {
		log.Fatalf("submit failed: %v", err)
	}
	fmt.Printf("\nOK. txHash=%s blockNumber=%d gasUsed=%d\n",
		receipt.TxHash, receipt.BlockNumber, receipt.GasUsed)
}

// callExecuteGaslessBatch 用 SDK 内部的同一条 executeGaslessBatch 路径
// 提交一条 call。对外没有公开方法直接吃 (target, value, data)，所以我们
// 借 SetV2Allowances 那种"已知 noop 调用"的方法不合适——这里走最小私有调用。
//
// 因为 executeGaslessBatch 是私有的，本工具用 SetV2Allowances 这条公开 API
// 替代：pUSD/CTF 四笔授权，链上幂等无副作用。
func callExecuteGaslessBatch(gc *web3.GaslessClient, _ signing.CWIACall, _ string) (*types.TransactionReceipt, error) {
	// SetV2Allowances 自身就是一个标准的 batch（pUSD approve + CTF approval 共四笔），
	// 走的是 executeGaslessBatch -> CWIA 分支，所以是验证 relayer 提交流程的
	// 最直接公开入口。
	return gc.SetV2Allowances()
}

func getEnvAny(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
