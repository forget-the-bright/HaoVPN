//go:build !windows

package autostart

import "fmt"

func logonStatus() (bool, string, error) {
	return false, "本平台请用 systemd/launchd（见 docs/deploy.md）；GUI 登录自启未实现", nil
}

func logonEnable(guiExe, configPath string) error {
	return fmt.Errorf("本平台不支持 GUI 登录自启计划任务，请用 systemd/launchd 配置 CLI（见 docs/deploy.md）")
}

func logonDisable() error {
	return nil
}
