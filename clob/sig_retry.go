package clob

import "strings"

// signatureRetryNeeded 判断一条 per-order 错误是否属于"签名域选错、应改用
// NegRisk Exchange 域重签重试"的情况。
//
// 历史 bug：判据曾写死 strings.Contains(msg, "invalid signature")。但 type-3
// (CWIA / POLY_1271) 合约钱包走 EIP-1271 校验，签名域选错时服务端返回的真实
// 错误串是：
//
//	invalid POLY_1271 signature: signature does not match order hash
//
// 它**不含**连续子串 "invalid signature"（中间隔着 "POLY_1271"），所以旧判据
// 对 NegRisk 市场的 1271 钱包永不触发重试，单子永久失败。这里额外匹配
// "POLY_1271" 与 "does not match order hash" 两个特征，覆盖 1271 钱包错误串，
// 同时保留对 EOA/Proxy/Safe 老式 "invalid signature" 的兼容。
//
// V1（orders.go）与 V2（orders_v2.go）两条下单路径共用此判据。
func signatureRetryNeeded(errorMsg string) bool {
	if errorMsg == "" {
		return false
	}
	return strings.Contains(errorMsg, "invalid signature") ||
		strings.Contains(errorMsg, "POLY_1271") ||
		strings.Contains(errorMsg, "does not match order hash")
}
