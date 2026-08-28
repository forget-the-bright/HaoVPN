package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic 以「临时文件 → Sync → Rename」方式原子写入目标文件。
//
// 为何原子写：进程崩溃或并发读时，调用方不会读到半截内容；配置/凭据/密钥等敏感文件尤其需要。
// 参数：
//   path — 最终目标路径（非空）；
//   data — 完整文件内容；
//   perm — 目标文件权限（如 0o600）；临时文件先按 perm 创建。
// 返回：EnsureParentDir、创建/写入/Sync/Rename 失败时的 error；成功为 nil。
// 副作用：若 path 的父目录不存在则创建（0o755）；失败时尽量删除临时文件。
// 并发：对同一 path 的并发调用不保证顺序，调用方应串行化敏感写。
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if path == "" {
		return fmt.Errorf("fileutil.WriteFileAtomic: 路径为空")
	}
	if err := EnsureParentDir(path, 0o755); err != nil {
		return fmt.Errorf("创建父目录: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	tmpName := tmp.Name()
	// 任一步失败都尽量清掉临时文件，避免目录堆满 .tmp-*。
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("设置临时文件权限: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("Sync 临时文件: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("原子替换 %s: %w", path, err)
	}
	cleanup = false
	return nil
}
