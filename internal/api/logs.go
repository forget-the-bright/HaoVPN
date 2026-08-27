package api

import (
	"fmt"
	"os"

	"haovpn/internal/paginate"
)

// readLogTail 从文件末尾读取最多 tail 行（大文件只读尾部块，避免全文扫描）。
//
// 参数：path — live.log 或滚动日志路径；tail — 最大行数（内部 clamp 2000）。
// 返回：lines 按时间顺序；truncated 表示文件过大仅读了尾部；不存在时空切片。
func readLogTail(path string, tail int) (lines []string, truncated bool, err error) {
	if tail <= 0 {
		tail = 200
	}
	tail = paginate.ClampLimit(tail, 200, 2000)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("open log: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	size := stat.Size()
	if size == 0 {
		return nil, false, nil
	}

	// 估算尾部字节：平均每行 256B，至少 64KB
	readSize := int64(tail * 256)
	if readSize < 65536 {
		readSize = 65536
	}
	if readSize > size {
		readSize = size
	} else {
		truncated = true
	}
	start := size - readSize
	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, start); err != nil {
		return nil, false, err
	}
	text := string(buf)
	if start > 0 {
		if idx := findFirstNewline(text); idx >= 0 && idx+1 < len(text) {
			text = text[idx+1:]
		}
	}
	parts := splitKeepEmpty(text)
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) > tail {
		truncated = true
		parts = parts[len(parts)-tail:]
	}
	return parts, truncated, nil
}

// parseLogTailQuery 解析 ?tail= 查询参数并限制在 [200, 2000]。
func parseLogTailQuery(q string) int {
	return paginate.ClampLimit(parseIntDefault(q, 200), 200, 2000)
}

// findFirstNewline 返回字符串中首个 \n 的下标；无换行时返回 -1。
//
// 用于从大文件尾部块截断不完整的首行。
func findFirstNewline(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return i
		}
	}
	return -1
}

// splitKeepEmpty 按 \n 分割字符串并保留空行（日志尾块解析用）。
func splitKeepEmpty(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s) {
		out = append(out, s[start:])
	}
	return out
}
