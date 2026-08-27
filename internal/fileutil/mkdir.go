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
