// redeem_eoa_direct：EOA 自付 gas 直接发 Safe.execTransaction(NegRiskCtfCollateralAdapter, redeem)。
//
// ⚠️ 2026-05-29 起这条已**非必需**：当初以为 relayer 封 NegRisk redeem，后来查清是 SDK adapter
// 地址过时，迁到新地址后 gasless RedeemPositions 已能正常走（实测 n=3 通过）。本例留作
// **无 relayer / relayer 异常时的兜底**（EOA 自付、不经中继）。优先用 web3.GaslessClient.RedeemPositions。
//
// 用法：
//
//	PK=0x... COND=0x... IDX=1 [RPC=https://...] go run ./examples/redeem_eoa_direct
//
// 走 V2 NegRiskCtfCollateralAdapter（新地址，长签名，collateralToken=pUSD）→ Safe 收 pUSD。
// 注意：新 adapter 需 Safe 已 CTF.setApprovalForAll(新 adapter,true)（本例不含授权，发送前模拟会拦住未授权）。
package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

const (
	defaultRPC = "https://polygon-bor-rpc.publicnode.com"
	pusd       = "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB"
	nrCta      = "0xadA2005600Dec949baf300f4C6120000bDB6eAab" // V2 NegRiskCtfCollateralAdapter（2026-05 迁移后新地址）
	chainID    = 137
)

func main() {
	pk := strings.TrimSpace(firstEnv("PK", "POLY_PRIVATE_KEY"))
	if pk == "" {
		log.Fatal("env PK / POLY_PRIVATE_KEY required")
	}
	condStr := strings.TrimSpace(os.Getenv("COND"))
	if condStr == "" {
		log.Fatal("env COND required (0x… conditionId)")
	}
	cond := common.HexToHash(condStr)
	idx := 1
	if v := os.Getenv("IDX"); v != "" {
		idx, _ = strconv.Atoi(v)
	}
	rpcURL := strings.TrimSpace(os.Getenv("RPC"))
	if rpcURL == "" {
		rpcURL = defaultRPC
	}

	priv, err := crypto.HexToECDSA(strings.TrimPrefix(pk, "0x"))
	if err != nil {
		log.Fatalf("bad PK: %v", err)
	}
	eoa := crypto.PubkeyToAddress(priv.PublicKey)

	// SDK 派生 Safe 地址
	c, err := web3.NewGaslessClient(pk, types.SafeSignatureType, types.Polygon, nil, rpcURL)
	if err != nil {
		log.Fatalf("NewGaslessClient: %v", err)
	}
	safeHex, err := c.GetPolyProxyAddress()
	if err != nil {
		log.Fatalf("GetPolyProxyAddress: %v", err)
	}
	safe := common.HexToAddress(string(safeHex))
	fmt.Printf("EOA:  %s\n", eoa.Hex())
	fmt.Printf("Safe: %s\n", safe.Hex())
	fmt.Printf("cond: %s  idx=%d  → NegRiskCtfCollateralAdapter %s\n", cond.Hex(), idx, nrCta)

	ec, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	ctx := context.Background()

	target := common.HexToAddress(nrCta)
	innerData := encodeRedeemLong(common.HexToAddress(pusd), cond, idx)

	// 链上读 Safe nonce()  selector 0xaffed0e0
	nonceRaw, err := ec.CallContract(ctx, ethereum.CallMsg{To: &safe, Data: common.FromHex("0xaffed0e0")}, nil)
	if err != nil {
		log.Fatalf("read safe nonce: %v", err)
	}
	nonce := new(big.Int).SetBytes(nonceRaw)
	fmt.Printf("safe nonce: %s\n", nonce)

	// Safe.getTransactionHash(...)  — value=0, op=0, gas 全 0, gasToken/refund=0
	gthData := encodeGetTxHash(target, innerData, nonce)
	hashRaw, err := ec.CallContract(ctx, ethereum.CallMsg{To: &safe, Data: gthData}, nil)
	if err != nil {
		log.Fatalf("getTransactionHash: %v", err)
	}
	var safeTxHash [32]byte
	copy(safeTxHash[:], hashRaw)
	fmt.Printf("safeTxHash: 0x%x\n", safeTxHash)

	// eth_sign 风格：keccak("\x19Ethereum Signed Message:\n32" + hash)，再 v += 4（31/32）
	prefixed := crypto.Keccak256(append([]byte("\x19Ethereum Signed Message:\n32"), safeTxHash[:]...))
	sig, err := crypto.Sign(prefixed, priv)
	if err != nil {
		log.Fatalf("sign: %v", err)
	}
	sig[64] += 31 // recid 0/1 → 31/32（Safe eth_sign）
	fmt.Printf("sig: 0x%x\n", sig)

	execData := encodeExecTxn(target, innerData, sig)

	// 先模拟，避免白扔 gas
	if _, err := ec.CallContract(ctx, ethereum.CallMsg{From: eoa, To: &safe, Data: execData}, nil); err != nil {
		log.Fatalf("❌ execTransaction 模拟失败（不发链上）: %v", err)
	}
	fmt.Println("✅ execTransaction 模拟通过，发送中…")

	// EIP-1559 发送
	eoaNonce, err := ec.PendingNonceAt(ctx, eoa)
	if err != nil {
		log.Fatalf("PendingNonceAt: %v", err)
	}
	header, _ := ec.HeaderByNumber(ctx, nil)
	baseFee := big.NewInt(50_000_000_000)
	if header != nil && header.BaseFee != nil {
		baseFee = header.BaseFee
	}
	tip := big.NewInt(40_000_000_000)
	maxFee := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), tip)
	estGas, err := ec.EstimateGas(ctx, ethereum.CallMsg{From: eoa, To: &safe, Data: execData})
	if err != nil {
		fmt.Printf("⚠️  EstimateGas failed, fallback 600k: %v\n", err)
		estGas = 600_000
	} else {
		estGas = estGas * 130 / 100
	}
	tx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID: big.NewInt(chainID), Nonce: eoaNonce, GasTipCap: tip, GasFeeCap: maxFee,
		Gas: estGas, To: &safe, Value: big.NewInt(0), Data: execData,
	})
	signedTx, err := ethtypes.SignTx(tx, ethtypes.LatestSignerForChainID(big.NewInt(chainID)), priv)
	if err != nil {
		log.Fatalf("SignTx: %v", err)
	}
	if err := ec.SendTransaction(ctx, signedTx); err != nil {
		log.Fatalf("SendTransaction: %v", err)
	}
	h := signedTx.Hash().Hex()
	fmt.Printf("\n📤 tx: https://polygonscan.com/tx/%s\n\n等待 receipt…\n", h)

	dctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for {
		select {
		case <-dctx.Done():
			log.Fatalf("timeout waiting receipt")
		default:
		}
		r, err := ec.TransactionReceipt(ctx, signedTx.Hash())
		if err == nil {
			if r.Status == 1 {
				fmt.Printf("✅ SUCCESS  block=%d gasUsed=%d\n", r.BlockNumber, r.GasUsed)
			} else {
				fmt.Printf("❌ FAILED (reverted)  block=%d gasUsed=%d\n", r.BlockNumber, r.GasUsed)
			}
			return
		}
		time.Sleep(3 * time.Second)
	}
}

// encodeRedeemLong: redeemPositions(address,bytes32,bytes32,uint256[]) selector 0x01b7037c
func encodeRedeemLong(collateral common.Address, cond common.Hash, idx int) []byte {
	sel := crypto.Keccak256([]byte("redeemPositions(address,bytes32,bytes32,uint256[])"))[:4]
	out := append([]byte{}, sel...)
	out = append(out, leftPad(collateral.Bytes())...) // collateralToken
	out = append(out, make([]byte, 32)...)             // parentCollectionId = 0
	out = append(out, cond.Bytes()...)                 // conditionId
	out = append(out, word(0x80)...)                   // offset to indexSets
	out = append(out, word(1)...)                      // len = 1
	out = append(out, word(int64(1<<uint(idx)))...)    // indexSet = 1<<idx
	return out
}

// encodeGetTxHash: getTransactionHash(to,value,data,op,safeTxGas,baseGas,gasPrice,gasToken,refund,nonce)
func encodeGetTxHash(to common.Address, data []byte, nonce *big.Int) []byte {
	sel := crypto.Keccak256([]byte("getTransactionHash(address,uint256,bytes,uint8,uint256,uint256,uint256,address,address,uint256)"))[:4]
	return packSafeCall(sel, to, data, nonce)
}

// encodeExecTxn: execTransaction(to,value,data,op,safeTxGas,baseGas,gasPrice,gasToken,refund,signatures)
func encodeExecTxn(to common.Address, data, sig []byte) []byte {
	sel := crypto.Keccak256([]byte("execTransaction(address,uint256,bytes,uint8,uint256,uint256,uint256,address,address,bytes)"))[:4]
	return packSafeCall(sel, to, data, nil, sig)
}

// packSafeCall 编码 10 参数 Safe 调用。最后一个动态参数：getTransactionHash 是 nonce(uint256)，
// execTransaction 是 signatures(bytes)。lastNonce!=nil → uint256 nonce；否则 sig 作 bytes。
func packSafeCall(sel []byte, to common.Address, data []byte, lastNonce *big.Int, sigOpt ...[]byte) []byte {
	head := append([]byte{}, leftPad(to.Bytes())...) // to
	head = append(head, make([]byte, 32)...)          // value = 0
	dataOffSlot := len(head)
	head = append(head, make([]byte, 32)...) // data offset placeholder
	head = append(head, make([]byte, 32)...) // operation = 0
	head = append(head, make([]byte, 32)...) // safeTxGas = 0
	head = append(head, make([]byte, 32)...) // baseGas = 0
	head = append(head, make([]byte, 32)...) // gasPrice = 0
	head = append(head, make([]byte, 32)...) // gasToken = 0
	head = append(head, make([]byte, 32)...) // refundReceiver = 0
	var lastSlot int
	if lastNonce != nil {
		head = append(head, leftPad(lastNonce.Bytes())...) // nonce (static)
	} else {
		lastSlot = len(head)
		head = append(head, make([]byte, 32)...) // signatures offset placeholder
	}
	// head 现在 10*32 = 320 bytes
	dataBlob := encBytes(data)
	tail := append([]byte{}, dataBlob...)
	copy(head[dataOffSlot:dataOffSlot+32], word(int64(len(head))))
	if lastNonce == nil {
		sigOff := len(head) + len(dataBlob)
		copy(head[lastSlot:lastSlot+32], word(int64(sigOff)))
		tail = append(tail, encBytes(sigOpt[0])...)
	}
	return append(append(append([]byte{}, sel...), head...), tail...)
}

func encBytes(b []byte) []byte {
	out := append([]byte{}, word(int64(len(b)))...)
	out = append(out, b...)
	if pad := (32 - len(b)%32) % 32; pad > 0 {
		out = append(out, make([]byte, pad)...)
	}
	return out
}
func leftPad(b []byte) []byte { p := make([]byte, 32); copy(p[32-len(b):], b); return p }
func word(n int64) []byte     { return leftPad(big.NewInt(n).Bytes()) }

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
