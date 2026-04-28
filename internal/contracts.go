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
	// NegRiskAdapter 合约地址
	PolygonNegRiskAdapter = "0xd91E80cF2E7be2e162c6513ceD06f1dD0dA35296"
	// ProxyFactory 合约地址
	PolygonProxyFactory = "0xaB45c5A4B0c941a2F231C04C3f49182e1A254052"

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
	// CtfCollateralAdapter：V2 Exchange 的 pUSD 桥
	PolygonCtfCollateralAdapter = "0xADa100874d00e3331D00F2007a9c336a65009718"
	// NegRiskCtfCollateralAdapter：V2 NegRisk Exchange 的 pUSD 桥
	PolygonNegRiskCtfCollateralAdapter = "0xAdA200001000ef00D07553cEE7006808F895c6F1"

	// PolymarketAutoClaimer：Polymarket 后端自动 redeem 服务的合约地址。
	// 用户通过 CTF.setApprovalForAll(PolymarketAutoClaimer, true/false) 开/关自动 claim。
	// 开启后 Polymarket 服务在市场结算后自动把赢家仓位换成 pUSD 返给用户 Safe。
	// 参考链上 tx：0x22d6dcec5feb6bce8fdec26b86e1ee1b54aa333819aabc19d70f3006680cfd55
	PolymarketAutoClaimer = "0x05Cd9922a5D37faE921fc5DEe280A9dbc4C3B393"
)
