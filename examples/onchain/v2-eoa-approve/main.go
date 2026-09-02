// v2_eoa_approve：EOA 钱包直发 8 笔 approve + 1 笔 wrap（自付 MATIC gas，不走 Relayer）。
//
// V2 Exchange 的 collateralToken 是 pUSD（不是 USDC.e），所以 EOA 用户必须：
//   - approve USDC.e → CollateralOnramp（让 onramp 能拉 USDC.e）
//   - approve pUSD → V2 Exchange / NegRisk Exchange（让 V2 能扣 pUSD 结算）
//   - wrap 一笔 USDC.e → pUSD（pUSD 落到 EOA 自己）
//
// USDC.e → V2 Exchange / NegRisk Exchange / NegRisk Adapter 的 approve 是给
// split/merge/redeem 用的，单纯下单用不上但保留无害。
//
// 适用场景：POLY_SIGNATURE_TYPE=0（EOA 模式），Relayer gasless 路径只支持 Safe
// 时使用本脚本。每笔 tx 串行提交并等待回执。已经 MAX 的会跳过。
//
// 必选 env：poly_sec（或 POLY_PRIVATE_KEY）
// 可选 env：POLY_RPC_URL（自定义 RPC，默认走 SDK 内置主网列表第一个）
// 可选 env：POLY_WRAP_USDC（要 wrap 的 USDC.e 数量，单位 USDC，默认 1.0）
package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/internal"
)

const erc20ABI = `[
  {"name":"approve","type":"function","stateMutability":"nonpayable","inputs":[{"name":"s","type":"address"},{"name":"a","type":"uint256"}],"outputs":[{"type":"bool"}]},
  {"name":"allowance","type":"function","stateMutability":"view","inputs":[{"name":"o","type":"address"},{"name":"s","type":"address"}],"outputs":[{"type":"uint256"}]},
  {"name":"balanceOf","type":"function","stateMutability":"view","inputs":[{"name":"a","type":"address"}],"outputs":[{"type":"uint256"}]}
]`

const ctf1155ABI = `[
  {"name":"setApprovalForAll","type":"function","stateMutability":"nonpayable","inputs":[{"name":"op","type":"address"},{"name":"a","type":"bool"}],"outputs":[]},
  {"name":"isApprovedForAll","type":"function","stateMutability":"view","inputs":[{"name":"o","type":"address"},{"name":"op","type":"address"}],"outputs":[{"type":"bool"}]}
]`

const onrampABI = `[{"name":"wrap","type":"function","stateMutability":"nonpayable","inputs":[{"name":"asset","type":"address"},{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[]}]`

type approveStep struct {
	desc     string
	contract common.Address
	is1155   bool
	spender  common.Address // ERC20 spender / ERC1155 operator
}

func main() {
	pk := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if pk == "" {
		log.Fatalf("缺少 poly_sec 或 POLY_PRIVATE_KEY")
	}
	pk = strings.TrimPrefix(strings.TrimSpace(pk), "0x")
	priv, err := crypto.HexToECDSA(pk)
	if err != nil {
		log.Fatalf("私钥解析失败: %v", err)
	}
	pub, ok := priv.Public().(*ecdsa.PublicKey)
	if !ok {
		log.Fatalf("公钥类型断言失败")
	}
	eoa := crypto.PubkeyToAddress(*pub)

	rpcURL := os.Getenv("POLY_RPC_URL")
	if rpcURL == "" {
		rpcURL = internal.PolygonRPCMainnetList[0]
	}

	ctx := context.Background()
	rpc, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		log.Fatalf("dial RPC %s: %v", rpcURL, err)
	}
	defer rpc.Close()

	chainID, err := rpc.ChainID(ctx)
	if err != nil {
		log.Fatalf("ChainID: %v", err)
	}

	usdc := common.HexToAddress(internal.PolygonCollateral)
	pusd := common.HexToAddress(internal.PolygonPUSD)
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	exV2 := common.HexToAddress(internal.PolygonExchangeV2)
	exNRV2 := common.HexToAddress(internal.PolygonNegRiskExchangeV2)
	onramp := common.HexToAddress(internal.PolygonCollateralOnramp)

	steps := []approveStep{
		{"USDC.e.approve(V2 Exchange,         MAX)", usdc, false, exV2},
		{"USDC.e.approve(V2 NegRisk Exchange, MAX)", usdc, false, exNRV2},
		{"USDC.e.approve(CollateralOnramp,    MAX)", usdc, false, onramp},
		{"pUSD.approve(V2 Exchange,           MAX)", pusd, false, exV2},
		{"pUSD.approve(V2 NegRisk Exchange,   MAX)", pusd, false, exNRV2},
		{"CTF.setApprovalForAll(V2 Exchange,         true)", ctf, true, exV2},
		{"CTF.setApprovalForAll(V2 NegRisk Exchange, true)", ctf, true, exNRV2},
	}

	erc20, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		log.Fatalf("parse erc20 abi: %v", err)
	}
	ctfABIp, err := abi.JSON(strings.NewReader(ctf1155ABI))
	if err != nil {
		log.Fatalf("parse ctf abi: %v", err)
	}

	balPOL, err := rpc.BalanceAt(ctx, eoa, nil)
	if err != nil {
		log.Fatalf("BalanceAt: %v", err)
	}
	fmt.Println("=============================================")
	fmt.Println(" V2 EOA Approve (self-paid gas)")
	fmt.Printf(" EOA          : %s\n", eoa.Hex())
	fmt.Printf(" RPC          : %s\n", rpcURL)
	fmt.Printf(" ChainID      : %s\n", chainID.String())
	fmt.Printf(" POL balance  : %s wei (%.6f POL/MATIC)\n",
		balPOL.String(), weiToFloat(balPOL, 18))
	fmt.Println("=============================================")
	for i, s := range steps {
		fmt.Printf(" %d) %s\n", i+1, s.desc)
	}
	fmt.Println()

	if balPOL.Sign() == 0 {
		log.Fatalf("EOA 没有 POL/MATIC，无法付 gas。请先充入约 0.5 POL 再重试。")
	}

	maxUint := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

	for i, s := range steps {
		fmt.Printf("[%d/%d] %s\n", i+1, len(steps), s.desc)

		// 幂等检查：已是 MAX / true 的跳过
		skip, err := alreadyApproved(ctx, rpc, &erc20, &ctfABIp, s, eoa)
		if err != nil {
			fmt.Printf("    ⚠ 检查 allowance 失败: %v（继续提交）\n", err)
		} else if skip {
			fmt.Printf("    ✅ 已 MAX/true，跳过\n\n")
			continue
		}

		var data []byte
		if s.is1155 {
			data, err = ctfABIp.Pack("setApprovalForAll", s.spender, true)
		} else {
			data, err = erc20.Pack("approve", s.spender, maxUint)
		}
		if err != nil {
			log.Fatalf("    pack data: %v", err)
		}

		txHash, err := sendTx(ctx, rpc, priv, chainID, eoa, s.contract, data)
		if err != nil {
			log.Fatalf("    ❌ 发送失败: %v", err)
		}
		fmt.Printf("    tx       : %s\n", txHash.Hex())
		fmt.Printf("    等待回执 ...\n")

		receipt, err := waitMined(ctx, rpc, txHash, 90*time.Second)
		if err != nil {
			log.Fatalf("    ❌ 等待回执失败: %v", err)
		}
		if receipt.Status != 1 {
			log.Fatalf("    ❌ 链上 revert，status=%d block=%d", receipt.Status, receipt.BlockNumber.Uint64())
		}
		fmt.Printf("    ✅ block=%d gasUsed=%d\n\n", receipt.BlockNumber.Uint64(), receipt.GasUsed)
	}

	// === 第 9 步：wrap USDC.e -> pUSD（pUSD 落到 EOA 自己） ===
	wrapAmtFloat := 1.0
	if v := os.Getenv("POLY_WRAP_USDC"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			wrapAmtFloat = f
		}
	}
	wrapAmt := new(big.Int).SetUint64(uint64(wrapAmtFloat * 1e6))

	pusdBal, err := readUint(ctx, rpc, &erc20, pusd, "balanceOf", eoa)
	if err != nil {
		log.Fatalf("查 pUSD 余额失败: %v", err)
	}
	fmt.Printf("[9/9] CollateralOnramp.wrap(USDC.e, EOA, %s) → 给 EOA 铸 %.6f pUSD\n",
		wrapAmt.String(), wrapAmtFloat)
	fmt.Printf("    当前 EOA pUSD 余额: %.6f\n", weiToFloat(pusdBal, 6))

	if pusdBal.Cmp(wrapAmt) >= 0 {
		fmt.Printf("    ✅ 已有足够 pUSD，跳过\n\n")
	} else {
		usdcBal, err := readUint(ctx, rpc, &erc20, usdc, "balanceOf", eoa)
		if err != nil {
			log.Fatalf("查 USDC.e 余额失败: %v", err)
		}
		if usdcBal.Cmp(wrapAmt) < 0 {
			log.Fatalf("    ❌ EOA USDC.e 余额 %.6f 不足 %.6f", weiToFloat(usdcBal, 6), wrapAmtFloat)
		}
		onrampParsed, err := abi.JSON(strings.NewReader(onrampABI))
		if err != nil {
			log.Fatalf("parse onramp abi: %v", err)
		}
		data, err := onrampParsed.Pack("wrap", usdc, eoa, wrapAmt)
		if err != nil {
			log.Fatalf("    pack wrap: %v", err)
		}
		txHash, err := sendTx(ctx, rpc, priv, chainID, eoa, onramp, data)
		if err != nil {
			log.Fatalf("    ❌ 发送失败: %v", err)
		}
		fmt.Printf("    tx       : %s\n", txHash.Hex())
		fmt.Printf("    等待回执 ...\n")
		receipt, err := waitMined(ctx, rpc, txHash, 90*time.Second)
		if err != nil {
			log.Fatalf("    ❌ 等待回执失败: %v", err)
		}
		if receipt.Status != 1 {
			log.Fatalf("    ❌ 链上 revert，status=%d block=%d", receipt.Status, receipt.BlockNumber.Uint64())
		}
		newBal, _ := readUint(ctx, rpc, &erc20, pusd, "balanceOf", eoa)
		fmt.Printf("    ✅ block=%d gasUsed=%d  EOA pUSD = %.6f\n\n",
			receipt.BlockNumber.Uint64(), receipt.GasUsed, weiToFloat(newBal, 6))
	}

	fmt.Println("✅ 全部完成。下一步：")
	fmt.Println("   POLY_SIGNATURE_TYPE=0 go run ./examples/trading/eoa-btc5m-double-quote")
}

func readUint(
	ctx context.Context, rpc *ethclient.Client, a *abi.ABI,
	to common.Address, method string, args ...any,
) (*big.Int, error) {
	d, err := a.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	out, err := rpc.CallContract(ctx, ethereum.CallMsg{To: &to, Data: d}, nil)
	if err != nil {
		return nil, err
	}
	var v *big.Int
	if err := a.UnpackIntoInterface(&v, method, out); err != nil {
		return nil, err
	}
	return v, nil
}

func alreadyApproved(
	ctx context.Context,
	rpc *ethclient.Client,
	erc20, ctf *abi.ABI,
	s approveStep,
	eoa common.Address,
) (bool, error) {
	if s.is1155 {
		data, err := ctf.Pack("isApprovedForAll", eoa, s.spender)
		if err != nil {
			return false, err
		}
		out, err := rpc.CallContract(ctx, ethereum.CallMsg{To: &s.contract, Data: data}, nil)
		if err != nil {
			return false, err
		}
		var v bool
		if err := ctf.UnpackIntoInterface(&v, "isApprovedForAll", out); err != nil {
			return false, err
		}
		return v, nil
	}
	data, err := erc20.Pack("allowance", eoa, s.spender)
	if err != nil {
		return false, err
	}
	out, err := rpc.CallContract(ctx, ethereum.CallMsg{To: &s.contract, Data: data}, nil)
	if err != nil {
		return false, err
	}
	var v *big.Int
	if err := erc20.UnpackIntoInterface(&v, "allowance", out); err != nil {
		return false, err
	}
	maxUint := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	// allowance 大于 1e30 视作 max（容忍非严格 max 的实现，例如 USDC 的 max approval）
	threshold, _ := new(big.Int).SetString("1000000000000000000000000000000", 10)
	return v.Cmp(threshold) >= 0 || v.Cmp(maxUint) == 0, nil
}

func sendTx(
	ctx context.Context,
	rpc *ethclient.Client,
	priv *ecdsa.PrivateKey,
	chainID *big.Int,
	from, to common.Address,
	data []byte,
) (common.Hash, error) {
	nonce, err := rpc.PendingNonceAt(ctx, from)
	if err != nil {
		return common.Hash{}, fmt.Errorf("PendingNonceAt: %w", err)
	}
	tip, err := rpc.SuggestGasTipCap(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("SuggestGasTipCap: %w", err)
	}
	head, err := rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		return common.Hash{}, fmt.Errorf("HeaderByNumber: %w", err)
	}
	// EIP-1559 fee cap = baseFee * 2 + tip
	feeCap := new(big.Int).Add(new(big.Int).Mul(head.BaseFee, big.NewInt(2)), tip)

	// Polygon 链经常需要更高的最低 priority fee（gas station ~30 gwei）
	min := big.NewInt(30_000_000_000) // 30 gwei
	if tip.Cmp(min) < 0 {
		tip = min
		if feeCap.Cmp(tip) < 0 {
			feeCap = new(big.Int).Add(new(big.Int).Mul(head.BaseFee, big.NewInt(2)), tip)
		}
	}

	gas, err := rpc.EstimateGas(ctx, ethereum.CallMsg{
		From: from, To: &to, Data: data,
	})
	if err != nil {
		return common.Hash{}, fmt.Errorf("EstimateGas: %w", err)
	}
	gas = gas * 12 / 10 // 20% buffer

	tx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: feeCap,
		Gas:       gas,
		To:        &to,
		Data:      data,
	})
	signer := ethtypes.LatestSignerForChainID(chainID)
	signed, err := ethtypes.SignTx(tx, signer, priv)
	if err != nil {
		return common.Hash{}, fmt.Errorf("SignTx: %w", err)
	}
	if err := rpc.SendTransaction(ctx, signed); err != nil {
		return common.Hash{}, fmt.Errorf("SendTransaction: %w", err)
	}
	return signed.Hash(), nil
}

func waitMined(
	ctx context.Context,
	rpc *ethclient.Client,
	hash common.Hash,
	timeout time.Duration,
) (*ethtypes.Receipt, error) {
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

func weiToFloat(v *big.Int, dec int) float64 {
	f, _ := new(big.Float).Quo(
		new(big.Float).SetInt(v),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dec)), nil)),
	).Float64()
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
