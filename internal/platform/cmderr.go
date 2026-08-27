package platform

import (
	"fmt"
	"strings"
)

// CommandOutputError 包装子进程失败，附加 trim 后的 combined output 便于排障。
//
// 参数：
//   op — 操作描述（如 "netsh interface ipv4"、"reg IPEnableRouter"）。
//   out — CombinedOutput 原始字节。
//   err — exec 返回的错误；nil 时不应调用本函数。
//
// 返回：含 op 与输出摘要的 error；out 为空时仅包装 err。
func CommandOutputError(op string, out []byte, err error) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return fmt.Errorf("%s: %w", op, err)
	}
	return fmt.Errorf("%s: %w: %s", op, err, msg)
}
