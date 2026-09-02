// withdraw_pusd：从 Polymarket proxy 钱包把 pUSD 提到任意 Polygon 地址。
//
// 流程（EOA 自付 gas，绕开 Polymarket relayer 白名单）：
//  1. 用私钥派生 EOA + proxy（Polymarket 钱包）地址
//  2. 调 bridge.polymarket.com/withdraw（带 X-Builder-Code）拿到 Polygon deposit 地址
//  3. EOA 直接调用 user proxy 的 `proxy((uint8,address,uint256,bytes)[])`（selector 0x34ee9791），
//     批次内含 1 笔 pUSD.transfer(deposit, amount)。EOA 是 proxy 的 owner，所以无需 relayer 签名。
//  4. 桥的后端异步把资金转发给最终 recipientAddr。
//
// Polymarket gasless relayer 的 RelayHub.acceptRelayedCall 不允许 proxy 对任意地址做 ERC20.transfer，
// 但 proxy 合约本体没有这一限制——owner-pays-gas 模式就能跑通。
//
// 必选 env：
//
//	POLY_PRIVATE_KEY                EOA 私钥（必须是 proxy 的 owner）
//	WITHDRAW_TO                     最终收款地址（0x...）
//	POLY_API_PASSPHRASE             64 hex 字符的 builder code（前端 Settings → Builder 生成）
//
// 可选 env：
//
//	POLY_SIGNATURE_TYPE             默认 1 (proxy)
//	WITHDRAW_AMOUNT_PUSD            提现 pUSD 数量（人类单位）；未设 = 全部余额
//	WITHDRAW_PROBE_ONLY=1           只做余额查询 + bridge 接口探测，不真正转账
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	privateKey := firstEnv("POLY_PRIVATE_KEY", "poly_sec")
	if privateKey == "" {
		log.Fatalf("缺少 POLY_PRIVATE_KEY")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.ProxySignatureType)))

	recipient := strings.TrimSpace(os.Getenv("WITHDRAW_TO"))
	if !common.IsHexAddress(recipient) {
		log.Fatalf("WITHDRAW_TO 不是合法地址: %q", recipient)
	}
	recipientAddr := common.HexToAddress(recipient)

	probeOnly := os.Getenv("WITHDRAW_PROBE_ONLY") == "1"

	// 用 SDK 的 GaslessClient 只是为了拿 EOA / proxy 地址 + 链上读余额；提现 tx 不走它。
	creds := &types.ApiCreds{
		Key:        os.Getenv("POLY_API_KEY"),
		Secret:     os.Getenv("POLY_API_SECRET"),
		Passphrase: os.Getenv("POLY_API_PASSPHRASE"),
	}
	gasless, err := web3.NewGaslessClient(privateKey, sigType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("NewGaslessClient: %v", err)
	}
	defer gasless.Close()

	eoa := gasless.GetBaseAddress()
	proxyStr, err := gasless.GetPolyProxyAddress()
	if err != nil {
		log.Fatalf("GetPolyProxyAddress: %v", err)
	}
	proxyAddr := common.HexToAddress(string(proxyStr))

	priv, err := crypto.HexToECDSA(strings.TrimPrefix(privateKey, "0x"))
	if err != nil {
		log.Fatalf("解析私钥: %v", err)
	}
	pub, _ := priv.Public().(*ecdsa.PublicKey)
	if crypto.PubkeyToAddress(*pub).Hex() != string(eoa) {
		log.Fatalf("私钥派生的 EOA 与 SDK 返回不一致：%s vs %s", crypto.PubkeyToAddress(*pub).Hex(), eoa)
	}

	fmt.Println("============================================================")
	fmt.Println(" Polymarket pUSD 提现（EOA 自付 gas）")
	fmt.Println("============================================================")
	fmt.Printf(" EOA       : %s\n", eoa)
	fmt.Printf(" Proxy     : %s   (signature_type=%d)\n", proxyAddr.Hex(), sigType)
	fmt.Printf(" Recipient : %s\n", recipientAddr.Hex())
	fmt.Println("============================================================")

	// 1) pUSD 余额 + EOA 的 POL 余额（要付 gas）
	pUSDAddr := common.HexToAddress(internal.PolygonPUSD)
	balanceWei, err := gasless.GetERC20BalanceOf(pUSDAddr, proxyAddr)
	if err != nil {
		log.Fatalf("查 proxy pUSD 余额失败: %v", err)
	}
	balanceHuman := bfToFloat(new(big.Float).Quo(new(big.Float).SetInt(balanceWei), big.NewFloat(1e6)))
	fmt.Printf("Proxy pUSD 余额: %s (= %.6f pUSD)\n", balanceWei.String(), balanceHuman)

	polBal, _ := gasless.GetPOLBalance()
	fmt.Printf("EOA POL 余额  : %.6f\n", polBal)
	if polBal < 0.05 {
		log.Fatalf("EOA POL 余额 < 0.05，无法支付 gas")
	}

	// 2) Bridge /withdraw
	fmt.Println("\n── Polymarket Bridge /withdraw ──")
	builderCode := os.Getenv("POLY_BUILDER_CODE")
	if builderCode == "" {
		if p := os.Getenv("POLY_API_PASSPHRASE"); len(p) == 64 {
			builderCode = "0x" + p
		}
	}
	depAddrs, raw, err := callBridgeWithdraw(proxyAddr.Hex(), "137", pUSDAddr.Hex(), recipientAddr.Hex(), builderCode)
	if err != nil {
		log.Fatalf("bridge /withdraw 失败：%v\n  body: %s", err, raw)
	}
	fmt.Printf("raw: %s\n", raw)
	depHex, ok := depAddrs["evm"]
	if !ok || !common.IsHexAddress(depHex) {
		log.Fatalf("bridge 未返回 evm deposit 地址；raw=%s", raw)
	}
	depositAddr := common.HexToAddress(depHex)
	fmt.Printf("→ Polygon deposit 地址：%s\n", depositAddr.Hex())

	if probeOnly {
		fmt.Println("\nWITHDRAW_PROBE_ONLY=1，跳过实际转账。")
		return
	}

	// 3) 决定提现数量
	amountInt := new(big.Int).Set(balanceWei)
	if v := os.Getenv("WITHDRAW_AMOUNT_PUSD"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			log.Fatalf("WITHDRAW_AMOUNT_PUSD 解析失败: %v", err)
		}
		bf := new(big.Float).Mul(big.NewFloat(f), big.NewFloat(1e6))
		amountInt, _ = bf.Int(nil)
	}
	if amountInt.Sign() <= 0 {
		log.Fatalf("提现金额必须 > 0")
	}
	if amountInt.Cmp(balanceWei) > 0 {
		log.Fatalf("提现金额 (%s) > proxy 余额 (%s)", amountInt.String(), balanceWei.String())
	}
	amountHuman := bfToFloat(new(big.Float).Quo(new(big.Float).SetInt(amountInt), big.NewFloat(1e6)))

	// 4) 构造 proxy 批次：[{typeCode=1, to=pUSD, value=0, data=transfer(deposit, amount)}]
	erc20Src := `[{"name":"transfer","type":"function","stateMutability":"nonpayable","inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[{"type":"bool"}]}]`
	erc20, err := abi.JSON(strings.NewReader(erc20Src))
	if err != nil {
		log.Fatalf("parse erc20: %v", err)
	}
	transferData, err := erc20.Pack("transfer", depositAddr, amountInt)
	if err != nil {
		log.Fatalf("pack transfer: %v", err)
	}
	batch := encodeProxyBatch([]proxyCall{
		{typeCode: 1, to: pUSDAddr, value: big.NewInt(0), data: transferData},
	})

	fmt.Printf("\n准备 EOA 直发 proxy.proxy(...)，转 %.6f pUSD → %s（桥再转 → %s）\n",
		amountHuman, depositAddr.Hex(), recipientAddr.Hex())
	fmt.Printf("calldata: 0x%s\n", hex.EncodeToString(batch))

	// 5) 发 tx
	ctx := context.Background()
	rpc, err := ethclient.Dial(internal.PolygonRPCMainnet)
	if err != nil {
		log.Fatalf("RPC dial: %v", err)
	}
	chainID := big.NewInt(137)
	txHash, err := sendEOATx(ctx, rpc, priv, chainID, common.HexToAddress(string(eoa)), proxyAddr, batch)
	if err != nil {
		log.Fatalf("send tx: %v", err)
	}
	fmt.Printf("tx hash: %s\nhttps://polygonscan.com/tx/%s\n", txHash.Hex(), txHash.Hex())

	rec, err := waitMined(ctx, rpc, txHash, 90*time.Second)
	if err != nil {
		log.Fatalf("wait mined: %v", err)
	}
	fmt.Printf("✅ status=%d  block=%d  gasUsed=%d\n", rec.Status, rec.BlockNumber.Uint64(), rec.GasUsed)
	if rec.Status != 1 {
		log.Fatalf("❌ 链上 revert")
	}

	// 6) 核对
	if pBal, err := gasless.GetERC20BalanceOf(pUSDAddr, proxyAddr); err == nil {
		hb := bfToFloat(new(big.Float).Quo(new(big.Float).SetInt(pBal), big.NewFloat(1e6)))
		fmt.Printf("Proxy 剩余 pUSD: %.6f\n", hb)
	}
	if dBal, err := gasless.GetERC20BalanceOf(pUSDAddr, depositAddr); err == nil {
		hb := bfToFloat(new(big.Float).Quo(new(big.Float).SetInt(dBal), big.NewFloat(1e6)))
		fmt.Printf("Deposit 地址 pUSD: %.6f（桥会异步搬走）\n", hb)
	}
}

type proxyCall struct {
	typeCode uint8
	to       common.Address
	value    *big.Int
	data     []byte
}

// encodeProxyBatch 按 PolyProxy 的 proxy((uint8,address,uint256,bytes)[]) 拼 calldata。
// selector 0x34ee9791；编码方式与 SDK gasless_relay.encodeProxy 完全一致，只换 selector。
func encodeProxyBatch(calls []proxyCall) []byte {
	out := []byte{0x34, 0xee, 0x97, 0x91}

	// array 头部 offset
	out = append(out, leftPad32(big.NewInt(0x20).Bytes())...)
	// 元素数
	out = append(out, leftPad32(big.NewInt(int64(len(calls))).Bytes())...)

	// 与 SDK web3/gasless_relay.go encodeProxy 一一对应：
	// tupleOffsetValue = 0x20*(N+1) + len(已写入 data，包括 selector)
	for range calls {
		offsetVal := int64(0x20*(len(calls)+1) + len(out))
		out = append(out, leftPad32(big.NewInt(offsetVal).Bytes())...)
	}

	for _, c := range calls {
		// typeCode (uint8 → 32 bytes)
		tb := make([]byte, 32)
		tb[31] = c.typeCode
		out = append(out, tb...)
		// to (address → 32 bytes)
		ab := make([]byte, 32)
		copy(ab[12:], c.to.Bytes())
		out = append(out, ab...)
		// value
		out = append(out, leftPad32(c.value.Bytes())...)
		// data offset relative to tuple start = 0x60（Polymarket 自有打包方式，不是标准 ABI）
		out = append(out, leftPad32(big.NewInt(0x60).Bytes())...)
		// data len
		out = append(out, leftPad32(big.NewInt(int64(len(c.data))).Bytes())...)
		// data 原样追加，不补 32 字节倍数（与 SDK Rust 实现一致）
		out = append(out, c.data...)
	}
	return out
}

func leftPad32(b []byte) []byte {
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	return out
}

func sendEOATx(ctx context.Context, rpc *ethclient.Client, priv *ecdsa.PrivateKey, chainID *big.Int, from, to common.Address, data []byte) (common.Hash, error) {
	nonce, err := rpc.PendingNonceAt(ctx, from)
	if err != nil {
		return common.Hash{}, fmt.Errorf("PendingNonceAt: %w", err)
	}
	tip, err := rpc.SuggestGasTipCap(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("tip: %w", err)
	}
	head, err := rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		return common.Hash{}, fmt.Errorf("head: %w", err)
	}
	min := big.NewInt(30_000_000_000)
	if tip.Cmp(min) < 0 {
		tip = min
	}
	feeCap := new(big.Int).Add(new(big.Int).Mul(head.BaseFee, big.NewInt(2)), tip)
	gas, err := rpc.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &to, Data: data})
	if err != nil {
		return common.Hash{}, fmt.Errorf("EstimateGas: %w（说明 proxy 直调也被合约拒绝）", err)
	}
	gas = gas * 12 / 10
	tx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: feeCap,
		Gas:       gas,
		To:        &to,
		Data:      data,
	})
	signed, err := ethtypes.SignTx(tx, ethtypes.LatestSignerForChainID(chainID), priv)
	if err != nil {
		return common.Hash{}, err
	}
	if err := rpc.SendTransaction(ctx, signed); err != nil {
		return common.Hash{}, fmt.Errorf("SendTransaction: %w", err)
	}
	return signed.Hash(), nil
}

func waitMined(ctx context.Context, rpc *ethclient.Client, hash common.Hash, timeout time.Duration) (*ethtypes.Receipt, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		r, err := rpc.TransactionReceipt(cctx, hash)
		if err == nil {
			return r, nil
		}
		select {
		case <-cctx.Done():
			return nil, fmt.Errorf("超时等待 tx %s", hash.Hex())
		case <-time.After(2 * time.Second):
		}
	}
}

func callBridgeWithdraw(srcPolyAddr, toChainID, toToken, recipient, builderCode string) (map[string]string, string, error) {
	body := map[string]string{
		"address":        srcPolyAddr,
		"toChainId":      toChainID,
		"toTokenAddress": toToken,
		"recipientAddr":  recipient,
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", "https://bridge.polymarket.com/withdraw", bytes.NewReader(buf))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if builderCode != "" {
		req.Header.Set("X-Builder-Code", builderCode)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, string(raw), fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, string(raw), err
	}
	out := map[string]string{}
	if addr, ok := parsed["address"].(map[string]any); ok {
		for k, v := range addr {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out, string(raw), nil
}

func bfToFloat(b *big.Float) float64 { v, _ := b.Float64(); return v }

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
