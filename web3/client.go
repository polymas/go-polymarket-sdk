package web3

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

// Client 定义Web3客户端的接口，供外部包使用
type Client interface {
	GetSigner() *signing.Signer
	GetPrivateKey() *ecdsa.PrivateKey
	GetBaseAddress() types.EthAddress
	GetPolyProxyAddress() (types.EthAddress, error)
	GetChainID() types.ChainID
	GetSignatureType() types.SignatureType
	GetPOLBalance() (float64, error)
	// GetCollateralBalance returns the balance of the current V2 trading
	// collateral (pUSD) for address.
	GetCollateralBalance(address types.EthAddress) (float64, error)
	// GetUSDCEBalance returns the legacy bridged USDC.e balance for address.
	GetUSDCEBalance(address types.EthAddress) (float64, error)
	// Deprecated: use GetCollateralBalance for V2 trading funds, or
	// GetUSDCEBalance when USDC.e is explicitly required.
	GetUSDCBalance(address types.EthAddress) (float64, error)
	GetTokenBalance(tokenID string, address types.EthAddress) (float64, error)
	Close()
}

// baseClient 是Polymarket的基础Web3客户端
// 不允许直接导出，只能通过 NewClient 创建
type baseClient struct {
	clients          []*ethclient.Client // 多个 RPC 客户端，支持轮询和故障转移
	currentIndex     int64               // 当前使用的客户端索引（使用 atomic 操作）
	clientMu         sync.RWMutex        // 保护 clients 切片的并发访问
	privateKey       *ecdsa.PrivateKey
	signer           *signing.Signer
	signatureType    types.SignatureType
	chainID          types.ChainID
	baseAddress      types.EthAddress
	proxyAddress     types.EthAddress
	exchangeAddress  common.Address
	exchangeABI      *abi.ABI
	depositWalletRPC *depositWalletRPC // test seam; nil uses the configured RPC pool
}

// NewClient 创建新的基础Web3客户端
// 返回 Client 接口，不允许直接访问实现类型
// rpcURLs 为可选：若传入至少一个 URL 则使用自定义 RPC 列表，否则使用 SDK 内置的 Polygon 主网/Amoy 测试网节点列表
func NewClient(
	privateKey string,
	signatureType types.SignatureType,
	chainID types.ChainID,
	rpcURLs ...string,
) (Client, error) {
	// 根据 chainID 或调用方传入选择 RPC 节点列表（未传入时使用内置列表）
	var list []string
	if len(rpcURLs) > 0 {
		list = rpcURLs
	} else if chainID == internal.Amoy {
		list = internal.PolygonRPCAmoyList
	} else {
		list = internal.PolygonRPCMainnetList
	}

	if len(list) == 0 {
		return nil, fmt.Errorf("no RPC URLs configured for chain ID %d", chainID)
	}

	// 初始化多个 RPC 客户端
	var clients []*ethclient.Client
	var failedURLs []string

	for _, rpcURL := range list {
		client, err := ethclient.Dial(rpcURL)
		if err != nil {
			failedURLs = append(failedURLs, fmt.Sprintf("%s: %v", rpcURL, err))
			continue
		}
		clients = append(clients, client)
	}

	// 至少需要一个节点成功连接
	if len(clients) == 0 {
		return nil, fmt.Errorf("failed to connect to any RPC node. Errors: %v", failedURLs)
	}

	// 如果有部分节点连接失败，记录警告（但不影响使用）
	if len(failedURLs) > 0 {
		// 可以选择记录日志，这里暂时不记录以避免引入日志依赖
		_ = failedURLs
	}

	// Create signer
	signer, err := signing.NewSigner(privateKey, chainID)
	if err != nil {
		// 清理已连接的客户端
		for _, client := range clients {
			client.Close()
		}
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	baseAddress := signer.Address()

	// Parse exchange ABI (simplified - we only need getPolyProxyWalletAddress)
	exchangeABI, err := getExchangeABI()
	if err != nil {
		// 清理已连接的客户端
		for _, client := range clients {
			client.Close()
		}
		return nil, fmt.Errorf("failed to parse exchange ABI: %w", err)
	}

	web3Client := &baseClient{
		clients:         clients,
		currentIndex:    0,
		privateKey:      signer.PrivateKey(),
		signer:          signer,
		signatureType:   signatureType,
		chainID:         chainID,
		baseAddress:     baseAddress,
		exchangeAddress: common.HexToAddress(internal.PolygonExchange),
		exchangeABI:     exchangeABI,
	}

	// Initialize proxy address (will be lazy-loaded on first call)
	// For non-proxy types, proxy address equals base address
	if signatureType != types.ProxySignatureType &&
		signatureType != types.SafeSignatureType &&
		signatureType != types.CWIASignatureType {
		web3Client.proxyAddress = baseAddress
	}

	return web3Client, nil
}

// isRateLimitError 检查错误是否为 429 rate limit 错误
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "rate exceeded")
}

// isRetryableError 检查错误是否可重试（网络错误、429 等）
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return isRateLimitError(err) ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "dial")
}

// getNextClientIndex 获取下一个客户端索引（轮询）
func (c *baseClient) getNextClientIndex() int {
	c.clientMu.RLock()
	defer c.clientMu.RUnlock()
	if len(c.clients) == 0 {
		return 0
	}
	current := int(atomic.AddInt64(&c.currentIndex, 1) - 1)
	return current % len(c.clients)
}

// callContractWithRetry 带重试的合约调用，支持多节点轮询和故障转移
func (c *baseClient) callContractWithRetry(
	ctx context.Context,
	msg ethereum.CallMsg,
	blockNumber *big.Int,
) ([]byte, error) {
	c.clientMu.RLock()
	clients := c.clients
	c.clientMu.RUnlock()

	if len(clients) == 0 {
		return nil, fmt.Errorf("no RPC clients available")
	}

	// 从当前索引开始，尝试所有节点
	startIndex := c.getNextClientIndex()
	var lastErr error

	for i := 0; i < len(clients); i++ {
		index := (startIndex + i) % len(clients)
		client := clients[index]

		result, err := client.CallContract(ctx, msg, blockNumber)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// 如果是 429 错误或可重试错误，继续尝试下一个节点
		if isRetryableError(err) {
			continue
		}

		// 对于其他错误（如合约执行错误），直接返回
		return nil, err
	}

	// 所有节点都失败了，返回最后一个错误
	return nil, fmt.Errorf("all RPC nodes failed, last error: %w", lastErr)
}

// balanceAtWithRetry 带重试的余额查询，支持多节点轮询和故障转移
func (c *baseClient) balanceAtWithRetry(
	ctx context.Context,
	account common.Address,
	blockNumber *big.Int,
) (*big.Int, error) {
	c.clientMu.RLock()
	clients := c.clients
	c.clientMu.RUnlock()

	if len(clients) == 0 {
		return nil, fmt.Errorf("no RPC clients available")
	}

	// 从当前索引开始，尝试所有节点
	startIndex := c.getNextClientIndex()
	var lastErr error

	for i := 0; i < len(clients); i++ {
		index := (startIndex + i) % len(clients)
		client := clients[index]

		balance, err := client.BalanceAt(ctx, account, blockNumber)
		if err == nil {
			return balance, nil
		}

		lastErr = err

		// 如果是 429 错误或可重试错误，继续尝试下一个节点
		if isRetryableError(err) {
			continue
		}

		// 对于其他错误，直接返回
		return nil, err
	}

	// 所有节点都失败了，返回最后一个错误
	return nil, fmt.Errorf("all RPC nodes failed, last error: %w", lastErr)
}

// estimateGasWithRetry 带重试的 Gas 估算，支持多节点轮询和故障转移
func (c *baseClient) estimateGasWithRetry(
	ctx context.Context,
	msg ethereum.CallMsg,
) (uint64, error) {
	c.clientMu.RLock()
	clients := c.clients
	c.clientMu.RUnlock()

	if len(clients) == 0 {
		return 0, fmt.Errorf("no RPC clients available")
	}

	// 从当前索引开始，尝试所有节点
	startIndex := c.getNextClientIndex()
	var lastErr error

	for i := 0; i < len(clients); i++ {
		index := (startIndex + i) % len(clients)
		client := clients[index]

		gas, err := client.EstimateGas(ctx, msg)
		if err == nil {
			return gas, nil
		}

		lastErr = err

		// 如果是 429 错误或可重试错误，继续尝试下一个节点
		if isRetryableError(err) {
			continue
		}

		// 对于其他错误，直接返回
		return 0, err
	}

	// 所有节点都失败了，返回最后一个错误
	return 0, fmt.Errorf("all RPC nodes failed, last error: %w", lastErr)
}

// transactionReceiptWithRetry 带重试的交易回执查询，支持多节点轮询和故障转移
func (c *baseClient) transactionReceiptWithRetry(
	ctx context.Context,
	txHash common.Hash,
) (*ethtypes.Receipt, error) {
	c.clientMu.RLock()
	clients := c.clients
	c.clientMu.RUnlock()

	if len(clients) == 0 {
		return nil, fmt.Errorf("no RPC clients available")
	}

	// 从当前索引开始，尝试所有节点
	startIndex := c.getNextClientIndex()
	var lastErr error

	for i := 0; i < len(clients); i++ {
		index := (startIndex + i) % len(clients)
		client := clients[index]

		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil {
			return receipt, nil
		}

		lastErr = err

		// 如果是 429 错误或可重试错误，继续尝试下一个节点
		if isRetryableError(err) {
			continue
		}

		// 对于其他错误（如交易不存在），直接返回
		return nil, err
	}

	// 所有节点都失败了，返回最后一个错误
	return nil, fmt.Errorf("all RPC nodes failed, last error: %w", lastErr)
}

// transactionByHashWithRetry 带重试的交易查询，支持多节点轮询和故障转移
func (c *baseClient) transactionByHashWithRetry(
	ctx context.Context,
	txHash common.Hash,
) (*ethtypes.Transaction, bool, error) {
	c.clientMu.RLock()
	clients := c.clients
	c.clientMu.RUnlock()

	if len(clients) == 0 {
		return nil, false, fmt.Errorf("no RPC clients available")
	}

	// 从当前索引开始，尝试所有节点
	startIndex := c.getNextClientIndex()
	var lastErr error

	for i := 0; i < len(clients); i++ {
		index := (startIndex + i) % len(clients)
		client := clients[index]

		tx, pending, err := client.TransactionByHash(ctx, txHash)
		if err == nil {
			return tx, pending, nil
		}

		lastErr = err

		// 如果是 429 错误或可重试错误，继续尝试下一个节点
		if isRetryableError(err) {
			continue
		}

		// 对于其他错误（如交易不存在），直接返回
		return nil, false, err
	}

	// 所有节点都失败了，返回最后一个错误
	return nil, false, fmt.Errorf("all RPC nodes failed, last error: %w", lastErr)
}

func (c *baseClient) GetPrivateKey() *ecdsa.PrivateKey {
	return c.privateKey
}

func (c *baseClient) GetSigner() *signing.Signer {
	return c.signer
}

// GetBaseAddress 返回基础EOA地址
func (c *baseClient) GetBaseAddress() types.EthAddress {
	return c.baseAddress
}

// GetChainID 返回链ID
func (c *baseClient) GetChainID() types.ChainID {
	return c.chainID
}

// GetSignatureType 返回签名类型
func (c *baseClient) GetSignatureType() types.SignatureType {
	return c.signatureType
}

// GetPolyProxyAddress 根据签名类型获取代理地址（使用内部缓存的 baseAddress）
// 如果已经缓存了 proxyAddress，直接返回；否则调用合约获取并缓存
func (c *baseClient) GetPolyProxyAddress() (types.EthAddress, error) {
	// 如果已经缓存了，直接返回
	if c.proxyAddress != "" {
		return c.proxyAddress, nil
	}

	// 根据签名类型获取代理地址
	var proxyAddr types.EthAddress
	var err error

	switch c.signatureType {
	case types.ProxySignatureType:
		proxyAddr, err = c.getPolyProxyWalletAddress(c.baseAddress)
	case types.SafeSignatureType:
		proxyAddr, err = c.getSafeProxyAddress(c.baseAddress)
	case types.CWIASignatureType:
		proxyAddr, err = c.getCWIAProxyAddress(c.baseAddress)
	default:
		// For EOA or unknown types, return the base address
		proxyAddr = c.baseAddress
	}

	if err != nil {
		return "", err
	}

	// 缓存结果
	c.proxyAddress = proxyAddr
	return proxyAddr, nil
}

// getPolyProxyWalletAddress 获取给定地址的Polymarket代理钱包地址
func (c *baseClient) getPolyProxyWalletAddress(address types.EthAddress) (types.EthAddress, error) {
	// Call getPolyProxyWalletAddress on the exchange contract
	addr := common.HexToAddress(string(address))

	// Pack the function call
	packed, err := c.exchangeABI.Pack("getPolyProxyWalletAddress", addr)
	if err != nil {
		return "", fmt.Errorf("failed to pack function call: %w", err)
	}

	// Call the contract
	callMsg := ethereum.CallMsg{
		To:   &c.exchangeAddress,
		Data: packed,
	}

	result, err := c.callContractWithRetry(context.Background(), callMsg, nil)
	if err != nil {
		return "", fmt.Errorf("failed to call contract: %w", err)
	}

	// Unpack the result - getPolyProxyWalletAddress returns (address)
	var proxyAddr common.Address
	err = c.exchangeABI.UnpackIntoInterface(&proxyAddr, "getPolyProxyWalletAddress", result)
	if err != nil {
		return "", fmt.Errorf("failed to unpack result: %w", err)
	}

	return types.EthAddress(proxyAddr.Hex()), nil
}

// getSafeProxyAddress 获取给定地址的Safe代理地址
func (c *baseClient) getSafeProxyAddress(address types.EthAddress) (types.EthAddress, error) {
	// Safe proxy factory address
	safeProxyFactoryAddr := common.HexToAddress(internal.SafeProxyFactory)

	// Parse SafeProxyFactory ABI
	safeProxyFactoryABI, err := getSafeProxyFactoryABI()
	if err != nil {
		return "", fmt.Errorf("failed to parse safe proxy factory ABI: %w", err)
	}

	// Call computeProxyAddress(address owner) on SafeProxyFactory
	addr := common.HexToAddress(string(address))
	packed, err := safeProxyFactoryABI.Pack("computeProxyAddress", addr)
	if err != nil {
		return "", fmt.Errorf("failed to pack computeProxyAddress: %w", err)
	}

	// Call the contract
	callMsg := ethereum.CallMsg{
		To:   &safeProxyFactoryAddr,
		Data: packed,
	}

	result, err := c.callContractWithRetry(context.Background(), callMsg, nil)
	if err != nil {
		return "", fmt.Errorf("failed to call computeProxyAddress: %w", err)
	}

	// Unpack the result - computeProxyAddress returns (address)
	var proxyAddr common.Address
	err = safeProxyFactoryABI.UnpackIntoInterface(&proxyAddr, "computeProxyAddress", result)
	if err != nil {
		return "", fmt.Errorf("failed to unpack result: %w", err)
	}

	return types.EthAddress(proxyAddr.Hex()), nil
}

// getCWIAProxyAddress 按官方双架构算法解析 Deposit Wallet。地址在本地派生；
// RPC 只用于查询 BEACON() 和确认旧 UUPS 地址是否已经部署。
func (c *baseClient) getCWIAProxyAddress(owner types.EthAddress) (types.EthAddress, error) {
	rpc := depositWalletRPC{beacon: c.depositWalletFactoryBeacon, code: c.codeAtWithRetry}
	if c.depositWalletRPC != nil {
		rpc = *c.depositWalletRPC
	}
	proxy, err := resolveDepositWallet(context.Background(), common.HexToAddress(string(owner)), common.HexToAddress(internal.PolygonDepositWalletFactory), common.HexToAddress(internal.PolygonDepositWalletImpl), rpc)
	if err != nil {
		return "", err
	}
	return types.EthAddress(proxy.Hex()), nil
}

// GetPOLBalance 获取基础地址的POL余额
func (c *baseClient) GetPOLBalance() (float64, error) {
	balance, err := c.balanceAtWithRetry(context.Background(), common.HexToAddress(string(c.baseAddress)), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}
	balanceFloat := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))
	result, _ := balanceFloat.Float64()
	return result, nil
}

// GetCollateralBalance returns address's current V2 trading collateral balance.
// Polymarket V2 uses pUSD; USDC.e is only an on-ramp asset.
func (c *baseClient) GetCollateralBalance(address types.EthAddress) (float64, error) {
	return c.getERC20Balance(address, common.HexToAddress(internal.PolygonPUSD), "pUSD")
}

// GetUSDCEBalance returns address's legacy bridged USDC.e balance.
func (c *baseClient) GetUSDCEBalance(address types.EthAddress) (float64, error) {
	return c.getERC20Balance(address, common.HexToAddress(internal.PolygonCollateral), "USDC.e")
}

// GetUSDCBalance returns the legacy bridged USDC.e balance.
//
// Deprecated: use GetCollateralBalance for V2 trading funds, or
// GetUSDCEBalance when USDC.e is explicitly required.
func (c *baseClient) GetUSDCBalance(address types.EthAddress) (float64, error) {
	return c.GetUSDCEBalance(address)
}

func (c *baseClient) getERC20Balance(address types.EthAddress, token common.Address, symbol string) (float64, error) {
	if err := address.Validate(); err != nil {
		return 0, fmt.Errorf("invalid balance address: %w", err)
	}

	balanceOfABI := `[{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(balanceOfABI))
	if err != nil {
		return 0, fmt.Errorf("failed to parse ERC20 ABI: %w", err)
	}

	addr := common.HexToAddress(address.String())
	packed, err := parsedABI.Pack("balanceOf", addr)
	if err != nil {
		return 0, fmt.Errorf("failed to pack %s.balanceOf: %w", symbol, err)
	}

	callMsg := ethereum.CallMsg{
		To:   &token,
		Data: packed,
	}

	result, err := c.callContractWithRetry(context.Background(), callMsg, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to call %s.balanceOf: %w", symbol, err)
	}

	var balance *big.Int
	err = parsedABI.UnpackIntoInterface(&balance, "balanceOf", result)
	if err != nil {
		return 0, fmt.Errorf("failed to unpack %s.balanceOf: %w", symbol, err)
	}

	// Both pUSD and USDC.e use 6 decimals on Polygon.
	balanceFloat := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e6))
	resultFloat, _ := balanceFloat.Float64()
	return resultFloat, nil
}

// GetTokenBalance 获取地址的代币余额
func (c *baseClient) GetTokenBalance(tokenID string, address types.EthAddress) (float64, error) {
	// Parse token ID as big.Int
	tokenIDBig, ok := new(big.Int).SetString(tokenID, 10)
	if !ok {
		return 0, fmt.Errorf("invalid token ID: %s", tokenID)
	}

	// Get ConditionalTokens contract address
	conditionalTokensAddr := common.HexToAddress(internal.PolygonConditionalTokens)

	// Create ABI for balanceOf(address account, uint256 id)
	balanceOfABI := `[{"constant":true,"inputs":[{"name":"account","type":"address"},{"name":"id","type":"uint256"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(balanceOfABI))
	if err != nil {
		return 0, fmt.Errorf("failed to parse ABI: %w", err)
	}

	// Pack the function call
	addr := common.HexToAddress(string(address))
	packed, err := parsedABI.Pack("balanceOf", addr, tokenIDBig)
	if err != nil {
		return 0, fmt.Errorf("failed to pack function call: %w", err)
	}

	// Call the contract
	callMsg := ethereum.CallMsg{
		To:   &conditionalTokensAddr,
		Data: packed,
	}

	result, err := c.callContractWithRetry(context.Background(), callMsg, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to call contract: %w", err)
	}

	// Unpack the result
	var balance *big.Int
	err = parsedABI.UnpackIntoInterface(&balance, "balanceOf", result)
	if err != nil {
		return 0, fmt.Errorf("failed to unpack result: %w", err)
	}

	// Convert to float64 (divide by 1e6)
	balanceFloat := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e6))
	resultFloat, _ := balanceFloat.Float64()
	return resultFloat, nil
}

// Close 关闭所有客户端连接
func (c *baseClient) Close() {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	for _, client := range c.clients {
		if client != nil {
			client.Close()
		}
	}
	c.clients = nil
}

// getExchangeABI 返回交易所合约的最小ABI
// 仅包含getPolyProxyWalletAddress函数
func getExchangeABI() (*abi.ABI, error) {
	// Minimal ABI for getPolyProxyWalletAddress
	abiJSON := `[
		{
			"inputs": [
				{
					"internalType": "address",
					"name": "_addr",
					"type": "address"
				}
			],
			"name": "getPolyProxyWalletAddress",
			"outputs": [
				{
					"internalType": "address",
					"name": "",
					"type": "address"
				}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`

	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}

	return &parsedABI, nil
}

// getDepositWalletFactoryABI 返回 Polymarket DepositWalletFactory 的最小 ABI。
// 仅包含派生新版代理地址需要的两个 view 函数。
func getDepositWalletFactoryABI() (*abi.ABI, error) {
	abiJSON := `[
		{
			"inputs": [],
			"name": "implementation",
			"outputs": [{"internalType": "address", "name": "", "type": "address"}],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{"internalType": "address", "name": "_implementation", "type": "address"},
				{"internalType": "bytes32", "name": "_id", "type": "bytes32"}
			],
			"name": "predictWalletAddress",
			"outputs": [{"internalType": "address", "name": "", "type": "address"}],
			"stateMutability": "view",
			"type": "function"
		}
	]`

	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	return &parsedABI, nil
}

// getSafeProxyFactoryABI returns a minimal ABI for the SafeProxyFactory contract
// Only includes computeProxyAddress function
func getSafeProxyFactoryABI() (*abi.ABI, error) {
	abiJSON := `[
		{
			"inputs": [
				{"internalType": "address", "name": "owner", "type": "address"}
			],
			"name": "computeProxyAddress",
			"outputs": [
				{"internalType": "address", "name": "", "type": "address"}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`

	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}

	return &parsedABI, nil
}
