package client

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymarket/go-polymarket-sdk/internal"
	"github.com/polymarket/go-polymarket-sdk/internal/signing"
	"github.com/polymarket/go-polymarket-sdk/types"
)

// Web3Client 定义Web3客户端的接口，供外部包使用
type Web3Client interface {
	GetSigner() *signing.Signer
	GetPrivateKey() *ecdsa.PrivateKey
	GetBaseAddress() types.EthAddress
	GetPolyProxyAddress() (types.EthAddress, error)
	GetChainID() types.ChainID
	GetSignatureType() types.SignatureType
	GetPOLBalance() (float64, error)
	GetUSDCBalance(address types.EthAddress) (float64, error)
	GetTokenBalance(tokenID string, address types.EthAddress) (float64, error)
	Close()
}

// baseWeb3Client 是Polymarket的基础Web3客户端
// 不允许直接导出，只能通过 NewBaseWeb3Client 创建
type baseWeb3Client struct {
	client          *ethclient.Client
	privateKey      *ecdsa.PrivateKey
	signer          *signing.Signer
	signatureType   types.SignatureType
	chainID         types.ChainID
	baseAddress     types.EthAddress
	proxyAddress    types.EthAddress
	exchangeAddress common.Address
	exchangeABI     *abi.ABI
}

// NewBaseWeb3Client 创建新的基础Web3客户端
// 返回 Web3Client 接口，不允许直接访问实现类型
func NewBaseWeb3Client(
	privateKey string,
	signatureType types.SignatureType,
	chainID types.ChainID,
) (Web3Client, error) {
	// Connect to Polygon RPC
	rpcURL := internal.PolygonRPCMainnet
	if chainID == internal.Amoy {
		rpcURL = internal.PolygonRPCAmoy
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RPC: %w", err)
	}

	// Create signer
	signer, err := signing.NewSigner(privateKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	baseAddress := signer.Address()

	// Parse exchange ABI (simplified - we only need getPolyProxyWalletAddress)
	exchangeABI, err := getExchangeABI()
	if err != nil {
		return nil, fmt.Errorf("failed to parse exchange ABI: %w", err)
	}

	web3Client := &baseWeb3Client{
		client:          client,
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
	if signatureType != types.ProxySignatureType && signatureType != types.SafeSignatureType {
		web3Client.proxyAddress = baseAddress
	}

	return web3Client, nil
}

func (c *baseWeb3Client) GetPrivateKey() *ecdsa.PrivateKey {
	return c.privateKey
}

func (c *baseWeb3Client) GetSigner() *signing.Signer {
	return c.signer
}

// GetBaseAddress 返回基础EOA地址
func (c *baseWeb3Client) GetBaseAddress() types.EthAddress {
	return c.baseAddress
}

// GetChainID 返回链ID
func (c *baseWeb3Client) GetChainID() types.ChainID {
	return c.chainID
}

// GetSignatureType 返回签名类型
func (c *baseWeb3Client) GetSignatureType() types.SignatureType {
	return c.signatureType
}

// GetPolyProxyAddress 根据签名类型获取代理地址（使用内部缓存的 baseAddress）
// 如果已经缓存了 proxyAddress，直接返回；否则调用合约获取并缓存
func (c *baseWeb3Client) GetPolyProxyAddress() (types.EthAddress, error) {
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
func (c *baseWeb3Client) getPolyProxyWalletAddress(address types.EthAddress) (types.EthAddress, error) {
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

	result, err := c.client.CallContract(context.Background(), callMsg, nil)
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
func (c *baseWeb3Client) getSafeProxyAddress(address types.EthAddress) (types.EthAddress, error) {
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

	result, err := c.client.CallContract(context.Background(), callMsg, nil)
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

// GetPOLBalance 获取基础地址的POL余额
func (c *baseWeb3Client) GetPOLBalance() (float64, error) {
	balance, err := c.client.BalanceAt(context.Background(), common.HexToAddress(string(c.baseAddress)), nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get balance: %w", err)
	}
	balanceFloat := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e18))
	result, _ := balanceFloat.Float64()
	return result, nil
}

// GetUSDCBalance 获取地址的USDC余额
func (c *baseWeb3Client) GetUSDCBalance(address types.EthAddress) (float64, error) {
	// 获取 USDC 合约地址（PolygonCollateral）
	usdcAddr := common.HexToAddress(internal.PolygonCollateral)

	// 创建 ERC20 标准的 balanceOf(address) ABI
	balanceOfABI := `[{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`
	parsedABI, err := abi.JSON(strings.NewReader(balanceOfABI))
	if err != nil {
		return 0, fmt.Errorf("failed to parse ABI: %w", err)
	}

	// 打包函数调用
	addr := common.HexToAddress(string(address))
	packed, err := parsedABI.Pack("balanceOf", addr)
	if err != nil {
		return 0, fmt.Errorf("failed to pack function call: %w", err)
	}

	// 调用合约
	callMsg := ethereum.CallMsg{
		To:   &usdcAddr,
		Data: packed,
	}

	result, err := c.client.CallContract(context.Background(), callMsg, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to call contract: %w", err)
	}

	// 解包结果
	var balance *big.Int
	err = parsedABI.UnpackIntoInterface(&balance, "balanceOf", result)
	if err != nil {
		return 0, fmt.Errorf("failed to unpack result: %w", err)
	}

	// USDC 有 6 位小数，所以除以 1e6
	balanceFloat := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e6))
	resultFloat, _ := balanceFloat.Float64()
	return resultFloat, nil
}

// GetTokenBalance 获取地址的代币余额
func (c *baseWeb3Client) GetTokenBalance(tokenID string, address types.EthAddress) (float64, error) {
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

	result, err := c.client.CallContract(context.Background(), callMsg, nil)
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

// Close 关闭客户端连接
func (c *baseWeb3Client) Close() {
	if c.client != nil {
		c.client.Close()
	}
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
