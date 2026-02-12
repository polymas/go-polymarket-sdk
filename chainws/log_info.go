package chainws

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// EventKind 链上事件类型
type EventKind int

const (
	EventUnknown EventKind = iota
	EventTransfer
	EventApproval
	EventTransferSingle // ERC1155
)

// EventInfo 从 log 解析出的事件摘要（Transfer/Approval 通用字段）
type EventInfo struct {
	Kind         EventKind      // 事件类型
	Contract     common.Address // 发出事件的合约
	From         common.Address // 来源地址（Transfer 转出方 / Approval 所有者）
	To           common.Address // 目标地址（Transfer 接收方 / Approval 被授权方）
	Value        *big.Int       // 数量（uint256）
	BlockNumber  uint64
	TxHash       common.Hash
	LogIndex     uint
	ContractName string // 合约含义（便于展示）
}

// ContractNameByAddress 返回 Polymarket 合约地址对应的含义（主网）
func ContractNameByAddress(addr common.Address) string {
	switch addr.Hex() {
	case internal.PolygonCollateral:
		return "USDC"
	case internal.PolygonExchange:
		return "Exchange"
	case internal.PolygonNegRiskExchange:
		return "NegRiskExchange"
	case internal.PolygonConditionalTokens:
		return "ConditionalTokens"
	case internal.PolygonNegRiskAdapter:
		return "NegRiskAdapter"
	case internal.PolygonProxyFactory:
		return "ProxyFactory"
	default:
		return ""
	}
}

// EventKindOf 根据 log.Topics[0] 判断事件类型
func EventKindOf(log *ethtypes.Log) EventKind {
	if len(log.Topics) == 0 {
		return EventUnknown
	}
	switch log.Topics[0] {
	case transferEventSig:
		return EventTransfer
	case approvalEventSig:
		return EventApproval
	case transferSingleEventSig:
		return EventTransferSingle
	default:
		return EventUnknown
	}
}

// ParseTransfer 从 log 解析 Transfer(address,address,uint256)。若非 Transfer 事件返回 nil。
func ParseTransfer(log *ethtypes.Log) *EventInfo {
	if EventKindOf(log) != EventTransfer || len(log.Topics) < 3 {
		return nil
	}
	from := common.BytesToAddress(log.Topics[1].Bytes())
	to := common.BytesToAddress(log.Topics[2].Bytes())
	value := new(big.Int).SetBytes(log.Data)
	return &EventInfo{
		Kind:         EventTransfer,
		Contract:     log.Address,
		From:         from,
		To:           to,
		Value:        value,
		BlockNumber:  log.BlockNumber,
		TxHash:       log.TxHash,
		LogIndex:     log.Index,
		ContractName: ContractNameByAddress(log.Address),
	}
}

// ParseApproval 从 log 解析 Approval(address,address,uint256)。若非 Approval 事件返回 nil。
func ParseApproval(log *ethtypes.Log) *EventInfo {
	if EventKindOf(log) != EventApproval || len(log.Topics) < 3 {
		return nil
	}
	owner := common.BytesToAddress(log.Topics[1].Bytes())
	spender := common.BytesToAddress(log.Topics[2].Bytes())
	value := new(big.Int).SetBytes(log.Data)
	return &EventInfo{
		Kind:         EventApproval,
		Contract:     log.Address,
		From:         owner,
		To:           spender,
		Value:        value,
		BlockNumber:  log.BlockNumber,
		TxHash:       log.TxHash,
		LogIndex:     log.Index,
		ContractName: ContractNameByAddress(log.Address),
	}
}

// TransferSingleInfo ERC1155 TransferSingle 解析结果
type TransferSingleInfo struct {
	Operator     common.Address
	From         common.Address
	To           common.Address
	TokenID      *big.Int
	Value        *big.Int
	BlockNumber  uint64
	TxHash       common.Hash
	LogIndex     uint
	ContractName string
}

// ParseTransferSingle 从 log 解析 ERC1155 TransferSingle(operator,from,to,id,value)。
// Topics: topic1=operator, topic2=from, topic3=to；Data: id(32) + value(32)。
func ParseTransferSingle(log *ethtypes.Log) *TransferSingleInfo {
	if EventKindOf(log) != EventTransferSingle || len(log.Topics) < 4 {
		return nil
	}
	if len(log.Data) < 64 {
		return nil
	}
	operator := common.BytesToAddress(log.Topics[1].Bytes())
	from := common.BytesToAddress(log.Topics[2].Bytes())
	to := common.BytesToAddress(log.Topics[3].Bytes())
	tokenID := new(big.Int).SetBytes(log.Data[0:32])
	value := new(big.Int).SetBytes(log.Data[32:64])
	return &TransferSingleInfo{
		Operator:     operator,
		From:         from,
		To:           to,
		TokenID:      tokenID,
		Value:        value,
		BlockNumber:  log.BlockNumber,
		TxHash:       log.TxHash,
		LogIndex:     log.Index,
		ContractName: ContractNameByAddress(log.Address),
	}
}

// ParseEvent 根据 log 类型解析为 EventInfo（Transfer 或 Approval），否则返回 nil。
func ParseEvent(log *ethtypes.Log) *EventInfo {
	if info := ParseTransfer(log); info != nil {
		return info
	}
	return ParseApproval(log)
}

// FromAddress 返回 from 的 SDK 类型
func (e *EventInfo) FromAddress() types.EthAddress { return types.EthAddress(e.From.Hex()) }

// ToAddress 返回 to 的 SDK 类型
func (e *EventInfo) ToAddress() types.EthAddress { return types.EthAddress(e.To.Hex()) }
