// Package clientgui Fyne 桌面客户端 UI（登录、主窗口、托盘、日志面板）。
//
// 关键文件：
//
//	run.go — 创建 Fyne 应用并进入事件循环
//	login.go — 登录窗与连接逻辑
//	app.go — 主窗口、轮询、登出与 shutdown
//	log.go — 日志缓冲与 logger sink
//
// 上游：cmd/client-gui（标志解析、UAC、单实例、主题）。
// 下游：clientapp（Engine）、config、credentials、platform、logger。
// 并发：appendLog 持 logMu；状态轮询在独立 goroutine，UI 更新经 fyne.Do。
package clientgui
