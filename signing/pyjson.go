package signing

import (
	"encoding/json"
	"regexp"
)

// CLOB 的 HMAC 签名在服务端按 Python `str(body).replace("'", '"')` 的字节序列
// 校验，所以 Go 侧必须把 encoding/json 的紧凑输出改写成带空格的 Python 风格。
// 三条正则在整个 SDK 里只编译一次；此前每次下单/撤单都在热路径里重新编译。
var (
	pyJSONColonSpace = regexp.MustCompile(`":(\S)`)
	pyJSONCommaQuote = regexp.MustCompile(`,(")`)
	pyJSONCommaOpen  = regexp.MustCompile(`,(\{|\[)`)
)

// FormattedJSON 标记一个已经过 FormatPythonJSON 处理的请求体。传给
// BuildHMACSignature 时直接参与签名，不再重复 marshal 和正则改写。
type FormattedJSON string

// FormatPythonJSON 把紧凑 JSON 改写成 Python json.dumps 默认的
// `{"k": "v", "k2": [1, 2]}` 空格风格。幂等：对已格式化的输入再次调用不变。
func FormatPythonJSON(compact string) string {
	s := pyJSONColonSpace.ReplaceAllString(compact, `": $1`)
	s = pyJSONCommaQuote.ReplaceAllString(s, `, $1`)
	return pyJSONCommaOpen.ReplaceAllString(s, `, $1`)
}

// MarshalPythonJSON 序列化 v 并直接输出 Python 风格字节，供请求体和 HMAC
// 共用同一份，避免"发送体 marshal 一次、签名再 marshal 一次"。
func MarshalPythonJSON(v any) (FormattedJSON, error) {
	compact, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return FormattedJSON(FormatPythonJSON(string(compact))), nil
}
