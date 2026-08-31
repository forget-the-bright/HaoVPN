package fileutil

import (
	"os"
	"path/filepath"
)

// EnsureParentDir 确保 path 的父目录存在。
//
// 参数：path — 目标文件路径；perm — MkdirAll 权限（如 0o755、0o700）。
// 返回：path 无父目录（如 "foo.txt"）时 nil；MkdirAll 失败时 error。
func EnsureParentDir(path string, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, perm)
}

// CheckDirWritable 检测目录存在且当前进程可写（不创建目录）。
//
// 用途：health 启动自检；父目录应由 boot_persist EnsureParentDir 预先创建。
func CheckDirWritable(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.ErrInvalid
	}
	f, err := os.CreateTemp(dir, ".haovpn-write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}
