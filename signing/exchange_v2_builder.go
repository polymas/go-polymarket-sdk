package signing

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/types"
)

// V2OrderSide mirrors the on-chain uint8: 0 = BUY, 1 = SELL.
type V2OrderSide uint8

const (
	V2SideBUY  V2OrderSide = 0
	V2SideSELL V2OrderSide = 1
)

// V2OrderData is the input for signing a V2 order. Amount strings are
// base-unit decimal integers (same semantics as V1).
type V2OrderData struct {
	Maker         string              // order funder (proxy / safe / EOA)
	Signer        string              // signer address (defaults to Maker when empty)
	TokenID       string              // decimal tokenId
	MakerAmount   string              // decimal maker amount
	TakerAmount   string              // decimal taker amount
	Side          V2OrderSide         //
	SignatureType types.SignatureType // 0 EOA / 1 Proxy / 2 Safe
	TimestampMS   int64               // creation time in ms, 0 = now
	Metadata      [32]byte            // optional bytes32, zero when unused
	Builder       [32]byte            // builder attribution bytes32, zero = no builder
	Salt          *big.Int            // optional, nil = random
}

// V2Order is the fully-populated V2 order (pre- or post-signing).
type V2Order struct {
	Salt          *big.Int
	Maker         common.Address
	Signer        common.Address
	TokenID       *big.Int
	MakerAmount   *big.Int
	TakerAmount   *big.Int
	Side          V2OrderSide
	SignatureType types.SignatureType
	TimestampMS   *big.Int
	Metadata      [32]byte
	Builder       [32]byte
}

// V2SignedOrder carries the signed V2 order plus the 65-byte signature (v ∈ {27,28}).
type V2SignedOrder struct {
	V2Order
	Signature []byte
}

// BuildSignedV2Order builds and signs a V2 order against the given V2 Exchange address.
//
// chainID:          typically 137 (Polygon mainnet) — use types.Polygon converted to *big.Int.
// exchangeAddress:  V2 Exchange (internal.PolygonExchangeV2) or V2 NegRisk Exchange
//                   (internal.PolygonNegRiskExchangeV2) wrapped via common.HexToAddress.
func BuildSignedV2Order(
	privateKey *ecdsa.PrivateKey,
	data *V2OrderData,
	chainID *big.Int,
	exchangeAddress common.Address,
) (*V2SignedOrder, error) {
	order, err := buildV2Order(data)
	if err != nil {
		return nil, err
	}

	digest, err := V2OrderDigest(order, chainID, exchangeAddress)
	if err != nil {
		return nil, err
	}

	signature, err := crypto.Sign(digest.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("sign v2 order: %w", err)
	}
	// Normalize v to {27, 28} — matches on-chain ecrecover and V1 behavior.
	if len(signature) == 65 && signature[64] < 27 {
		signature[64] += 27
	}

	return &V2SignedOrder{V2Order: *order, Signature: signature}, nil
}

// V2OrderDigest returns the EIP-712 digest that gets signed (0x1901 || domain || structHash).
// Exposed for tests / off-line verification tooling.
func V2OrderDigest(o *V2Order, chainID *big.Int, verifyingContract common.Address) (common.Hash, error) {
	domain, err := buildV2DomainSeparator(chainID, verifyingContract)
	if err != nil {
		return common.Hash{}, err
	}
	structHash, err := v2OrderStructHash(o)
	if err != nil {
		return common.Hash{}, err
	}

	buf := make([]byte, 0, 2+32+32)
	buf = append(buf, 0x19, 0x01)
	buf = append(buf, domain.Bytes()...)
	buf = append(buf, structHash.Bytes()...)
	return crypto.Keccak256Hash(buf), nil
}

func buildV2Order(d *V2OrderData) (*V2Order, error) {
	if d == nil {
		return nil, fmt.Errorf("order data is nil")
	}

	makerAddr := common.HexToAddress(d.Maker)
	signerAddr := makerAddr
	if d.Signer != "" {
		signerAddr = common.HexToAddress(d.Signer)
	}

	tokenID, ok := new(big.Int).SetString(d.TokenID, 10)
	if !ok {
		return nil, fmt.Errorf("invalid tokenId: %q", d.TokenID)
	}
	makerAmt, ok := new(big.Int).SetString(d.MakerAmount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid makerAmount: %q", d.MakerAmount)
	}
	takerAmt, ok := new(big.Int).SetString(d.TakerAmount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid takerAmount: %q", d.TakerAmount)
	}

	ts := d.TimestampMS
	if ts == 0 {
		ts = time.Now().UnixMilli()
	}

	salt := d.Salt
	if salt == nil {
		salt = big.NewInt(rand.Int63())
	}

	return &V2Order{
		Salt:          salt,
		Maker:         makerAddr,
		Signer:        signerAddr,
		TokenID:       tokenID,
		MakerAmount:   makerAmt,
		TakerAmount:   takerAmt,
		Side:          d.Side,
		SignatureType: d.SignatureType,
		TimestampMS:   new(big.Int).SetInt64(ts),
		Metadata:      d.Metadata,
		Builder:       d.Builder,
	}, nil
}

func buildV2DomainSeparator(chainID *big.Int, verifyingContract common.Address) (common.Hash, error) {
	typeHash := crypto.Keccak256Hash(
		[]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
	)
	args := []abi.Type{
		v2Bytes32Type, // typehash
		v2Bytes32Type, // name
		v2Bytes32Type, // version
		v2Uint256Type, // chainId
		v2AddressType, // verifyingContract
	}
	values := []interface{}{
		typeHash,
		V2ExchangeProtocolName,
		V2ExchangeProtocolVersion,
		chainID,
		verifyingContract,
	}
	encoded, err := packABI(args, values)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode v2 domain: %w", err)
	}
	return crypto.Keccak256Hash(encoded), nil
}

func v2OrderStructHash(o *V2Order) (common.Hash, error) {
	values := []interface{}{
		V2OrderTypehash,
		o.Salt,
		o.Maker,
		o.Signer,
		o.TokenID,
		o.MakerAmount,
		o.TakerAmount,
		uint8(o.Side),
		uint8(o.SignatureType),
		o.TimestampMS,
		o.Metadata,
		o.Builder,
	}
	encoded, err := packABI(V2OrderABIStruct, values)
	if err != nil {
		return common.Hash{}, fmt.Errorf("encode v2 order struct: %w", err)
	}
	return crypto.Keccak256Hash(encoded), nil
}

func packABI(types []abi.Type, values []interface{}) ([]byte, error) {
	args := make(abi.Arguments, 0, len(types))
	for _, t := range types {
		args = append(args, abi.Argument{Type: t})
	}
	return args.Pack(values...)
}
