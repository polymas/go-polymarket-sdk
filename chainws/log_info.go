package chainws

import (
	"fmt"
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
	EventTransferBatch  // ERC1155
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
	for _, contract := range PolygonContractRegistry(true) {
		if contract.Address == addr {
			return contract.Name
		}
	}
	if addr == common.HexToAddress(internal.PolygonProxyFactory) {
		return "ProxyFactory"
	}
	return ""
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
	case transferBatchEventSig:
		return EventTransferBatch
	default:
		return EventUnknown
	}
}

// TransferBatchInfo is an ERC-1155 TransferBatch event.
type TransferBatchInfo struct {
	Operator     common.Address
	From         common.Address
	To           common.Address
	TokenIDs     []*big.Int
	Values       []*big.Int
	BlockNumber  uint64
	TxHash       common.Hash
	LogIndex     uint
	ContractName string
}

// ParseTransferBatch parses TransferBatch(operator,from,to,ids,values).
func ParseTransferBatch(log *ethtypes.Log) *TransferBatchInfo {
	if EventKindOf(log) != EventTransferBatch || len(log.Topics) < 4 {
		return nil
	}
	ids, values, err := decodeUint256Arrays(log.Data)
	if err != nil || len(ids) != len(values) {
		return nil
	}
	return &TransferBatchInfo{
		Operator:     common.BytesToAddress(log.Topics[1].Bytes()),
		From:         common.BytesToAddress(log.Topics[2].Bytes()),
		To:           common.BytesToAddress(log.Topics[3].Bytes()),
		TokenIDs:     ids,
		Values:       values,
		BlockNumber:  log.BlockNumber,
		TxHash:       log.TxHash,
		LogIndex:     log.Index,
		ContractName: ContractNameByAddress(log.Address),
	}
}

func decodeUint256Arrays(data []byte) ([]*big.Int, []*big.Int, error) {
	if len(data) < 64 {
		return nil, nil, fmt.Errorf("transfer batch data is too short")
	}
	decode := func(offsetWord []byte) ([]*big.Int, error) {
		offset := new(big.Int).SetBytes(offsetWord)
		if !offset.IsUint64() {
			return nil, fmt.Errorf("array offset overflows uint64")
		}
		start := offset.Uint64()
		if start > uint64(len(data)-32) {
			return nil, fmt.Errorf("array offset outside data")
		}
		countInt := new(big.Int).SetBytes(data[start : start+32])
		if !countInt.IsUint64() {
			return nil, fmt.Errorf("array length overflows uint64")
		}
		count := countInt.Uint64()
		if count > (uint64(len(data))-start-32)/32 {
			return nil, fmt.Errorf("array exceeds data")
		}
		values := make([]*big.Int, count)
		for i := uint64(0); i < count; i++ {
			from := start + 32 + i*32
			values[i] = new(big.Int).SetBytes(data[from : from+32])
		}
		return values, nil
	}
	ids, err := decode(data[:32])
	if err != nil {
		return nil, nil, err
	}
	values, err := decode(data[32:64])
	if err != nil {
		return nil, nil, err
	}
	return ids, values, nil
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
