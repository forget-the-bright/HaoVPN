// Package paginate 提供分页参数规范化等无业务状态的纯函数。
//
// 上游：api（HTTP limit/offset）、persist（SQL LIMIT）、logstore（历史日志分页）。
// 下游：无（标准库 only）。
// 并发：纯函数，任意 goroutine 可并行调用。
// 不变量：ClampLimit 保证返回值在 [defaultVal, maxVal] 内；ParseIntDefault 解析失败回退默认值。
package paginate
