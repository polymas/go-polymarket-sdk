// approve_nrcta_direct：EOA 自付 gas 直接调 factory.proxy([batch], [sig]) 给 CWIA
// DepositWallet 补 NegRiskCtfCollateralAdapter approve。绕开 relayer 白名单。
//
// 注意：必须打到 DepositWalletFactory，不是 wallet 本身 —— DepositWallet.execute()
// 有 onlyFactory 修饰器，EOA 直调 wallet 会 revert OnlyFactory()（selector 0x0c6d42ae）。
//
// 用法：
//
//	PK=0x... [RPC=https://...] go run ./examples/approve_nrcta_direct
package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

const (
	defaultRPC  = "https://rpc-mainnet.matic.quiknode.pro"
	ctfAddr     = "0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"
	nrCtaAddr   = "0xadA2005600Dec949baf300f4C6120000bDB6eAab" // 2026-05 迁移后新地址（旧 0xAdA200001000…）
	ctaAddr     = "0xAdA100Db00Ca00073811820692005400218FcE1f" // 2026-05 迁移后新地址（旧 0xADa100874d…）
	pusdAddr    = "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB"
	factoryAddr = "0x00000000000Fb5C9ADea0298D729A0CB3823Cc07"
	chainID     = 137
)

func main() {
	pk := strings.TrimSpace(os.Getenv("PK"))
	if pk == "" {
		log.Fatal("env PK required")
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
	fmt.Printf("EOA: %s\n", eoa.Hex())

	// 用 SDK 派生 CWIA wallet 地址 + 读 wallet.nonce()
	c, err := web3.NewGaslessClient(pk, types.CWIASignatureType, types.Polygon, nil, rpcURL)
	if err != nil {
		log.Fatalf("NewGaslessClient: %v", err)
	}
	walletHex, _ := c.GetPolyProxyAddress()
	wallet := common.HexToAddress(string(walletHex))
	fmt.Printf("Wallet: %s\n", wallet.Hex())

	ec, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("ethclient.Dial: %v", err)
	}
	ctx := context.Background()

	// 链上 wallet.nonce()
	walletNonce, err := c.GetDepositWalletNonce(ctx, wallet)
	if err != nil {
		log.Fatalf("GetDepositWalletNonce: %v", err)
	}
	fmt.Printf("wallet.nonce(): %d\n", walletNonce)

	// 4 笔 approve calls — 与 ApproveAdaptersV2 同。旧 NegRiskAdapter 已退役。
	calls := []signing.CWIACall{
		packApproveMaxCall(pusdAddr, ctaAddr),
		packApproveMaxCall(pusdAddr, nrCtaAddr),
		packSetApprovalForAllCall(ctfAddr, ctaAddr),
		packSetApprovalForAllCall(ctfAddr, nrCtaAddr),
	}

	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())
	batch := signing.CWIABatch{
		Wallet:   wallet,
		Nonce:    new(big.Int).SetUint64(walletNonce),
		Deadline: deadline,
		Calls:    calls,
	}

	sig, err := signing.SignCWIABatch(priv, big.NewInt(chainID), batch)
	if err != nil {
		log.Fatalf("sign batch: %v", err)
	}
	fmt.Printf("batch deadline: %d  nonce: %d  calls: %d\n", deadline, walletNonce, len(calls))
	fmt.Printf("sig: 0x%x\n", sig)

	// 用 SDK 的 helper 编码 factory.proxy([batch], [sig]) calldata
	calldata, err := web3.BuildCWIABatchCalldata(batch, sig)
	if err != nil {
		log.Fatalf("BuildCWIABatchCalldata: %v", err)
	}
	factory := common.HexToAddress(factoryAddr)
	fmt.Printf("calldata len: %d (selector=0x%x → factory.proxy)\n", len(calldata), calldata[:4])
	fmt.Printf("target: %s (DepositWalletFactory)\n", factory.Hex())

	// 构造并发送 EIP-1559 tx
	eoaNonce, err := ec.PendingNonceAt(ctx, eoa)
	if err != nil {
		log.Fatalf("PendingNonceAt: %v", err)
	}
	chainIDBig, err := ec.ChainID(ctx)
	if err != nil {
		log.Fatalf("ChainID: %v", err)
	}

	header, err := ec.HeaderByNumber(ctx, nil)
	if err != nil {
		log.Fatalf("HeaderByNumber: %v", err)
	}
	baseFee := header.BaseFee
	if baseFee == nil {
		baseFee = big.NewInt(50_000_000_000) // 50 gwei fallback
	}
	tipCap := big.NewInt(50_000_000_000) // 50 gwei tip
	maxFee := new(big.Int).Add(new(big.Int).Mul(baseFee, big.NewInt(2)), tipCap)

	estGas, err := ec.EstimateGas(ctx, ethereumCallMsg(eoa, factory, calldata))
	if err != nil {
		fmt.Printf("⚠️  EstimateGas failed, falling back to 1.5M: %v\n", err)
		estGas = 1_500_000
	} else {
		estGas = estGas * 120 / 100 // +20% buffer
	}
	fmt.Printf("EOA nonce: %d, gas: %d, maxFee: %s gwei, tip: %s gwei\n",
		eoaNonce, estGas, new(big.Int).Quo(maxFee, big.NewInt(1e9)), new(big.Int).Quo(tipCap, big.NewInt(1e9)))

	tx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:   chainIDBig,
		Nonce:     eoaNonce,
		GasTipCap: tipCap,
		GasFeeCap: maxFee,
		Gas:       estGas,
		To:        &factory,
		Value:     big.NewInt(0),
		Data:      calldata,
	})
	signedTx, err := ethtypes.SignTx(tx, ethtypes.LatestSignerForChainID(chainIDBig), priv)
	if err != nil {
		log.Fatalf("SignTx: %v", err)
	}
	if err := ec.SendTransaction(ctx, signedTx); err != nil {
		log.Fatalf("SendTransaction: %v", err)
	}
	hashHex := signedTx.Hash().Hex()
	fmt.Printf("\n📤 tx submitted: %s\n", hashHex)
	fmt.Printf("polygonscan: https://polygonscan.com/tx/%s\n\n", hashHex)
	fmt.Println("waiting for receipt...")

	deadlineCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for {
		select {
		case <-deadlineCtx.Done():
			log.Fatalf("timeout waiting receipt")
		default:
		}
		r, err := ec.TransactionReceipt(ctx, signedTx.Hash())
		if err == nil {
			status := "❌ FAILED"
			if r.Status == 1 {
				status = "✅ SUCCESS"
			}
			fmt.Printf("%s  block=%d gasUsed=%d\n", status, r.BlockNumber, r.GasUsed)
			if r.Status == 1 {
				fmt.Println("\nApproval 已上链，现在跑 claim_test 应该 NegRisk redeem 就成功了。")
			}
			return
		}
		time.Sleep(3 * time.Second)
	}
}

func packApproveMaxCall(token, spender string) signing.CWIACall {
	// approve(address,uint256) = 0x095ea7b3
	sel, _ := hex.DecodeString("095ea7b3")
	addrPad := make([]byte, 32)
	copy(addrPad[12:], common.HexToAddress(spender).Bytes())
	maxUint := make([]byte, 32)
	for i := range maxUint {
		maxUint[i] = 0xff
	}
	data := append([]byte{}, sel...)
	data = append(data, addrPad...)
	data = append(data, maxUint...)
	return signing.CWIACall{
		Target: common.HexToAddress(token),
		Value:  big.NewInt(0),
		Data:   data,
	}
}

func packSetApprovalForAllCall(token, operator string) signing.CWIACall {
	// setApprovalForAll(address,bool) = 0xa22cb465
	sel, _ := hex.DecodeString("a22cb465")
	addrPad := make([]byte, 32)
	copy(addrPad[12:], common.HexToAddress(operator).Bytes())
	flag := make([]byte, 32)
	flag[31] = 1
	data := append([]byte{}, sel...)
	data = append(data, addrPad...)
	data = append(data, flag...)
	return signing.CWIACall{
		Target: common.HexToAddress(token),
		Value:  big.NewInt(0),
		Data:   data,
	}
}

func ethereumCallMsg(from, to common.Address, data []byte) ethereum.CallMsg {
	return ethereum.CallMsg{From: from, To: &to, Data: data}
}
