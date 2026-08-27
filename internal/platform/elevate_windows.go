//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	shell32           = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

const (
	swNormal     = 1
	errorFileNotFound = 2
)

// RelaunchElevated 以管理员重新启动当前进程（UAC）；成功启动后本进程应退出。
// 返回 true 表示已拉起提权进程；false 表示用户取消或失败（err 说明原因）。
func RelaunchElevated() (launched bool, err error) {
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return false, err
	}
	// 保留除本程序名外的参数（含 -c）
	args := strings.Join(os.Args[1:], " ")
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	param, _ := syscall.UTF16PtrFromString(args)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	r, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(param)),
		uintptr(unsafe.Pointer(dir)),
		uintptr(swNormal),
	)
	// ShellExecute 返回值 >32 表示成功；≤32 为错误码。
	// 用户点「否」常见为 SE_ERR_ACCESSDENIED(5)。
	if r <= 32 {
		if r == errorFileNotFound {
			return false, fmt.Errorf("找不到程序: %s", exe)
		}
		if r == 5 { // SE_ERR_ACCESSDENIED：常为 UAC 取消
			return false, fmt.Errorf("用户取消了管理员提权")
		}
		return false, fmt.Errorf("提权启动失败 code=%d", r)
	}
	return true, nil
}
