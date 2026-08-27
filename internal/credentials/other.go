//go:build !windows

package credentials

import "fmt"

// CredPath 非 Windows 返回空路径占位（仅测试）。
func CredPath() string {
	return ""
}

// SaveService 非 Windows 平台不支持。
func SaveService(username, password string) error {
	return fmt.Errorf("服务凭据存储仅支持 Windows")
}

// LoadService 非 Windows 无凭据。
func LoadService() (username, password string, err error) {
	return "", "", nil
}

// DeleteService 非 Windows 无操作。
func DeleteService() error {
	return nil
}
