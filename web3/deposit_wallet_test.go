package web3

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	legacyOwner        = "0x727C86DdAB8e4B048cE5b9319c50011f707ffFAa"
	legacyWallet       = "0xFC60457b6edaEAEaac856365bd38D2871b24C406"
	knownBeaconOwner   = "0xe53a298f46d6cc1e2c5a5B428aC8d0d526C3c827"
	knownBeaconFactory = "0x00000000000Fb5C9ADea0298D729A0CB3823Cc07"
	knownBeacon        = "0x7A18EDfe055488A3128f01F563e5B479D92ffc3a"
	knownBeaconWallet  = "0x9459f742585ed608259352ccac473338131042fd"
	wrongBeaconWallet  = "0x37747DD591387c1ED529c70BCd6E6816c84A2941"
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
}

func TestBeaconDepositWalletKnownVector(t *testing.T) {
	got, err := deriveBeaconDepositWallet(
		common.HexToAddress(knownBeaconOwner),
		common.HexToAddress(knownBeaconFactory),
		common.HexToAddress(knownBeacon),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(got.Hex(), knownBeaconWallet) {
		t.Fatalf("want Beacon wallet %s, got %s", knownBeaconWallet, got.Hex())
	}
	if strings.EqualFold(got.Hex(), wrongBeaconWallet) {
		t.Fatalf("returned v1.12.4 incorrectly derived address %s", got.Hex())
	}
}

func TestSoladyCloneHashAddsImmutableArgsLengthToBeaconPrefix(t *testing.T) {
	immutableArgs := make([]byte, 64)
	got := soladyCloneHash(erc1967BeaconPrefix, immutableArgs)

	// 0x40 << 56 overlaps a set bit in the Beacon prefix. Solady uses addition,
	// so the overlap must carry; bitwise OR produces the v1.12.4 bug.
	want := crypto.Keccak256Hash(common.FromHex("0x6100923d8160233d3973"), immutableArgs)
	orHash := crypto.Keccak256Hash(common.FromHex("0x6100523d8160233d3973"), immutableArgs)
	if got != want {
		t.Fatalf("want Add-based clone hash %s, got %s", want.Hex(), got.Hex())
	}
	if got == orHash {
		t.Fatalf("clone hash unexpectedly uses OR semantics: %s", got.Hex())
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
