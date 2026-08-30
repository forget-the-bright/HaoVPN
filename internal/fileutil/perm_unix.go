//go:build !windows

package fileutil

import "os"

// CheckWorldReadable 检测 Unix 上路径是否对「组/其他人」开放权限位。
//
// 返回：worldReadable 为 true 表示 perm&0o077≠0；Stat 失败返回 false。
// 为何不直接打日志：本包不得依赖 logger（logger→fileutil 会循环）；由 health 等调用方 Warn。
func CheckWorldReadable(path string) (worldReadable bool, perm os.FileMode) {
	if path == "" {
		return false, 0
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false, 0
	}
	p := fi.Mode().Perm()
	return p&0o077 != 0, p
}

// RestrictToAdminsOnly Unix 上无 Windows ACL；chmod 600 由调用方 WriteFileAtomic 完成。
func RestrictToAdminsOnly(path string) error {
	_ = path
	return nil
}
