package clientapp

import "strings"

// SingleInstanceHint 根据单实例冲突场景返回 stderr 提示（纯函数，便于单测）。
//
// serviceRunning 为 true 表示 Windows 服务 HaoVPNClient 仍在跑；否则为 CLI/GUI 互斥。
func SingleInstanceHint(serviceRunning bool) string {
	if serviceRunning {
		return strings.TrimSpace(`客户端已在运行（Windows 服务 HaoVPNClient 占用单实例锁）。
处理建议：用 haovpn-client-gui 托盘「停止服务并接管」；或执行 haovpn-client --service stop 后再启动 CLI。`)
	}
	return strings.TrimSpace(`客户端已在运行（CLI/GUI/服务共用单实例锁）。
处理建议：结束已有 haovpn-client 或 haovpn-client-gui 进程；或托盘「退出」。`)
}

// SingleInstanceUserMessage 供 GUI 对话框展示的单实例文案（与 SingleInstanceHint 同源）。
func SingleInstanceUserMessage(serviceRunning bool) string {
	return SingleInstanceHint(serviceRunning)
}
