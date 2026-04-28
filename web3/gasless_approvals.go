package web3

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	approveERC20ABISource      = `[{"name":"approve","type":"function","stateMutability":"nonpayable","inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[{"type":"bool"}]}]`
	setApprovalForAllABISource = `[{"name":"setApprovalForAll","type":"function","stateMutability":"nonpayable","inputs":[{"name":"operator","type":"address"},{"name":"approved","type":"bool"}],"outputs":[]}]`
	collateralOnrampABISource  = `[{"name":"wrap","type":"function","stateMutability":"nonpayable","inputs":[{"name":"asset","type":"address"},{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[]}]`
)

var (
	// 预解析的 ABI 在 init() 里一次成型后整个包复用。ABI 来源是 const，解析失败即编程错误。
	erc20ABI            abi.ABI
	setApprovalForAllAB abi.ABI // 避免与 const 名冲突的缩写
	collateralOnrampABI abi.ABI

	// maxUint256 = 2^256 - 1，ERC20 approve 的无限额度。
	maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
)

func init() {
	mustParse := func(src string, name string) abi.ABI {
		parsed, err := abi.JSON(strings.NewReader(src))
		if err != nil {
			panic(fmt.Sprintf("parse %s: %v", name, err))
		}
		return parsed
	}
	erc20ABI = mustParse(approveERC20ABISource, "approveERC20ABI")
	setApprovalForAllAB = mustParse(setApprovalForAllABISource, "setApprovalForAllABI")
	collateralOnrampABI = mustParse(collateralOnrampABISource, "collateralOnrampABI")
}

// callTxn 构造一笔 proxy Call 类型的 gasless 交易条目（typeCode=1）。
// executeGaslessBatch 里 encodeProxy 会以 map[string]any 断言，这里保留这个外形。
func callTxn(to common.Address, data []byte) map[string]any {
	return map[string]any{
		"typeCode": 1,
		"to":       to.Hex(),
		"value":    0,
		"data":     "0x" + hex.EncodeToString(data),
	}
}

// packApproveMax 生成 ERC20.approve(spender, MAX) 的 calldata。
func packApproveMax(spender common.Address) ([]byte, error) {
	return erc20ABI.Pack("approve", spender, maxUint256)
}

// packSetApprovalForAll 生成 ERC1155.setApprovalForAll(operator, true) 的 calldata。
func packSetApprovalForAll(operator common.Address) ([]byte, error) {
	return setApprovalForAllAB.Pack("setApprovalForAll", operator, true)
}

// SetV2Allowances 让 Safe/Proxy 钱包对 V2 相关合约一次性完成：
//   - USDC.e.approve(V2_Exchange, MAX)
//   - USDC.e.approve(V2_NegRiskExchange, MAX)
//   - USDC.e.approve(NegRiskAdapter, MAX)     —— V2 CLOB 的 balance-allowance 把它列进 spender 集合
//   - CTF.setApprovalForAll(V2_Exchange, true)
//   - CTF.setApprovalForAll(V2_NegRiskExchange, true)
//
// 五笔通过 Relayer 打包在一次 gasless 交易里。
func (c *GaslessClient) SetV2Allowances() (*types.TransactionReceipt, error) {
	usdc := common.HexToAddress(internal.PolygonCollateral)
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	exV2 := common.HexToAddress(internal.PolygonExchangeV2)
	exNRV2 := common.HexToAddress(internal.PolygonNegRiskExchangeV2)
	nrAdapter := common.HexToAddress(internal.PolygonNegRiskAdapter)

	usdcApproves := []common.Address{exV2, exNRV2, nrAdapter}
	ctfApprovals := []common.Address{exV2, exNRV2}

	proxyTxns := make([]map[string]any, 0, len(usdcApproves)+len(ctfApprovals))
	for _, spender := range usdcApproves {
		data, err := packApproveMax(spender)
		if err != nil {
			return nil, fmt.Errorf("pack approve USDC.e→%s: %w", spender.Hex(), err)
		}
		proxyTxns = append(proxyTxns, callTxn(usdc, data))
	}
	for _, op := range ctfApprovals {
		data, err := packSetApprovalForAll(op)
		if err != nil {
			return nil, fmt.Errorf("pack setApprovalForAll CTF→%s: %w", op.Hex(), err)
		}
		proxyTxns = append(proxyTxns, callTxn(ctf, data))
	}
	return c.executeGaslessBatch(proxyTxns, "Set V2 Allowances", "approve-v2")
}

// WrapAndApproveV2 一次性完成 V2 trading 的资金准备，打包成一笔 gasless batch：
//  1) USDC.e.approve(CollateralOnramp, MAX)
//  2) CollateralOnramp.wrap(USDC.e, safeProxy, amountUSDC*1e6)   —— 铸等量 pUSD 给 Safe
//  3) pUSD.approve(V2_Exchange, MAX)
//  4) pUSD.approve(V2_NegRiskExchange, MAX)
//  5) pUSD.approve(NegRiskAdapter, MAX)
//  6) pUSD.approve(CtfCollateralAdapter, MAX)                    —— split 时拉 pUSD
//  7) pUSD.approve(NegRiskCtfCollateralAdapter, MAX)
//  8) CTF.setApprovalForAll(CtfCollateralAdapter, true)          —— merge/redeem 时拉 CTF 位置 token
//  9) CTF.setApprovalForAll(NegRiskCtfCollateralAdapter, true)
//
// amountUSDC 传人类单位（如 40.0 = 40 USDC.e），内部按 6 decimals 转成最小单位。
// 只接受 Safe 签名类型（wrap 的 to 用 proxy 地址，EOA 路径下 pUSD 会发到 EOA，V2 Exchange 看不到）。
func (c *GaslessClient) WrapAndApproveV2(amountUSDC float64) (*types.TransactionReceipt, error) {
	amountInt, err := toUnits6(amountUSDC)
	if err != nil {
		return nil, fmt.Errorf("amountUSDC: %w", err)
	}

	usdc := common.HexToAddress(internal.PolygonCollateral)
	pusd := common.HexToAddress(internal.PolygonPUSD)
	onramp := common.HexToAddress(internal.PolygonCollateralOnramp)
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	ctfAdapter := common.HexToAddress(internal.PolygonCtfCollateralAdapter)
	nrCtfAdapter := common.HexToAddress(internal.PolygonNegRiskCtfCollateralAdapter)

	// wrap 的 to 必须是 Safe proxy，否则 pUSD 会发到 EOA，V2 Exchange 看不到
	safeAddrStr, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("GetPolyProxyAddress: %w", err)
	}
	safe := common.HexToAddress(string(safeAddrStr))

	approveUsdcToOnramp, err := packApproveMax(onramp)
	if err != nil {
		return nil, fmt.Errorf("pack approve USDC.e→onramp: %w", err)
	}
	wrapData, err := collateralOnrampABI.Pack("wrap", usdc, safe, amountInt)
	if err != nil {
		return nil, fmt.Errorf("pack wrap: %w", err)
	}

	pusdSpenders := []common.Address{
		common.HexToAddress(internal.PolygonExchangeV2),
		common.HexToAddress(internal.PolygonNegRiskExchangeV2),
		common.HexToAddress(internal.PolygonNegRiskAdapter),
		ctfAdapter,
		nrCtfAdapter,
	}
	ctfApprovals := []common.Address{ctfAdapter, nrCtfAdapter}

	proxyTxns := make([]map[string]any, 0, 2+len(pusdSpenders)+len(ctfApprovals))
	proxyTxns = append(proxyTxns, callTxn(usdc, approveUsdcToOnramp), callTxn(onramp, wrapData))
	for _, spender := range pusdSpenders {
		data, err := packApproveMax(spender)
		if err != nil {
			return nil, fmt.Errorf("pack approve pUSD→%s: %w", spender.Hex(), err)
		}
		proxyTxns = append(proxyTxns, callTxn(pusd, data))
	}
	for _, op := range ctfApprovals {
		data, err := packSetApprovalForAll(op)
		if err != nil {
			return nil, fmt.Errorf("pack setApprovalForAll CTF→%s: %w", op.Hex(), err)
		}
		proxyTxns = append(proxyTxns, callTxn(ctf, data))
	}
	return c.executeGaslessBatch(proxyTxns, "Wrap USDC.e and approve pUSD for V2", "wrap-v2")
}

// ApproveAdaptersV2 补上 V2 split/merge/redeem 必需、WrapAndApproveV2 旧版漏掉的 4 笔 approve：
//   - pUSD.approve(CtfCollateralAdapter, MAX)                  —— split 时 adapter 拉 pUSD
//   - pUSD.approve(NegRiskCtfCollateralAdapter, MAX)
//   - CTF.setApprovalForAll(CtfCollateralAdapter, true)        —— merge/redeem 时 adapter 拉 CTF 位置 token
//   - CTF.setApprovalForAll(NegRiskCtfCollateralAdapter, true)
//
// 钱包若只跑过老版 WrapAndApproveV2（只补到 V2 Exchange / NegRisk / NRAdapter 三个 spender），
// 必须再跑一次此函数才能调 splitPosition / mergePositions / redeemPositions via adapter。
func (c *GaslessClient) ApproveAdaptersV2() (*types.TransactionReceipt, error) {
	pusd := common.HexToAddress(internal.PolygonPUSD)
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	ctfAdapter := common.HexToAddress(internal.PolygonCtfCollateralAdapter)
	nrCtfAdapter := common.HexToAddress(internal.PolygonNegRiskCtfCollateralAdapter)

	pusdSpenders := []common.Address{ctfAdapter, nrCtfAdapter}
	ctfApprovals := []common.Address{ctfAdapter, nrCtfAdapter}

	proxyTxns := make([]map[string]any, 0, len(pusdSpenders)+len(ctfApprovals))
	for _, spender := range pusdSpenders {
		data, err := packApproveMax(spender)
		if err != nil {
			return nil, fmt.Errorf("pack approve pUSD→%s: %w", spender.Hex(), err)
		}
		proxyTxns = append(proxyTxns, callTxn(pusd, data))
	}
	for _, op := range ctfApprovals {
		data, err := packSetApprovalForAll(op)
		if err != nil {
			return nil, fmt.Errorf("pack setApprovalForAll CTF→%s: %w", op.Hex(), err)
		}
		proxyTxns = append(proxyTxns, callTxn(ctf, data))
	}
	return c.executeGaslessBatch(proxyTxns, "Approve V2 CTF collateral adapters", "approve-adapters-v2")
}
