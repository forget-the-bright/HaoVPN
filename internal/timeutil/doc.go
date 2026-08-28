// Package timeutil 提供 SQLite 文本时间列的统一 layout 与解析/格式化。
//
// 上游：persist（haovpn.db）、logstore（logs.db）及任何写入 UTC 文本时间列的代码。
// 下游：仅标准库 time / database/sql。
// 为何独立成包：避免 logstore 为共用格式而 import persist（层次倒置）。
// 不变量：FormatUTC 始终输出 UTC；ParseUTC 失败返回零值，不 panic。
package timeutil
