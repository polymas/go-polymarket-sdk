package web3

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/internal"
)

var (
	depositWalletArgsABI  = abi.Arguments{{Type: mustABIType("address")}, {Type: mustABIType("bytes32")}}
	factoryBeaconSelector = common.FromHex("0x49493a4d")
)

const (
	erc1967Prefix       = "61003d3d8160233d3973"
	erc1967Const2       = "5155f3363d3d373d3d363d7f360894a13ba1a3210667c828492db98dca3e2076"
	erc1967Const1       = "cc3735a920a3ca505d382bbc545af43d6000803e6038573d6000fd5b3d6000f3"
	erc1967BeaconPrefix = "6100523d8160233d3973"
	erc1967BeaconConst3 = "60195155f3363d3d373d3d363d602036600436635c60da"
	erc1967BeaconConst2 = "1b60e01b36527fa3f0ad74e5423aebfd80d3ef4346578335a9a72aeaee59ff6c"
	erc1967BeaconConst1 = "b3582b35133d50545afa5036515af43d6000803e604d573d6000fd5b3d6000f3"
)

func mustABIType(name string) abi.Type {
	t, err := abi.NewType(name, "", nil)
	if err != nil {
		panic(err)
	}
	return t
}

func depositWalletImmutableArgs(owner, factory common.Address) ([]byte, error) {
	var walletID [32]byte
	copy(walletID[12:], owner.Bytes())
	return depositWalletArgsABI.Pack(factory, walletID)
}

func soladyCloneHash(prefixHex string, immutableArgs []byte, parts ...[]byte) common.Hash {
	prefix := new(big.Int).SetBytes(common.FromHex("0x" + prefixHex))
	argLength := new(big.Int).Lsh(big.NewInt(int64(len(immutableArgs))), 56)
	prefix.Add(prefix, argLength)
	prefixBytes := make([]byte, 10)
	prefix.FillBytes(prefixBytes)
	initCode := append([]byte{}, prefixBytes...)
	for _, part := range parts {
		initCode = append(initCode, part...)
	}
	initCode = append(initCode, immutableArgs...)
	return crypto.Keccak256Hash(initCode)
}

func deriveUUPSDepositWallet(owner, factory, implementation common.Address) (common.Address, error) {
	args, err := depositWalletImmutableArgs(owner, factory)
	if err != nil {
		return common.Address{}, fmt.Errorf("encode deposit wallet immutable args: %w", err)
	}
	salt := crypto.Keccak256Hash(args)
	hash := soladyCloneHash(erc1967Prefix, args, implementation.Bytes(), common.FromHex("0x6009"), common.FromHex("0x"+erc1967Const2), common.FromHex("0x"+erc1967Const1))
	return crypto.CreateAddress2(factory, salt, hash.Bytes()), nil
}

func deriveBeaconDepositWallet(owner, factory, beacon common.Address) (common.Address, error) {
	args, err := depositWalletImmutableArgs(owner, factory)
	if err != nil {
		return common.Address{}, fmt.Errorf("encode deposit wallet immutable args: %w", err)
	}
	salt := crypto.Keccak256Hash(args)
	hash := soladyCloneHash(erc1967BeaconPrefix, args, beacon.Bytes(), common.FromHex("0x"+erc1967BeaconConst3), common.FromHex("0x"+erc1967BeaconConst2), common.FromHex("0x"+erc1967BeaconConst1))
	return crypto.CreateAddress2(factory, salt, hash.Bytes()), nil
}

type depositWalletRPC struct {
	beacon func(context.Context) (common.Address, error)
	code   func(context.Context, common.Address) ([]byte, error)
}

func resolveDepositWallet(ctx context.Context, owner, factory, implementation common.Address, rpc depositWalletRPC) (common.Address, error) {
	uups, err := deriveUUPSDepositWallet(owner, factory, implementation)
	if err != nil {
		return common.Address{}, err
	}
	beacon, err := rpc.beacon(ctx)
	if err != nil {
		if isContractRevertError(err) {
			return uups, nil
		}
		return common.Address{}, fmt.Errorf("read DepositWalletFactory.BEACON(): %w", err)
	}
	if beacon == (common.Address{}) {
		return uups, nil
	}
	code, err := rpc.code(ctx, uups)
	if err != nil {
		return common.Address{}, fmt.Errorf("verify legacy UUPS Deposit Wallet %s deployment: %w", uups.Hex(), err)
	}
	if len(code) != 0 {
		return uups, nil
	}
	return deriveBeaconDepositWallet(owner, factory, beacon)
}

func isContractRevertError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "execution reverted") || strings.Contains(s, "vm execution error") || strings.Contains(s, "revert")
}

func (c *baseClient) depositWalletFactoryBeacon(ctx context.Context) (common.Address, error) {
	factory := common.HexToAddress(internal.PolygonDepositWalletFactory)
	out, err := c.callContractWithRetry(ctx, ethereum.CallMsg{To: &factory, Data: factoryBeaconSelector}, nil)
	if err != nil {
		return common.Address{}, err
	}
	if len(out) < 32 {
		return common.Address{}, fmt.Errorf("invalid BEACON() response: got %d bytes (%s)", len(out), hex.EncodeToString(out))
	}
	return common.BytesToAddress(out[len(out)-20:]), nil
}

func (c *baseClient) codeAtWithRetry(ctx context.Context, account common.Address) ([]byte, error) {
	c.clientMu.RLock()
	clients := append([]*ethclient.Client(nil), c.clients...)
	c.clientMu.RUnlock()
	if len(clients) == 0 {
		return nil, fmt.Errorf("no RPC clients available")
	}
	start := c.getNextClientIndex()
	var lastErr error
	for i := range clients {
		code, err := clients[(start+i)%len(clients)].CodeAt(ctx, account, nil)
		if err == nil {
			return code, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("all RPC nodes failed during eth_getCode(%s), last error: %w", account.Hex(), lastErr)
}
