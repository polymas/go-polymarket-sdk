package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecimalStringJSONPreservesSourcePrecision(t *testing.T) {
	for _, input := range []string{`"0.123456789012345678901234567890"`, `123456789.000000001`} {
		var value DecimalString
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Fatalf("Unmarshal(%s): %v", input, err)
		}
		want := strings.Trim(input, `"`)
		if value.String() != want {
			t.Fatalf("Unmarshal(%s) = %q, want %q", input, value, want)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("Marshal(%q): %v", value, err)
		}
		if string(encoded) != `"`+want+`"` {
			t.Fatalf("Marshal(%q) = %s", value, encoded)
		}
	}
}

func TestDecimalStringRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{`null`, `""`, `"NaN"`, `"1e-3"`, `true`, `"1/2"`} {
		var value DecimalString
		if err := json.Unmarshal([]byte(input), &value); err == nil {
			t.Fatalf("Unmarshal(%s) unexpectedly succeeded with %q", input, value)
		}
	}
}

func TestDecimalStringFloat64IsExplicit(t *testing.T) {
	value := DecimalString("0.125")
	got, err := value.Float64()
	if err != nil || got != 0.125 {
		t.Fatalf("Float64() = %v, %v", got, err)
	}
	if _, err := DecimalString("").Float64(); err == nil {
		t.Fatal("empty DecimalString Float64 unexpectedly succeeded")
	}
}

func TestOrderBookLastTradePriceNullableSemantics(t *testing.T) {
	for _, field := range []string{"", `,"last_trade_price":null`, `,"last_trade_price":""`} {
		var book OrderBookSummary
		if err := json.Unmarshal([]byte(`{"bids":[],"asks":[],"min_order_size":"5","tick_size":"0.01"`+field+`}`), &book); err != nil {
			t.Fatalf("Unmarshal field %q: %v", field, err)
		}
		if book.LastTradePrice != nil {
			t.Fatalf("field %q produced last trade %q, want nil", field, book.LastTradePrice)
		}
	}

	var book OrderBookSummary
	if err := json.Unmarshal([]byte(`{"bids":[],"asks":[],"min_order_size":"5","tick_size":"0.01","last_trade_price":"0"}`), &book); err != nil {
		t.Fatalf("Unmarshal zero last trade: %v", err)
	}
	if book.LastTradePrice == nil || book.LastTradePrice.String() != "0" {
		t.Fatalf("zero last trade = %v", book.LastTradePrice)
	}
}
