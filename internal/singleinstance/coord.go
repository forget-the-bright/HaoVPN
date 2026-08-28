package singleinstance

import (
	"fmt"
	"hash/fnv"

	"haovpn/internal/brand"
)

// coordAddr 返回本机单实例协调地址（仅 127.0.0.1）。
//
// 使用稳定哈希端口，CLI/GUI/服务共用；localhost TCP 在 Windows 上不受 UAC 提权影响，
// 非管理员进程可探测管理员实例是否在监听。
func coordAddr() string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(brand.GUIAppID + ":client-singleinstance"))
	port := 49152 + int(h.Sum32()%10000)
	return fmt.Sprintf("127.0.0.1:%d", port)
}
