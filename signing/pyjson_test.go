package signing

import (
	"encoding/json"
	"testing"
)

type pyJSONProbe struct {
	Order struct {
		Salt   int64  `json:"salt"`
		Maker  string `json:"maker"`
		Side   string `json:"side"`
		Nested []int  `json:"nested"`
	} `json:"order"`
	Owner    string `json:"owner"`
	PostOnly bool   `json:"postOnly"`
}

func samplePyJSONBody() []pyJSONProbe {
	var p pyJSONProbe
	p.Order.Salt = 123456789
	p.Order.Maker = "0x0000000000000000000000000000000000000001"
	p.Order.Side = "BUY"
	p.Order.Nested = []int{1, 2}
	p.Owner = "key"
	p.PostOnly = true
	return []pyJSONProbe{p, p}
}

func TestFormatPythonJSONMatchesLegacyAndIsIdempotent(t *testing.T) {
	compact, err := json.Marshal(samplePyJSONBody())
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"order": {"salt": 123456789, "maker": "0x0000000000000000000000000000000000000001", "side": "BUY", "nested": [1,2]}, "owner": "key", "postOnly": true}, {"order": {"salt": 123456789, "maker": "0x0000000000000000000000000000000000000001", "side": "BUY", "nested": [1,2]}, "owner": "key", "postOnly": true}]`
	got := FormatPythonJSON(string(compact))
	if got != want {
		t.Fatalf("FormatPythonJSON mismatch\n got=%s\nwant=%s", got, want)
	}
	if again := FormatPythonJSON(got); again != got {
		t.Fatalf("FormatPythonJSON is not idempotent:\n1st=%s\n2nd=%s", got, again)
	}
}

// HMAC 必须与旧路径逐字节一致：struct 直接传入、紧凑字符串传入、以及新的
// MarshalPythonJSON 预格式化三种方式要产出同一个签名。
func TestBuildHMACSignatureFormattedBodyMatchesLegacyPaths(t *testing.T) {
	const secret = "c2VjcmV0LXNlY3JldC1zZWNyZXQtc2VjcmV0LXNlY3JldA==" // base64("secret-secret-secret-secret-secret")
	body := samplePyJSONBody()

	fromStruct, err := BuildHMACSignature(secret, "1700000000", "POST", "/orders", body)
	if err != nil {
		t.Fatal(err)
	}
	compact, _ := json.Marshal(body)
	fromString, err := BuildHMACSignature(secret, "1700000000", "POST", "/orders", string(compact))
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := MarshalPythonJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	fromFormatted, err := BuildHMACSignature(secret, "1700000000", "POST", "/orders", formatted)
	if err != nil {
		t.Fatal(err)
	}
	if fromStruct != fromString || fromStruct != fromFormatted {
		t.Fatalf("HMAC diverged: struct=%s string=%s formatted=%s", fromStruct, fromString, fromFormatted)
	}
}
