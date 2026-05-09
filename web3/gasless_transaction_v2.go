package web3

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// V2 下 split / merge / redeem 不再直接调 ConditionalTokens / NegRiskAdapter，
// 而是走 CtfCollateralAdapter（Regular）或 NegRiskCtfCollateralAdapter（NegRisk）。
// 两个 adapter 对外暴露的都是 V1 CTF 的「长签名」：
//   - splitPosition(address collateralToken, bytes32 parentCollectionId,
//                   bytes32 conditionId, uint256[] partition, uint256 amount)
//   - mergePositions(address, bytes32, bytes32, uint256[], uint256)
//   - redeemPositions(address, bytes32, bytes32, uint256[] indexSets)
//
// 关键点：第一个参数 `collateralToken` 依然填 USDC.e 地址（不是 pUSD）。
// Adapter 内部会拉 pUSD → unwrap 成 USDC.e → 调底层 CTF → 结果（CTF 位置 token 或
// USDC.e）再 wrap 成 pUSD 发给 Safe。所以**调用者实际花/收的是 pUSD**。

// SplitPosition 把 pUSD split 成 YES+NO 位置 token。
// amount 为人类单位（1.0 = 1 pUSD），internal 按 6 decimals 转 uint256。
// negRisk=false 走 CtfCollateralAdapter；negRisk=true 走 NegRiskCtfCollateralAdapter。
func (c *GaslessClient) SplitPosition(amount float64, conditionID types.Keccak256, negRisk bool) (*types.TransactionReceipt, error) {
	if err := conditionID.Validate(); err != nil {
		return nil, fmt.Errorf("conditionID: %w", err)
	}
	intAmount, err := toUnits6(amount)
	if err != nil {
		return nil, err
	}

	data, err := c.encodeSplit(conditionID, intAmount) // 长签名 & collateralToken=USDC.e 与 V1 一样
	if err != nil {
		return nil, fmt.Errorf("encode split V2: %w", err)
	}

	proxyTxns := []map[string]any{callTxn(adapterAddrV2(negRisk), data)}
	return c.executeGaslessBatch(proxyTxns, "Split Position V2", "split-v2")
}

// MergePositions 把 YES+NO 位置 token 合回 pUSD。随时可做，无需等结算。
// amount 为人类单位（1.0 = 合并 1 pUSD 规模的完整 set）。
func (c *GaslessClient) MergePositions(conditionID types.Keccak256, amount float64, negRisk bool) (*types.TransactionReceipt, error) {
	if err := conditionID.Validate(); err != nil {
		return nil, fmt.Errorf("conditionID: %w", err)
	}
	intAmount, err := toUnits6(amount)
	if err != nil {
		return nil, err
	}

	data, err := c.encodeMerge(conditionID, intAmount)
	if err != nil {
		return nil, fmt.Errorf("encode merge V2: %w", err)
	}

	proxyTxns := []map[string]any{callTxn(adapterAddrV2(negRisk), data)}
	return c.executeGaslessBatch(proxyTxns, "Merge Positions V2", "merge-v2")
}

// RedeemPositionInfo V2 redeem 单位参数。
//
// 实测 Polymarket Gasless Relayer 在 Unverified Builder Tier 下封禁了所有
// `redeemPositions` 走 NegRiskAdapter / NegRiskCtfCollateralAdapter 的 calldata
// （27ms 内 STATE_FAILED 不广播，TS/Go 同症状），但放行 CTF 长签名 redeem。
// 因此 NegRisk 路径绕开 NegRiskAdapter，直接拆成与 Regular 同 selector 的两步：
//
//	Regular → CTF.redeemPositions(USDC.e, 0x00, conditionId, [1<<OutcomeIndex])
//
//	NegRisk → 一笔 batch 里两个 op：
//	   1) CTF.redeemPositions(wcol, 0x00, conditionId, [1<<OutcomeIndex])
//	      —— 烧 Safe 持有的 NRPositionToken，给 Safe 转 size 个 wcol
//	   2) WCol.unwrap(safe, size)
//	      —— 烧 wcol，给 Safe 转等额 USDC.e
//
// 字段说明：
//   - OutcomeIndex：胜出的 outcome 下标（YES=0 / NO=1）
//   - Size：押在 OutcomeIndex 上的位置 token 数量（人类单位，6 decimals）；
//     必填——NegRisk 用来给 WCol.unwrap 的 amount，Regular 也要求 > 0 以与
//     链上余额一致（不一致会 ERC1155 InsufficientBalance）
type RedeemPositionInfo struct {
	ConditionID  types.Keccak256
	OutcomeIndex int
	Size         float64
	NegRisk      bool
}

// RedeemPositions 在一个 gasless batch 里对多个 position 做 redeem。
// 结算后才能拿到 collateral；未结算时对 conditionId 调用会 revert。
//
// NegRisk 每笔会展开成 2 个 op（CTF.redeemPositions + WCol.unwrap），
// Regular 每笔 1 个 op。所有 op 一起进 Safe MultiSend。
func (c *GaslessClient) RedeemPositions(positions []RedeemPositionInfo) (*types.TransactionReceipt, error) {
	if len(positions) == 0 {
		return nil, fmt.Errorf("no positions to redeem")
	}

	safeAddrStr, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("GetPolyProxyAddress: %w", err)
	}
	safe := common.HexToAddress(string(safeAddrStr))

	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	wcol := common.HexToAddress(internal.PolygonNegRiskWrappedCollateral)

	proxyTxns := make([]map[string]any, 0, len(positions)*2)
	for i, pos := range positions {
		if err := pos.ConditionID.Validate(); err != nil {
			return nil, fmt.Errorf("position %d conditionID: %w", i, err)
		}
		if pos.OutcomeIndex != 0 && pos.OutcomeIndex != 1 {
			return nil, fmt.Errorf("position %d: OutcomeIndex must be 0 (YES) or 1 (NO), got %d", i, pos.OutcomeIndex)
		}
		if pos.Size <= 0 {
			return nil, fmt.Errorf("position %d: redeem requires positive Size (实际持仓数量，6 decimals 人类单位)", i)
		}
		intSize, err := toUnits6(pos.Size)
		if err != nil {
			return nil, fmt.Errorf("position %d size: %w", i, err)
		}
		indexSet := new(big.Int).Lsh(big.NewInt(1), uint(pos.OutcomeIndex))

		var collateralToken common.Address
		if pos.NegRisk {
			collateralToken = wcol
		} else {
			collateralToken = common.HexToAddress(internal.PolygonCollateral) // USDC.e
		}

		// op #1: CTF.redeemPositions(collateral, 0, conditionId, [1<<idx])
		redeemData, err := encodeRedeemCTF(collateralToken, pos.ConditionID, []*big.Int{indexSet})
		if err != nil {
			return nil, fmt.Errorf("encode CTF.redeemPositions for position %d: %w", i, err)
		}
		proxyTxns = append(proxyTxns, callTxn(ctf, redeemData))

		// NegRisk 多一个 op：把上一步收到的 wcol unwrap 回 USDC.e
		if pos.NegRisk {
			unwrapData, err := encodeWColUnwrap(safe, intSize)
			if err != nil {
				return nil, fmt.Errorf("encode WCol.unwrap for position %d: %w", i, err)
			}
			proxyTxns = append(proxyTxns, callTxn(wcol, unwrapData))
		}
	}
	return c.executeGaslessBatch(proxyTxns, "Redeem Positions V2", "redeem-v2")
}

// toUnits6 把 float 金额转成 6 decimals uint256，负数或零会报错。
func toUnits6(amount float64) (*big.Int, error) {
	amountFloat := big.NewFloat(amount)
	multiplier := big.NewFloat(1e6)
	result := new(big.Float).Mul(amountFloat, multiplier)
	intAmount, _ := result.Int(nil)
	if intAmount == nil || intAmount.Sign() <= 0 {
		return nil, fmt.Errorf("amount must be positive, got %f", amount)
	}
	return intAmount, nil
}

// adapterAddrV2 按 negRisk 返回对应 CtfCollateralAdapter 地址（split/merge 走 adapter）。
func adapterAddrV2(negRisk bool) common.Address {
	if negRisk {
		return common.HexToAddress(internal.PolygonNegRiskCtfCollateralAdapter)
	}
	return common.HexToAddress(internal.PolygonCtfCollateralAdapter)
}

// encodeRedeemCTF 编码 CTF 长签名 redeemPositions(address,bytes32,bytes32,uint256[])。
// collateralToken：Regular 传 USDC.e；NegRisk 传 wcol（NegRiskAdapter 的 wrap 合约）。
// indexSets 通常只放一个元素 [1<<outcomeIndex]，对齐官方 builder example。
func encodeRedeemCTF(collateralToken common.Address, conditionID types.Keccak256, indexSets []*big.Int) ([]byte, error) {
	selector := crypto.Keccak256([]byte("redeemPositions(address,bytes32,bytes32,uint256[])"))[:4]

	hashZero := common.HexToHash(internal.HashZero)
	condHash := common.HexToHash(string(conditionID))

	data := append([]byte{}, selector...)

	collateralBytes := make([]byte, 32)
	copy(collateralBytes[12:], collateralToken.Bytes())
	data = append(data, collateralBytes...)

	data = append(data, hashZero.Bytes()...)
	data = append(data, condHash.Bytes()...)

	// indexSets 动态数组：offset = 0x80（4 个固定参数 * 0x20）
	offset := big.NewInt(0x80)
	offsetBytes := make([]byte, 32)
	offset.FillBytes(offsetBytes)
	data = append(data, offsetBytes...)

	arrayLen := big.NewInt(int64(len(indexSets)))
	lenBytes := make([]byte, 32)
	arrayLen.FillBytes(lenBytes)
	data = append(data, lenBytes...)

	for _, v := range indexSets {
		elemBytes := make([]byte, 32)
		v.FillBytes(elemBytes)
		data = append(data, elemBytes...)
	}

	return data, nil
}

// encodeWColUnwrap 编码 WrappedCollateral.unwrap(address to, uint256 amount)。
// 没有访问控制：caller 烧自己的 wcol，underlying USDC.e 转给 to。
// selector=0x39f47693。
func encodeWColUnwrap(to common.Address, amount *big.Int) ([]byte, error) {
	selector, err := hex.DecodeString("39f47693")
	if err != nil {
		return nil, fmt.Errorf("decode selector: %w", err)
	}

	data := append([]byte{}, selector...)

	addrBytes := make([]byte, 32)
	copy(addrBytes[12:], to.Bytes())
	data = append(data, addrBytes...)

	amountBytes := make([]byte, 32)
	amount.FillBytes(amountBytes)
	data = append(data, amountBytes...)

	return data, nil
}
