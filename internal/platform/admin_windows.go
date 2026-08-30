//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
)

// IsAdmin 当前进程是否属于本地 Administrators 组（Windows）。
//
// 用途：GUI/CLI 决定是否弹出 UAC、能否装服务/改路由。
// 与 Web 角色 persist.User.IsAdmin（管理控制台权限）无关，勿混用。
// 失败（SID 分配等）视为非管理员，保守处理。
func IsAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	return err == nil && member
}
