package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTradeOfficialJSONGolden(t *testing.T) {
	golden := []byte(`{
		"proxyWallet":"0x38cc5cf506aff32b8e26c5d19c7b288561805c4f",
		"side":"SELL",
		"asset":"4628019593646202999311148585905739123922459739376721340103915124537975508627",
		"conditionId":"0x9ae51adf340f442c2201fc186115cb6aff5a21019212d0717f81e5ee1635a631",
		"size":10,
		"price":0.999,
		"timestamp":1780392950,
		"title":"Chisinau: Hugo Grenier vs Luca Nardi",
		"slug":"atp-grenier-nardi-2026-05-25",
		"icon":"https://example.com/icon.jpg",
		"eventSlug":"atp-grenier-nardi-2026-05-25",
		"outcome":"Luca Nardi",
		"outcomeIndex":1,
		"name":"ace.",
		"pseudonym":"Acidic-Loft",
		"bio":"bio",
		"profileImage":"https://example.com/profile.jpg",
		"profileImageOptimized":"https://example.com/profile-optimized.jpg",
		"transactionHash":"0x77f44539b4ca38fac4cc022c3c5f835b004b7fa578a73fe9a6aa38aba32bd02c"
	}`)

	var trade Trade
	if err := json.Unmarshal(golden, &trade); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if trade.ProxyWallet == "" || trade.ConditionID == "" || trade.TokenID == "" ||
		trade.TransactionHash == "" || trade.Timestamp != 1780392950 || trade.Size != 10 || trade.Price != 0.999 {
		t.Fatalf("critical Trade fields were not decoded: %+v", trade)
	}
	if trade.Side != OrderSideSELL || trade.OutcomeIndex != 1 || trade.EventSlug == "" || trade.Pseudonym == "" {
		t.Fatalf("Trade metadata was not decoded: %+v", trade)
	}
	assertJSONEquivalent(t, golden, trade)
}

func TestActivityOfficialJSONGolden(t *testing.T) {
	golden := []byte(`{
		"proxyWallet":"0x38cc5cf506aff32b8e26c5d19c7b288561805c4f",
		"timestamp":1780392950,
		"conditionId":"0x9ae51adf340f442c2201fc186115cb6aff5a21019212d0717f81e5ee1635a631",
		"type":"TRADE",
		"size":10,
		"usdcSize":9.98971,
		"transactionHash":"0x77f44539b4ca38fac4cc022c3c5f835b004b7fa578a73fe9a6aa38aba32bd02c",
		"price":0.999,
		"asset":"4628019593646202999311148585905739123922459739376721340103915124537975508627",
		"side":"SELL",
		"outcomeIndex":1,
		"title":"Chisinau: Hugo Grenier vs Luca Nardi",
		"slug":"atp-grenier-nardi-2026-05-25",
		"icon":"https://example.com/icon.jpg",
		"eventSlug":"atp-grenier-nardi-2026-05-25",
		"outcome":"Luca Nardi",
		"name":"ace.",
		"pseudonym":"Acidic-Loft",
		"bio":"bio",
		"profileImage":"https://example.com/profile.jpg",
		"profileImageOptimized":"https://example.com/profile-optimized.jpg",
		"isCombo":true
	}`)

	var activity Activity
	if err := json.Unmarshal(golden, &activity); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if activity.ProxyWallet == "" || activity.ConditionID == "" || activity.TokenID == "" ||
		activity.TransactionHash == "" || activity.Timestamp != 1780392950 || activity.USDCSize != 9.98971 {
		t.Fatalf("critical Activity fields were not decoded: %+v", activity)
	}
	if activity.Type != ActivityTypeTrade || activity.Side != OrderSideSELL || !activity.IsCombo || activity.EventSlug == "" {
		t.Fatalf("Activity metadata was not decoded: %+v", activity)
	}
	assertJSONEquivalent(t, golden, activity)
}

func TestTradeAndActivityRejectInvalidTimestamp(t *testing.T) {
	tests := []struct {
		name string
		data string
		out  any
	}{
		{name: "trade string", data: `{"timestamp":"not-a-timestamp"}`, out: &Trade{}},
		{name: "trade fractional", data: `{"timestamp":1.5}`, out: &Trade{}},
		{name: "activity string", data: `{"timestamp":"1780392950"}`, out: &Activity{}},
		{name: "activity fractional", data: `{"timestamp":1.5}`, out: &Activity{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.data), tc.out); err == nil {
				t.Fatal("json.Unmarshal() error = nil")
			}
		})
	}
}

func TestActivityTypeValues(t *testing.T) {
	got := []ActivityType{
		ActivityTypeTrade,
		ActivityTypeSplit,
		ActivityTypeMerge,
		ActivityTypeRedeem,
		ActivityTypeReward,
		ActivityTypeConversion,
		ActivityTypeDeposit,
		ActivityTypeWithdrawal,
		ActivityTypeYield,
		ActivityTypeMakerRebate,
		ActivityTypeTakerRebate,
		ActivityTypeReferralReward,
	}
	want := []ActivityType{
		"TRADE", "SPLIT", "MERGE", "REDEEM", "REWARD", "CONVERSION",
		"DEPOSIT", "WITHDRAWAL", "YIELD", "MAKER_REBATE", "TAKER_REBATE", "REFERRAL_REWARD",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ActivityType values = %v, want %v", got, want)
	}
}

func TestValueResponseOfficialJSONGolden(t *testing.T) {
	golden := []byte(`{"user":"0x38cc5cf506aff32b8e26c5d19c7b288561805c4f","value":12.345}`)
	var value ValueResponse
	if err := json.Unmarshal(golden, &value); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if value.User == "" || value.Value != 12.345 {
		t.Fatalf("ValueResponse = %+v", value)
	}
	assertJSONEquivalent(t, golden, value)
}

func TestValueResponseArrayKeepsEveryEntry(t *testing.T) {
	golden := []byte(`[
		{"user":"0x38cc5cf506aff32b8e26c5d19c7b288561805c4f","value":1.25},
		{"user":"0x38cc5cf506aff32b8e26c5d19c7b288561805c4f","value":2.75}
	]`)
	var values []ValueResponse
	if err := json.Unmarshal(golden, &values); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(values) != 2 || values[0].Value != 1.25 || values[1].Value != 2.75 {
		t.Fatalf("ValueResponse array = %+v", values)
	}
	assertJSONEquivalent(t, golden, values)
}

func assertJSONEquivalent(t *testing.T, golden []byte, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var want, got any
	if err := json.Unmarshal(golden, &want); err != nil {
		t.Fatalf("decode golden JSON: %v", err)
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode re-encoded JSON: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON round trip = %s, want %s", encoded, golden)
	}
}
