//go:build !windows && !linux && !darwin

package autostart

import (
	"fmt"
	"time"
)

// 其它 Unix（如 FreeBSD）：尚无具体实现，Enable/Disable 一律返回明确错误（禁止伪成功）。

func logonStatus() (bool, string, error) {
	return false, "本平台未实现 GUI 登录自启（见 docs/deploy.md）", nil
}

func logonEnable(guiExe, configPath string) error {
	_ = guiExe
	_ = configPath
	return fmt.Errorf("本平台未实现 GUI 登录自启，请手工配置（见 docs/deploy.md）")
}

func logonDisable() error {
	return fmt.Errorf("本平台未实现 GUI 登录自启，无法 Disable（见 docs/deploy.md）")
}

func serviceStatus() (bool, bool, string, error) {
	return false, false, "本平台未实现开机无界面服务（见 docs/deploy.md）", nil
}

func serviceInstall(exe string) error {
	_ = exe
	return fmt.Errorf("本平台未实现开机无界面服务安装（见 docs/deploy.md）")
}

func serviceStart() error {
	return fmt.Errorf("本平台未实现开机无界面服务启动（见 docs/deploy.md）")
}

func serviceEnable(exe string, startNow bool) error {
	_ = exe
	_ = startNow
	return fmt.Errorf("本平台未实现开机无界面服务（见 docs/deploy.md）")
}

func serviceDisable() error {
	return fmt.Errorf("本平台未实现开机无界面服务，无法 Disable（见 docs/deploy.md）")
}

func serviceStopAndWait(timeout time.Duration) error {
	_ = timeout
	return fmt.Errorf("本平台未实现开机无界面服务，无法 Stop（见 docs/deploy.md）")
}
