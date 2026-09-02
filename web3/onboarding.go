package web3

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/types"
)

// WalletType is the account wallet model. Values match Polymarket's current
// AccountIdentity WalletType enum and the corresponding SignatureType.
type WalletType uint8

const (
	WalletTypeEOA           WalletType = 0
	WalletTypePolyProxy     WalletType = 1
	WalletTypeGnosisSafe    WalletType = 2
	WalletTypeDepositWallet WalletType = 3
)

func (walletType WalletType) String() string {
	switch walletType {
	case WalletTypeEOA:
		return "EOA"
	case WalletTypePolyProxy:
		return "POLY_PROXY"
	case WalletTypeGnosisSafe:
		return "GNOSIS_SAFE"
	case WalletTypeDepositWallet:
		return "DEPOSIT_WALLET"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", walletType)
	}
}

// WalletAccount makes the SDK's signer/funder semantics explicit.
// Signer is the controlling EOA; Wallet/Funder/Maker hold trading assets.
// OrderSigner is the signer field written into a V2 order. For a Deposit
// Wallet it equals Wallet because the order is verified through EIP-1271.
type WalletAccount struct {
	Signer        types.EthAddress
	Wallet        types.EthAddress
	Funder        types.EthAddress
	Maker         types.EthAddress
	OrderSigner   types.EthAddress
	SignatureType types.SignatureType
	WalletType    WalletType
	Deployed      bool
}

// ResolveWalletAccount resolves the effective account identity and checks
// whether a smart-account wallet currently has bytecode on Polygon.
func (c *BaseClient) ResolveWalletAccount(ctx context.Context) (*WalletAccount, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wallet, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("resolve account wallet: %w", err)
	}
	account := &WalletAccount{
		Signer:        c.baseAddress,
		Wallet:        wallet,
		Funder:        wallet,
		Maker:         wallet,
		OrderSigner:   c.baseAddress,
		SignatureType: c.signatureType,
		WalletType:    WalletType(c.signatureType),
		Deployed:      c.signatureType == types.EOASignatureType,
	}
	if c.signatureType == types.DepositWalletSignatureType {
		account.OrderSigner = wallet
	}
	if c.signatureType != types.EOASignatureType {
		deployed, err := c.IsDepositWalletDeployed(ctx, common.HexToAddress(string(wallet)))
		if err != nil {
			return nil, fmt.Errorf("check %s deployment: %w", account.WalletType, err)
		}
		account.Deployed = deployed
	}
	return account, nil
}
