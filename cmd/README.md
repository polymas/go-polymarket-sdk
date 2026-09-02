# 命令行工具

`cmd/` 保存可直接运行的维护工具，每个叶子目录对应一个独立命令。

```text
cmd/
├── check-addrs/
├── poly1271-negrisk-repro/
└── diagnostics/
```

- 顶层命令用于名称稳定、目标明确的维护任务。
- 临时协议核对、生产探针、修复和冒烟程序放在 [`diagnostics/`](diagnostics/README.md)。
- 面向 SDK 使用者的教学代码放在 [`examples/`](../examples/README.md)。
