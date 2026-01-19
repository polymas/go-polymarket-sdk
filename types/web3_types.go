package types

// TransactionReceipt 表示区块链交易收据
type TransactionReceipt struct {
	TxHash            Keccak256   `json:"transaction_hash"`
	BlockNumber       uint64      `json:"block_number"`
	BlockHash         Keccak256   `json:"block_hash"`
	Status            int         `json:"status"` // 1 = success, 0 = failed
	GasUsed           uint64      `json:"gas_used"`
	EffectiveGasPrice string      `json:"effective_gas_price"`
	From              EthAddress  `json:"from"`
	To                *EthAddress `json:"to,omitempty"`
	Logs              []Log       `json:"logs"`
}

// Log 表示交易日志
type Log struct {
	Address EthAddress  `json:"address"`
	Topics  []Keccak256 `json:"topics"`
	Data    string      `json:"data"`
}
