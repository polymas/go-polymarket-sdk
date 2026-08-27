package clob

import "testing"

func TestWithV2CompatibilityOptionIsNoOp(t *testing.T) {
	base := &baseClient{baseURL: "sentinel"}
	WithV2()(base)
	if base.baseURL != "sentinel" {
		t.Fatal("WithV2 compatibility option unexpectedly changed client configuration")
	}
}
