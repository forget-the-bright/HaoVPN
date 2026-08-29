// Package timeutil 提供 SQLite 文本时间列的统一 layout、解析/格式化，以及配置秒→Duration 映射。
//
// 上游：persist（haovpn.db）、logstore（logs.db）及任何写入 UTC 文本时间列的代码；
// 传输层/鉴权/探针等把 YAML 秒数转为 time.Duration 时用 Seconds。
// 下游：仅标准库 time / database/sql。
// 为何独立成包：避免 logstore 为共用格式而 import persist（层次倒置）。
// 不变量：FormatUTC 始终输出 UTC；ParseUTC 失败返回零值，不 panic。
package timeutil
