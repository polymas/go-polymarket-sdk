package web3

import (
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
// NegRisk 同 conditionId 的多个 outcome 会合并成一个 adapter.redeemPositions 调用
// （amounts 按 OutcomeIndex 排位，缺位补 0），Regular 每个 outcome 一个 op。
func (c *GaslessClient) RedeemPositions(positions []RedeemPositionInfo) (*types.TransactionReceipt, error) {
	if len(positions) == 0 {
		return nil, fmt.Errorf("no positions to redeem")
	}

	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	// NegRisk redeem 走 legacy NegRiskAdapter（USDC.e 出口），不走
	// NegRiskCtfCollateralAdapter（pUSD 出口）。原因：CWIA wallet 通常已有
	// CTF.setApprovalForAll(NegRiskAdapter,true) 但缺 setApprovalForAll(NegRiskCtfCollateralAdapter,
	// true)；后者补不了 — DepositWalletFactory.proxy() 有 onlyOperator 修饰，
	// EOA 无法旁路 relayer，而 relayer 又把 approve adapter 列入 allowlist 拒绝。
	// 代价：wallet 收 USDC.e 而非 pUSD，调用方可自行后续 wrap。
	nrAdapter := common.HexToAddress(internal.PolygonNegRiskAdapter)
	usdcE := common.HexToAddress(internal.PolygonCollateral)

	// NegRisk 按 conditionId 分组 → 同组合并到一笔 redeemPositions(amounts[])。
	type nrGroup struct {
		amounts map[int]*big.Int // outcomeIndex → intSize
		maxIdx  int
	}
	nrGroups := map[string]*nrGroup{}
	regularOps := []map[string]any{}

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
		intSize, err := toUnits6(pos.Size)
		if err != nil {
			return nil, fmt.Errorf("position %d size: %w", i, err)
		}

		if pos.NegRisk {
			condHex := string(pos.ConditionID)
			g, ok := nrGroups[condHex]
			if !ok {
				g = &nrGroup{amounts: map[int]*big.Int{}, maxIdx: -1}
				nrGroups[condHex] = g
			}
			if existing, dup := g.amounts[pos.OutcomeIndex]; dup {
				g.amounts[pos.OutcomeIndex] = new(big.Int).Add(existing, intSize)
			} else {
				g.amounts[pos.OutcomeIndex] = intSize
			}
			if pos.OutcomeIndex > g.maxIdx {
				g.maxIdx = pos.OutcomeIndex
			}
			continue
		}

		// Regular：直调底层 CTF.redeemPositions(USDC.e, 0, conditionId, [1<<idx])，
		// 与 v1.10.13 保持一致，Safe 收到 USDC.e（不是 pUSD）。
		indexSet := new(big.Int).Lsh(big.NewInt(1), uint(pos.OutcomeIndex))
		data, err := encodeRedeemCTF(usdcE, pos.ConditionID, []*big.Int{indexSet})
		if err != nil {
			return nil, fmt.Errorf("encode Regular redeem for position %d: %w", i, err)
		}
		regularOps = append(regularOps, callTxn(ctf, data))
	}

	proxyTxns := make([]map[string]any, 0, len(regularOps)+len(nrGroups))
	proxyTxns = append(proxyTxns, regularOps...)

	for condHex, g := range nrGroups {
		amounts := make([]*big.Int, g.maxIdx+1)
		for i := range amounts {
			if a, ok := g.amounts[i]; ok {
				amounts[i] = a
			} else {
				amounts[i] = big.NewInt(0)
			}
		}
		data, err := encodeNegRiskRedeem(types.Keccak256(condHex), amounts)
		if err != nil {
			return nil, fmt.Errorf("encode NegRisk redeem for %s: %w", condHex, err)
		}
		proxyTxns = append(proxyTxns, callTxn(nrAdapter, data))
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

// encodeNegRiskRedeem 编码 NegRiskCtfCollateralAdapter.redeemPositions(bytes32 conditionId, uint256[] amounts)。
// 短签名（2 参数），与 NegRiskAdapter 同 selector。adapter 内部 NegRiskAdapter→
// unwrap WCol→wrap pUSD→给 Safe。amounts[i] = outcome i 持有数量（6 decimals）。
func encodeNegRiskRedeem(conditionID types.Keccak256, amounts []*big.Int) ([]byte, error) {
	selector := crypto.Keccak256([]byte("redeemPositions(bytes32,uint256[])"))[:4]
	condHash := common.HexToHash(string(conditionID))

	data := append([]byte{}, selector...)
	data = append(data, condHash.Bytes()...)

	// amounts 动态数组：offset = 0x40（2 个固定参数 * 0x20）
	offset := big.NewInt(0x40)
	offsetBytes := make([]byte, 32)
	offset.FillBytes(offsetBytes)
	data = append(data, offsetBytes...)

	arrayLen := big.NewInt(int64(len(amounts)))
	lenBytes := make([]byte, 32)
	arrayLen.FillBytes(lenBytes)
	data = append(data, lenBytes...)

	for _, v := range amounts {
		elemBytes := make([]byte, 32)
		v.FillBytes(elemBytes)
		data = append(data, elemBytes...)
	}

	return data, nil
}
