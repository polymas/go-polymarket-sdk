package types

import (
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

// EthAddress 表示以太坊地址（0x + 40个十六进制字符）
type EthAddress string

// Validate 验证以太坊地址格式
func (e EthAddress) Validate() error {
	addr := string(e)
	if !strings.HasPrefix(addr, "0x") {
		addr = "0x" + addr
	}
	matched, _ := regexp.MatchString(`^0x[a-fA-F0-9]{40}$`, addr)
	if !matched {
		return ErrInvalidEthAddress
	}
	return nil
}

// String 返回地址的字符串形式
func (e EthAddress) String() string {
	addr := string(e)
	if !strings.HasPrefix(addr, "0x") {
		return "0x" + addr
	}
	return addr
}

// Keccak256 表示Keccak256哈希（0x + 64个十六进制字符）
type Keccak256 string

// Validate 验证Keccak256哈希格式
func (k Keccak256) Validate() error {
	hash := string(k)
	if !strings.HasPrefix(hash, "0x") {
		hash = "0x" + hash
	}
	matched, _ := regexp.MatchString(`^0x[a-fA-F0-9]{64}$`, hash)
	if !matched {
		return ErrInvalidKeccak256
	}
	return nil
}

// String 返回哈希的字符串形式
func (k Keccak256) String() string {
	hash := string(k)
	if !strings.HasPrefix(hash, "0x") {
		return "0x" + hash
	}
	return hash
}

// HexString 表示带0x前缀的十六进制字符串
type HexString string

// Bytes 将十六进制字符串转换为字节
func (h HexString) Bytes() ([]byte, error) {
	s := string(h)
	s = strings.TrimPrefix(s, "0x")
	return hex.DecodeString(s)
}

// TimeseriesPoint 表示时间序列中的一个点
type TimeseriesPoint struct {
	Value     float64   `json:"p"`
	Timestamp time.Time `json:"t"`
}

// ChainID 表示区块链链ID
type ChainID int

const (
	// Polygon mainnet
	Polygon ChainID = 137
	// Amoy testnet
	Amoy ChainID = 80002
)

// SignatureType 表示钱包签名类型
type SignatureType int

const (
	// EOASignatureType - EOA wallet
	EOASignatureType SignatureType = 0
	// ProxySignatureType - Proxy wallet
	ProxySignatureType SignatureType = 1
	// SafeSignatureType - Safe/Gnosis wallet
	SafeSignatureType SignatureType = 2
	// Poly1271SignatureType is the official CLOB V2 POLY_1271 signature type.
	// It describes ERC-1271 signature verification, not a particular wallet
	// implementation. Polymarket Deposit Wallet/CWIA accounts currently use it.
	// Some CLOB OpenAPI enum locations still list only 0/1/2; the official
	// py-clob-client-v2 and current wallet APIs include type 3.
	Poly1271SignatureType SignatureType = 3

	// DepositWalletSignatureType is the wallet-oriented name for type 3.
	DepositWalletSignatureType = Poly1271SignatureType

	// Deprecated: use Poly1271SignatureType for signing semantics or
	// DepositWalletSignatureType when selecting a Polymarket wallet type.
	CWIASignatureType = Poly1271SignatureType
)

func (s SignatureType) String() string {
	switch s {
	case EOASignatureType:
		return "EOA"
	case ProxySignatureType:
		return "POLY_PROXY"
	case SafeSignatureType:
		return "POLY_GNOSIS_SAFE"
	case Poly1271SignatureType:
		return "POLY_1271"
	default:
		return "UNKNOWN"
	}
}

// ValidV2 reports whether the current CLOB V2 signing implementation supports
// this value. It deliberately includes POLY_1271 even while older OpenAPI enum
// fragments still stop at POLY_GNOSIS_SAFE.
func (s SignatureType) ValidV2() bool {
	return s >= EOASignatureType && s <= Poly1271SignatureType
}

func (s SignatureType) UsesContractSignature() bool {
	return s == Poly1271SignatureType
}

// BookSnapshot 表示订单簿快照
type BookSnapshot struct {
	BestBid *BookSide
	BestAsk *BookSide
	TS      time.Time
}

// BookSide 表示订单簿的一侧
type BookSide struct {
	Price float64
	Size  float64
}

// Ptr 返回传入类型的指针（泛型工具函数）
func Ptr[T any](v T) *T {
	return &v
}
