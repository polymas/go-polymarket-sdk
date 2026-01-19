package web3

import (
	"testing"

	"github.com/polymas/go-polymarket-sdk/test"
)

// newTestWeb3Client 创建测试用的Web3客户端
func newTestWeb3Client(t *testing.T) Client {
	config := test.LoadTestConfig()
	if config.PrivateKey == "" {
		t.Skip("Skipping test: POLY_PRIVATE_KEY not set")
	}

	client, err := NewClient(config.PrivateKey, config.SignatureType, config.ChainID)
	if err != nil {
		t.Fatalf("Failed to create Web3 client: %v", err)
	}

	return client
}

func TestGetPOLBalance(t *testing.T) {
	client := newTestWeb3Client(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		balance, err := client.GetPOLBalance()
		if err != nil {
			t.Fatalf("GetPOLBalance failed: %v", err)
		}
		if balance < 0 {
			t.Errorf("Expected non-negative balance, got %f", balance)
		}
		t.Logf("GetPOLBalance returned: %f", balance)
	})
}

func TestGetUSDCBalance(t *testing.T) {
	client := newTestWeb3Client(t)
	config := test.LoadTestConfig()

	// 使用测试用户地址
	userAddr := test.GetTestUserAddress(config)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		balance, err := client.GetUSDCBalance(userAddr)
		if err != nil {
			t.Fatalf("GetUSDCBalance failed: %v", err)
		}
		if balance < 0 {
			t.Errorf("Expected non-negative balance, got %f", balance)
		}
		t.Logf("GetUSDCBalance returned: %f", balance)
	})

	// 使用base地址测试
	t.Run("BaseAddress", func(t *testing.T) {
		baseAddr := client.GetBaseAddress()
		balance, err := client.GetUSDCBalance(baseAddr)
		if err != nil {
			t.Fatalf("GetUSDCBalance with base address failed: %v", err)
		}
		if balance < 0 {
			t.Errorf("Expected non-negative balance, got %f", balance)
		}
		t.Logf("GetUSDCBalance for base address returned: %f", balance)
	})
}

func TestGetTokenBalance(t *testing.T) {
	client := newTestWeb3Client(t)
	config := test.LoadTestConfig()

	if config.TestTokenID == "" {
		t.Skip("Skipping test: POLY_TEST_TOKEN_ID not set")
	}

	// 使用base地址测试
	t.Run("Basic", func(t *testing.T) {
		baseAddr := client.GetBaseAddress()
		balance, err := client.GetTokenBalance(config.TestTokenID, baseAddr)
		if err != nil {
			t.Fatalf("GetTokenBalance failed: %v", err)
		}
		if balance < 0 {
			t.Errorf("Expected non-negative balance, got %f", balance)
		}
		t.Logf("GetTokenBalance returned: %f", balance)
	})

	// 边界条件测试 - 无效token ID
	t.Run("InvalidTokenID", func(t *testing.T) {
		baseAddr := client.GetBaseAddress()
		_, err := client.GetTokenBalance("invalid-token-id", baseAddr)
		if err == nil {
			t.Error("Expected error for invalid token ID")
		}
	})
}

func TestGetBaseAddress(t *testing.T) {
	client := newTestWeb3Client(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		addr := client.GetBaseAddress()
		if addr == "" {
			t.Fatal("GetBaseAddress returned empty address")
		}
		if err := addr.Validate(); err != nil {
			t.Fatalf("Invalid base address: %v", err)
		}
		t.Logf("GetBaseAddress returned: %s", addr)
	})
}

func TestGetPolyProxyAddress(t *testing.T) {
	client := newTestWeb3Client(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		proxyAddr, err := client.GetPolyProxyAddress()
		if err != nil {
			t.Fatalf("GetPolyProxyAddress failed: %v", err)
		}
		if proxyAddr == "" {
			t.Fatal("GetPolyProxyAddress returned empty address")
		}
		if err := proxyAddr.Validate(); err != nil {
			t.Fatalf("Invalid proxy address: %v", err)
		}
		t.Logf("GetPolyProxyAddress returned: %s", proxyAddr)
	})
}

func TestGetChainID(t *testing.T) {
	client := newTestWeb3Client(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		chainID := client.GetChainID()
		if chainID == 0 {
			t.Error("GetChainID returned 0")
		}
		t.Logf("GetChainID returned: %d", chainID)
	})
}

func TestGetSignatureType(t *testing.T) {
	client := newTestWeb3Client(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		sigType := client.GetSignatureType()
		if sigType < 0 || sigType > 2 {
			t.Errorf("Invalid signature type: %d", sigType)
		}
		t.Logf("GetSignatureType returned: %d", sigType)
	})
}

func TestClose(t *testing.T) {
	client := newTestWeb3Client(t)

	// 基本功能测试
	t.Run("Basic", func(t *testing.T) {
		// 关闭客户端不应该出错
		client.Close()
		t.Logf("Close succeeded")
	})
}
