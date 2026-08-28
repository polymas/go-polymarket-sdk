package signing

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/crypto"
)

// V2 Exchange EIP-712 常量。仅定义，未接线 —— Phase 2 再实现 BuildSignedOrderV2。
// 参考：https://docs.polymarket.com/v2-migration
//
// 关键差异（相对 V1）：
//   - Exchange domain version "1" -> "2"（ClobAuthDomain 保持 "1"，不影响 L1 鉴权）
//   - Order 结构移除 taker / expiration / nonce
//   - Order 结构新增 timestamp(uint256 毫秒) / metadata(bytes32) / builder(bytes32)

var (
	V2ExchangeProtocolName    = crypto.Keccak256Hash([]byte("Polymarket CTF Exchange"))
	V2ExchangeProtocolVersion = crypto.Keccak256Hash([]byte("2"))

	// V2OrderTypeString 是 V2 Order 的 EIP-712 类型字符串。注意：solady ERC-1271
	// 嵌套签名（POLY_1271 路径）需要把这个字符串原样附在最终签名末尾，所以
	// 单独保留为变量，不要内联。
	V2OrderTypeString = "Order(uint256 salt,address maker,address signer,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint8 side,uint8 signatureType,uint256 timestamp,bytes32 metadata,bytes32 builder)"

	V2OrderTypehash = crypto.Keccak256Hash([]byte(V2OrderTypeString))

	// solady TypedDataSign 类型字符串（POLY_1271 嵌套签名专用）。
	// 来源：py-clob-client-v2 exchange_order_builder_v2.SOLADY_TYPE_STRING
	V2SoladyTypeString = "TypedDataSign(Order contents,string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)" + V2OrderTypeString
	V2SoladyTypeHash   = crypto.Keccak256Hash([]byte(V2SoladyTypeString))

	// DepositWallet 域参数（POLY_1271 内层 domain，verifyingContract = wallet）
	V2DepositWalletNameHash    = crypto.Keccak256Hash([]byte("DepositWallet"))
	V2DepositWalletVersionHash = crypto.Keccak256Hash([]byte("1"))
)

// V2OrderABIStruct 与 V2OrderTypehash 的字段顺序严格对齐，供 EIP-712 ABI 编码使用。
var V2OrderABIStruct = []abi.Type{
	v2Bytes32Type, // typehash
	v2Uint256Type, // salt
	v2AddressType, // maker
	v2AddressType, // signer
	v2Uint256Type, // tokenId
	v2Uint256Type, // makerAmount
	v2Uint256Type, // takerAmount
	v2Uint8Type,   // side
	v2Uint8Type,   // signatureType
	v2Uint256Type, // timestamp
	v2Bytes32Type, // metadata
	v2Bytes32Type, // builder
}

var (
	v2Bytes32Type, _ = abi.NewType("bytes32", "", nil)
	v2Uint256Type, _ = abi.NewType("uint256", "", nil)
	v2AddressType, _ = abi.NewType("address", "", nil)
	v2Uint8Type, _   = abi.NewType("uint8", "", nil)
)
