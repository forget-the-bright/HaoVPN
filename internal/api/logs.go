package api

import (
	"fmt"
	"os"
	"strconv"
)

// readLogTail 从文件末尾读取最多 tail 行（大文件只读尾部块，避免全文扫描）。
func readLogTail(path string, tail int) (lines []string, truncated bool, err error) {
	if tail <= 0 {
		tail = 200
	}
	if tail > 2000 {
		tail = 2000
	}
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

func parseLogTailQuery(q string) int {
	if q == "" {
		return 200
	}
	n, err := strconv.Atoi(q)
	if err != nil || n <= 0 {
		return 200
	}
	return n
}

func findFirstNewline(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return i
		}
	}
	return -1
}

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
