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

// RedeemPositionInfo represents a single position to redeem
type RedeemPositionInfo struct {
	ConditionID types.Keccak256
	Amounts     []float64
	NegRisk     bool
}

// RedeemPositions redeems multiple positions into USDC in a single batch transaction
func (c *GaslessClient) RedeemPositions(
	positions []RedeemPositionInfo,
) (*types.TransactionReceipt, error) {
	if len(positions) == 0 {
		return nil, fmt.Errorf("no positions to redeem")
	}

	// Build proxy transactions for each position
	proxyTxns := make([]map[string]interface{}, 0, len(positions))

	for i, pos := range positions {
		// Validate amounts
		if len(pos.Amounts) == 0 {
			return nil, fmt.Errorf("position %d: amounts array is empty", i)
		}

		// For Negative Risk markets, validate amounts according to Python implementation:
		// - WCOL Aggregator allows amounts array with zeros (at least one must be > 0)
		// - This matches Python's group_wcol_amounts which allows amount0 or amount1 to be 0
		if pos.NegRisk {
			hasPositiveAmount := false
			for j, amount := range pos.Amounts {
				if amount < 0 {
					return nil, fmt.Errorf("position %d: Negative Risk market amounts cannot be negative, but amounts[%d]=%f", i, j, amount)
				}
				if amount > 0 {
					hasPositiveAmount = true
				}
			}
			if !hasPositiveAmount {
				return nil, fmt.Errorf("position %d: Negative Risk market requires at least one positive amount", i)
			}
		}

		// Convert amounts to int (multiply by 1e6) - 使用big.Int避免精度损失
		intAmounts := make([]*big.Int, len(pos.Amounts))
		for j, amount := range pos.Amounts {
			// 使用big.Float进行精确计算，避免int64溢出和精度损失
			amountFloat := big.NewFloat(amount)
			multiplier := big.NewFloat(1e6)
			result := new(big.Float).Mul(amountFloat, multiplier)
			intAmount, _ := result.Int(nil)
			if intAmount.Sign() < 0 {
				return nil, fmt.Errorf("position %d: amount[%d] resulted in negative value: %f", i, j, amount)
			}
			intAmounts[j] = intAmount
		}

		// Determine contract address and encode data
		var to common.Address
		var data []byte
		var err error

		if pos.NegRisk {
			// Use neg risk adapter
			to = common.HexToAddress(internal.PolygonNegRiskAdapter)
			data, err = c.encodeRedeemNegRisk(pos.ConditionID, intAmounts)
		} else {
			// Use conditional tokens
			to = common.HexToAddress(internal.PolygonConditionalTokens)
			data, err = c.encodeRedeem(pos.ConditionID)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to encode redeem for position %d (conditionID: %s, negRisk: %v): %w",
				i, string(pos.ConditionID), pos.NegRisk, err)
		}

		// Add to proxy transactions
		proxyTxns = append(proxyTxns, map[string]interface{}{
			"typeCode": 1,
			"to":       to.Hex(),
			"value":    0,
			"data":     "0x" + hex.EncodeToString(data),
		})
	}

	// Execute batch transaction via gasless relay
	return c.executeGaslessBatch(proxyTxns, "Redeem Positions", "redeem")
}

// encodeRedeem encodes redeem positions transaction for regular markets
func (c *GaslessClient) encodeRedeem(conditionID types.Keccak256) ([]byte, error) {
	usdcAddr := common.HexToAddress(internal.PolygonCollateral)
	hashZero := common.HexToHash(internal.HashZero)
	indexSets := []*big.Int{big.NewInt(1), big.NewInt(2)}

	// redeemPositions(address collateralToken, bytes32 parentCollectionId, bytes32 conditionId, uint256[] indexSets)
	return c.conditionalABI.Pack("redeemPositions", usdcAddr, hashZero, common.HexToHash(string(conditionID)), indexSets)
}

// encodeRedeemNegRisk encodes redeem positions transaction for neg risk markets
// According to Python implementation, Negative Risk markets use WCOL Aggregator
// with selector 0xdbeccb23, not the redeemPositions function
// Function signature: function(bytes32 conditionId, uint256[] amounts)
// Data format: selector(4) + conditionId(32) + offset(32) + arrayLength(32) + amounts...
func (c *GaslessClient) encodeRedeemNegRisk(conditionID types.Keccak256, amounts []*big.Int) ([]byte, error) {
	// Use manual encoding to match Python's build_wcol_call_data format
	// Python: selector + encode_bytes32(conditionId) + encode_u256(0x40) + encode_u256(2) + encode_u256(amount0) + encode_u256(amount1)

	selector, err := hex.DecodeString("dbeccb23")
	if err != nil {
		return nil, fmt.Errorf("failed to decode selector: %w", err)
	}

	// Encode conditionId (32 bytes)
	conditionHash := common.HexToHash(string(conditionID))
	conditionBytes := conditionHash.Bytes()

	// Encode offset 0x40 (32 bytes, big-endian)
	offset := big.NewInt(0x40)
	offsetBytes := make([]byte, 32)
	offset.FillBytes(offsetBytes)

	// Encode array length (32 bytes, big-endian)
	arrayLen := big.NewInt(int64(len(amounts)))
	arrayLenBytes := make([]byte, 32)
	arrayLen.FillBytes(arrayLenBytes)

	// Encode amounts (each 32 bytes, big-endian)
	amountsBytes := make([]byte, 0, len(amounts)*32)
	for _, amount := range amounts {
		amountBytes := make([]byte, 32)
		amount.FillBytes(amountBytes)
		amountsBytes = append(amountsBytes, amountBytes...)
	}

	// Combine all parts
	data := append(selector, conditionBytes...)
	data = append(data, offsetBytes...)
	data = append(data, arrayLenBytes...)
	data = append(data, amountsBytes...)

	return data, nil
}

// SplitUSDC splits USDC into outcome tokens
func (c *GaslessClient) SplitUSDC(amount float64, conditionID types.Keccak256, negRisk bool) (*types.TransactionReceipt, error) {
	// Convert amount to int (multiply by 1e6)
	amountFloat := big.NewFloat(amount)
	multiplier := big.NewFloat(1e6)
	result := new(big.Float).Mul(amountFloat, multiplier)
	intAmount, _ := result.Int(nil)

	if intAmount.Sign() <= 0 {
		return nil, fmt.Errorf("amount must be positive, got: %f", amount)
	}

	// Determine contract addresses
	var ctfContract common.Address

	if negRisk {
		ctfContract = common.HexToAddress(internal.PolygonNegRiskAdapter)
	} else {
		ctfContract = common.HexToAddress(internal.PolygonConditionalTokens)
	}

	// Execute split
	var splitData []byte
	var err error
	if negRisk {
		splitData, err = c.encodeSplitNegRisk(conditionID, intAmount)
	} else {
		splitData, err = c.encodeSplit(conditionID, intAmount)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to encode split: %w", err)
	}

	proxyTxns := []map[string]interface{}{
		{
			"typeCode": 1,
			"to":       ctfContract.Hex(),
			"value":    0,
			"data":     "0x" + hex.EncodeToString(splitData),
		},
	}

	return c.executeGaslessBatch(proxyTxns, "Split USDC", "split")
}

// MergeTokens merges outcome tokens back into USDC
func (c *GaslessClient) MergeTokens(conditionID types.Keccak256, amount float64, negRisk bool) (*types.TransactionReceipt, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive, got: %f", amount)
	}

	// Convert amount to int (multiply by 1e6)
	amountFloat := big.NewFloat(amount)
	multiplier := big.NewFloat(1e6)
	result := new(big.Float).Mul(amountFloat, multiplier)
	intAmount, _ := result.Int(nil)

	if intAmount.Sign() <= 0 {
		return nil, fmt.Errorf("amount must be positive, got: %f", amount)
	}

	var to common.Address
	var data []byte
	var err error

	if negRisk {
		// Use neg risk adapter for merge
		to = common.HexToAddress(internal.PolygonNegRiskAdapter)
		data, err = c.encodeMergeNegRisk(conditionID, intAmount)
	} else {
		// Use conditional tokens
		to = common.HexToAddress(internal.PolygonConditionalTokens)
		data, err = c.encodeMerge(conditionID, intAmount)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to encode merge: %w", err)
	}

	proxyTxns := []map[string]interface{}{
		{
			"typeCode": 1,
			"to":       to.Hex(),
			"value":    0,
			"data":     "0x" + hex.EncodeToString(data),
		},
	}

	return c.executeGaslessBatch(proxyTxns, "Merge Tokens", "merge")
}

// encodeSplit encodes split USDC transaction for regular markets
// Manual encoding to match Rust implementation order:
// 1. selector
// 2. collateralToken (address, 32 bytes padded)
// 3. parentCollectionId (bytes32)
// 4. conditionId (bytes32)
// 5. partition offset (uint256) = 0xa0
// 6. amount (uint256) - comes after partition offset (non-standard ABI order)
// 7. partition length (uint256)
// 8. partition[0] (uint256)
// 9. partition[1] (uint256)
func (c *GaslessClient) encodeSplit(conditionID types.Keccak256, amount *big.Int) ([]byte, error) {
	usdcAddr := common.HexToAddress(internal.PolygonCollateral)
	hashZero := common.HexToHash(internal.HashZero)
	partition := []*big.Int{big.NewInt(1), big.NewInt(2)} // Partition [1, 2] for binary markets (YES|NO)

	// Function selector: splitPosition(address,bytes32,bytes32,uint256[],uint256)
	// Calculate selector using keccak256 hash of function signature
	// Matching Rust: keccak256(b"splitPosition(address,bytes32,bytes32,uint256[],uint256)")[0:4]
	funcSig := []byte("splitPosition(address,bytes32,bytes32,uint256[],uint256)")
	hash := crypto.Keccak256(funcSig)
	selector := hash[:4]

	var data []byte
	data = append(data, selector...)

	// Encode collateralToken (address, 32 bytes padded)
	collateralBytes := make([]byte, 32)
	copy(collateralBytes[12:], usdcAddr.Bytes())
	data = append(data, collateralBytes...)

	// Encode parentCollectionId (bytes32)
	hashZeroBytes := hashZero.Bytes()
	data = append(data, hashZeroBytes...)

	// Encode conditionId (bytes32)
	conditionHash := common.HexToHash(string(conditionID))
	conditionBytes := conditionHash.Bytes()
	data = append(data, conditionBytes...)

	// Encode partition array offset (32 bytes) = 0xa0
	// 0x04 (selector) + 0x20*5 (5 params before array) = 0xa0
	arrayOffset := big.NewInt(0xa0)
	offsetBytes := make([]byte, 32)
	arrayOffset.FillBytes(offsetBytes)
	data = append(data, offsetBytes...)

	// Encode amount (uint256, 32 bytes) - comes after partition offset (matching Rust)
	amountBytes := make([]byte, 32)
	amount.FillBytes(amountBytes)
	data = append(data, amountBytes...)

	// Encode partition array length (32 bytes)
	arrayLen := big.NewInt(int64(len(partition)))
	lenBytes := make([]byte, 32)
	arrayLen.FillBytes(lenBytes)
	data = append(data, lenBytes...)

	// Encode partition array elements (each 32 bytes)
	for _, elem := range partition {
		elemBytes := make([]byte, 32)
		elem.FillBytes(elemBytes)
		data = append(data, elemBytes...)
	}

	return data, nil
}

// encodeSplitNegRisk encodes split USDC transaction for neg risk markets
func (c *GaslessClient) encodeSplitNegRisk(conditionID types.Keccak256, amount *big.Int) ([]byte, error) {
	// Neg risk adapter uses a different function signature
	// Function signature: function(bytes32 conditionId, uint256 amount)
	selector, err := hex.DecodeString("a8c4e5c4") // splitPosition selector for neg risk adapter
	if err != nil {
		return nil, fmt.Errorf("failed to decode selector: %w", err)
	}

	conditionHash := common.HexToHash(string(conditionID))
	conditionBytes := conditionHash.Bytes()

	amountBytes := make([]byte, 32)
	amount.FillBytes(amountBytes)

	data := append(selector, conditionBytes...)
	data = append(data, amountBytes...)

	return data, nil
}

// encodeMerge encodes merge tokens transaction for regular markets
// Manual encoding to match Rust implementation order (same as split):
// 1. selector
// 2. collateralToken (address, 32 bytes padded)
// 3. parentCollectionId (bytes32)
// 4. conditionId (bytes32)
// 5. partition offset (uint256) = 0xa0
// 6. amount (uint256) - comes after partition offset (non-standard ABI order)
// 7. partition length (uint256)
// 8. partition[0] (uint256)
// 9. partition[1] (uint256)
func (c *GaslessClient) encodeMerge(conditionID types.Keccak256, amount *big.Int) ([]byte, error) {
	usdcAddr := common.HexToAddress(internal.PolygonCollateral)
	hashZero := common.HexToHash(internal.HashZero)
	partition := []*big.Int{big.NewInt(1), big.NewInt(2)} // Partition [1, 2] for binary markets (YES|NO)

	// Function selector: mergePositions(address,bytes32,bytes32,uint256[],uint256)
	// Calculate selector using keccak256 hash of function signature
	// Matching Rust: keccak256(b"mergePositions(address,bytes32,bytes32,uint256[],uint256)")[0:4]
	funcSig := []byte("mergePositions(address,bytes32,bytes32,uint256[],uint256)")
	hash := crypto.Keccak256(funcSig)
	selector := hash[:4]

	var data []byte
	data = append(data, selector...)

	// Encode collateralToken (address, 32 bytes padded)
	collateralBytes := make([]byte, 32)
	copy(collateralBytes[12:], usdcAddr.Bytes())
	data = append(data, collateralBytes...)

	// Encode parentCollectionId (bytes32)
	hashZeroBytes := hashZero.Bytes()
	data = append(data, hashZeroBytes...)

	// Encode conditionId (bytes32)
	conditionHash := common.HexToHash(string(conditionID))
	conditionBytes := conditionHash.Bytes()
	data = append(data, conditionBytes...)

	// Encode partition array offset (32 bytes) = 0xa0
	arrayOffset := big.NewInt(0xa0)
	offsetBytes := make([]byte, 32)
	arrayOffset.FillBytes(offsetBytes)
	data = append(data, offsetBytes...)

	// Encode amount (uint256, 32 bytes) - comes after partition offset (matching Rust)
	amountBytes := make([]byte, 32)
	amount.FillBytes(amountBytes)
	data = append(data, amountBytes...)

	// Encode partition array length (32 bytes)
	arrayLen := big.NewInt(int64(len(partition)))
	lenBytes := make([]byte, 32)
	arrayLen.FillBytes(lenBytes)
	data = append(data, lenBytes...)

	// Encode partition array elements (each 32 bytes)
	for _, elem := range partition {
		elemBytes := make([]byte, 32)
		elem.FillBytes(elemBytes)
		data = append(data, elemBytes...)
	}

	return data, nil
}

// encodeMergeNegRisk encodes merge tokens transaction for neg risk markets
// Function signature: mergePositions(bytes32 _conditionId, uint256 _amount)
func (c *GaslessClient) encodeMergeNegRisk(conditionID types.Keccak256, amount *big.Int) ([]byte, error) {
	// Function selector: mergePositions(bytes32,uint256)
	// keccak256("mergePositions(bytes32,uint256)")[0:4]
	selector, err := hex.DecodeString("c5b2b0c4") // mergePositions selector for neg risk adapter
	if err != nil {
		return nil, fmt.Errorf("failed to decode selector: %w", err)
	}

	conditionHash := common.HexToHash(string(conditionID))
	conditionBytes := conditionHash.Bytes()

	amountBytes := make([]byte, 32)
	amount.FillBytes(amountBytes)

	data := append(selector, conditionBytes...)
	data = append(data, amountBytes...)

	return data, nil
}
