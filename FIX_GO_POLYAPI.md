# 修复 go-polyapi 项目中的模块路径问题

## 问题描述

`go-polyapi` 项目中使用了旧的导入路径 `github.com/polymarket/go-polymarket-sdk`，但实际仓库路径已更改为 `github.com/polymas/go-polymarket-sdk`。

## 解决方案

### 方案1：使用 replace 指令（推荐，快速修复）

在 `go-polyapi` 项目的 `go.mod` 文件中添加 `replace` 指令：

```go
module github.com/polymarket/go-polyapi

go 1.24

require (
    // ... 其他依赖
    github.com/polymarket/go-polymarket-sdk v0.0.2
)

// 添加这行，将旧路径重定向到新路径
replace github.com/polymarket/go-polymarket-sdk => github.com/polymas/go-polymarket-sdk v0.0.2
```

然后执行：
```bash
cd /path/to/go-polyapi
go mod tidy
```

### 方案2：更新所有导入路径（永久解决方案）

1. **更新所有 Go 文件中的导入路径**：

```bash
cd /path/to/go-polyapi
find . -name "*.go" -type f -exec sed -i '' 's|github.com/polymarket/go-polymarket-sdk|github.com/polymas/go-polymarket-sdk|g' {} \;
```

2. **更新 go.mod**：

```bash
# 删除旧依赖
go mod edit -droprequire=github.com/polymarket/go-polymarket-sdk

# 添加新依赖
go get github.com/polymas/go-polymarket-sdk@v0.0.2

# 整理依赖
go mod tidy
```

### 方案3：清理模块缓存后重试

如果上述方案都不行，清理模块缓存：

```bash
# 清理所有模块缓存
go clean -modcache

# 然后重新执行 go mod tidy
cd /path/to/go-polyapi
go mod tidy
```

## 验证

执行以下命令验证修复是否成功：

```bash
go list -m github.com/polymas/go-polymarket-sdk
```

应该显示：
```
github.com/polymas/go-polymarket-sdk v0.0.2
```

## 注意事项

- 如果使用 `replace` 指令，需要确保所有团队成员都添加了相同的 `replace` 指令
- 推荐使用方案2（更新导入路径），这是永久解决方案
- 清理模块缓存可能会删除其他项目的缓存，请谨慎操作
