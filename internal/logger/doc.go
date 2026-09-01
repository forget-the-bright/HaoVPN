// Package logger 提供全局分级日志（Info/Warn/Error）与 live.log 同步写盘。
//
// 关键文件：logger.go — InitGlobal/Info/Warn/Error；redact.go — Authorization/session 脱敏。
//
// 上游：cmd/*、serverapp、clientapp、api 等全项目。
// 下游：os 文件、可选 logstore 历史写入回调。
// 并发：InitGlobal 后多 goroutine 可并发写；内部 mutex 保护文件句柄。
// 不变量：密码/token/私钥禁止出现在日志行；RecentErrors 供 health API。
package logger
