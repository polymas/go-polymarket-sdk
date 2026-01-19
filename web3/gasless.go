package web3

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// GaslessClient 是通过中继进行免gas交易的Web3客户端
type GaslessClient struct {
	*baseClient            // 嵌入实现类型以访问私有字段
	web3Client      Client // 保存接口引用供外部使用
	httpClient      *http.Client
	relayURL        string
	relayHub        string
	relayAddress    string
	builderCreds    *types.ApiCreds
	localSigner     *LocalSigner
	conditionalABI  *abi.ABI
	negRiskABI      *abi.ABI
	proxyFactoryABI *abi.ABI
	// relayer 调用统计
	relayerCallCount int64 // 使用 atomic 操作，记录总调用次数
}

// NewGaslessClient creates a new gasless Web3 client
func NewGaslessClient(
	privateKey string,
	signatureType types.SignatureType,
	chainID types.ChainID,
	builderCreds *types.ApiCreds,
) (*GaslessClient, error) {
	// Only support proxy (1) and safe (2) wallets
	if signatureType != types.ProxySignatureType && signatureType != types.SafeSignatureType {
		return nil, fmt.Errorf("gaslessClient only supports signature_type=1 (proxy) and signature_type=2 (safe)")
	}

	baseClientInterface, err := NewClient(privateKey, signatureType, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create base client: %w", err)
	}

	// Parse ABIs
	conditionalABI, err := getConditionalTokensABI()
	if err != nil {
		return nil, fmt.Errorf("failed to parse conditional tokens ABI: %w", err)
	}

	negRiskABI, err := getNegRiskAdapterABI()
	if err != nil {
		return nil, fmt.Errorf("failed to parse neg risk adapter ABI: %w", err)
	}

	proxyFactoryABI, err := getProxyFactoryABI()
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy factory ABI: %w", err)
	}

	// Create LocalSigner for signing requests (matching Python's LocalSigner)
	// 需要类型断言访问私有字段
	baseClientImpl, ok := baseClientInterface.(*baseClient)
	if !ok {
		return nil, fmt.Errorf("failed to cast base client to implementation type")
	}
	localSigner := NewLocalSigner(baseClientImpl.GetSigner(), builderCreds)

	client := &GaslessClient{
		baseClient: baseClientImpl,
		web3Client:     baseClientInterface, // 保存接口引用
		httpClient: &http.Client{
			Timeout: internal.HTTPClientLongTimeout,
			// Use default transport (automatically handles proxy, TLS, etc.)
		},
		relayURL:        internal.RelayerDomain,
		relayHub:        internal.RelayHub,
		relayAddress:    internal.RelayAddress,
		builderCreds:    builderCreds,
		localSigner:     localSigner,
		conditionalABI:  conditionalABI,
		negRiskABI:      negRiskABI,
		proxyFactoryABI: proxyFactoryABI,
	}

	return client, nil
}

// Helper functions to get ABIs
func getConditionalTokensABI() (*abi.ABI, error) {
	// Extended ABI for redeemPositions, splitPosition, and mergePositions
	abiJSON := `[
		{
			"inputs": [
				{"internalType": "address", "name": "collateralToken", "type": "address"},
				{"internalType": "bytes32", "name": "parentCollectionId", "type": "bytes32"},
				{"internalType": "bytes32", "name": "conditionId", "type": "bytes32"},
				{"internalType": "uint256[]", "name": "indexSets", "type": "uint256[]"}
			],
			"name": "redeemPositions",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{"internalType": "address", "name": "collateralToken", "type": "address"},
				{"internalType": "bytes32", "name": "parentCollectionId", "type": "bytes32"},
				{"internalType": "bytes32", "name": "conditionId", "type": "bytes32"},
				{"internalType": "uint256", "name": "partition", "type": "uint256"},
				{"internalType": "uint256", "name": "amount", "type": "uint256"},
				{"internalType": "uint256[]", "name": "indexSets", "type": "uint256[]"}
			],
			"name": "splitPosition",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{"internalType": "address", "name": "collateralToken", "type": "address"},
				{"internalType": "bytes32", "name": "parentCollectionId", "type": "bytes32"},
				{"internalType": "bytes32", "name": "conditionId", "type": "bytes32"},
				{"internalType": "uint256", "name": "partition", "type": "uint256"},
				{"internalType": "uint256[]", "name": "indexSets", "type": "uint256[]"},
				{"internalType": "uint256[]", "name": "amounts", "type": "uint256[]"}
			],
			"name": "mergePositions",
			"outputs": [],
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	return &parsedABI, nil
}

func getNegRiskAdapterABI() (*abi.ABI, error) {
	// Minimal ABI for redeemPositions
	abiJSON := `[{
		"inputs": [
			{"internalType": "bytes32", "name": "_conditionId", "type": "bytes32"},
			{"internalType": "uint256[]", "name": "_amounts", "type": "uint256[]"}
		],
		"name": "redeemPositions",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	return &parsedABI, nil
}

func getProxyFactoryABI() (*abi.ABI, error) {
	// Minimal ABI for proxy
	abiJSON := `[{
		"inputs": [{
			"components": [
				{"internalType": "uint8", "name": "typeCode", "type": "uint8"},
				{"internalType": "address", "name": "to", "type": "address"},
				{"internalType": "uint256", "name": "value", "type": "uint256"},
				{"internalType": "bytes", "name": "data", "type": "bytes"}
			],
			"internalType": "struct ProxyWalletFactory.Transaction[]",
			"name": "transactions",
			"type": "tuple[]"
		}],
		"name": "proxy",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	return &parsedABI, nil
}
