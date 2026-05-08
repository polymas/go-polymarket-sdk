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
// 与 split/merge 不同，V2 redeem 不能走 CtfCollateralAdapter ——
// Polymarket Gasless Relayer 的白名单只放行底层 ConditionalTokens / NegRiskAdapter，
// 看到 to=CollateralAdapter 会直接静默返回 STATE_FAILED（无 error/reason）。
// 这里的 to + calldata 与官方 builder example 对齐：
//
//	Regular → ConditionalTokens.redeemPositions(USDC.e, 0x00, conditionId, [1<<OutcomeIndex])
//	NegRisk → NegRiskAdapter.redeemPositions(conditionId, amounts)
//	          其中 amounts[OutcomeIndex]=Size（6 decimals），另一边为 0
//
// 字段说明：
//   - OutcomeIndex：胜出的 outcome 下标（YES=0 / NO=1）
//   - Size：押在 OutcomeIndex 上的位置 token 数量（人类单位，6 decimals）；
//     仅 NegRisk 使用，Regular 直接按 [1<<OutcomeIndex] 索引集合 redeem，无需金额
type RedeemPositionInfo struct {
	ConditionID  types.Keccak256
	OutcomeIndex int
	Size         float64
	NegRisk      bool
}

// RedeemPositions 在一个 gasless batch 里对多个 position 做 redeem。
// 结算后才能拿到 collateral；未结算时对 conditionId 调用会 revert。
func (c *GaslessClient) RedeemPositions(positions []RedeemPositionInfo) (*types.TransactionReceipt, error) {
	if len(positions) == 0 {
		return nil, fmt.Errorf("no positions to redeem")
	}

	proxyTxns := make([]map[string]any, 0, len(positions))
	for i, pos := range positions {
		if err := pos.ConditionID.Validate(); err != nil {
			return nil, fmt.Errorf("position %d conditionID: %w", i, err)
		}
		if pos.OutcomeIndex != 0 && pos.OutcomeIndex != 1 {
			return nil, fmt.Errorf("position %d: OutcomeIndex must be 0 (YES) or 1 (NO), got %d", i, pos.OutcomeIndex)
		}

		var to common.Address
		var data []byte
		var err error
		if pos.NegRisk {
			if pos.Size <= 0 {
				return nil, fmt.Errorf("position %d: NegRisk redeem requires positive Size", i)
			}
			intSize, err2 := toUnits6(pos.Size)
			if err2 != nil {
				return nil, fmt.Errorf("position %d: %w", i, err2)
			}
			amounts := []*big.Int{big.NewInt(0), big.NewInt(0)}
			amounts[pos.OutcomeIndex] = intSize
			to = common.HexToAddress(internal.PolygonNegRiskAdapter)
			data, err = encodeRedeemNegRiskShort(pos.ConditionID, amounts)
		} else {
			to = common.HexToAddress(internal.PolygonConditionalTokens)
			indexSet := new(big.Int).Lsh(big.NewInt(1), uint(pos.OutcomeIndex))
			data, err = encodeRedeemRegularV2(pos.ConditionID, []*big.Int{indexSet})
		}
		if err != nil {
			return nil, fmt.Errorf("encode redeem V2 position %d: %w", i, err)
		}
		proxyTxns = append(proxyTxns, callTxn(to, data))
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

// encodeRedeemNegRiskShort 编码 NegRiskAdapter.redeemPositions(bytes32,uint256[])
// selector=0xdbeccb23，amounts 长度=2：amounts[winningOutcomeIndex]=size，另一边为 0。
func encodeRedeemNegRiskShort(conditionID types.Keccak256, amounts []*big.Int) ([]byte, error) {
	selector, err := hex.DecodeString("dbeccb23")
	if err != nil {
		return nil, fmt.Errorf("decode selector: %w", err)
	}
	condHash := common.HexToHash(string(conditionID))

	offset := big.NewInt(0x40)
	offsetBytes := make([]byte, 32)
	offset.FillBytes(offsetBytes)

	arrayLen := big.NewInt(int64(len(amounts)))
	lenBytes := make([]byte, 32)
	arrayLen.FillBytes(lenBytes)

	data := append([]byte{}, selector...)
	data = append(data, condHash.Bytes()...)
	data = append(data, offsetBytes...)
	data = append(data, lenBytes...)
	for _, a := range amounts {
		b := make([]byte, 32)
		a.FillBytes(b)
		data = append(data, b...)
	}
	return data, nil
}

// encodeRedeemRegularV2 用 CTF 长签名 redeemPositions(address,bytes32,bytes32,uint256[]) 编码。
// 直接调底层 ConditionalTokens（不经 adapter），collateralToken=USDC.e。
// indexSets 通常只放一个元素 [1<<outcomeIndex]，对齐官方 builder example。
func encodeRedeemRegularV2(conditionID types.Keccak256, indexSets []*big.Int) ([]byte, error) {
	selector := crypto.Keccak256([]byte("redeemPositions(address,bytes32,bytes32,uint256[])"))[:4]

	usdc := common.HexToAddress(internal.PolygonCollateral)
	hashZero := common.HexToHash(internal.HashZero)
	condHash := common.HexToHash(string(conditionID))

	data := append([]byte{}, selector...)

	collateralBytes := make([]byte, 32)
	copy(collateralBytes[12:], usdc.Bytes())
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
