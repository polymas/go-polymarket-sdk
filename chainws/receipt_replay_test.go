package chainws

import (
	"encoding/hex"
	"math"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	replayWallet = "0xc789151b4dd1f4fd16044742ab17aaa895ae10fd"
	tradeToken   = "202f0ae64a10621d06e09cf357d9b846de2925ae17da891fc10fafb77ca29b22"
	splitToken0  = "0e096da543f9543dc1522ae26de826871df36797b9f7b969c3c3889c4ed5ba58"
	splitToken1  = "8014489b7ca0136d5b14284bf866fc683e72131b77a30aba5b4042de3004d7e7"
)

func TestReceiptReplayBuySell(t *testing.T) {
	tracker := replayTracker(10)
	// Production BUY 0xb19313996c80ecdb6eadbf56de3ba048d83dd8f97e1bcc0d1447d338b81fe2ef.
	tracker.applyLog(erc20ReplayLog(internal.PolygonPUSD, replayWallet, internal.PolygonExchangeV2, "283e94"))
	tracker.applyLog(singleReplayLog(internal.PolygonExchangeV2, "0x0000000000000000000000000000000000000000", replayWallet, tradeToken, "4c4b40"))
	positions := tracker.GetPositions()
	closeEnough(t, positions[PositionKeyPUSD], 7.36254)
	closeEnough(t, positions[decimalTokenID(tradeToken)], 5)

	// Production SELL 0xda794ea7f7f54225d074fd7656f119940b7d63ecf6e6288e7c78629a4dab86d3.
	tracker.applyLog(singleReplayLog(internal.PolygonExchangeV2, replayWallet, "0x4d8bc628487bbc9931b4d039e6a7529b8ae1a00d", tradeToken, "4c4b40"))
	tracker.applyLog(erc20ReplayLog(internal.PolygonPUSD, internal.PolygonExchangeV2, replayWallet, "24cfd4"))
	positions = tracker.GetPositions()
	closeEnough(t, positions[PositionKeyPUSD], 9.77504)
	if _, ok := positions[decimalTokenID(tradeToken)]; ok {
		t.Fatal("sold position was not removed")
	}
}

func TestReceiptReplaySplitMerge(t *testing.T) {
	tracker := replayTracker(10)
	data := batchData(splitToken0, splitToken1, "2710", "2710")
	// Production SPLIT 0x4c6d98e4bd7d4c4f2214e084ae2f540616709059164903d80aabef70c0233c1d.
	tracker.applyLog(erc20ReplayLog(internal.PolygonPUSD, replayWallet, internal.PolygonPUSD, "2710"))
	tracker.applyLog(batchReplayLog(internal.PolygonNegRiskCtfCollateralAdapter, internal.PolygonNegRiskCtfCollateralAdapter, replayWallet, data))
	positions := tracker.GetPositions()
	closeEnough(t, positions[PositionKeyPUSD], 9.99)
	closeEnough(t, positions[decimalTokenID(splitToken0)], 0.01)
	closeEnough(t, positions[decimalTokenID(splitToken1)], 0.01)

	// Production MERGE 0x08c139937a95f7221d0b96b5bb0a283f7a1c953c46f28bd0f0beaf39464b4401.
	tracker.applyLog(batchReplayLog(internal.PolygonNegRiskCtfCollateralAdapter, replayWallet, internal.PolygonNegRiskCtfCollateralAdapter, data))
	tracker.applyLog(erc20ReplayLog(internal.PolygonPUSD, "0x0000000000000000000000000000000000000000", replayWallet, "2710"))
	positions = tracker.GetPositions()
	closeEnough(t, positions[PositionKeyPUSD], 10)
	if len(positions) != 2 {
		t.Fatalf("split+merge left outcome balances: %#v", positions)
	}
}

func TestReceiptReplayRedeem(t *testing.T) {
	const (
		losingToken  = "bf63d515ca68023ba6e8462e4c319bed522b6c3ccc54b84843adfe76ba9c736b"
		winningToken = "a010044ae361b794e3ea9570243addd324eb1aa61348f7ef6d82a7c2a7b3286a"
	)
	tracker := replayTracker(10)
	tracker.tokenBalances[decimalTokenID(winningToken)] = 5
	// Final redemption in production receipt
	// 0x80b7dd9f3361bed8e5bb44eb3efe4b026a4d7725b5fe404be604349c4d99047d.
	tracker.applyLog(batchReplayLog(internal.PolymarketAutoClaimer, replayWallet, internal.PolymarketAutoClaimer, batchData(losingToken, winningToken, "00", "4c4b40")))
	tracker.applyLog(erc20ReplayLog(internal.PolygonPUSD, "0x0000000000000000000000000000000000000000", replayWallet, "4c4b40"))
	positions := tracker.GetPositions()
	closeEnough(t, positions[PositionKeyPUSD], 15)
	if _, ok := positions[decimalTokenID(winningToken)]; ok {
		t.Fatal("redeemed position was not removed")
	}
}

func TestReceiptReplayWrapUnwrap(t *testing.T) {
	// Production wrap 0xde01c5c2578db1a36c7d002fcc9c75bd46724db59cb83d391509427e7e6d906d.
	tracker := replayTracker(0)
	tracker.usdceBalance = 10
	tracker.applyLog(erc20ReplayLog(internal.PolygonCollateral, replayWallet, internal.PolygonPUSD, "989680"))
	tracker.applyLog(erc20ReplayLog(internal.PolygonPUSD, "0x0000000000000000000000000000000000000000", replayWallet, "989680"))
	positions := tracker.GetPositions()
	closeEnough(t, positions[PositionKeyUSDCE], 0)
	closeEnough(t, positions[PositionKeyPUSD], 10)

	// Production unwrap 0x4b6894e01792af8d13babf3130f0abe97a75153e73fa2f8eac024a36b3384f5c.
	tracker = replayTracker(310)
	tracker.applyLog(erc20ReplayLog(internal.PolygonPUSD, replayWallet, internal.PolygonPUSD, "127a3980"))
	tracker.applyLog(erc20ReplayLog(internal.PolygonCollateral, "0xc417fd8e9661c0d2120b64a04bb3278c17e99db1", replayWallet, "127a3980"))
	positions = tracker.GetPositions()
	closeEnough(t, positions[PositionKeyPUSD], 0)
	closeEnough(t, positions[PositionKeyUSDCE], 310)
}

func TestRemovedLogReversesDeltaAndRequiresReconcile(t *testing.T) {
	tracker := replayTracker(10)
	log := erc20ReplayLog(internal.PolygonPUSD, replayWallet, internal.PolygonExchangeV2, "2710")
	tracker.applyLog(log)
	removed := *log
	removed.Removed = true
	tracker.applyLog(&removed)
	closeEnough(t, tracker.GetPositions()[PositionKeyPUSD], 10)
	if !tracker.NeedsReconcile() {
		t.Fatal("removed log did not mark snapshot stale")
	}
}

func TestParseTransferBatchRejectsMalformedData(t *testing.T) {
	log := batchReplayLog("0x1", "0x2", "0x3", []byte{1, 2, 3})
	if ParseTransferBatch(log) != nil {
		t.Fatal("malformed TransferBatch should not parse")
	}
}

func replayTracker(pusd float64) *Tracker {
	return &Tracker{
		watchAddr:     types.EthAddress(replayWallet),
		pusdBalance:   pusd,
		tokenBalances: make(map[string]float64),
		dirty:         true,
	}
}

func addressTopic(address string) common.Hash { return common.HexToHash(address) }

func erc20ReplayLog(contract, from, to, amount string) *ethtypes.Log {
	return &ethtypes.Log{
		Address: common.HexToAddress(contract),
		Topics:  []common.Hash{transferEventSig, addressTopic(from), addressTopic(to)},
		Data:    common.HexToHash(amount).Bytes(),
	}
}

func singleReplayLog(operator, from, to, tokenID, amount string) *ethtypes.Log {
	data, _ := hex.DecodeString(tokenID + strings.Repeat("0", 64-len(amount)) + amount)
	return &ethtypes.Log{
		Address: common.HexToAddress(internal.PolygonConditionalTokens),
		Topics:  []common.Hash{transferSingleEventSig, addressTopic(operator), addressTopic(from), addressTopic(to)},
		Data:    data,
	}
}

func batchReplayLog(operator, from, to string, data []byte) *ethtypes.Log {
	return &ethtypes.Log{
		Address: common.HexToAddress(internal.PolygonConditionalTokens),
		Topics:  []common.Hash{transferBatchEventSig, addressTopic(operator), addressTopic(from), addressTopic(to)},
		Data:    data,
	}
}

func batchData(id0, id1, value0, value1 string) []byte {
	words := []string{
		"40", "a0", "02", id0, id1, "02", value0, value1,
	}
	var encoded strings.Builder
	for _, word := range words {
		encoded.WriteString(strings.Repeat("0", 64-len(word)))
		encoded.WriteString(word)
	}
	data, _ := hex.DecodeString(encoded.String())
	return data
}

func decimalTokenID(tokenHex string) string {
	return common.HexToHash(tokenHex).Big().String()
}

func closeEnough(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %.9f, want %.9f", got, want)
	}
}
