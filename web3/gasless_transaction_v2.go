package web3

import (
	"fmt"
	"math"
	"math/big"
	"strconv"

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

	data, err := c.encodeSplit(conditionID, intAmount) // CTF 长签名，collateralToken=USDC.e
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
// 2026-05 修正：之前版本以为 NegRisk 必须绕开 NegRiskAdapter 是 relayer 白名单
// 问题，实测发现是 SDK 还在用已弃用的 POLY_BUILDER_* HMAC 协议被 relayer 401。
// 切到 V2 (RELAYER_API_KEY) 协议后，NegRisk 重新走官方推荐路径：
//
//	Regular → CtfCollateralAdapter.redeemPositions(USDC.e, 0, conditionId, [1<<idx])
//	          —— 长签名（4 参数），与底层 CTF 同 selector
//
//	NegRisk → NegRiskCtfCollateralAdapter.redeemPositions(conditionId, amounts[])
//	          —— **短签名（2 参数）**，amounts 数组按 OutcomeIndex 排好；
//	          adapter 内部 NegRiskAdapter→unwrap WCol→wrap pUSD→给 Safe 一气呵成。
//	          原 v1.10.13 试图直调 WCol.unwrap 注定 revert（OnlyOwner=NegRiskAdapter）。
//
// 字段说明：
//   - OutcomeIndex：胜出的 outcome 下标（YES=0 / NO=1，多结果 NegRisk 市场可 ≥2）
//   - Size：押在 OutcomeIndex 上的位置 token 数量（6 decimals 人类单位）；
//     必填 — Regular 也要求 > 0 以保证链上余额一致（否则 ERC1155 InsufficientBalance）
type RedeemPositionInfo struct {
	ConditionID  types.Keccak256
	OutcomeIndex int
	Size         float64
	NegRisk      bool
}

// RedeemPositions 在一个 gasless batch 里对多个 position 做 redeem。
// 结算后才能拿到 collateral；未结算时对 conditionId 调用会 revert。
//
// 2026-05-29 修正历史：之前以为 gasless relayer 硬封 NegRisk redeem（提交秒拒
// STATE_FAILED）。真实根因是 **adapter 地址过时**——Polymarket 在 2026-04 之后把 V2
// adapter 重新部署到新 vanity 地址（见 internal.PolygonNegRiskCtfCollateralAdapter 注释），
// relayer 白名单随官方 ts-sdk/py-sdk 切到新地址，旧地址链上还活着但不再被放行。
// 切到新地址后 NegRisk redeem 可正常走 gasless（官方两个 SDK 即如此）。
//
// 与官方 ts-sdk/py-sdk 对齐的口径：
//   - to：Regular→collateralAdapter，NegRisk→negRiskCollateralAdapter（均为新地址）
//   - calldata：redeemPositions(pUSD, 0, conditionId, [1,2]) 长签名 → adapter 内部
//     redeem + wrap → Safe 收 **pUSD**
//   - 前置 CTF.setApprovalForAll(adapter,true)（adapter 要 burn Safe 的 position token）
func (c *GaslessClient) RedeemPositions(positions []RedeemPositionInfo) (*types.TransactionReceipt, error) {
	if len(positions) == 0 {
		return nil, fmt.Errorf("no positions to redeem")
	}

	// 官方 ts-sdk/py-sdk 现行口径（V2）：redeem 走 collateralAdapter（Regular）/
	// negRiskCollateralAdapter（NegRisk），collateralToken 一律填 pUSD，indexSets=[1,2]
	// （二元市场两 outcome 一起，未持有的那个余额 0、redeem 0），长签名
	// redeemPositions(address,bytes32,bytes32,uint256[])。adapter 内部 → CTF redeem →
	// wrap pUSD → 回 Safe。非 EOA 钱包全程走 gasless relayer。
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	pusd := common.HexToAddress(internal.PolygonPUSD)
	collAdapter := common.HexToAddress(internal.PolygonCtfCollateralAdapter)
	negRiskAdapter := common.HexToAddress(internal.PolygonNegRiskCtfCollateralAdapter)
	indexSets := []*big.Int{big.NewInt(1), big.NewInt(2)}

	for i, pos := range positions {
		if err := pos.ConditionID.Validate(); err != nil {
			return nil, fmt.Errorf("position %d conditionID: %w", i, err)
		}
		if pos.OutcomeIndex < 0 {
			return nil, fmt.Errorf("position %d: OutcomeIndex must be >= 0, got %d", i, pos.OutcomeIndex)
		}
		if !pos.NegRisk && pos.OutcomeIndex > 1 {
			return nil, fmt.Errorf("position %d: Regular 二元市场 OutcomeIndex 只能 0 或 1，got %d", i, pos.OutcomeIndex)
		}
		if pos.Size <= 0 {
			return nil, fmt.Errorf("position %d: redeem requires positive Size", i)
		}
	}

	// adapter 要 burn Safe 持有的 CTF position token，需 CTF.setApprovalForAll(adapter,true)。
	// 把用到的 adapter 去重，各前置一笔（幂等：已 true 时链上 no-op），让 redeem 自洽。
	adapterUsed := map[common.Address]bool{}
	for _, pos := range positions {
		if pos.NegRisk {
			adapterUsed[negRiskAdapter] = true
		} else {
			adapterUsed[collAdapter] = true
		}
	}
	proxyTxns := make([]map[string]any, 0, len(adapterUsed)+len(positions))
	for adapter := range adapterUsed {
		apprData, err := packSetApprovalForAllWithFlag(adapter, true)
		if err != nil {
			return nil, fmt.Errorf("pack setApprovalForAll %s: %w", adapter.Hex(), err)
		}
		proxyTxns = append(proxyTxns, callTxn(ctf, apprData))
	}

	for i, pos := range positions {
		to := collAdapter
		if pos.NegRisk {
			to = negRiskAdapter
		}
		data, err := encodeRedeemCTF(pusd, pos.ConditionID, indexSets)
		if err != nil {
			return nil, fmt.Errorf("encode redeem for position %d: %w", i, err)
		}
		proxyTxns = append(proxyTxns, callTxn(to, data))
	}

	return c.executeGaslessBatch(proxyTxns, "Redeem Positions V2", "redeem-v2")
}

// toUnits6 把 float 金额按其十进制表示精确转换成 6 decimals uint256。
// 不允许静默截断超过 6 位的小数，也拒绝非有限值及 uint256 溢出。
func toUnits6(amount float64) (*big.Int, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
		return nil, fmt.Errorf("amount must be positive, got %f", amount)
	}

	// FormatFloat(..., 'f', -1, 64) 保留能往返 float64 的最短十进制表示，
	// 再由 big.Rat 做精确十进制运算，避免 1.000001*1e6 因二进制浮点
	// 变成 1000000.999999... 后被错误截断。
	decimal := strconv.FormatFloat(amount, 'f', -1, 64)
	amountRat, ok := new(big.Rat).SetString(decimal)
	if !ok {
		return nil, fmt.Errorf("invalid amount %q", decimal)
	}
	scaled := new(big.Rat).Mul(amountRat, big.NewRat(1_000_000, 1))
	if !scaled.IsInt() {
		return nil, fmt.Errorf("amount supports at most 6 decimal places, got %s", decimal)
	}

	units := new(big.Int).Set(scaled.Num())
	if units.Sign() <= 0 {
		return nil, fmt.Errorf("amount must be at least 0.000001, got %s", decimal)
	}
	if units.BitLen() > 256 {
		return nil, fmt.Errorf("amount exceeds uint256: %s", decimal)
	}
	return units, nil
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
