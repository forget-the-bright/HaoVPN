//go:build linux

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"haovpn/internal/fileutil"
	"haovpn/internal/logger"
)

func logonStatus() (bool, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, "", fmt.Errorf("取 HOME 失败: %w", err)
	}
	p := XDGDesktopPath(home)
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false, "未注册 XDG 登录自启（" + p + "）", nil
		}
		return false, "", err
	}
	return true, "已注册 XDG 登录自启: " + p, nil
}

func logonEnable(guiExe, configPath string) error {
	exe, cfg, err := absExeAndOptionalConfig(guiExe, configPath)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("取 HOME 失败: %w", err)
	}
	dir := XDGAutostartDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 autostart 目录失败: %w", err)
	}
	body := BuildXDGDesktopEntry(exe, cfg)
	p := XDGDesktopPath(home)
	if err := fileutil.WriteFileAtomic(p, []byte(body), 0o644); err != nil {
		return fmt.Errorf("写入 desktop 失败: %w", err)
	}
	logger.Info("autostart logon enabled xdg=%s", p)
	return nil
}

func logonDisable() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("取 HOME 失败: %w", err)
	}
	p := XDGDesktopPath(home)
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 desktop 失败: %w", err)
	}
	logger.Info("autostart logon disabled xdg=%s", p)
	return nil
}

func serviceStatus() (bool, bool, string, error) {
	p := SystemdUnitPath()
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false, false, "systemd 单元未安装（" + p + "）", nil
		}
		return false, false, "", err
	}
	running := exec.Command("systemctl", "is-active", "--quiet", SystemdUnitName).Run() == nil
	if running {
		return true, true, "systemd 单元已安装且 active: " + SystemdUnitName, nil
	}
	return true, false, "systemd 单元已安装（未 active）: " + SystemdUnitName, nil
}

func serviceInstall(exe string) error {
	abs, _, err := absExeAndOptionalConfig(exe, "")
	if err != nil {
		return err
	}
	body := BuildSystemdUnit(abs, "")
	p := SystemdUnitPath()
	if err := fileutil.WriteFileAtomic(p, []byte(body), 0o644); err != nil {
		return fmt.Errorf("写入 systemd 单元失败（须 root，见 docs/deploy.md）: %w", err)
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload 失败: %v %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("systemctl", "enable", SystemdUnitName).CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable 失败: %v %s", err, strings.TrimSpace(string(out)))
	}
	logger.Info("autostart service installed unit=%s bin=%s", SystemdUnitName, abs)
	return nil
}

func serviceStart() error {
	out, err := exec.Command("systemctl", "start", SystemdUnitName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl start 失败: %v %s", err, strings.TrimSpace(string(out)))
	}
	logger.Info("autostart service started unit=%s", SystemdUnitName)
	return nil
}

func serviceEnable(exe string, startNow bool) error {
	if err := serviceInstall(exe); err != nil {
		return err
	}
	if !startNow {
		return nil
	}
	return serviceStart()
}

func serviceDisable() error {
	_ = exec.Command("systemctl", "stop", SystemdUnitName).Run()
	_ = exec.Command("systemctl", "disable", SystemdUnitName).Run()
	p := SystemdUnitPath()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 systemd 单元失败（须 root）: %w", err)
	}
	_ = exec.Command("systemctl", "daemon-reload").Run()
	logger.Info("autostart service disabled unit=%s", SystemdUnitName)
	return nil
}

func serviceStopAndWait(timeout time.Duration) error {
	_ = timeout
	if _, err := os.Stat(SystemdUnitPath()); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	out, err := exec.Command("systemctl", "stop", SystemdUnitName).CombinedOutput()
	if err != nil {
		if exec.Command("systemctl", "is-active", "--quiet", SystemdUnitName).Run() != nil {
			return nil
		}
		return fmt.Errorf("systemctl stop 失败: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
