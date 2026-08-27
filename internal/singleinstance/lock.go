package singleinstance

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"haovpn/internal/brand"
	"haovpn/internal/fileutil"
)

// Lock 持有进程级互斥锁；进程退出或 Release 后释放。
type Lock struct {
	path string
	file *os.File
}

// AcquireClient 尝试获取客户端单实例锁。已被占用时返回 ErrAlreadyRunning。
func AcquireClient() (*Lock, error) {
	path, err := lockPath()
	if err != nil {
		return nil, err
	}
	if err := fileutil.EnsureParentDir(path, 0o755); err != nil {
		return nil, fmt.Errorf("创建锁目录: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件: %w", err)
	}
	if err := tryLock(f); err != nil {
		_ = f.Close()
		if isWouldBlock(err) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	if err := writePID(f); err != nil {
		unlock(f)
		_ = f.Close()
		return nil, err
	}
	return &Lock{path: path, file: f}, nil
}

// Release 释放单实例锁。
func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	unlock(l.file)
	_ = l.file.Close()
	l.file = nil
}

// ErrAlreadyRunning 表示已有客户端实例在运行。
var ErrAlreadyRunning = fmt.Errorf("HaoVPN 客户端已在运行")

// AlreadyRunningMessage 返回面向用户的提示文案。
func AlreadyRunningMessage() string {
	if pid, ok := readOtherPID(); ok {
		return fmt.Sprintf("HaoVPN 客户端已在运行（PID %d）。请先退出已有实例再启动。", pid)
	}
	return "HaoVPN 客户端已在运行。请先退出已有实例再启动。"
}

func lockPath() (string, error) {
	// Windows 服务与 GUI 均可能以 SYSTEM/管理员运行，ProgramData 各身份可见。
	if dir := os.Getenv("PROGRAMDATA"); dir != "" {
		return filepath.Join(dir, brand.CredDirName, "client.lock"), nil
	}
	// Linux/macOS：优先 XDG 运行时目录，避免 /tmp 被其他用户干扰。
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "haovpn-client.lock"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "haovpn-client.lock"), nil
	}
	return filepath.Join(cache, brand.CredDirName, "client.lock"), nil
}

func writePID(f *os.File) error {
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("清空锁文件: %w", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("定位锁文件: %w", err)
	}
	_, err := fmt.Fprintf(f, "%d\n", os.Getpid())
	return err
}

func readOtherPID() (int, bool) {
	path, err := lockPath()
	if err != nil {
		return 0, false
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return 0, false
	}
	line := string(b)
	for i, c := range line {
		if c == '\n' || c == '\r' {
			line = line[:i]
			break
		}
	}
	pid, err := strconv.Atoi(line)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
