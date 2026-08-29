package timeutil

import "time"

// Seconds 将整数秒转为 time.Duration，避免各包重复写 time.Duration(n)*time.Second。
//
// 参数：n — 秒数；可为 0 或负（调用方自行解释语义，本函数不钳制）。
// 返回：对应的 Duration。
// 用途：心跳/重连/封禁窗口/会话 TTL 等「配置秒 → 运行时 Duration」映射。
func Seconds(n int) time.Duration {
	return time.Duration(n) * time.Second
}
