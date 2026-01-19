# 模块路径迁移指南

## 问题说明

如果遇到以下错误：
```
go: github.com/polymarket/go-polymarket-sdk@v0.0.0: reading ... 404 Not Found
```

这是因为项目已经从 `github.com/polymarket/go-polymarket-sdk` 迁移到 `github.com/polymas/go-polymarket-sdk`。

## 解决方案

### 方案1：更新导入路径（推荐）

如果您的项目使用了本 SDK，需要更新所有导入路径：

**旧路径：**
```go
import "github.com/polymarket/go-polymarket-sdk/client"
import "github.com/polymarket/go-polymarket-sdk/types"
```

**新路径：**
```go
import "github.com/polymas/go-polymarket-sdk/client"
import "github.com/polymas/go-polymarket-sdk/types"
```

然后更新 `go.mod`：
```bash
go mod edit -replace=github.com/polymarket/go-polymarket-sdk=github.com/polymas/go-polymarket-sdk@v0.0.2
go mod tidy
```

### 方案2：清理模块缓存

如果 Go 仍然尝试从旧路径获取模块，清理模块缓存：

```bash
# 清理所有模块缓存
go clean -modcache

# 或者只清理特定模块
go clean -modcache -i github.com/polymarket/go-polymarket-sdk
```

### 方案3：使用 replace 指令（临时方案）

在您的项目的 `go.mod` 中添加 replace 指令：

```go
module your-project

go 1.24

require (
    github.com/polymas/go-polymarket-sdk v0.0.2
)

replace github.com/polymarket/go-polymarket-sdk => github.com/polymas/go-polymarket-sdk v0.0.2
```

### 方案4：更新依赖版本

如果您的项目通过 `go get` 安装，使用新路径：

```bash
# 删除旧依赖
go mod edit -droprequire=github.com/polymarket/go-polymarket-sdk

# 添加新依赖
go get github.com/polymas/go-polymarket-sdk@v0.0.2

# 更新所有导入路径
find . -name "*.go" -type f -exec sed -i '' 's|github.com/polymarket/go-polymarket-sdk|github.com/polymas/go-polymarket-sdk|g' {} \;

# 整理依赖
go mod tidy
```

## 验证

验证模块是否正确下载：

```bash
go list -m github.com/polymas/go-polymarket-sdk
```

应该显示：
```
github.com/polymas/go-polymarket-sdk v0.0.2
```

## 可用版本

- `v0.0.1` - 初始版本，包含安全修复
- `v0.0.2` - 修复模块路径问题（推荐使用）

## 获取最新版本

```bash
go get github.com/polymas/go-polymarket-sdk@latest
```

或指定版本：

```bash
go get github.com/polymas/go-polymarket-sdk@v0.0.2
```
