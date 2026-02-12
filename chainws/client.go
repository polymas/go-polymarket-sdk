// Package chainws 提供链上 WebSocket 订阅与钱包仓位跟踪（Tracker）：监控地址相关日志与新区块，维护 USDC/token 仓位并对外暴露 GetPositions() 一致 map。
package chainws

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// 常用事件签名（keccak256）
var (
	// Transfer(address,address,uint256)
	transferEventSig = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	// Approval(address,address,uint256)
	approvalEventSig = common.HexToHash("0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925")
	// TransferSingle(address,address,address,uint256,uint256) ERC1155
	transferSingleEventSig = common.BytesToHash(crypto.Keccak256([]byte("TransferSingle(address,address,address,uint256,uint256)")))
)

// ErrNoWSSURL 未配置 WSS URL 且内置列表为空
var ErrNoWSSURL = errors.New("chainws: no WSS URL configured and built-in list is empty")

// ErrWatchAddrsRequired 未设置 WatchAddresses 或数量超过上限时返回，禁止全量订阅以控制流量
var ErrWatchAddrsRequired = errors.New("chainws: WatchAddresses required (1 to 100 addresses); refusing full subscription")

// maxWatchAddrsForTopicFilter 使用服务端 topic 过滤时，watch 地址数量上限（避免 RPC 对 topics 数量限制）
const maxWatchAddrsForTopicFilter = 100

// Client 链上 WebSocket 客户端接口，用于监控当前地址相关的链上数据变化
type Client interface {
	// SetOnLog 设置日志回调，仅当日志与监控地址相关时触发（如 ERC20 Transfer 的 from/to）
	SetOnLog(callback func(log *ethtypes.Log))
	// SetOnBlock 设置新区块回调
	SetOnBlock(callback func(blockNumber uint64))
	// WatchAddresses 设置要监控的地址（如 base + proxy），这些地址在 Transfer/Approval 等事件中会被过滤
	WatchAddresses(addrs []types.EthAddress)
	// Start 开始订阅，需先设置 WatchAddresses 或通过 NewClientWithAddresses 传入
	Start(ctx context.Context) error
	// Stop 停止订阅并断开连接
	Stop()
	// IsRunning 是否正在运行
	IsRunning() bool
}

// chainWSClient 链上 WSS 客户端实现
type chainWSClient struct {
	wssURL        string
	watchAddrs    []common.Address
	watchAddrsMu  sync.RWMutex
	contractAddrs []common.Address // 监听的合约地址（Polymarket 相关）

	onLog   func(*ethtypes.Log)
	onBlock func(blockNumber uint64)

	client       *ethclient.Client
	subLogsList  []ethereum.Subscription // 可能为多个（按 topic 过滤时），或单个（全量订阅）
	subHead      ethereum.Subscription
	logCh        chan ethtypes.Log
	errCh        chan error // 合并多个 sub.Err()，供 runLogLoop 使用
	stopChan  chan struct{}
	stopOnce  sync.Once
	running   bool
	runningMu sync.RWMutex

	reconnectDelay time.Duration
}

// NewClient 创建链上 WSS 客户端。wssURL 为空时根据 chainID 使用内置 WSS 列表中的第一个。
func NewClient(chainID types.ChainID, wssURL string) (Client, error) {
	if wssURL == "" {
		if chainID == internal.Amoy {
			if len(internal.PolygonWSSAmoyList) == 0 {
				return nil, ErrNoWSSURL
			}
			wssURL = internal.PolygonWSSAmoyList[0]
		} else {
			if len(internal.PolygonWSSMainnetList) == 0 {
				return nil, ErrNoWSSURL
			}
			wssURL = internal.PolygonWSSMainnetList[0]
		}
	}
	contracts := polymarketContractAddresses()
	return &chainWSClient{
		wssURL:         wssURL,
		contractAddrs:  contracts,
		stopChan:       make(chan struct{}),
		reconnectDelay: 5 * time.Second,
	}, nil
}

// NewClientWithAddresses 创建并设置要监控的地址
func NewClientWithAddresses(chainID types.ChainID, wssURL string, watchAddrs []types.EthAddress) (Client, error) {
	c, err := NewClient(chainID, wssURL)
	if err != nil {
		return nil, err
	}
	c.WatchAddresses(watchAddrs)
	return c, nil
}

func polymarketContractAddresses() []common.Address {
	return []common.Address{
		common.HexToAddress(internal.PolygonCollateral),
		common.HexToAddress(internal.PolygonExchange),
		common.HexToAddress(internal.PolygonNegRiskExchange),
		common.HexToAddress(internal.PolygonConditionalTokens),
		common.HexToAddress(internal.PolygonNegRiskAdapter),
		common.HexToAddress(internal.PolygonProxyFactory),
	}
}

// buildTopicFilterQueries 根据监控地址生成「仅推送与这些地址相关」的 FilterQuery 列表，供服务端过滤以降低流量。
// Transfer/Approval: topic1=from, topic2=to；TransferSingle: topic2=from, topic3=to。需分别订阅 from 侧与 to 侧。
// 若仅做仓位追踪，可进一步只订阅 Collateral + ConditionalTokens + NegRiskAdapter（见 polymarketContractAddresses）以再减流量。
func (c *chainWSClient) buildTopicFilterQueries(addrs []common.Address) []ethereum.FilterQuery {
	if len(addrs) == 0 || len(addrs) > maxWatchAddrsForTopicFilter {
		return nil
	}
	addrHashes := make([]common.Hash, 0, len(addrs))
	for _, a := range addrs {
		addrHashes = append(addrHashes, common.BytesToHash(common.LeftPadBytes(a.Bytes(), 32)))
	}
	contracts := c.contractAddrs
	return []ethereum.FilterQuery{
		{Addresses: contracts, Topics: [][]common.Hash{{transferEventSig}, addrHashes, nil}},           // Transfer, topic1=from
		{Addresses: contracts, Topics: [][]common.Hash{{transferEventSig}, nil, addrHashes}},           // Transfer, topic2=to
		{Addresses: contracts, Topics: [][]common.Hash{{approvalEventSig}, addrHashes, nil}},            // Approval, topic1=owner
		{Addresses: contracts, Topics: [][]common.Hash{{approvalEventSig}, nil, addrHashes}},           // Approval, topic2=spender
		{Addresses: contracts, Topics: [][]common.Hash{{transferSingleEventSig}, nil, addrHashes, nil}}, // TransferSingle, topic2=from
		{Addresses: contracts, Topics: [][]common.Hash{{transferSingleEventSig}, nil, nil, addrHashes}}, // TransferSingle, topic3=to
	}
}

func (c *chainWSClient) SetOnLog(callback func(*ethtypes.Log)) {
	c.onLog = callback
}

func (c *chainWSClient) SetOnBlock(callback func(uint64)) {
	c.onBlock = callback
}

func (c *chainWSClient) WatchAddresses(addrs []types.EthAddress) {
	c.watchAddrsMu.Lock()
	defer c.watchAddrsMu.Unlock()
	c.watchAddrs = make([]common.Address, 0, len(addrs))
	for _, a := range addrs {
		if a == "" {
			continue
		}
		c.watchAddrs = append(c.watchAddrs, common.HexToAddress(string(a)))
	}
}

func (c *chainWSClient) IsRunning() bool {
	c.runningMu.RLock()
	defer c.runningMu.RUnlock()
	return c.running
}

// isLogRelatedToWatchAddress 判断日志是否与监控地址相关（如 Transfer/Approval 的 from/to）
func (c *chainWSClient) isLogRelatedToWatchAddress(log *ethtypes.Log) bool {
	c.watchAddrsMu.RLock()
	addrs := c.watchAddrs
	c.watchAddrsMu.RUnlock()
	if len(addrs) == 0 {
		return true
	}
	if len(log.Topics) < 1 {
		return false
	}
	switch log.Topics[0] {
	case transferEventSig, approvalEventSig:
		// Transfer/Approval: topic1 = from, topic2 = to (indexed)
		for _, addr := range addrs {
			addrHash := common.BytesToHash(common.LeftPadBytes(addr.Bytes(), 32))
			if len(log.Topics) >= 2 && log.Topics[1] == addrHash {
				return true
			}
			if len(log.Topics) >= 3 && log.Topics[2] == addrHash {
				return true
			}
		}
		return false
	case transferSingleEventSig:
		// TransferSingle: topic2 = from, topic3 = to (indexed)
		for _, addr := range addrs {
			addrHash := common.BytesToHash(common.LeftPadBytes(addr.Bytes(), 32))
			if len(log.Topics) >= 3 && log.Topics[2] == addrHash {
				return true
			}
			if len(log.Topics) >= 4 && log.Topics[3] == addrHash {
				return true
			}
		}
		return false
	default:
		// 其他事件：若未指定监控地址则全部推送；若指定了则仅当 data 或 topics 中含监控地址时推送（此处简化：全部推送）
		if len(addrs) == 0 {
			return true
		}
		// 简单策略：未知事件且监听了地址时，只推送已知事件；未知事件不推送，避免噪音
		return false
	}
}

func (c *chainWSClient) Start(ctx context.Context) error {
	c.runningMu.Lock()
	if c.running {
		c.runningMu.Unlock()
		return nil
	}
	c.running = true
	c.runningMu.Unlock()

	client, err := ethclient.Dial(c.wssURL)
	if err != nil {
		c.runningMu.Lock()
		c.running = false
		c.runningMu.Unlock()
		return err
	}
	c.client = client
	c.logCh = make(chan ethtypes.Log, 64)
	c.errCh = make(chan error, 8)

	c.watchAddrsMu.RLock()
	watchAddrs := make([]common.Address, len(c.watchAddrs))
	copy(watchAddrs, c.watchAddrs)
	c.watchAddrsMu.RUnlock()

	queries := c.buildTopicFilterQueries(watchAddrs)
	if len(queries) == 0 {
		client.Close()
		c.runningMu.Lock()
		c.running = false
		c.runningMu.Unlock()
		return ErrWatchAddrsRequired
	}
	// 服务端按 topic 过滤，只推送与 watchAddrs 相关的日志，控制流量
	for _, q := range queries {
		sub, err := client.SubscribeFilterLogs(ctx, q, c.logCh)
		if err != nil {
			for _, s := range c.subLogsList {
				s.Unsubscribe()
			}
			client.Close()
			c.runningMu.Lock()
			c.running = false
			c.runningMu.Unlock()
			return err
		}
		c.subLogsList = append(c.subLogsList, sub)
		go func(s ethereum.Subscription) {
			if e := <-s.Err(); e != nil {
				select {
				case c.errCh <- e:
				default:
				}
			}
		}(sub)
	}

	if c.onBlock != nil {
		headCh := make(chan *ethtypes.Header, 8)
		subHead, err := client.SubscribeNewHead(ctx, headCh)
		if err != nil {
			for _, s := range c.subLogsList {
				s.Unsubscribe()
			}
			client.Close()
			c.runningMu.Lock()
			c.running = false
			c.runningMu.Unlock()
			return err
		}
		c.subHead = subHead
		go c.runHeadLoop(headCh)
	}

	go c.runLogLoop()
	go c.runReconnect(ctx)
	return nil
}

func (c *chainWSClient) runLogLoop() {
	for {
		select {
		case <-c.stopChan:
			return
		case err := <-c.errCh:
			if err != nil {
				// 连接错误，由 runReconnect 处理
				return
			}
		case vLog, ok := <-c.logCh:
			if !ok {
				return
			}
			if !c.isLogRelatedToWatchAddress(&vLog) {
				continue
			}
			if c.onLog != nil {
				c.onLog(&vLog)
			}
		}
	}
}

func (c *chainWSClient) runHeadLoop(headCh chan *ethtypes.Header) {
	for {
		select {
		case <-c.stopChan:
			return
		case h, ok := <-headCh:
			if !ok {
				return
			}
			if c.onBlock != nil && h != nil {
				c.onBlock(h.Number.Uint64())
			}
		}
	}
}

func (c *chainWSClient) runReconnect(ctx context.Context) {
	<-c.stopChan
	// 可选：在此实现断线重连逻辑（当前 Stop 后不自动重连）
}

func (c *chainWSClient) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
		c.runningMu.Lock()
		c.running = false
		c.runningMu.Unlock()
		for _, s := range c.subLogsList {
			if s != nil {
				s.Unsubscribe()
			}
		}
		c.subLogsList = nil
		if c.subHead != nil {
			c.subHead.Unsubscribe()
			c.subHead = nil
		}
		if c.client != nil {
			c.client.Close()
			c.client = nil
		}
	})
}
