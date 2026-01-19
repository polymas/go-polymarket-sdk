# 测试文档

本文档说明如何配置和运行Polymarket SDK的集成测试。

## 测试结构

测试文件按包组织，每个包有对应的测试文件：

- `data/client_test.go` - Data客户端测试（只读）
- `gamma/client_test.go` - Gamma客户端测试（只读）
- `clob/client_readonly_test.go` - CLOB客户端只读接口测试
- `clob/client_readwrite_test.go` - CLOB客户端读写接口测试（需要认证）
- `web3/client_test.go` - Web3客户端测试（只读）
- `web3/gasless_test.go` - Gasless客户端测试（读写，需要认证）
- `websocket/client_test.go` - WebSocket客户端测试（只读）
- `websocket/sports_client_test.go` - Sports WebSocket客户端测试（只读）
- `rfq/client_test.go` - RFQ客户端测试（读写，需要认证）
- `rtds/client_test.go` - RTDS客户端测试（只读）
- `subgraph/client_test.go` - Subgraph客户端测试（只读）

测试辅助函数位于 `test/helpers.go`，包含环境变量读取、客户端初始化等通用功能。

## 环境变量配置

### 必需的环境变量（读写测试）

以下环境变量对于读写接口测试是必需的：

```bash
# 私钥（必需）
export POLY_PRIVATE_KEY="your_private_key_here"

# 链ID（可选，默认137主网）
export POLY_CHAIN_ID=137

# 签名类型（可选，默认0=EOA）
# 0 = EOA, 1 = Proxy, 2 = Safe
export POLY_SIGNATURE_TYPE=0

# Builder API凭证（Gasless测试需要）
export POLY_BUILDER_API_KEY="your_builder_api_key"
export POLY_BUILDER_SECRET="your_builder_secret"
export POLY_BUILDER_PASSPHRASE="your_builder_passphrase"
```

### 可选的环境变量（测试数据）

以下环境变量用于提供测试数据，如果未设置，某些测试可能会跳过：

```bash
# 测试用户地址
export POLY_TEST_USER_ADDRESS="0x1234567890123456789012345678901234567890"

# 测试代币ID
export POLY_TEST_TOKEN_ID="1234567890123456789012345678901234567890123456789012345678901234"

# 测试条件ID
export POLY_TEST_CONDITION_ID="0x1234567890123456789012345678901234567890123456789012345678901234"

# 测试市场ID
export POLY_TEST_MARKET_ID="12345"

# 测试市场slug
export POLY_TEST_MARKET_SLUG="test-market-slug"

# 测试事件ID
export POLY_TEST_EVENT_ID=12345

# RTDS认证token（可选）
export POLY_RTDS_TOKEN="your_rtds_token"
```

## 运行测试

### 运行所有测试

```bash
go test ./...
```

### 运行只读测试（快速测试）

只读测试不需要认证，可以快速运行：

```bash
# 运行所有只读测试
go test ./data/... ./gamma/... ./clob/... -run Readonly ./web3/... -run "^(?!.*Gasless)" ./websocket/... ./rtds/... ./subgraph/...

# 或者使用short标志（如果测试支持）
go test -short ./...
```

### 运行特定包的测试

```bash
# Data包测试
go test ./data/

# Gamma包测试
go test ./gamma/

# CLOB只读测试
go test ./clob/ -run Readonly

# CLOB读写测试（需要认证）
go test ./clob/ -run Readwrite

# Web3测试
go test ./web3/ -run "^(?!.*Gasless)"

# Gasless测试（需要认证）
go test ./web3/ -run Gasless

# WebSocket测试
go test ./websocket/

# RFQ测试（需要认证）
go test ./rfq/

# RTDS测试
go test ./rtds/

# Subgraph测试
go test ./subgraph/
```

### 运行特定测试函数

```bash
# 运行GetPositions测试
go test ./data/ -run TestGetPositions

# 运行GetMarket测试
go test ./gamma/ -run TestGetMarket
```

### 详细输出

使用 `-v` 标志查看详细输出：

```bash
go test -v ./data/
```

### 并行运行测试

使用 `-parallel` 标志并行运行测试（注意：某些测试可能不支持并行运行）：

```bash
go test -parallel 4 ./...
```

## 测试注意事项

### 读写测试安全

1. **使用测试账户**：读写测试会实际创建/取消订单、赎回仓位等操作，请使用测试账户，避免影响生产环境。

2. **资金要求**：某些读写测试（如创建订单、赎回仓位）需要账户有足够的资金。

3. **测试清理**：测试后可能会留下测试数据（如创建的订单、API密钥等），请手动清理或使用测试账户。

4. **交易确认**：某些测试需要等待交易确认，可能需要较长时间。

### WebSocket测试

1. **异步消息**：WebSocket测试需要处理异步消息，测试中使用了超时机制。

2. **连接稳定性**：网络不稳定可能导致连接失败，测试会重试。

3. **消息接收**：某些测试可能不会立即收到消息，这是正常的（取决于市场活动）。

### 测试跳过

如果缺少必需的环境变量或测试数据，测试会自动跳过并显示跳过原因。这是正常行为，不会导致测试失败。

## 测试覆盖

当前测试覆盖了所有102个接口：

- **只读接口**：78个
- **读写接口**：24个

每个接口至少包含：
- 基本功能测试
- 边界条件测试
- 错误处理测试

## 故障排除

### 常见错误

1. **导入循环错误**：确保测试文件使用正确的导入路径，不要创建循环依赖。

2. **认证失败**：检查环境变量是否正确设置，私钥格式是否正确。

3. **网络错误**：检查网络连接，某些测试需要访问外部API。

4. **超时错误**：某些测试（如WebSocket）可能需要更长的超时时间。

### 调试技巧

1. 使用 `-v` 标志查看详细输出
2. 使用 `-run` 标志运行特定测试
3. 检查环境变量是否正确设置
4. 查看测试日志了解具体错误信息

## 贡献

添加新测试时，请遵循以下规范：

1. 测试文件命名：`*_test.go`
2. 测试函数命名：`TestFunctionName`
3. 使用 `test.SkipIfNoAuth` 等辅助函数跳过需要认证的测试
4. 添加适当的错误处理和日志输出
5. 确保测试独立，不依赖其他测试的执行顺序
