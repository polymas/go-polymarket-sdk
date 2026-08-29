package web3

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/test"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestClientCloseClearsSigner(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client, err := NewClient(
		"1111111111111111111111111111111111111111111111111111111111111111",
		types.EOASignatureType,
		types.Polygon,
		server.URL,
	)
	if err != nil {
		t.Fatal(err)
	}
	signer := client.GetSigner()
	client.Close()
	client.Close()

	if !signer.Closed() {
		t.Fatal("client signer should be closed")
	}
	if _, err := signer.SignWithRecovery(crypto.Keccak256Hash(nil)); !errors.Is(err, signing.ErrSignerClosed) {
		t.Fatalf("sign after client Close error = %v, want ErrSignerClosed", err)
	}
}

func TestGaslessClientCloseClearsSigningState(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	client, err := NewGaslessClient(
		"1111111111111111111111111111111111111111111111111111111111111111",
		types.SafeSignatureType,
		types.Polygon,
		nil,
		server.URL,
	)
	if err != nil {
		t.Fatal(err)
	}
	client.localSigner.SetV2Key(&internal.V2RelayerKey{
		Key:     "sensitive-relayer-key",
		Address: client.GetBaseAddress().String(),
	})
	signer := client.GetSigner()

	client.Close()
	client.Close()

	if client.localSigner.HasV2Key() {
		t.Fatal("gasless relayer key should be cleared")
	}
	if !signer.Closed() {
		t.Fatal("gasless signer should be closed")
	}
	if _, err := client.localSigner.SignPayload(map[string]interface{}{"method": "GET", "path": "/"}); !errors.Is(err, signing.ErrSignerClosed) {
		t.Fatalf("SignPayload after Close error = %v, want ErrSignerClosed", err)
	}
}

func TestNewClientRejectsUnknownSignatureTypeBeforeRPC(t *testing.T) {
	_, err := NewClient("", types.SignatureType(4), types.Polygon, "http://127.0.0.1:1")
	if err == nil || !strings.Contains(err.Error(), "unsupported CLOB V2 signature type 4") {
		t.Fatalf("err=%v", err)
	}
}

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

	// 测试无效地址格式
	t.Run("InvalidAddress", func(t *testing.T) {
		invalidAddr := types.EthAddress("invalid-address")
		_, err := client.GetUSDCBalance(invalidAddr)
		if err != nil {
			t.Logf("GetUSDCBalance with invalid address returned error (expected): %v", err)
		} else {
			t.Error("Expected error for invalid address")
		}
	})
}

func TestGetCollateralBalance(t *testing.T) {
	client := newTestWeb3Client(t)
	config := test.LoadTestConfig()
	userAddr := test.GetTestUserAddress(config)

	balance, err := client.GetCollateralBalance(userAddr)
	if err != nil {
		t.Fatalf("GetCollateralBalance failed: %v", err)
	}
	if balance < 0 {
		t.Errorf("Expected non-negative balance, got %f", balance)
	}
	t.Logf("GetCollateralBalance returned: %f", balance)
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
		if sigType < 0 || sigType > 3 {
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

	// 测试多次调用Close
	t.Run("MultipleClose", func(t *testing.T) {
		client.Close()
		client.Close() // 第二次调用不应该panic
		t.Logf("Multiple Close calls succeeded")
	})

	// 测试Close后调用其他方法（应该失败或返回错误）
	t.Run("AfterClose", func(t *testing.T) {
		client.Close()
		// 尝试调用一个方法，应该失败或返回错误
		_, err := client.GetPOLBalance()
		if err != nil {
			t.Logf("GetPOLBalance after Close returned error (expected): %v", err)
		} else {
			t.Logf("GetPOLBalance after Close succeeded (may reconnect)")
		}
	})
}
