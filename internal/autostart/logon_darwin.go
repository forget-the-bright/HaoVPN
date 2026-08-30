//go:build darwin

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
	p := LaunchAgentPlistPath(home)
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false, "未注册 LaunchAgent（" + p + "）", nil
		}
		return false, "", err
	}
	return true, "已注册 LaunchAgent: " + LaunchAgentLabel, nil
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
	dir := LaunchAgentsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 LaunchAgents 目录失败: %w", err)
	}
	body := BuildLaunchAgentPlist(exe, cfg)
	p := LaunchAgentPlistPath(home)
	if err := fileutil.WriteFileAtomic(p, []byte(body), 0o644); err != nil {
		return fmt.Errorf("写入 LaunchAgent 失败: %w", err)
	}
	_ = exec.Command("launchctl", "unload", p).Run()
	if out, err := exec.Command("launchctl", "load", p).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load 失败: %v %s", err, strings.TrimSpace(string(out)))
	}
	logger.Info("autostart logon enabled agent=%s path=%s", LaunchAgentLabel, p)
	return nil
}

func logonDisable() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("取 HOME 失败: %w", err)
	}
	p := LaunchAgentPlistPath(home)
	_ = exec.Command("launchctl", "unload", p).Run()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 LaunchAgent 失败: %w", err)
	}
	logger.Info("autostart logon disabled agent=%s", LaunchAgentLabel)
	return nil
}

func serviceStatus() (bool, bool, string, error) {
	p := LaunchDaemonPlistPath()
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false, false, "LaunchDaemon 未安装（" + p + "）", nil
		}
		return false, false, "", err
	}
	out, err := exec.Command("launchctl", "print", "system/"+LaunchDaemonLabel).CombinedOutput()
	running := err == nil && strings.Contains(string(out), "state = running")
	if running {
		return true, true, "LaunchDaemon 已安装且运行: " + LaunchDaemonLabel, nil
	}
	return true, false, "LaunchDaemon 已安装（可能未运行）: " + LaunchDaemonLabel, nil
}

func serviceInstall(exe string) error {
	abs, _, err := absExeAndOptionalConfig(exe, "")
	if err != nil {
		return err
	}
	body := BuildLaunchDaemonPlist(abs)
	p := LaunchDaemonPlistPath()
	if err := fileutil.WriteFileAtomic(p, []byte(body), 0o644); err != nil {
		return fmt.Errorf("写入 LaunchDaemon 失败（须 root，见 docs/deploy.md）: %w", err)
	}
	_ = exec.Command("launchctl", "bootout", "system/"+LaunchDaemonLabel).Run()
	if out, err := exec.Command("launchctl", "bootstrap", "system", p).CombinedOutput(); err != nil {
		if out2, err2 := exec.Command("launchctl", "load", p).CombinedOutput(); err2 != nil {
			return fmt.Errorf("launchctl 加载失败: %v %s / %v %s", err, strings.TrimSpace(string(out)), err2, strings.TrimSpace(string(out2)))
		}
	}
	logger.Info("autostart service installed daemon=%s bin=%s", LaunchDaemonLabel, abs)
	return nil
}

func serviceStart() error {
	if out, err := exec.Command("launchctl", "kickstart", "-k", "system/"+LaunchDaemonLabel).CombinedOutput(); err != nil {
		if out2, err2 := exec.Command("launchctl", "start", LaunchDaemonLabel).CombinedOutput(); err2 != nil {
			return fmt.Errorf("启动 LaunchDaemon 失败: %v %s / %v %s", err, strings.TrimSpace(string(out)), err2, strings.TrimSpace(string(out2)))
		}
	}
	logger.Info("autostart service started daemon=%s", LaunchDaemonLabel)
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
	p := LaunchDaemonPlistPath()
	_ = exec.Command("launchctl", "bootout", "system/"+LaunchDaemonLabel).Run()
	_ = exec.Command("launchctl", "unload", p).Run()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 LaunchDaemon 失败（须 root）: %w", err)
	}
	logger.Info("autostart service disabled daemon=%s", LaunchDaemonLabel)
	return nil
}

func serviceStopAndWait(timeout time.Duration) error {
	_ = timeout
	p := LaunchDaemonPlistPath()
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_ = exec.Command("launchctl", "kill", "SIGTERM", "system/"+LaunchDaemonLabel).Run()
	_ = exec.Command("launchctl", "stop", LaunchDaemonLabel).Run()
	return nil
}
