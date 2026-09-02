// v2_allowance_check：并行读 V1/V2 Exchange 的 USDC.e / pUSD allowance + CTF approval，
// 诊断 V2 下单被 "not enough balance / allowance" 拒的根因。
//
// 必选 env: poly_sec（或 POLY_PRIVATE_KEY）。
// 可选 env: POLY_SIGNATURE_TYPE（默认 2=Safe）。
package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"sync"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

const erc20ABI = `[
  {"name":"balanceOf","type":"function","stateMutability":"view","inputs":[{"name":"a","type":"address"}],"outputs":[{"type":"uint256"}]},
  {"name":"allowance","type":"function","stateMutability":"view","inputs":[{"name":"o","type":"address"},{"name":"s","type":"address"}],"outputs":[{"type":"uint256"}]}
]`

const ctfABI = `[
  {"name":"isApprovedForAll","type":"function","stateMutability":"view","inputs":[{"name":"o","type":"address"},{"name":"op","type":"address"}],"outputs":[{"type":"bool"}]}
]`

// balRow / allowRow / ctfRow 都是一条要并发查询+稍后按定义顺序打印的探针。
type balRow struct {
	label string
	dec   int
	token common.Address
	value *big.Int
	err   error
}

type allowRow struct {
	section string // "USDC.e" / "pUSD"；section 头按首次出现打印一次
	label   string
	token   common.Address
	spender common.Address
	value   *big.Int
	err     error
}

type ctfRow struct {
	label    string
	operator common.Address
	approved bool
	err      error
}

func main() {
	privateKey := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if privateKey == "" {
		log.Fatalf("缺少 poly_sec 或 POLY_PRIVATE_KEY")
	}
	sigType := types.SignatureType(getEnvInt("POLY_SIGNATURE_TYPE", int(types.SafeSignatureType)))

	w3, err := web3.NewClient(privateKey, sigType, types.Polygon)
	if err != nil {
		log.Fatalf("web3 client: %v", err)
	}
	defer w3.Close()

	eoa := common.HexToAddress(string(w3.GetBaseAddress()))
	proxyStr, err := w3.GetPolyProxyAddress()
	if err != nil {
		log.Fatalf("GetPolyProxyAddress: %v", err)
	}
	proxy := common.HexToAddress(string(proxyStr))

	fmt.Println("=== Address ===")
	fmt.Printf(" EOA            : %s\n", eoa.Hex())
	fmt.Printf(" Safe Proxy     : %s\n", proxy.Hex())
	fmt.Printf(" SignatureType  : %d\n\n", sigType)

	ctx := context.Background()
	rpc, err := ethclient.DialContext(ctx, internal.PolygonRPCMainnet)
	if err != nil {
		log.Fatalf("dial RPC: %v", err)
	}
	defer rpc.Close()

	erc20, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		log.Fatalf("parse erc20 abi: %v", err)
	}
	ctf, err := abi.JSON(strings.NewReader(ctfABI))
	if err != nil {
		log.Fatalf("parse ctf abi: %v", err)
	}

	usdc := common.HexToAddress(internal.PolygonCollateral)
	pusd := common.HexToAddress(internal.PolygonPUSD)
	cond := common.HexToAddress(internal.PolygonConditionalTokens)
	exV1 := common.HexToAddress(internal.PolygonExchange)
	exV2 := common.HexToAddress(internal.PolygonExchangeV2)
	exNRV1 := common.HexToAddress(internal.PolygonNegRiskExchange)
	exNRV2 := common.HexToAddress(internal.PolygonNegRiskExchangeV2)
	nrAdapter := common.HexToAddress(internal.PolygonNegRiskAdapter)
	ctfAdapter := common.HexToAddress(internal.PolygonCtfCollateralAdapter)
	nrCtfAdapter := common.HexToAddress(internal.PolygonNegRiskCtfCollateralAdapter)
	onramp := common.HexToAddress(internal.PolygonCollateralOnramp)
	autoClaimer := common.HexToAddress(internal.PolymarketAutoClaimer)

	bals := []balRow{
		{label: "USDC.e on Safe Proxy", dec: 6, token: usdc},
		{label: "pUSD on Safe Proxy", dec: 6, token: pusd},
	}
	allows := []allowRow{
		{section: "USDC.e", label: "V1 Exchange          ", token: usdc, spender: exV1},
		{section: "USDC.e", label: "V1 NegRisk Exchange  ", token: usdc, spender: exNRV1},
		{section: "USDC.e", label: "V2 Exchange          ", token: usdc, spender: exV2},
		{section: "USDC.e", label: "V2 NegRisk Exchange  ", token: usdc, spender: exNRV2},
		{section: "USDC.e", label: "CollateralOnramp     ", token: usdc, spender: onramp},
		{section: "pUSD", label: "V2 Exchange          ", token: pusd, spender: exV2},
		{section: "pUSD", label: "V2 NegRisk Exchange  ", token: pusd, spender: exNRV2},
		// Legacy/read-only: retained to diagnose stale approvals created before 2026-07-17.
		{section: "pUSD", label: "NegRiskAdapter(legacy)", token: pusd, spender: nrAdapter},
		{section: "pUSD", label: "CtfCollateralAdapter ", token: pusd, spender: ctfAdapter},
		{section: "pUSD", label: "NegRiskCtfCollAdapter", token: pusd, spender: nrCtfAdapter},
	}
	ctfs := []ctfRow{
		{label: "V1 Exchange          ", operator: exV1},
		{label: "V1 NegRisk Exchange  ", operator: exNRV1},
		{label: "V2 Exchange          ", operator: exV2},
		{label: "V2 NegRisk Exchange  ", operator: exNRV2},
		{label: "CtfCollateralAdapter ", operator: ctfAdapter},
		{label: "NegRiskCtfCollAdapter", operator: nrCtfAdapter},
		{label: "Polymarket AutoClaim ", operator: autoClaimer},
	}

	var wg sync.WaitGroup
	for i := range bals {
		wg.Add(1)
		go func(r *balRow) {
			defer wg.Done()
			r.value, r.err = callUint(ctx, rpc, erc20, r.token, "balanceOf", proxy)
		}(&bals[i])
	}
	for i := range allows {
		wg.Add(1)
		go func(r *allowRow) {
			defer wg.Done()
			r.value, r.err = callUint(ctx, rpc, erc20, r.token, "allowance", proxy, r.spender)
		}(&allows[i])
	}
	for i := range ctfs {
		wg.Add(1)
		go func(r *ctfRow) {
			defer wg.Done()
			r.approved, r.err = callBool(ctx, rpc, ctf, cond, "isApprovedForAll", proxy, r.operator)
		}(&ctfs[i])
	}
	wg.Wait()

	for _, r := range bals {
		fmt.Printf("=== %s ===\n", r.label)
		if r.err != nil {
			fmt.Printf(" ⚠ %v\n\n", r.err)
			continue
		}
		fmt.Printf(" balance        : %s (%.6f)\n\n", r.value.String(), float64ByDec(r.value, r.dec))
	}

	var lastSection string
	for _, r := range allows {
		if r.section != lastSection {
			if lastSection != "" {
				fmt.Println()
			}
			fmt.Printf("=== %s allowance(proxy → spender) ===\n", r.section)
			lastSection = r.section
		}
		if r.err != nil {
			fmt.Printf(" %s → %s : ⚠ %v\n", r.label, r.spender.Hex(), r.err)
			continue
		}
		status := "❌ NONE"
		if r.value.Sign() > 0 {
			status = "✅ set"
		}
		fmt.Printf(" %s → %s : %s  %s\n", r.label, r.spender.Hex(), humanAllow(r.value), status)
	}

	fmt.Println("\n=== CTF isApprovedForAll(proxy, operator) ===")
	for _, r := range ctfs {
		if r.err != nil {
			fmt.Printf(" %s → %s : ⚠ %v\n", r.label, r.operator.Hex(), r.err)
			continue
		}
		mark := "❌ false"
		if r.approved {
			mark = "✅ true"
		}
		fmt.Printf(" %s → %s : %s\n", r.label, r.operator.Hex(), mark)
	}
}

func callUint(ctx context.Context, rpc *ethclient.Client, a abi.ABI, to common.Address, method string, args ...any) (*big.Int, error) {
	data, err := a.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", method, err)
	}
	out, err := rpc.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", method, err)
	}
	var v *big.Int
	if err := a.UnpackIntoInterface(&v, method, out); err != nil {
		return nil, fmt.Errorf("unpack %s: %w", method, err)
	}
	return v, nil
}

func callBool(ctx context.Context, rpc *ethclient.Client, a abi.ABI, to common.Address, method string, args ...any) (bool, error) {
	data, err := a.Pack(method, args...)
	if err != nil {
		return false, fmt.Errorf("pack %s: %w", method, err)
	}
	out, err := rpc.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, nil)
	if err != nil {
		return false, fmt.Errorf("call %s: %w", method, err)
	}
	var v bool
	if err := a.UnpackIntoInterface(&v, method, out); err != nil {
		return false, fmt.Errorf("unpack %s: %w", method, err)
	}
	return v, nil
}

func humanAllow(v *big.Int) string {
	if v.Sign() == 0 {
		return "0"
	}
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	if v.Cmp(max) == 0 {
		return "MAX uint256"
	}
	return v.String()
}

func float64ByDec(v *big.Int, dec int) float64 {
	f, _ := new(big.Float).Quo(new(big.Float).SetInt(v), new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dec)), nil))).Float64()
	return f
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
