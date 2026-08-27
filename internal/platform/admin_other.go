//go:build !windows

package platform

import "os"

// IsAdmin 非 Windows：uid 0 视为管理员。
func IsAdmin() bool {
	return os.Getuid() == 0
}
