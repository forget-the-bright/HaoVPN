//go:build windows

package autostart

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"haovpn/internal/fileutil"
)

func logonStatus() (bool, string, error) {
	out, err := runSchtasks("/Query", "/TN", LogonTaskName, "/FO", "LIST")
	if err != nil {
		// 任务不存在时 schtasks 非 0
		if strings.Contains(strings.ToLower(out+err.Error()), "cannot find") ||
			strings.Contains(out, "错误:") || strings.Contains(out, "ERROR:") {
			return false, "未注册登录自启任务", nil
		}
		// 部分系统中文：找不到
		if strings.Contains(out, "找不到") || strings.Contains(out, "不存在") {
			return false, "未注册登录自启任务", nil
		}
		return false, "", fmt.Errorf("查询计划任务失败: %v (%s)", err, strings.TrimSpace(out))
	}
	return true, "已注册计划任务 " + LogonTaskName + "（登录后最高权限启动 GUI）", nil
}

func logonEnable(guiExe, configPath string) error {
	exe, cfg, err := fileutil.AbsPair(guiExe, configPath)
	if err != nil {
		return err
	}
	// /TR 整段命令；路径含空格须引号。Highest = 管理员，避免每次 UAC。
	tr := fmt.Sprintf(`"%s"`, exe)
	if cfg != "" {
		tr = fmt.Sprintf(`"%s" -c "%s"`, exe, cfg)
	}
	out, err := runSchtasks(
		"/Create", "/TN", LogonTaskName,
		"/TR", tr,
		"/SC", "ONLOGON",
		"/RL", "HIGHEST",
		"/F",
	)
	if err != nil {
		return fmt.Errorf("创建计划任务失败（须管理员）: %v %s", err, strings.TrimSpace(out))
	}
	return nil
}

func logonDisable() error {
	out, err := runSchtasks("/Delete", "/TN", LogonTaskName, "/F")
	if err != nil {
		msg := strings.ToLower(out + err.Error())
		if strings.Contains(msg, "cannot find") || strings.Contains(out, "找不到") || strings.Contains(out, "不存在") {
			return nil
		}
		return fmt.Errorf("删除计划任务失败: %v %s", err, strings.TrimSpace(out))
	}
	return nil
}

func runSchtasks(args ...string) (string, error) {
	cmd := exec.Command("schtasks", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	b, err := cmd.CombinedOutput()
	return string(b), err
}
