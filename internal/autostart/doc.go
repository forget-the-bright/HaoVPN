// Package autostart 管理客户端「登录后自启」与「开机无界面服务」。
//
// 两条能力与平台实现状态：
//
//	| 能力           | Windows                         | Linux                              | macOS                                   |
//	|----------------|---------------------------------|------------------------------------|-----------------------------------------|
//	| 登录后起 GUI   | 计划任务 ONLOGON + Highest      | XDG ~/.config/autostart/*.desktop  | LaunchAgent ~/Library/LaunchAgents      |
//	| 开机无界面服务 | SCM（brand.WinServiceName）     | systemd /etc/systemd/system（须 root） | LaunchDaemon /Library/LaunchDaemons（须 root） |
//
// 关键文件：gen.go（纯函数生成物；systemd ExecStart 对含空格路径加引号）；
// paths_unix.go（linux|darwin：AbsPair 解析 exe+可选配置）；
// logon_*.go / service_*.go / stub_other.go。
//
// 安装/启停经本包导出 API；clientapp 仅保留「service」无界面主循环与 Windows CLI 薄封装。
package autostart

import "haovpn/internal/brand"

// LogonTaskName Windows 计划任务名（登录后最高权限启动 GUI）。
const LogonTaskName = "HaoVPNClientGUI"

// ServiceName 与 brand.WinServiceName 一致，避免 CLI/GUI 装两套服务。
func ServiceName() string { return brand.WinServiceName }
