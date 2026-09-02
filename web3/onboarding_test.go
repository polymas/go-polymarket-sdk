package web3

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	onboardingSigner = types.EthAddress("0x1111111111111111111111111111111111111111")
	onboardingWallet = types.EthAddress("0x2222222222222222222222222222222222222222")
)

func TestResolveWalletAccountSemantics(t *testing.T) {
	tests := []struct {
		name            string
		signatureType   types.SignatureType
		wallet          types.EthAddress
		wantType        WalletType
		wantOrderSigner types.EthAddress
	}{
		{"EOA", types.EOASignatureType, onboardingSigner, WalletTypeEOA, onboardingSigner},
		{"Proxy", types.ProxySignatureType, onboardingWallet, WalletTypePolyProxy, onboardingSigner},
		{"Safe", types.SafeSignatureType, onboardingWallet, WalletTypeGnosisSafe, onboardingSigner},
		{"DepositWallet", types.CWIASignatureType, onboardingWallet, WalletTypeDepositWallet, onboardingWallet},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &BaseClient{
				baseAddress:   onboardingSigner,
				proxyAddress:  test.wallet,
				signatureType: test.signatureType,
				depositWalletRPC: &depositWalletRPC{code: func(context.Context, common.Address) ([]byte, error) {
					return []byte{0x60}, nil
				}},
			}
			account, err := client.ResolveWalletAccount(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if account.Signer != onboardingSigner || account.Wallet != test.wallet || account.Funder != test.wallet || account.Maker != test.wallet {
				t.Fatalf("unexpected account addresses: %+v", account)
			}
			if account.WalletType != test.wantType || account.OrderSigner != test.wantOrderSigner || !account.Deployed {
				t.Fatalf("unexpected account semantics: %+v", account)
			}
		})
	}
}

func TestResolveWalletAccountLive(t *testing.T) {
	if testing.Short() || os.Getenv("POLY_RUN_ONBOARDING_LIVE") != "1" {
		t.Skip("set POLY_RUN_ONBOARDING_LIVE=1 to run read-only Polygon account resolution")
	}
	privateKey := os.Getenv("POLY_PRIVATE_KEY")
	if privateKey == "" {
		t.Skip("POLY_PRIVATE_KEY is not configured")
	}
	signatureType, err := strconv.Atoi(os.Getenv("POLY_SIGNATURE_TYPE"))
	if err != nil {
		t.Fatalf("invalid POLY_SIGNATURE_TYPE: %v", err)
	}
	client, err := NewClient(privateKey, types.SignatureType(signatureType), types.Polygon)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	account, err := client.ResolveWalletAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if account.Signer == "" || account.Wallet == "" || account.Funder != account.Wallet || account.Maker != account.Wallet {
		t.Fatalf("invalid resolved account: %+v", account)
	}
	if account.SignatureType != types.SignatureType(signatureType) || account.WalletType != WalletType(signatureType) {
		t.Fatalf("resolved type mismatch: %+v", account)
	}
	if signatureType != int(types.EOASignatureType) && !account.Deployed {
		t.Fatalf("configured production account wallet is not deployed: %s", account.Wallet)
	}
}
