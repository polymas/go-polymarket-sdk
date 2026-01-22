package web3

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/gamma"
	"github.com/polymas/go-polymarket-sdk/types"
)

// TestSplitAndMergeBTC4H 测试 BTC 4小时线市场的 split 和 merge 操作
func TestSplitAndMergeBTC4H(t *testing.T) {
	// 从环境变量读取配置
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		t.Skipf("Skipping test: POLY_PRIVATE_KEY not set. " +
			"Please set POLY_PRIVATE_KEY environment variable or use .env file.")
	}

	apiKey := os.Getenv("POLY_API_KEY")
	apiSecret := os.Getenv("POLY_API_SECRET")
	apiPassphrase := os.Getenv("POLY_API_PASSPHRASE")
	if apiKey == "" || apiSecret == "" || apiPassphrase == "" {
		t.Skipf("Skipping test: Missing API credentials. " +
			"Please set POLY_API_KEY, POLY_API_SECRET, and POLY_API_PASSPHRASE environment variables.")
	}

	// 解析 SIGNATURE_TYPE (1=Proxy, 2=Safe, 默认=2)
	signatureType := types.SafeSignatureType
	if st := os.Getenv("POLY_SIGNATURE_TYPE"); st != "" {
		if parsed, err := strconv.Atoi(st); err == nil {
			signatureType = types.SignatureType(parsed)
		}
	}

	// 创建 GaslessClient
	builderCreds := &types.ApiCreds{
		Key:        apiKey,
		Secret:     apiSecret,
		Passphrase: apiPassphrase,
	}

	gaslessClient, err := NewGaslessClient(privateKey, signatureType, types.Polygon, builderCreds)
	if err != nil {
		t.Fatalf("Failed to create GaslessClient: %v", err)
	}

	// 获取市场信息
	gammaClient := gamma.NewClient()
	var targetMarket *types.GammaMarket

	// 优先使用环境变量指定的市场
	if slug := os.Getenv("POLY_TEST_MARKET_SLUG"); slug != "" {
		if market, err := gammaClient.GetMarketBySlug(slug, nil); err == nil && market != nil && len(market.TokenIDs) > 0 {
			targetMarket = market
		}
	} else if conditionID := os.Getenv("POLY_TEST_CONDITION_ID"); conditionID != "" {
		if markets, err := gammaClient.GetMarketsByConditionIDs([]string{conditionID}); err == nil && len(markets) > 0 && len(markets[0].TokenIDs) > 0 {
			targetMarket = &markets[0]
		}
	}

	// 如果未指定，尝试搜索 BTC 4小时市场
	if targetMarket == nil {
		searchResult, err := gammaClient.Search("BTC 4h", gamma.WithSearchStatus("open"), gamma.WithSearchLimitPerType(10))
		if err == nil && searchResult != nil {
			for _, m := range searchResult.Markets {
				slugLower := strings.ToLower(m.Slug)
				if strings.Contains(slugLower, "btc") && strings.Contains(slugLower, "4h") && m.Active && !m.Closed && len(m.TokenIDs) > 0 {
					targetMarket = &m
					break
				}
			}
		}
	}

	if targetMarket == nil {
		t.Skipf("Skipping test: Market not found. Set POLY_TEST_MARKET_SLUG or POLY_TEST_CONDITION_ID to specify a market.")
		return
	}

	conditionID := targetMarket.ConditionID
	if conditionID == "" {
		t.Fatalf("Market has no condition ID")
	}

	negRisk := targetMarket.MarketType == "neg_risk" || strings.Contains(strings.ToLower(targetMarket.Slug), "neg-risk")
	testAmount := 5.0

	t.Run("SplitUSDC", func(t *testing.T) {
		receipt, err := gaslessClient.SplitUSDC(testAmount, conditionID, negRisk)
		if err != nil {
			t.Fatalf("SplitUSDC failed: %v", err)
		}
		if receipt == nil || receipt.Status != 1 {
			t.Fatalf("SplitUSDC failed: receipt=%+v", receipt)
		}
		t.Logf("✅ SplitUSDC succeeded: %s (block: %d)", receipt.TxHash, receipt.BlockNumber)
	})

	t.Run("MergeTokens", func(t *testing.T) {
		receipt, err := gaslessClient.MergeTokens(conditionID, testAmount, negRisk)
		if err != nil {
			t.Fatalf("MergeTokens failed: %v", err)
		}
		if receipt == nil || receipt.Status != 1 {
			t.Fatalf("MergeTokens failed: receipt=%+v", receipt)
		}
		t.Logf("✅ MergeTokens succeeded: %s (block: %d)", receipt.TxHash, receipt.BlockNumber)
	})

	t.Run("SplitAndMergeFlow", func(t *testing.T) {
		flowAmount := 0.5
		splitReceipt, err := gaslessClient.SplitUSDC(flowAmount, conditionID, negRisk)
		if err != nil {
			t.Fatalf("SplitUSDC failed: %v", err)
		}
		mergeReceipt, err := gaslessClient.MergeTokens(conditionID, flowAmount, negRisk)
		if err != nil {
			t.Fatalf("MergeTokens failed: %v", err)
		}
		t.Logf("✅ Flow completed: split=%s, merge=%s", splitReceipt.TxHash, mergeReceipt.TxHash)
	})
}
