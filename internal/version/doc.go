// Package version 从构建时注入或 VERSION 文件读取产品版本字符串。
//
// 上游：cmd/* -ldflags、api /system/info、clientgui 登录页展示。
// 下游：根目录 VERSION（仅开发者维护，AI 禁止修改）。
// 并发：只读变量，无锁。
// 不变量：运行时不得硬编码版本号；String() 供 UI 与日志。
package version
