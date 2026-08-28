package web3

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// v2KeyRetryBackoff 是 V2 relayer key 获取失败后的最小重试间隔。
// 失败原因多为瞬时网络/代理问题，退避避免每个请求都触发一次 SIWE 全流程。
const v2KeyRetryBackoff = 30 * time.Second

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
	// V2 relayer key 懒加载。首次发请求前 ensureV2RelayerKey 触发 SIWE 派生或读
	// 本地缓存，成功后注入 localSigner.v2Key 让后续请求走 V2 双头。
	// 获取失败不会永久降级：每次请求都可触发重试（带 v2KeyRetryBackoff 退避），
	// /submit 401 时通过 invalidateV2RelayerKey 作废后重新派生。
	v2KeyMu              sync.Mutex
	v2KeyLastAttempt     time.Time
	v2KeyLastErr         error
	v2KeyDisabledLogOnce sync.Once
	// relayer 调用统计
	relayerCallCount int64 // 使用 atomic 操作，记录总调用次数
}

// NewGaslessClient creates a new gasless Web3 client.
// rpcURLs 为可选：若传入至少一个 URL 则使用自定义 RPC 列表，否则使用 SDK 内置列表（与 NewClient 一致）
func NewGaslessClient(
	privateKey string,
	signatureType types.SignatureType,
	chainID types.ChainID,
	builderCreds *types.ApiCreds,
	rpcURLs ...string,
) (*GaslessClient, error) {
	// Supported: proxy (1), safe (2), CWIA / DepositWallet (3).
	// CWIA 走完全不同的链上路径（DepositWalletFactory.proxy(Batch[],bytes[])），
	// 但 Polymarket relayer 仍在同一域名下，只是 type=WALLET。
	if signatureType != types.ProxySignatureType &&
		signatureType != types.SafeSignatureType &&
		signatureType != types.DepositWalletSignatureType {
		return nil, fmt.Errorf("gaslessClient only supports signature_type=1 (proxy), 2 (safe), 3 (CWIA/DepositWallet)")
	}

	baseClientInterface, err := NewClient(privateKey, signatureType, chainID, rpcURLs...)
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
		web3Client: baseClientInterface, // 保存接口引用
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
				{"internalType": "uint256[]", "name": "partition", "type": "uint256[]"},
				{"internalType": "uint256", "name": "amount", "type": "uint256"}
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
				{"internalType": "uint256[]", "name": "partition", "type": "uint256[]"},
				{"internalType": "uint256", "name": "amount", "type": "uint256"}
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

// ensureV2RelayerKey 懒加载 V2 relayer key 并注入到 localSigner。
// 与旧版（sync.Once，一次失败永久降级到旧鉴权 → relayer 永远 401）不同：
// 获取失败只影响本次请求，后续请求会在 v2KeyRetryBackoff 退避后自动重试，直到成功注入。
// 设置环境变量 POLYMARKET_DISABLE_V2_RELAYER_KEY=1 可禁用 V2，强制走旧协议。
func (c *GaslessClient) ensureV2RelayerKey(ctx context.Context) error {
	if os.Getenv("POLYMARKET_DISABLE_V2_RELAYER_KEY") == "1" {
		c.v2KeyDisabledLogOnce.Do(func() {
			log.Printf("[INFO] V2 relayer key 被环境变量禁用，继续走旧鉴权")
		})
		return nil
	}

	c.v2KeyMu.Lock()
	defer c.v2KeyMu.Unlock()

	if c.localSigner.HasV2Key() {
		return nil
	}
	// 退避窗口内不重复打 SIWE，直接返回上次错误。
	if c.v2KeyLastErr != nil && time.Since(c.v2KeyLastAttempt) < v2KeyRetryBackoff {
		return c.v2KeyLastErr
	}
	c.v2KeyLastAttempt = time.Now()

	priv := c.signer.PrivateKey()
	if priv == nil {
		c.v2KeyLastErr = fmt.Errorf("signer has no private key")
		log.Printf("[WARN] V2 relayer key 获取失败：%v；本次降级到旧鉴权", c.v2KeyLastErr)
		return c.v2KeyLastErr
	}
	k, err := internal.EnsureV2RelayerKey(ctx, priv, "")
	if err != nil {
		c.v2KeyLastErr = err
		log.Printf("[WARN] V2 relayer key 获取失败：%v；本次降级到旧鉴权（relayer 会 401），%v 后允许重试", err, v2KeyRetryBackoff)
		return c.v2KeyLastErr
	}
	c.v2KeyLastErr = nil
	c.localSigner.SetV2Key(k)
	// Relayer API key 属于凭证，只记录关联地址，绝不把 key 写入日志。
	log.Printf("[OK] V2 relayer key 已注入：address=%s", k.Address)
	return nil
}

// invalidateV2RelayerKey 作废当前 V2 relayer key（内存注入 + 磁盘缓存），
// 下次 ensureV2RelayerKey 会立即重新走 SIWE 派生（不受退避窗口限制）。
// 用于 /submit 返回 401 invalid authorization 时自愈。
func (c *GaslessClient) invalidateV2RelayerKey() {
	c.v2KeyMu.Lock()
	defer c.v2KeyMu.Unlock()
	c.localSigner.SetV2Key(nil)
	c.v2KeyLastErr = nil
	c.v2KeyLastAttempt = time.Time{}
	if path := internal.DefaultV2KeyCachePath(string(c.baseAddress)); path != "" {
		_ = os.Remove(path)
	}
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
