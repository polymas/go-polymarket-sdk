# 环境变量配置指南

本文档说明如何配置 Polymarket SDK 所需的环境变量。

## 快速开始

1. 复制示例文件：
   ```bash
   cp .env.example .env
   ```

2. 编辑 `.env` 文件，填入你的实际凭证

3. 加载环境变量：
   ```bash
   source .env
   ```

4. 运行测试：
   ```bash
   go test ./web3 -run TestSplitAndMergeBTC4H
   ```

## 环境变量说明

### 必需变量

| 变量名                | 说明                      | 示例           |
| --------------------- | ------------------------- | -------------- |
| `POLY_PRIVATE_KEY`    | Web3 私钥（带 0x 前缀）   | `0x1234...`    |
| `POLY_API_KEY`        | Polymarket API Key        | `019a7120-...` |
| `POLY_API_SECRET`     | Polymarket API Secret     | `VoFDPxQ...`   |
| `POLY_API_PASSPHRASE` | Polymarket API Passphrase | `66c1e1c0...`  |

### 可选变量

| 变量名                   | 说明                  | 默认值 | 可选值                         |
| ------------------------ | --------------------- | ------ | ------------------------------ |
| `POLY_SIGNATURE_TYPE`    | 签名类型              | `2`    | `1`=Proxy, `2`=Safe            |
| `POLY_TEST_MARKET_SLUG`  | 测试市场 slug         | -      | 如: `btc-updown-4h-1769086800` |
| `POLY_TEST_CONDITION_ID` | 测试市场 condition ID | -      | 如: `0x7114...`                |

## 签名类型说明

### Proxy (1)
- 使用 Polymarket Proxy Wallet
- 适用于大多数用户
- Proxy wallet 地址通过 Exchange 合约的 `getPolyProxyWalletAddress` 获取

### Safe (2)
- 使用 Gnosis Safe Wallet
- 适用于需要多签功能的用户
- Safe wallet 地址通过 SafeProxyFactory 的 `computeProxyAddress` 获取

## 示例配置

### Proxy Wallet 配置
```bash
export POLY_PRIVATE_KEY="0x..."
export POLY_API_KEY="your-api-key"
export POLY_API_SECRET="your-api-secret"
export POLY_API_PASSPHRASE="your-api-passphrase"
export POLY_SIGNATURE_TYPE=1
```

### Safe Wallet 配置
```bash
export POLY_PRIVATE_KEY="0x..."
export POLY_API_KEY="your-api-key"
export POLY_API_SECRET="your-api-secret"
export POLY_API_PASSPHRASE="your-api-passphrase"
export POLY_SIGNATURE_TYPE=2
```

## 获取 API 凭证

API 凭证可以从 Polymarket 网站获取：
1. 登录 Polymarket
2. 进入 API 设置页面
3. 创建新的 API Key
4. 保存 Key、Secret 和 Passphrase

## 注意事项

1. **安全**: `.env` 文件包含敏感信息，不要提交到版本控制系统
2. **格式**: 私钥必须包含 `0x` 前缀
3. **签名类型**: 确保 `POLY_SIGNATURE_TYPE` 与你的钱包类型匹配，否则会获取到错误的 proxy 地址
