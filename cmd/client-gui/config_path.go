package main

import (
	"os"
	"path/filepath"
)

// resolveClientConfigPath 未指定 -c 时：exe 同目录 client.yaml → 当前目录 client.yaml → 默认写到 exe 旁。
func resolveClientConfigPath() string {
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "client.yaml")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	if st, err := os.Stat("client.yaml"); err == nil && !st.IsDir() {
		if abs, err := filepath.Abs("client.yaml"); err == nil {
			return abs
		}
		return "client.yaml"
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "client.yaml")
	}
	return "client.yaml"
}
