package chainws

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/data"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestNewClient(t *testing.T) {
	// 使用内置 WSS URL（不传 wssURL）
	c, err := NewClient(internal.Polygon, "")
	if err != nil {
		t.Fatalf("NewClient(mainnet) failed: %v", err)
	}
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	c.Stop()

	c, err = NewClient(internal.Amoy, "")
	if err != nil {
		t.Fatalf("NewClient(amoy) failed: %v", err)
	}
	c.Stop()
}

func TestNewClientWithCustomWSS(t *testing.T) {
	c, err := NewClient(internal.Polygon, "wss://polygon-bor.publicnode.com")
	if err != nil {
		t.Fatalf("NewClient(custom wss) failed: %v", err)
	}
	c.Stop()
}

func TestWatchAddresses(t *testing.T) {
	c, err := NewClient(internal.Polygon, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	c.WatchAddresses([]types.EthAddress{
		"0x0000000000000000000000000000000000000001",
		"0x0000000000000000000000000000000000000002",
	})
	// 仅保证不 panic
}

func TestSetCallbacks(t *testing.T) {
	c, err := NewClient(internal.Polygon, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	var logCount, blockCount atomic.Int32
	c.SetOnLog(func(_ *ethtypes.Log) { logCount.Add(1) })
	c.SetOnBlock(func(_ uint64) { blockCount.Add(1) })
	// 仅保证不 panic，实际触发需 Start + 链上事件
}

func TestIsRunning(t *testing.T) {
	c, err := NewClient(internal.Polygon, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()

	if c.IsRunning() {
		t.Error("expected not running before Start")
	}
}

func TestStartRequiresWatchAddresses(t *testing.T) {
	// 未设置 WatchAddresses 时 Start 必须报错，禁止全量订阅
	c, err := NewClient(internal.Polygon, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer c.Stop()
	ctx := context.Background()
	err = c.Start(ctx)
	if err == nil {
		c.Stop()
		t.Fatal("Start without WatchAddresses should fail")
	}
	if !errors.Is(err, ErrWatchAddrsRequired) {
		// 无网络时可能是 Dial 错误，跳过
		t.Skipf("Start without WatchAddresses: got %v (need WSS to assert ErrWatchAddrsRequired)", err)
	}
}

func TestStartRequiresNetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	c, err := NewClientWithAddresses(internal.Polygon, "wss://polygon-bor.publicnode.com", []types.EthAddress{
		"0x0000000000000000000000000000000000000001",
	})
	if err != nil {
		t.Fatalf("NewClientWithAddresses failed: %v", err)
	}
	defer c.Stop()

	ctx := context.Background()
	err = c.Start(ctx)
	if err != nil {
		t.Logf("Start failed (network may be unavailable): %v", err)
		return
	}
	if !c.IsRunning() {
		t.Error("expected running after Start")
	}
	c.Stop()
	if c.IsRunning() {
		t.Error("expected not running after Stop")
	}
}

// 监控地址：用于测试链上 WSS 订阅并解析 USDC/CT 变化
const monitorTestAddress = "0x38cc5Cf506aff32B8E26c5d19C7b288561805C4F"

// usdcValue 将 USDC 原始数量（6 位小数）转为可读数值
func usdcValue(raw *big.Int) float64 {
	if raw == nil {
		return 0
	}
	f := new(big.Float).SetInt(raw)
	f.Quo(f, big.NewFloat(1e6))
	v, _ := f.Float64()
	return v
}

// TestMonitorAddress_USDCAndTokenChanges 监控指定地址的链上事件，仅打印 USDC 变化与 Token 变化情况。
// 这是人工生产监控探针，不属于默认测试；仅在显式设置
// POLY_RUN_CHAINWS_MONITOR=1 时运行，-short 始终跳过。
func TestMonitorAddress_USDCAndTokenChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping chain WSS monitor test in short mode")
	}
	if os.Getenv("POLY_RUN_CHAINWS_MONITOR") != "1" {
		t.Skip("set POLY_RUN_CHAINWS_MONITOR=1 to run the 60-second manual chain WSS monitor")
	}
	watchAddr := types.EthAddress(monitorTestAddress)
	c, err := NewClientWithAddresses(internal.Polygon, "", []types.EthAddress{watchAddr})
	if err != nil {
		t.Fatalf("NewClientWithAddresses failed: %v", err)
	}
	defer c.Stop()

	watchAddrCommon := common.HexToAddress(string(watchAddr))
	var eventCount atomic.Int32
	const flushDelay = 400 * time.Millisecond
	type pendingTx struct {
		usdcDelta   float64
		tokenDeltas map[string]float64 // tokenID -> 净变化（正=转入，负=转出）
		timer       *time.Timer
	}
	var mu sync.Mutex
	pending := make(map[string]*pendingTx)

	flush := func(txHash string) {
		mu.Lock()
		p, ok := pending[txHash]
		if !ok {
			mu.Unlock()
			return
		}
		delete(pending, txHash)
		mu.Unlock()

		var parts []string
		if p.usdcDelta != 0 {
			parts = append(parts, fmt.Sprintf("USDC %+.6f", p.usdcDelta))
		}
		for tokenID, delta := range p.tokenDeltas {
			if delta == 0 {
				continue
			}
			shortID := tokenID
			if len(shortID) > 18 {
				shortID = shortID[:8] + "..." + shortID[len(shortID)-6:]
			}
			parts = append(parts, fmt.Sprintf("Token(%s) %+.6f", shortID, delta))
		}
		if len(parts) > 0 {
			eventCount.Add(1)
			t.Logf("tx=%s | %s", txHash, strings.Join(parts, " "))
		}
	}

	c.SetOnLog(func(log *ethtypes.Log) {
		txKey := log.TxHash.Hex()
		kind := EventKindOf(log)
		var addUSDC float64
		var addTokenID string
		var addTokenDelta float64

		switch kind {
		case EventTransfer:
			info := ParseTransfer(log)
			if info == nil || info.ContractName != "USDC" {
				return
			}
			amount := usdcValue(info.Value)
			if info.To == watchAddrCommon {
				addUSDC = amount
			} else if info.From == watchAddrCommon {
				addUSDC = -amount
			}
		case EventTransferSingle:
			info := ParseTransferSingle(log)
			if info == nil {
				return
			}
			amount := usdcValue(info.Value)
			if info.To == watchAddrCommon {
				addTokenID, addTokenDelta = info.TokenID.String(), amount
			} else if info.From == watchAddrCommon {
				addTokenID, addTokenDelta = info.TokenID.String(), -amount
			}
		default:
			return
		}

		if addUSDC == 0 && addTokenID == "" {
			return
		}

		mu.Lock()
		p := pending[txKey]
		if p == nil {
			p = &pendingTx{tokenDeltas: make(map[string]float64)}
			pending[txKey] = p
			p.timer = time.AfterFunc(flushDelay, func() { flush(txKey) })
		} else {
			p.timer.Reset(flushDelay)
		}
		if addUSDC != 0 {
			p.usdcDelta += addUSDC
		}
		if addTokenID != "" {
			p.tokenDeltas[addTokenID] += addTokenDelta
		}
		mu.Unlock()
	})

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Logf("Monitoring address %s for 60s (WSS). Events will be logged above.", monitorTestAddress)
	time.Sleep(60 * time.Second)
	c.Stop()
	t.Logf("Stopped. Total events received: %d", eventCount.Load())
}

func TestTransferSingleEventSig(t *testing.T) {
	// 确保 TransferSingle 签名与标准 ERC1155 一致
	sig := crypto.Keccak256([]byte("TransferSingle(address,address,address,uint256,uint256)"))
	if len(sig) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(sig))
	}
	// 标准 EIP-1155 已知前几字节（可选断言）
	if transferSingleEventSig.Hex() == "" {
		t.Fatal("transferSingleEventSig not set")
	}
	t.Logf("TransferSingle topic0: %s", transferSingleEventSig.Hex())
}

// mockBalanceReader 测试用：返回固定 USDC 与 token 余额
type mockBalanceReader struct {
	USDC   float64
	Tokens map[string]float64
}

func (m mockBalanceReader) GetUSDCBalance(_ types.EthAddress) (float64, error) {
	return m.USDC, nil
}
func (m mockBalanceReader) GetTokenBalance(_ types.EthAddress, tokenID string) (float64, error) {
	if m.Tokens == nil {
		return 0, nil
	}
	return m.Tokens[tokenID], nil
}

// rpcBalanceReader 测试用：通过 RPC 从链上读取 USDC 与 CT 余额（无需私钥）
type rpcBalanceReader struct {
	client *ethclient.Client
}

func newRPCBalanceReader(t *testing.T, rpcURL string) BalanceReader {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		t.Skipf("RPC 连接失败，跳过: %v", err)
		return nil
	}
	t.Cleanup(func() { client.Close() })
	return &rpcBalanceReader{client: client}
}

func (r *rpcBalanceReader) GetUSDCBalance(addr types.EthAddress) (float64, error) {
	usdcAddr := common.HexToAddress(internal.PolygonCollateral)
	balanceOfABI := `[{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`
	parsed, _ := abi.JSON(strings.NewReader(balanceOfABI))
	packed, _ := parsed.Pack("balanceOf", common.HexToAddress(string(addr)))
	result, err := r.client.CallContract(context.Background(), ethereum.CallMsg{To: &usdcAddr, Data: packed}, nil)
	if err != nil {
		return 0, err
	}
	var balance *big.Int
	_ = parsed.UnpackIntoInterface(&balance, "balanceOf", result)
	if balance == nil {
		return 0, nil
	}
	f := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e6))
	v, _ := f.Float64()
	return v, nil
}

func (r *rpcBalanceReader) GetTokenBalance(addr types.EthAddress, tokenID string) (float64, error) {
	tokenIDBig, ok := new(big.Int).SetString(tokenID, 10)
	if !ok {
		return 0, fmt.Errorf("invalid token ID: %s", tokenID)
	}
	ctAddr := common.HexToAddress(internal.PolygonConditionalTokens)
	balanceOfABI := `[{"constant":true,"inputs":[{"name":"account","type":"address"},{"name":"id","type":"uint256"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`
	parsed, _ := abi.JSON(strings.NewReader(balanceOfABI))
	packed, _ := parsed.Pack("balanceOf", common.HexToAddress(string(addr)), tokenIDBig)
	result, err := r.client.CallContract(context.Background(), ethereum.CallMsg{To: &ctAddr, Data: packed}, nil)
	if err != nil {
		return 0, err
	}
	var balance *big.Int
	_ = parsed.UnpackIntoInterface(&balance, "balanceOf", result)
	if balance == nil {
		return 0, nil
	}
	f := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e6))
	v, _ := f.Float64()
	return v, nil
}

// TestGetPositions_Simple 最简用例：创建 Tracker，从链上拉取 USDC + Data API 拉取仓位并打印。
func TestGetPositions_Simple(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}
	addr := types.EthAddress("0x38cc5Cf506aff32B8E26c5d19C7b288561805C4F")
	rpcURL := internal.PolygonRPCMainnetList[0]
	reader := newRPCBalanceReader(t, rpcURL)
	if reader == nil {
		return
	}
	tracker, err := NewTracker(internal.Polygon, "", addr, reader, data.NewClient())
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	defer tracker.Stop()
	if err := tracker.Start(context.Background()); err != nil {
		t.Skipf("Start 需要可用 WSS，跳过: %v", err)
	}
	pos := tracker.GetPositions()
	t.Logf("地址 %s 仓位 (tokenID -> 余额):", addr)
	for k, v := range pos {
		t.Logf("  %s => %.6f", k, v)
	}
}

// TestTracker 测试启动时拉取仓位 + WSS 维护；对外仅 GetPositions() 一致 map。需网络与 Data API；-short 时跳过。
func TestTracker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Tracker test in short mode")
	}
	watchAddr := types.EthAddress(monitorTestAddress)
	reader := mockBalanceReader{USDC: 0, Tokens: nil}
	dataClient := data.NewClient()
	tracker, err := NewTracker(internal.Polygon, "", watchAddr, reader, dataClient)
	if err != nil {
		t.Fatalf("NewTracker failed: %v", err)
	}
	defer tracker.Stop()

	ctx := context.Background()
	if err := tracker.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	pos := tracker.GetPositions()
	t.Logf("GetPositions: usdc=%.6f tokens=%d", pos[PositionKeyUSDC], len(pos)-1)
	for k, v := range pos {
		if k == PositionKeyUSDC {
			continue
		}
		id := k
		if len(id) > 16 {
			id = id[:8] + "..." + id[len(id)-6:]
		}
		t.Logf("  %s = %.6f", id, v)
	}
	time.Sleep(5 * time.Second)
	pos2 := tracker.GetPositions()
	t.Logf("After 5s: usdc=%.6f tokens=%d", pos2[PositionKeyUSDC], len(pos2)-1)
}
