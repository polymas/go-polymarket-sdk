package types

import "testing"

func TestSignatureTypeV2Contract(t *testing.T) {
	tests := []struct {
		value           SignatureType
		name            string
		valid, contract bool
	}{
		{EOASignatureType, "EOA", true, false},
		{ProxySignatureType, "POLY_PROXY", true, false},
		{SafeSignatureType, "POLY_GNOSIS_SAFE", true, false},
		{Poly1271SignatureType, "POLY_1271", true, true},
		{SignatureType(4), "UNKNOWN", false, false},
		{SignatureType(-1), "UNKNOWN", false, false},
	}
	for _, tt := range tests {
		if got := tt.value.String(); got != tt.name {
			t.Errorf("%d String=%q want %q", tt.value, got, tt.name)
		}
		if got := tt.value.ValidV2(); got != tt.valid {
			t.Errorf("%d ValidV2=%v want %v", tt.value, got, tt.valid)
		}
		if got := tt.value.UsesContractSignature(); got != tt.contract {
			t.Errorf("%d UsesContractSignature=%v want %v", tt.value, got, tt.contract)
		}
	}
	if CWIASignatureType != Poly1271SignatureType || DepositWalletSignatureType != Poly1271SignatureType {
		t.Fatal("type 3 compatibility aliases diverged")
	}
}
