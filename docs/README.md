# 文档导航

文档按“先使用、后深入、历史归档”组织。第一次接入从[快速开始](getting-started.md)进入，不需要顺序阅读所有文件。

## 使用者文档

| 文档 | 适用场景 |
| --- | --- |
| [快速开始与配置](getting-started.md) | 安装、只读/认证客户端、钱包类型、环境变量与安全边界 |
| [API 与包概览](api-overview.md) | 找到正确的包、构造函数和 API 事实来源 |
| [V2 业务接入](v2-guide.md) | pUSD、授权、auto-claim、split/merge/redeem 与 gasless 流程 |
| [示例索引](../examples/README.md) | 按目标选择可运行示例，并识别诊断脚本与写操作 |
| [Signing 包说明](../signing/README.md) | 直接使用 signer、HMAC 或签名底层能力 |

## 维护者文档

| 文档 | 适用场景 |
| --- | --- |
| [架构说明](architecture.md) | 模块边界、依赖方向和扩展原则 |
| [测试指南](testing.md) | 单元、联网、认证和真实写入测试 |
| [不兼容变更](breaking-changes.md) | 已移除包、错误 API 和路径迁移方式 |
| [兼容性审计与路线图](roadmap.md) | 已核对事项、缺口和后续批次 |
| [历史归档](archive/README.md) | 旧迁移说明、开发日志和时点性报告 |

## 维护原则

- README 只承担定位、安装和最短可运行路径。
- Markdown 解释使用方式和跨包约定，不复制完整方法清单。
- 导出 API 的签名和注释以 Go 源码、`go doc`、pkg.go.dev 为准。
- 会随外部协议变化的结论标注日期；过期材料移入 `archive/`，不与当前指南并列。
- 可运行代码集中在 `examples/`，文档中的片段保持短小。
