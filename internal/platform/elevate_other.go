//go:build !windows

package platform

import "fmt"

// RelaunchElevated 非 Windows：不支持 UAC 式提权。
func RelaunchElevated() (launched bool, err error) {
	return false, fmt.Errorf("当前平台不支持自动提权，请使用 root/sudo 运行")
}
