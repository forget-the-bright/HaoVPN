package autostart

import "fmt"

// LogonStatus 登录后自启是否已注册。
func LogonStatus() (enabled bool, detail string, err error) {
	return logonStatus()
}

// LogonEnable 注册「用户登录后以最高权限启动 GUI」。
//
// guiExe / configPath 须为绝对路径；Windows 用计划任务 HighestPrivileges。
func LogonEnable(guiExe, configPath string) error {
	if guiExe == "" {
		return fmt.Errorf("GUI 可执行文件路径为空")
	}
	return logonEnable(guiExe, configPath)
}

// LogonDisable 取消登录后自启。
func LogonDisable() error {
	return logonDisable()
}
