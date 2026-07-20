package internal

// 智能合约地址配置（Polygon 主网）
const (
	// Relay 相关合约地址
	RelayHub     = "0xD216153c06E857cD7f72665E0aF1d7D82172F494"
	RelayAddress = "0x7db63fe6d62eb73fb01f8009416f4c2bb4fbda6a"

	// Safe 代理工厂地址
	SafeProxyFactory = "0xaacFeEa03eb1561C4e67d661e40682Bd20E3541b"

	// Safe multiSend 合约地址
	SafeMultiSend = "0xA238CBeb142c10Ef7Ad8442C6D1f9E89e07e7761"

	// 零地址（用于 Gas Token 和 Refund Receiver）
	ZeroAddress = "0x0000000000000000000000000000000000000000"

	// Polymarket 合约地址（Polygon 主网）
	// Exchange 合约地址（Regular）
	PolygonExchange = "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"
	// Exchange 合约地址（Negative Risk）
	PolygonNegRiskExchange = "0xC5d563A36AE78145C45a50134d48A1215220f80a"
	// Collateral 合约地址（USDC）
	PolygonCollateral = "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"
	// ConditionalTokens 合约地址
	PolygonConditionalTokens = "0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"
	// PolygonNegRiskAdapter is retired (2026-07-17). It is retained only for
	// decoding historical transactions/events. Never use it as a V2 call target,
	// ERC-20 spender, or ERC-1155 operator.
	PolygonNegRiskAdapter = "0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296"
	// Legacy WrappedCollateral：旧 NegRiskAdapter 内部 wrap 出来的 USDC.e ERC20。
	// NegRisk position token 在 CTF 里的 collateralToken 实际是它（不是 USDC.e）。
	// 仅用于历史 V1 状态/事件解释；V2 写路径不得使用。
	PolygonNegRiskWrappedCollateral = "0x3A3BD7bb9528E159577F7C2e685CC81A765002E2"
	// ProxyFactory 合约地址
	PolygonProxyFactory = "0xaB45c5A4B0c941a2F231C04C3f49182e1A254052"

	// DepositWalletFactory：Polymarket 新版代理钱包工厂（ERC-7760 / ERC-1967 +
	// immutable args）。新注册账号默认走这套代理；与老的 PolyProxy / Safe 并存，
	// 老账号不迁移。派生函数：predictWalletAddress(address impl, bytes32 id)。
	// 来源：proxy 0x6CBf…4859 -> impl 0x58cA…b1eB -> factory.implementation()
	// = 0x58cA…b1eB（与样例匹配）。如未来 Polymarket 升级 impl，本 SDK 会通过
	// 调用 factory.implementation() 自动获取最新 impl，无需改代码。
	PolygonDepositWalletFactory = "0x00000000000Fb5C9ADea0298D729A0CB3823Cc07"
	// Legacy UUPS DepositWallet implementation. It is part of the deterministic
	// address formula and must not be replaced by the Beacon implementation.
	PolygonDepositWalletImpl = "0x58cA52EbE0dAdFDf531CDe7062E76746de4Db1eB"
	// Beacon used by new ERC-1967 BeaconProxy Deposit Wallet clones.
	PolygonDepositWalletBeacon = "0x7A18EDfe055488A3128f01F563e5B479D92ffc3a"

	// V2 Exchange 合约地址（2026-04-28 切换后启用）
	// 参考：https://docs.polymarket.com/resources/contracts
	PolygonExchangeV2        = "0xE111180000d2663C0091e4f400237545B87B996B"
	PolygonNegRiskExchangeV2 = "0xe2222d279d744050d28e00520010520000310F59"

	// V2 collateral：pUSD（1:1 USDC.e 封装，V2 CLOB 真正记账用的 token）
	PolygonPUSD = "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB"
	// CollateralOnramp：USDC.e → pUSD wrap
	PolygonCollateralOnramp = "0x93070a847efEf7F70739046A929D47a521F5B8ee"
	// CollateralOfframp：pUSD → USDC.e unwrap
	PolygonCollateralOfframp = "0x2957922Eb93258b93368531d39fAcCA3B4dC5854"
	// CtfCollateralAdapter：V2 Exchange 的 pUSD 桥。
	// 2026-05 迁移：Polymarket 把 V2 adapter 重新部署到新 vanity 地址，relayer 白名单
	// 与官方 ts-sdk/py-sdk 一并切到新地址；旧部署链上仍在但 relayer 不再放行（提交即
	// STATE_FAILED）。来源：Polymarket/ts-sdk packages/client/src/environments.ts +
	// Polymarket/py-sdk src/polymarket/environments.py（两个驱动 gasless 的官方 SDK 一致）。
	// 旧部署（deprecated，勿用）：0xADa100874d00e3331D00F2007a9c336a65009718
	PolygonCtfCollateralAdapter = "0xAdA100Db00Ca00073811820692005400218FcE1f"
	// NegRiskCtfCollateralAdapter：V2 NegRisk Exchange 的 pUSD 桥。
	// 旧部署（deprecated，勿用）：0xAdA200001000ef00D07553cEE7006808F895c6F1
	PolygonNegRiskCtfCollateralAdapter = "0xadA2005600Dec949baf300f4C6120000bDB6eAab"

	// PolymarketAutoClaimer：Polymarket V2 后端自动 redeem 服务的合约地址。
	// 用户通过 CTF.setApprovalForAll(PolymarketAutoClaimer, true/false) 开/关自动 claim。
	// 开启后 Polymarket 服务在市场结算后自动把赢家仓位换成 pUSD 返给用户 Safe。
	// 来源：从前端 SAFE setApprovalForAll(operator, true) 的 calldata 反推。
	PolymarketAutoClaimer = "0xf3cFb6A6EbfEB51876289Eb235719eb1c65252b0"

	// PolymarketAutoClaimerDeprecated：早期 V1 试验地址，前端已弃用。
	// 留作历史标记，便于排查老 Safe 上的残留 approval。
	PolymarketAutoClaimerDeprecated = "0x05Cd9922a5D37faE921fc5DEe280A9dbc4C3B393"
)
