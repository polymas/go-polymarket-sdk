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
)

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
