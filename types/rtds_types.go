package types

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

type ChainlinkTWAPEvent struct {
	Topic     string               `json:"topic"`
	Type      string               `json:"type"`
	Timestamp int64                `json:"timestamp"` // RTDS publish time, Unix milliseconds
	Payload   ChainlinkTWAPPayload `json:"payload"`
}

type ChainlinkTWAPPayload struct {
	Symbol            string      `json:"symbol"`
	DisplayValue      json.Number `json:"value"` // display only; use ExactValue for calculations
	FullAccuracyValue string      `json:"full_accuracy_value"`
	ExactValue        string      `json:"-"`         // FullAccuracyValue formatted as an exact E18 decimal
	Timestamp         int64       `json:"timestamp"` // Chainlink observation time, Unix milliseconds
	WindowSeconds     int         `json:"window_s"`
}

func (p *ChainlinkTWAPPayload) UnmarshalJSON(data []byte) error {
	type alias ChainlinkTWAPPayload
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	exact, err := FormatSignedE18(decoded.FullAccuracyValue)
	if err != nil {
		return fmt.Errorf("invalid full_accuracy_value: %w", err)
	}
	*p = ChainlinkTWAPPayload(decoded)
	p.ExactValue = exact
	return nil
}

// FormatSignedE18 formats a signed E18 integer without losing precision.
func FormatSignedE18(raw string) (string, error) {
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return "", fmt.Errorf("%q is not a base-10 integer", raw)
	}
	negative := value.Sign() < 0
	value.Abs(value)
	digits := value.String()
	if len(digits) <= 18 {
		digits = strings.Repeat("0", 19-len(digits)) + digits
	}
	whole, fraction := digits[:len(digits)-18], strings.TrimRight(digits[len(digits)-18:], "0")
	formatted := whole
	if fraction != "" {
		formatted += "." + fraction
	}
	if negative && value.Sign() != 0 {
		formatted = "-" + formatted
	}
	return formatted, nil
}
