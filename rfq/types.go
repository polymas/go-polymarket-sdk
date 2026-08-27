package rfq

import (
	"encoding/json"

	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	RESTURL = "https://combos-rfq-api.polymarket.com"
	WSURL   = "wss://combos-rfq-gateway-quoter.polymarket.com/ws/rfq"
)

type Identity struct {
	SignerAddress string              `json:"signer_address"`
	MakerAddress  string              `json:"maker_address"`
	SignatureType types.SignatureType `json:"signature_type"`
}

type Config struct {
	Signer      *signing.Signer
	Credentials *types.ApiCreds
	Identity    Identity
}

type ComboMarket struct {
	ID            string   `json:"id"`
	ConditionID   string   `json:"condition_id"`
	PositionIDs   []string `json:"position_ids"`
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Outcomes      []string `json:"outcomes"`
	OutcomePrices []string `json:"outcome_prices"`
	Image         string   `json:"image"`
	Volume        float64  `json:"volume"`
	Tags          []string `json:"tags"`
}

type ComboMarketsResponse struct {
	Markets    []ComboMarket `json:"markets"`
	NextCursor *string       `json:"next_cursor"`
}

type ExchangeV3Order struct {
	Salt          string              `json:"salt"`
	Maker         string              `json:"maker"`
	Signer        string              `json:"signer"`
	TokenID       string              `json:"tokenId"`
	MakerAmount   string              `json:"makerAmount"`
	TakerAmount   string              `json:"takerAmount"`
	Side          int                 `json:"side"`
	SignatureType types.SignatureType `json:"signatureType"`
	Timestamp     string              `json:"timestamp"`
	Metadata      string              `json:"metadata,omitempty"`
	Builder       string              `json:"builder,omitempty"`
	Signature     string              `json:"signature"`
}

type Quote struct {
	QuoteID       string              `json:"quote_id"`
	RFQID         string              `json:"rfq_id"`
	SignerAddress string              `json:"signer_address"`
	MakerAddress  string              `json:"maker_address"`
	SignatureType types.SignatureType `json:"signature_type"`
	PriceE6       string              `json:"price_e6"`
	SizeE6        string              `json:"size_e6"`
	ValidUntil    int64               `json:"valid_until,omitempty"`
	SignedOrder   ExchangeV3Order     `json:"signed_order"`
}

type CancelQuoteRequest struct {
	RFQID         string              `json:"rfq_id"`
	QuoteID       string              `json:"quote_id"`
	SignerAddress string              `json:"signer_address"`
	MakerAddress  string              `json:"maker_address"`
	SignatureType types.SignatureType `json:"signature_type"`
}

type ConfirmationDecision string

const (
	DecisionConfirm ConfirmationDecision = "CONFIRM"
	DecisionDecline ConfirmationDecision = "DECLINE"
)

type MakerConfirmationRequest struct {
	RFQID         string               `json:"rfq_id"`
	QuoteID       string               `json:"quote_id"`
	SignerAddress string               `json:"signer_address"`
	MakerAddress  string               `json:"maker_address"`
	SignatureType types.SignatureType  `json:"signature_type"`
	Decision      ConfirmationDecision `json:"decision"`
}

type RequestedSize struct {
	Unit    string `json:"unit"`
	ValueE6 string `json:"value_e6"`
}

type RFQRequest struct {
	RFQID             string              `json:"rfq_id"`
	AuthAddress       string              `json:"auth_address,omitempty"`
	SignerAddress     string              `json:"signer_address,omitempty"`
	MakerAddress      string              `json:"maker_address,omitempty"`
	SignatureType     types.SignatureType `json:"signature_type,omitempty"`
	RequestorPublicID string              `json:"requestor_public_id,omitempty"`
	LegPositionIDs    []string            `json:"leg_position_ids"`
	ConditionID       string              `json:"condition_id,omitempty"`
	YesPositionID     string              `json:"yes_position_id,omitempty"`
	NoPositionID      string              `json:"no_position_id,omitempty"`
	Direction         string              `json:"direction"`
	Side              string              `json:"side"`
	RequestedSize     *RequestedSize      `json:"requested_size,omitempty"`
	CreatedAt         int64               `json:"created_at,omitempty"`
}

type FillAllocation struct {
	MakerQuoteID  string `json:"maker_quote_id"`
	SignerAddress string `json:"signer_address"`
	MakerAddress  string `json:"maker_address"`
	SizeE6        string `json:"size_e6"`
	PriceE6       string `json:"price_e6"`
	ReceivedAt    int64  `json:"received_at,omitempty"`
}

type FillBundle struct {
	RequestedSharesE6   string           `json:"requested_shares_e6"`
	RequestedNotionalE6 string           `json:"requested_notional_e6,omitempty"`
	BlendedPriceE6      string           `json:"blended_price_e6"`
	Allocations         []FillAllocation `json:"allocations"`
}

type MakerConfirmationSnapshot struct {
	QuoteID       string               `json:"quote_id"`
	SignerAddress string               `json:"signer_address"`
	MakerAddress  string               `json:"maker_address"`
	Decision      ConfirmationDecision `json:"decision,omitempty"`
	Reason        string               `json:"reason,omitempty"`
	RespondedAt   int64                `json:"responded_at,omitempty"`
}

type RFQSnapshot struct {
	Request               RFQRequest                  `json:"request"`
	Status                string                      `json:"status"`
	CompetitionStartedAt  int64                       `json:"competition_started_at,omitempty"`
	CompetitionEndsAt     int64                       `json:"competition_ends_at,omitempty"`
	ConfirmationStartedAt int64                       `json:"confirmation_started_at,omitempty"`
	ConfirmationEndsAt    int64                       `json:"confirmation_ends_at,omitempty"`
	QuoteID               string                      `json:"quote_id,omitempty"`
	Bundle                *FillBundle                 `json:"bundle,omitempty"`
	MakerConfirmations    []MakerConfirmationSnapshot `json:"maker_confirmations,omitempty"`
}

type RequesterAcceptance struct {
	RFQID         string          `json:"rfq_id"`
	QuoteID       string          `json:"quote_id"`
	AuthAddress   string          `json:"auth_address,omitempty"`
	SignerAddress string          `json:"signer_address,omitempty"`
	MakerAddress  string          `json:"maker_address,omitempty"`
	SignatureType int             `json:"signature_type,omitempty"`
	SignedOrder   ExchangeV3Order `json:"signed_order"`
	AcceptedAt    int64           `json:"accepted_at,omitempty"`
}

type WalletAssetDelta struct {
	Asset   string `json:"asset"`
	AssetID string `json:"asset_id"`
	Amount  string `json:"amount"`
}

type WalletReservation struct {
	ActionID    string             `json:"action_id"`
	User        string             `json:"user"`
	WalletNonce int64              `json:"wallet_nonce"`
	Deltas      []WalletAssetDelta `json:"deltas"`
}

type ExecutionHandoff struct {
	ExecutionID         string              `json:"execution_id"`
	Request             RFQRequest          `json:"request"`
	QuoteID             string              `json:"quote_id"`
	Bundle              FillBundle          `json:"bundle"`
	RequesterAcceptance RequesterAcceptance `json:"requester_acceptance"`
	MakerQuotes         []Quote             `json:"maker_quotes"`
	Reservations        []WalletReservation `json:"reservations,omitempty"`
	ReadyAt             int64               `json:"ready_at,omitempty"`
}

type MakerConfirmationResult struct {
	Snapshot  *RFQSnapshot      `json:"snapshot,omitempty"`
	Execution *ExecutionHandoff `json:"execution,omitempty"`
}

type Event struct {
	Type    string
	Payload json.RawMessage
}

type WSQuote struct {
	Type        string          `json:"type"`
	RFQID       string          `json:"rfq_id"`
	PriceE6     string          `json:"price_e6"`
	SizeE6      string          `json:"size_e6"`
	SignedOrder ExchangeV3Order `json:"signed_order"`
}

type ExecutionStatus string

const (
	ExecutionMatched   ExecutionStatus = "MATCHED"
	ExecutionMined     ExecutionStatus = "MINED"
	ExecutionRetrying  ExecutionStatus = "RETRYING"
	ExecutionConfirmed ExecutionStatus = "CONFIRMED"
	ExecutionFailed    ExecutionStatus = "FAILED"
)

type ExecutionUpdate struct {
	Type   string          `json:"type"`
	RFQID  string          `json:"rfq_id"`
	Status ExecutionStatus `json:"status"`
	TxHash string          `json:"tx_hash,omitempty"`
}
