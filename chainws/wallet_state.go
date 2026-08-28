// 钱包仓位跟踪：初始化传入钱包地址，拉取 token/初始仓位后通过 WSS 持续同步；对外仅暴露 GetPositions() 返回一致性 map。

package chainws

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymas/go-polymarket-sdk/data"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// PositionKeyUSDC 表示 USDC 仓位的 map key（与 CT 一致，使用 tokenID：此处为 Collateral 合约地址）
var PositionKeyUSDC = internal.PolygonCollateral

// BalanceReader 读取链上旧 USDC.e 与 CT 余额。若用 web3.Client 可实现：GetUSDCBalance(addr)=client.GetUSDCBalance(addr)，GetTokenBalance(addr, tokenID)=client.GetTokenBalance(tokenID, addr)。
// V2 交易抵押物监控应使用 web3.Client.GetCollateralBalance（pUSD），不应将本 legacy reader 当作下单余额。
type BalanceReader interface {
	GetUSDCBalance(addr types.EthAddress) (float64, error)
	GetTokenBalance(addr types.EthAddress, tokenID string) (float64, error)
}

// Tracker 维护指定钱包的 USDC + token 仓位：启动时拉取初值，WSS 增量更新；仅通过 GetPositions 暴露一致快照。
type Tracker struct {
	watchAddr types.EthAddress
	reader    BalanceReader
	dataClient data.Client
	ws        Client

	mu            sync.RWMutex
	usdcBalance   float64
	tokenBalances map[string]float64
	snapshot      map[string]float64 // 最近一次拷贝的 map，无变更时直接返回同一引用
	dirty         bool               // 有写后为 true，下次 GetPositions 时拷贝并置 false
}

// NewTracker 创建跟踪器，传入钱包地址及用于拉取初值的 reader、dataClient；chainID/wssURL 用于 WSS。
func NewTracker(
	chainID types.ChainID,
	wssURL string,
	watchAddr types.EthAddress,
	reader BalanceReader,
	dataClient data.Client,
) (*Tracker, error) {
	ws, err := NewClientWithAddresses(chainID, wssURL, []types.EthAddress{watchAddr})
	if err != nil {
		return nil, err
	}
	return &Tracker{
		watchAddr:     watchAddr,
		reader:        reader,
		dataClient:   dataClient,
		ws:            ws,
		tokenBalances: make(map[string]float64),
	}, nil
}

// Start 拉取初始仓位后启动 WSS，持续根据链上事件更新仓位。
func (t *Tracker) Start(ctx context.Context) error {
	usdc, err := t.reader.GetUSDCBalance(t.watchAddr)
	if err != nil {
		return err
	}
	positions, err := t.dataClient.GetPositions(t.watchAddr)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.usdcBalance = usdc
	for _, p := range positions {
		if p.TokenID != "" && p.Size > 0 {
			t.tokenBalances[p.TokenID] = p.Size
		}
	}
	t.dirty = true
	t.mu.Unlock()

	watchCommon := common.HexToAddress(string(t.watchAddr))
	t.ws.SetOnLog(func(log *ethtypes.Log) {
		amount := float64(0)
		tokenID := ""
		switch EventKindOf(log) {
		case EventTransfer:
			info := ParseTransfer(log)
			if info == nil || info.ContractName != "USDC" {
				return
			}
			amount = rawToAmount(info.Value)
			if info.To != watchCommon && info.From != watchCommon {
				return
			}
			t.mu.Lock()
			if info.To == watchCommon {
				t.usdcBalance += amount
			} else {
				t.usdcBalance -= amount
			}
			t.dirty = true
			t.mu.Unlock()
			return
		case EventTransferSingle:
			info := ParseTransferSingle(log)
			if info == nil {
				return
			}
			amount = rawToAmount(info.Value)
			tokenID = info.TokenID.String()
			if info.To != watchCommon && info.From != watchCommon {
				return
			}
			t.mu.Lock()
			if info.To == watchCommon {
				t.tokenBalances[tokenID] += amount
			} else {
				t.tokenBalances[tokenID] -= amount
			}
			if t.tokenBalances[tokenID] <= 0 {
				delete(t.tokenBalances, tokenID)
			}
			t.dirty = true
			t.mu.Unlock()
		}
	})
	return t.ws.Start(ctx)
}

// Stop 停止 WSS，不再更新仓位。
func (t *Tracker) Stop() {
	t.ws.Stop()
}

// GetPositions 返回仓位 map 的拷贝；仅当内部数据更新时才做新拷贝，未更新时返回与上次相同的 map 引用。key 均为 tokenID（USDC 为 Collateral 合约地址，CT 为 outcome token ID）。请勿修改返回的 map。
func (t *Tracker) GetPositions() map[string]float64 {
	t.mu.RLock()
	if !t.dirty && t.snapshot != nil {
		m := t.snapshot
		t.mu.RUnlock()
		return m
	}
	t.mu.RUnlock()

	t.mu.Lock()
	if !t.dirty && t.snapshot != nil {
		m := t.snapshot
		t.mu.Unlock()
		return m
	}
	out := make(map[string]float64, 1+len(t.tokenBalances))
	out[PositionKeyUSDC] = t.usdcBalance
	for k, v := range t.tokenBalances {
		out[k] = v
	}
	t.snapshot = out
	t.dirty = false
	t.mu.Unlock()
	return t.snapshot
}

func rawToAmount(raw *big.Int) float64 {
	if raw == nil {
		return 0
	}
	f := new(big.Float).SetInt(raw)
	f.Quo(f, big.NewFloat(1e6))
	v, _ := f.Float64()
	return v
}
