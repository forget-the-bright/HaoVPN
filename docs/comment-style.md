# 注释规范（HaoVPN）

> 与 [development-principles.md](development-principles.md) 配套；所有 production 代码须遵守。

## 语言

- **导出符号**（类型、函数、常量）：中文 godoc，禁止纯英文。
- **包说明**：写在 `doc.go`，含上游/下游/并发/不变量。
- **非导出复杂逻辑**：可用简短中文行内注释说明「为什么」。

## Package（doc.go）

```go
// Package xxx 一句话职责。
//
// 上游：谁调用本包（如 cmd/client → clientapp）。
// 下游：本包依赖谁（如 transport、tunnel）。
// 并发：是否启动 goroutine、谁负责 Close/Teardown。
// 不变量：失败时必须保证什么（如 kill-switch 先阻断再清路由）。
```

## 导出类型

```go
// TypeName 在何种流程中出现（如「CLI/GUI 共用拨号引擎」）。
//
// 字段：
//   FieldA — 含义；何时写入；空值语义。
// 线程安全：调用方是否需持锁；是否仅单 goroutine 访问。
```

## 导出函数/方法

```go
// FuncName 做什么（一句话）。
//
// 参数：name — 约束（非空、格式、是否允许零值）。
// 返回：ok 表示…；err 常见原因（配置无效、权限不足）。
// 副作用：写 DB、改路由、踢线等。
// 并发：可否并行调用；是否阻塞。
```

## 配置（config）

- 每个 YAML 段在类型注释说明用途。
- 字段注明：默认值来源（ApplyDefaults）、是否可被握手覆盖。

## 长流程

`Run()`、`onConnect`、`RegisterVPN` 等用阶段注释：

```go
// --- 阶段 1：加载配置与日志 ---
```

## Web / 脚本

- `web/README.md` 说明 embed 与模板关系。
- `app.js` 文件头 + 主要函数一行说明。
