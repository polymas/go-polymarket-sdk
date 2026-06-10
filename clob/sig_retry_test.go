package clob

import "testing"

// TestSignatureRetryNeeded 锁定 POLY_1271 重试判据的回归。
//
// 核心回归：1271 钱包签名域选错时，服务端真实错误串为
//
//	invalid POLY_1271 signature: signature does not match order hash
//
// 它不含连续子串 "invalid signature"，旧判据（只 Contains "invalid signature"）
// 会漏判、重试永不触发、NegRisk 市场单子永久失败。本测试钉死新判据对该串返回
// true，同时保证不误伤正常成功 / orderbook 不存在等情况。
func TestSignatureRetryNeeded(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{
			// 本次修复的目标串：1271 钱包域选错。旧判据返回 false（bug）。
			name: "poly1271 nested signature mismatch",
			msg:  "invalid POLY_1271 signature: signature does not match order hash",
			want: true,
		},
		{
			// EOA/Proxy/Safe 老式签名错误，旧判据也能命中，需保持兼容。
			name: "legacy invalid signature",
			msg:  "invalid signature",
			want: true,
		},
		{
			// 仅 "does not match order hash" 特征（防服务端措辞微调）。
			name: "order hash mismatch only",
			msg:  "signature does not match order hash",
			want: true,
		},
		{
			name: "poly1271 keyword only",
			msg:  "some POLY_1271 verification error",
			want: true,
		},
		{
			name: "empty message means success",
			msg:  "",
			want: false,
		},
		{
			// orderbook 不存在是另一条剥离路径，绝不能误判为签名重试。
			name: "orderbook not exist is not a signature retry",
			msg:  "the orderbook 12345 does not exist",
			want: false,
		},
		{
			name: "unrelated error",
			msg:  "insufficient balance",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := signatureRetryNeeded(tc.msg); got != tc.want {
				t.Fatalf("signatureRetryNeeded(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}

	// 显式复刻任务描述里的"改动前 vs 改动后"断言，文档化根因：
	const poly1271 = "invalid POLY_1271 signature: signature does not match order hash"
	if contains := substr(poly1271, "invalid signature"); contains {
		t.Fatal("前提失效：POLY_1271 串竟含连续子串 \"invalid signature\"，旧判据本应漏判")
	}
	if !signatureRetryNeeded(poly1271) {
		t.Fatal("修复失效：新判据未能对 POLY_1271 串触发重试")
	}
}

// substr 是 strings.Contains 的本地别名，仅为让上面的"前提断言"读起来更贴近
// 任务描述（强调旧判据用的就是子串包含），避免再 import strings。
func substr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
