// Package logger 提供分级日志、文件滚动、live.log 同步输出与可选历史库写入钩子。
//
// 入口通过 config.LogSection.InitGlobal 初始化；业务包直接调用 Info/Warn/Error。
package logger
