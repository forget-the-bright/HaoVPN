//go:build windows

package platform

import (
	"context"
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW：GUI（-H windowsgui）进程 spawn 子进程时禁止弹出控制台黑窗。
const createNoWindow = 0x08000000

func hideWindowAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// Command 创建无控制台窗口的子进程（route / netsh / powershell 等）。
func Command(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = hideWindowAttr()
	return cmd
}

// CommandContext 同 Command，且 ctx 取消时 Kill 子进程（Stop 打断 ICS PowerShell）。
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.SysProcAttr = hideWindowAttr()
	return cmd
}
