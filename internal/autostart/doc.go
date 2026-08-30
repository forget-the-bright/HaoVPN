// Package autostart 管理客户端「登录后自启」与「开机无界面服务」。
//
// 两条能力与平台实现状态：
//
//   | 能力           | Windows                         | Linux / macOS                          |
//   |----------------|---------------------------------|----------------------------------------|
//   | 登录后起 GUI   | 计划任务 ONLOGON + Highest      | 未实现（Enable 返回提示）；应对 XDG/LaunchAgent |
//   | 开机无界面服务 | SCM 服务 HaoVPNClient           | 未实现（Enable 返回提示）；应对 systemd/LaunchDaemon |
//
// GUI 托盘两个开关在非 Windows 上可点，但不会写入系统配置；现场请按 docs/deploy.md §5.3 手工配置 CLI。
package autostart

import "haovpn/internal/brand"

// LogonTaskName Windows 计划任务名（登录后最高权限启动 GUI）。
const LogonTaskName = "HaoVPNClientGUI"

// ServiceName 与 brand.WinServiceName 一致，避免 CLI/GUI 装两套服务。
func ServiceName() string { return brand.WinServiceName }
