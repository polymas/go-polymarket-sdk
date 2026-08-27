package types

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestConditionIDRoundTrip(t *testing.T) {
	want := "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	id, err := ParseConditionID(want)
	if err != nil {
		t.Fatalf("ParseConditionID() error = %v", err)
	}
	if got := id.String(); got != want {
		t.Fatalf("ConditionID.String() = %q, want %q", got, want)
	}

	withoutPrefix, err := ParseConditionID(strings.TrimPrefix(want, "0x"))
	if err != nil {
		t.Fatalf("ParseConditionID() without prefix error = %v", err)
	}
	if withoutPrefix != id {
		t.Fatal("prefixed and unprefixed condition IDs differ")
	}

	upper, err := ParseConditionID(strings.ToUpper(want))
	if err != nil {
		t.Fatalf("ParseConditionID() uppercase error = %v", err)
	}
	if upper.String() != want {
		t.Fatalf("uppercase input normalized to %q, want %q", upper.String(), want)
	}
}

func TestConditionIDRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{
		"",
		"0x01",
		"0xzz23456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"0x00123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		_, err := ParseConditionID(value)
		if !errors.Is(err, ErrInvalidConditionID) {
			t.Errorf("ParseConditionID(%q) error = %v, want ErrInvalidConditionID", value, err)
		}
	}
}

func TestTokenIDRoundTrip(t *testing.T) {
	values := []string{
		"0",
		"1",
		"9981628014832906803729415829300259633349621731161422253687287444602814085073",
		"115792089237316195423570985008687907853269984665640564039457584007913129639935",
	}
	for _, want := range values {
		id, err := ParseTokenID(want)
		if err != nil {
			t.Fatalf("ParseTokenID(%q) error = %v", want, err)
		}
		if got := id.String(); got != want {
			t.Errorf("TokenID.String() = %q, want %q", got, want)
		}
	}

	id, err := ParseTokenID("00042")
	if err != nil {
		t.Fatalf("ParseTokenID() with leading zeroes error = %v", err)
	}
	if got := id.String(); got != "42" {
		t.Fatalf("TokenID.String() = %q, want canonical %q", got, "42")
	}
}

func TestTokenIDRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{
		"",
		"-1",
		"+1",
		" 1",
		"1.0",
		"0x01",
		"115792089237316195423570985008687907853269984665640564039457584007913129639936",
	} {
		_, err := ParseTokenID(value)
		if !errors.Is(err, ErrInvalidTokenID) {
			t.Errorf("ParseTokenID(%q) error = %v, want ErrInvalidTokenID", value, err)
		}
	}
}

func TestIDsJSONRoundTrip(t *testing.T) {
	conditionText := "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tokenText := "9981628014832906803729415829300259633349621731161422253687287444602814085073"
	conditionID, _ := ParseConditionID(conditionText)
	tokenID, _ := ParseTokenID(tokenText)

	type payload struct {
		ConditionID ConditionID `json:"condition_id"`
		TokenID     TokenID     `json:"token_id"`
	}
	want := payload{ConditionID: conditionID, TokenID: tokenID}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if got := string(data); got != `{"condition_id":"`+conditionText+`","token_id":"`+tokenText+`"}` {
		t.Fatalf("json.Marshal() = %s", got)
	}

	var got payload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got != want {
		t.Fatalf("JSON round trip = %#v, want %#v", got, want)
	}
}

func TestIDsAsMapKeys(t *testing.T) {
	conditionText := "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tokenText := "9981628014832906803729415829300259633349621731161422253687287444602814085073"
	conditionID, _ := ParseConditionID(conditionText)
	tokenID, _ := ParseTokenID(tokenText)

	conditionMap := map[ConditionID]string{conditionID: "market"}
	tokenMap := map[TokenID]string{tokenID: "book"}
	if conditionMap[conditionID] != "market" || tokenMap[tokenID] != "book" {
		t.Fatal("fixed-size IDs are not usable as map keys")
	}

	conditionJSON, err := json.Marshal(conditionMap)
	if err != nil {
		t.Fatalf("marshal condition map: %v", err)
	}
	if string(conditionJSON) != `{"`+conditionText+`":"market"}` {
		t.Fatalf("condition map JSON = %s", conditionJSON)
	}
	var decodedConditionMap map[ConditionID]string
	if err := json.Unmarshal(conditionJSON, &decodedConditionMap); err != nil {
		t.Fatalf("unmarshal condition map: %v", err)
	}
	if decodedConditionMap[conditionID] != "market" {
		t.Fatalf("decoded condition map = %#v", decodedConditionMap)
	}
	tokenJSON, err := json.Marshal(tokenMap)
	if err != nil {
		t.Fatalf("marshal token map: %v", err)
	}
	if string(tokenJSON) != `{"`+tokenText+`":"book"}` {
		t.Fatalf("token map JSON = %s", tokenJSON)
	}
	var decodedTokenMap map[TokenID]string
	if err := json.Unmarshal(tokenJSON, &decodedTokenMap); err != nil {
		t.Fatalf("unmarshal token map: %v", err)
	}
	if decodedTokenMap[tokenID] != "book" {
		t.Fatalf("decoded token map = %#v", decodedTokenMap)
	}
}

func TestIDUnmarshalDoesNotMutateOnError(t *testing.T) {
	conditionID, _ := ParseConditionID("0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	wantConditionID := conditionID
	if err := conditionID.UnmarshalText([]byte("bad")); err == nil {
		t.Fatal("ConditionID.UnmarshalText() error = nil")
	}
	if conditionID != wantConditionID {
		t.Fatal("ConditionID mutated after invalid input")
	}

	tokenID, _ := ParseTokenID("42")
	wantTokenID := tokenID
	if err := tokenID.UnmarshalText([]byte("bad")); err == nil {
		t.Fatal("TokenID.UnmarshalText() error = nil")
	}
	if tokenID != wantTokenID {
		t.Fatal("TokenID mutated after invalid input")
	}
}
