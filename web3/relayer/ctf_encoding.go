package relayer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// These encoders build the CTF-compatible calldata consumed by the current
// V2 collateral adapters. They are not legacy V1 transaction entry points.

// encodeSplit encodes CTF-compatible split calldata for the V2 collateral adapters.
// Manual encoding to match Rust implementation order:
// 1. selector
// 2. collateralToken (address, 32 bytes padded)
// 3. parentCollectionId (bytes32)
// 4. conditionId (bytes32)
// 5. partition offset (uint256) = 0xa0
// 6. amount (uint256) - final static head field
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

	// Encode amount (uint256, 32 bytes) as the final static head field.
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

// encodeMerge encodes CTF-compatible merge calldata for the V2 collateral adapters.
// Manual encoding to match Rust implementation order (same as split):
// 1. selector
// 2. collateralToken (address, 32 bytes padded)
// 3. parentCollectionId (bytes32)
// 4. conditionId (bytes32)
// 5. partition offset (uint256) = 0xa0
// 6. amount (uint256) - final static head field
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

	// Encode amount (uint256, 32 bytes) as the final static head field.
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
