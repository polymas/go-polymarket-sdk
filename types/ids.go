package types

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// ConditionID 是 Polymarket condition_id 的紧凑、可比较表示。
//
// 官方接口使用 bytes32 的规范字符串：0x + 64 位十六进制字符。
// String 始终返回小写的规范形式，适合作为 map key 与接口字符串互转。
type ConditionID [32]byte

// ParseConditionID 将 bytes32 十六进制字符串解析为 ConditionID。
// 为兼容现有 Keccak256 的使用习惯，0x 前缀可省略。
func ParseConditionID(value string) (ConditionID, error) {
	var id ConditionID
	hexValue := value
	if strings.HasPrefix(hexValue, "0x") || strings.HasPrefix(hexValue, "0X") {
		hexValue = hexValue[2:]
	}
	if len(hexValue) != hex.EncodedLen(len(id)) {
		return id, fmt.Errorf("%w: expected 32-byte hex string", ErrInvalidConditionID)
	}
	if _, err := hex.Decode(id[:], []byte(hexValue)); err != nil {
		return ConditionID{}, fmt.Errorf("%w: %v", ErrInvalidConditionID, err)
	}
	return id, nil
}

// String 返回官方接口使用的小写 bytes32 字符串。
func (id ConditionID) String() string {
	return "0x" + hex.EncodeToString(id[:])
}

// MarshalText 支持 JSON 字符串、map key 及其他文本编码。
func (id ConditionID) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

// UnmarshalText 从官方 bytes32 字符串解析 ConditionID。
func (id *ConditionID) UnmarshalText(text []byte) error {
	parsed, err := ParseConditionID(string(text))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// TokenID 是 Polymarket token_id/asset_id（uint256）的紧凑、可比较表示。
//
// 官方接口使用十进制字符串。String 始终返回无前导零的规范十进制形式。
type TokenID [32]byte

// ParseTokenID 将无符号 uint256 十进制字符串解析为 TokenID。
func ParseTokenID(value string) (TokenID, error) {
	var id TokenID
	if value == "" {
		return id, fmt.Errorf("%w: empty value", ErrInvalidTokenID)
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return id, fmt.Errorf("%w: expected unsigned decimal uint256", ErrInvalidTokenID)
		}
	}

	n, ok := new(big.Int).SetString(value, 10)
	if !ok || n.BitLen() > len(id)*8 {
		return id, fmt.Errorf("%w: value exceeds uint256", ErrInvalidTokenID)
	}
	n.FillBytes(id[:])
	return id, nil
}

// String 返回官方接口使用的规范 uint256 十进制字符串。
func (id TokenID) String() string {
	return new(big.Int).SetBytes(id[:]).String()
}

// MarshalText 支持 JSON 字符串、map key 及其他文本编码。
func (id TokenID) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

// UnmarshalText 从官方 uint256 十进制字符串解析 TokenID。
func (id *TokenID) UnmarshalText(text []byte) error {
	parsed, err := ParseTokenID(string(text))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}
