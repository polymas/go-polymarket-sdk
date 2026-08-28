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
	SignatureType types.SignatureType // 0 EOA / 1 Proxy / 2 Safe / 3 POLY_1271
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
//
//	(internal.PolygonNegRiskExchangeV2) wrapped via common.HexToAddress.
//
// 签名分支（按 V2OrderData.SignatureType）：
//   - 0/1/2 (EOA / PolyProxy / Safe)  : 标准 EIP-712 ECDSA，65 字节签名
//   - 3     (POLY_1271 / CWIA)         : solady ERC-1271 嵌套签名，~326 字节
//     包含 inner_sig || appDomainSep || contents_hash
//     || ORDER_TYPE_STRING || uint16(len)
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

	if order.SignatureType == types.Poly1271SignatureType {
		signature, err := signV2OrderPoly1271(order, chainID, exchangeAddress, privateKey)
		if err != nil {
			return nil, err
		}
		return &V2SignedOrder{V2Order: *order, Signature: signature}, nil
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

// signV2OrderPoly1271 实现 solady ERC-1271 nested TypedDataSign 签名。
//
// 对照 py-clob-client-v2 _build_poly_1271_order_signature。
// signer 字段必须 = DepositWallet 代理地址（这是内层 domain 的 verifyingContract）。
func signV2OrderPoly1271(
	o *V2Order,
	chainID *big.Int,
	exchangeAddress common.Address,
	privateKey *ecdsa.PrivateKey,
) ([]byte, error) {
	// 1) 外层 app domain separator = CTF Exchange V2 domain
	appDomain, err := buildV2DomainSeparator(chainID, exchangeAddress)
	if err != nil {
		return nil, fmt.Errorf("build app domain: %w", err)
	}

	// 2) Order struct hash（"contents"）—— 与普通 EIP-712 路径用同一份 ABI 编码
	contentsHash, err := v2OrderStructHash(o)
	if err != nil {
		return nil, fmt.Errorf("contents hash: %w", err)
	}

	// 3) TypedDataSign struct hash —— 内层 domain 用 DepositWallet 的参数，
	//    verifyingContract = order.signer（必须是钱包代理地址）
	tdsArgs := []abi.Type{
		v2Bytes32Type, // SOLADY_TYPE_HASH
		v2Bytes32Type, // contents_hash
		v2Bytes32Type, // name hash ("DepositWallet")
		v2Bytes32Type, // version hash ("1")
		v2Uint256Type, // chainId
		v2AddressType, // verifyingContract = wallet
		v2Bytes32Type, // salt = bytes32(0)
	}
	var zero32 [32]byte
	tdsValues := []interface{}{
		V2SoladyTypeHash,
		contentsHash,
		V2DepositWalletNameHash,
		V2DepositWalletVersionHash,
		chainID,
		o.Signer, // = wallet proxy
		zero32,
	}
	tdsEncoded, err := packABI(tdsArgs, tdsValues)
	if err != nil {
		return nil, fmt.Errorf("encode TypedDataSign: %w", err)
	}
	tdsHash := crypto.Keccak256Hash(tdsEncoded)

	// 4) digest = keccak256(0x1901 || appDomain || tdsHash)
	buf := make([]byte, 0, 2+32+32)
	buf = append(buf, 0x19, 0x01)
	buf = append(buf, appDomain.Bytes()...)
	buf = append(buf, tdsHash.Bytes()...)
	digest := crypto.Keccak256Hash(buf)

	// 5) inner signature = ECDSA(digest)
	innerSig, err := crypto.Sign(digest.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("sign POLY_1271 digest: %w", err)
	}
	if len(innerSig) == 65 && innerSig[64] < 27 {
		innerSig[64] += 27
	}

	// 6) 拼最终签名：inner_sig || appDomain || contents_hash || typeStr || uint16(len)
	typeStr := []byte(V2OrderTypeString)
	out := make([]byte, 0, 65+32+32+len(typeStr)+2)
	out = append(out, innerSig...)
	out = append(out, appDomain.Bytes()...)
	out = append(out, contentsHash.Bytes()...)
	out = append(out, typeStr...)
	out = append(out,
		byte(len(typeStr)>>8),
		byte(len(typeStr)),
	)
	return out, nil
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
	if !d.SignatureType.ValidV2() {
		return nil, fmt.Errorf("unsupported CLOB V2 signature type %d", d.SignatureType)
	}

	makerAddr := common.HexToAddress(d.Maker)
	signerAddr := makerAddr
	if d.Signer != "" {
		signerAddr = common.HexToAddress(d.Signer)
	}
	if d.SignatureType == types.Poly1271SignatureType && makerAddr != signerAddr {
		return nil, fmt.Errorf("POLY_1271 requires maker and signer to be the same Deposit Wallet (maker=%s signer=%s)", makerAddr.Hex(), signerAddr.Hex())
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
