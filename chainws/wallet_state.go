// 钱包仓位跟踪：用 RPC/Data API 快照初始化，再通过 WSS 增量同步当前 V2 资产。

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

var (
	// PositionKeyPUSD is the current V2 trading collateral balance key.
	PositionKeyPUSD = internal.PolygonPUSD
	// PositionKeyUSDCE is the bridged USDC.e balance key.
	PositionKeyUSDCE = internal.PolygonCollateral
	// PositionKeyUSDC is retained for source compatibility and means USDC.e.
	// Deprecated: use PositionKeyUSDCE.
	PositionKeyUSDC = PositionKeyUSDCE
)

// BalanceReader reads exact on-chain balances used to reconcile the tracker.
type BalanceReader interface {
	GetCollateralBalance(addr types.EthAddress) (float64, error)
	GetUSDCBalance(addr types.EthAddress) (float64, error)
	GetTokenBalance(addr types.EthAddress, tokenID string) (float64, error)
}

// Tracker maintains separate pUSD, USDC.e and ERC-1155 position balances.
type Tracker struct {
	watchAddr  types.EthAddress
	reader     BalanceReader
	dataClient data.Client
	ws         Client

	mu             sync.RWMutex
	pusdBalance    float64
	usdceBalance   float64
	tokenBalances  map[string]float64
	snapshot       map[string]float64 // 最近一次拷贝的 map，无变更时直接返回同一引用
	dirty          bool               // 有写后为 true，下次 GetPositions 时拷贝并置 false
	needsReconcile bool
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
		dataClient:    dataClient,
		ws:            ws,
		tokenBalances: make(map[string]float64),
	}, nil
}

// Start reconciles a full snapshot, then starts applying WSS deltas.
func (t *Tracker) Start(ctx context.Context) error {
	if err := t.Reconcile(ctx); err != nil {
		return err
	}
	t.ws.SetOnLog(t.applyLog)
	return t.ws.Start(ctx)
}

// Reconcile replaces incremental state with an authoritative RPC/Data API snapshot.
// Call it after NeedsReconcile reports true, for example after a removed log/reorg.
func (t *Tracker) Reconcile(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pusd, err := t.reader.GetCollateralBalance(t.watchAddr)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	usdce, err := t.reader.GetUSDCBalance(t.watchAddr)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	positions, err := t.dataClient.GetPositions(t.watchAddr)
	if err != nil {
		return err
	}
	tokenIDs := make(map[string]struct{}, len(positions))
	for _, position := range positions {
		if position.TokenID != "" {
			tokenIDs[position.TokenID] = struct{}{}
		}
	}
	t.mu.RLock()
	for tokenID := range t.tokenBalances {
		tokenIDs[tokenID] = struct{}{}
	}
	t.mu.RUnlock()
	tokens := make(map[string]float64, len(tokenIDs))
	for tokenID := range tokenIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		balance, err := t.reader.GetTokenBalance(t.watchAddr, tokenID)
		if err != nil {
			return err
		}
		if balance > 0 {
			tokens[tokenID] = balance
		}
	}
	t.mu.Lock()
	t.pusdBalance = pusd
	t.usdceBalance = usdce
	t.tokenBalances = tokens
	t.needsReconcile = false
	t.dirty = true
	t.mu.Unlock()
	return nil
}

// NeedsReconcile reports whether a removed log/reorg was observed.
func (t *Tracker) NeedsReconcile() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.needsReconcile
}

func (t *Tracker) applyLog(log *ethtypes.Log) {
	watch := common.HexToAddress(string(t.watchAddr))
	direction := func(from, to common.Address) float64 {
		delta := float64(0)
		if to == watch {
			delta++
		}
		if from == watch {
			delta--
		}
		if log.Removed {
			delta = -delta
		}
		return delta
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	changed := false
	switch EventKindOf(log) {
	case EventTransfer:
		info := ParseTransfer(log)
		if info == nil {
			return
		}
		delta := direction(info.From, info.To) * rawToAmount(info.Value)
		if delta == 0 {
			return
		}
		switch info.Contract {
		case common.HexToAddress(internal.PolygonPUSD):
			t.pusdBalance += delta
			changed = true
		case common.HexToAddress(internal.PolygonCollateral):
			t.usdceBalance += delta
			changed = true
		}
	case EventTransferSingle:
		info := ParseTransferSingle(log)
		if info == nil || log.Address != common.HexToAddress(internal.PolygonConditionalTokens) {
			return
		}
		delta := direction(info.From, info.To) * rawToAmount(info.Value)
		changed = t.applyTokenDelta(info.TokenID.String(), delta)
	case EventTransferBatch:
		info := ParseTransferBatch(log)
		if info == nil || log.Address != common.HexToAddress(internal.PolygonConditionalTokens) {
			return
		}
		factor := direction(info.From, info.To)
		for i, tokenID := range info.TokenIDs {
			if t.applyTokenDelta(tokenID.String(), factor*rawToAmount(info.Values[i])) {
				changed = true
			}
		}
	}
	if changed {
		t.dirty = true
	}
	if log.Removed {
		t.needsReconcile = true
	}
}

func (t *Tracker) applyTokenDelta(tokenID string, delta float64) bool {
	if delta == 0 {
		return false
	}
	t.tokenBalances[tokenID] += delta
	if t.tokenBalances[tokenID] <= 0 {
		delete(t.tokenBalances, tokenID)
	}
	return true
}

// Stop 停止 WSS，不再更新仓位。
func (t *Tracker) Stop() {
	t.ws.Stop()
}

// GetPositions returns pUSD, USDC.e and outcome balances keyed by their contract/token IDs.
// Do not modify the returned map.
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
	out := make(map[string]float64, 2+len(t.tokenBalances))
	out[PositionKeyPUSD] = t.pusdBalance
	out[PositionKeyUSDCE] = t.usdceBalance
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
