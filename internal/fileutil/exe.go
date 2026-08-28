package fileutil

import (
	"os"
	"path/filepath"
)

// ExecutableDir 返回当前进程可执行文件所在目录。
//
// 用途：解析「exe 旁」的 client.yaml、wintun.dll、凭据目录等，避免各包重复 os.Executable + Dir。
// 返回：绝对或平台给出的目录路径；os.Executable 失败时返回 error（调用方应回退到相对路径策略）。
func ExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}
