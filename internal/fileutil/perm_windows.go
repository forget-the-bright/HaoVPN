//go:build windows

package fileutil

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RestrictToAdminsOnly 将文件 ACL 收紧为 Administrators + SYSTEM（去掉继承）。
//
// 为何用 icacls：NTFS ACL API 样板冗长；icacls 为系统自带且行为稳定。
// SID：S-1-5-32-544=Administrators，S-1-5-18=SYSTEM。
func RestrictToAdminsOnly(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("路径为空")
	}
	// /inheritance:r 去掉继承；/grant:r 重置指定 SID 权限为完全控制
	cmd := exec.Command("icacls", path,
		"/inheritance:r",
		"/grant:r", "*S-1-5-32-544:(F)",
		"/grant:r", "*S-1-5-18:(F)",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("收紧 ACL 失败: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CheckWorldReadable 检测 Windows 上 Everyone 是否对文件有读权限。
//
// 返回：worldReadable=true 表示 DACL 含 Everyone 可读 ACE；perm 在 Windows 无 Unix 意义（恒 0）。
// 检测失败视为 false（避免误报阻塞启动）；生产仍依赖 RestrictToAdminsOnly 主动收紧。
func CheckWorldReadable(path string) (worldReadable bool, perm os.FileMode) {
	if path == "" {
		return false, 0
	}
	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil || sd == nil {
		return false, 0
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return false, 0
	}
	everyone, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		return false, 0
	}
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil || ace == nil {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !windows.EqualSid(sid, everyone) {
			continue
		}
		mask := ace.Mask
		if mask&(windows.FILE_GENERIC_READ|windows.GENERIC_READ|windows.GENERIC_ALL) != 0 {
			return true, 0
		}
	}
	return false, 0
}
