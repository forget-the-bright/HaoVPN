//go:build windows

package credentials

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"haovpn/internal/brand"
	"haovpn/internal/fileutil"
	"haovpn/internal/logger"
)

// CRYPTPROTECT_LOCAL_MACHINE：本机任意进程（含 LocalSystem 服务）可解密。
const cryptProtectLocalMachine = 0x4

var (
	crypt32           = syscall.NewLazyDLL("crypt32.dll")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procProtectData   = crypt32.NewProc("CryptProtectData")
	procUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree     = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

// CredPath 返回 DPAPI 凭据文件路径（ProgramData\HaoVPN\credentials）。
func CredPath() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = "C:\\ProgramData"
	}
	return filepath.Join(base, brand.CredDirName, "credentials")
}

func credPath() string { return CredPath() }

// SaveService 使用本机范围 DPAPI 加密保存隧道登录凭据（供 Windows 服务自启）。
func SaveService(username, password string) error {
	if strings.TrimSpace(username) == "" || password == "" {
		return fmt.Errorf("用户名或密码为空")
	}
	plain := username + "\n" + password
	enc, err := protect([]byte(plain))
	if err != nil {
		return fmt.Errorf("加密失败（须以管理员运行以写入本机凭据）: %w", err)
	}
	if err := fileutil.EnsureParentDir(credPath(), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(credPath(), enc, 0o600); err != nil {
		return fmt.Errorf("写入凭据失败（须管理员）: %w", err)
	}
	logger.Info("已保存服务凭据（LocalMachine DPAPI） path=%s user=%s", credPath(), username)
	return nil
}

// LoadService 读取服务凭据；文件不存在返回空；解密失败返回明确错误（须重存）。
func LoadService() (username, password string, err error) {
	path := credPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return "", "", err
	}
	plain, err := unprotect(b)
	if err != nil {
		return "", "", fmt.Errorf("解密服务凭据失败（可能为旧版 CurrentUser 加密，请删除 %s 后以管理员在 GUI 重新「保存供服务使用」）: %w", path, err)
	}
	parts := strings.SplitN(string(plain), "\n", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("凭据格式无效，请删除 %s 后重新保存", path)
	}
	return parts[0], parts[1], nil
}

// DeleteService 删除已保存的服务凭据。
func DeleteService() error {
	if err := os.Remove(credPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func protect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	in := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var out dataBlob
	r, _, err := procProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0,
		uintptr(cryptProtectLocalMachine),
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer localFree(out.pbData)
	buf := make([]byte, out.cbData)
	copy(buf, unsafe.Slice(out.pbData, out.cbData))
	return buf, nil
}

func unprotect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty blob")
	}
	in := dataBlob{cbData: uint32(len(data)), pbData: &data[0]}
	var out dataBlob
	r, _, err := procUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0,
		uintptr(cryptProtectLocalMachine),
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer localFree(out.pbData)
	buf := make([]byte, out.cbData)
	copy(buf, unsafe.Slice(out.pbData, out.cbData))
	return buf, nil
}

func localFree(p *byte) {
	if p != nil {
		procLocalFree.Call(uintptr(unsafe.Pointer(p)))
	}
}
