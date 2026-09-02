package web3

import (
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

// TestGetCWIAProxyAddress 验证新版 Polymarket DepositWallet 代理派生。
//
// 已知样例（链上真实部署）：
//
//	EOA   = 0x9789Bb9Cf37B243cBb3064f2828F197a0374cee4
//	proxy = 0x6CBf8F02408CCE619fE2E699f4EB4bf3d2Cc4859
//
// 派生路径：DepositWalletFactory.predictWalletAddress(impl, bytes32(owner))
// 其中 impl 通过 factory.implementation() 实时读取。
//
// 仅用作签名地址计算的纯只读 RPC 调用，不需要私钥；为复用 BaseClient 的
// 节点池/重试逻辑，这里随机生成一个一次性私钥。
func TestGetCWIAProxyAddress(t *testing.T) {
	// 一次性测试私钥（与真实账户无关；只是 NewClient 必须传一个）。
	const throwawayPK = "0000000000000000000000000000000000000000000000000000000000000001"
	const knownEOA = "0x9789Bb9Cf37B243cBb3064f2828F197a0374cee4"
	const knownProxy = "0x6CBf8F02408CCE619fE2E699f4EB4bf3d2Cc4859"

	cli, err := NewClient(throwawayPK, types.CWIASignatureType, types.Polygon)
	if err != nil {
		t.Skipf("skip: cannot create web3 client (no Polygon RPC reachable?): %v", err)
	}
	defer cli.Close()

	bc, ok := cli.(*BaseClient)
	if !ok {
		t.Fatalf("expected *BaseClient, got %T", cli)
	}

	got, err := bc.getCWIAProxyAddress(types.EthAddress(knownEOA))
	if err != nil {
		t.Skipf("skip: RPC call failed (network issue?): %v", err)
	}

	if !strings.EqualFold(string(got), knownProxy) {
		t.Fatalf("CWIA proxy mismatch:\n  owner    = %s\n  expected = %s\n  got      = %s",
			knownEOA, knownProxy, got)
	}
	t.Logf("getCWIAProxyAddress(%s) = %s ✓", knownEOA, got)
}
