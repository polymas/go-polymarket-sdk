package web3

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	legacyOwner  = "0x727C86DdAB8e4B048cE5b9319c50011f707ffFAa"
	legacyWallet = "0xFC60457b6edaEAEaac856365bd38D2871b24C406"
	wrongBeacon  = "0xA1A41DB17F90220BFe42FA725847257d8ac21D70"
)

func depositWalletConfig() (common.Address, common.Address) {
	return common.HexToAddress(internal.PolygonDepositWalletFactory), common.HexToAddress(internal.PolygonDepositWalletImpl)
}

func TestLegacyUUPSDepositWalletResolution(t *testing.T) {
	factory, implementation := depositWalletConfig()
	got, err := resolveDepositWallet(context.Background(), common.HexToAddress(legacyOwner), factory, implementation, depositWalletRPC{
		beacon: func(context.Context) (common.Address, error) {
			return common.HexToAddress(internal.PolygonDepositWalletBeacon), nil
		},
		code: func(_ context.Context, address common.Address) ([]byte, error) {
			if !strings.EqualFold(address.Hex(), legacyWallet) {
				t.Fatalf("UUPS derivation mismatch: got %s", address.Hex())
			}
			return []byte{0x60, 0x00}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(got.Hex(), legacyWallet) {
		t.Fatalf("want legacy UUPS %s, got %s", legacyWallet, got.Hex())
	}
	if strings.EqualFold(got.Hex(), wrongBeacon) {
		t.Fatalf("returned wrong Beacon address %s", got.Hex())
	}
}

func TestNewBeaconDepositWalletResolution(t *testing.T) {
	factory, implementation := depositWalletConfig()
	owner := common.HexToAddress("0x1111111111111111111111111111111111111111")
	beacon := common.HexToAddress(internal.PolygonDepositWalletBeacon)
	want, err := deriveBeaconDepositWallet(owner, factory, beacon)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveDepositWallet(context.Background(), owner, factory, implementation, depositWalletRPC{
		beacon: func(context.Context) (common.Address, error) { return beacon, nil },
		code:   func(context.Context, common.Address) ([]byte, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("want Beacon wallet %s, got %s", want.Hex(), got.Hex())
	}
}

func TestFactoryWithoutBeacon(t *testing.T) {
	factory, implementation := depositWalletConfig()
	owner := common.HexToAddress("0x2222222222222222222222222222222222222222")
	want, _ := deriveUUPSDepositWallet(owner, factory, implementation)
	for _, tc := range []struct {
		name   string
		beacon func(context.Context) (common.Address, error)
	}{
		{"revert", func(context.Context) (common.Address, error) {
			return common.Address{}, errors.New("execution reverted")
		}},
		{"zero", func(context.Context) (common.Address, error) { return common.Address{}, nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveDepositWallet(context.Background(), owner, factory, implementation, depositWalletRPC{beacon: tc.beacon, code: func(context.Context, common.Address) ([]byte, error) {
				t.Fatal("eth_getCode must not be called")
				return nil, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("want UUPS %s, got %s", want.Hex(), got.Hex())
			}
		})
	}
}

func TestRPCFailureDoesNotCacheWrongAddress(t *testing.T) {
	calls := 0
	beacon := common.HexToAddress(internal.PolygonDepositWalletBeacon)
	rpc := &depositWalletRPC{
		beacon: func(context.Context) (common.Address, error) { return beacon, nil },
		code: func(context.Context, common.Address) ([]byte, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("temporary RPC timeout")
			}
			return []byte{0x60}, nil
		},
	}
	c := &baseClient{signatureType: types.CWIASignatureType, baseAddress: types.EthAddress(legacyOwner), depositWalletRPC: rpc}
	if _, err := c.GetPolyProxyAddress(); err == nil {
		t.Fatal("expected transient eth_getCode error")
	}
	if c.proxyAddress != "" {
		t.Fatalf("RPC failure cached %s", c.proxyAddress)
	}
	got, err := c.GetPolyProxyAddress()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(string(got), legacyWallet) {
		t.Fatalf("retry should resolve legacy UUPS %s, got %s", legacyWallet, got)
	}
	if calls != 2 {
		t.Fatalf("expected eth_getCode retry, calls=%d", calls)
	}
}

func TestV2WritableTargetsExcludeLegacyNegRiskAdapter(t *testing.T) {
	legacy := common.HexToAddress(internal.PolygonNegRiskAdapter)
	for _, address := range append(pusdSpendersV2(), ctfOperatorsV2()...) {
		if address == legacy {
			t.Fatalf("retired NegRiskAdapter remains in a V2 writable target list: %s", address.Hex())
		}
	}
}
