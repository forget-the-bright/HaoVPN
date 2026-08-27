package persist

import (
	"database/sql"
	"encoding/json"
)

// marshalStringSlice 将字符串切片序列化为 JSON 文本列（如 allowed_ips）。
//
// 参数：items — 可为 nil；序列化失败时返回 "[]"。
// 返回：JSON 字符串；空切片为 "[]"。
func marshalStringSlice(items []string) string {
	if items == nil {
		items = []string{}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// unmarshalAllowedIPs 从 SQLite TEXT/NullString 解析 allowed_ips JSON 列。
//
// 参数：col — 数据库列；dest — 写入目标切片指针。
// 副作用：*dest 被替换为解析结果；空或无效 JSON 时 *dest 为 nil 或空切片。
func unmarshalAllowedIPs(col sql.NullString, dest *[]string) {
	if !col.Valid || col.String == "" {
		return
	}
	_ = json.Unmarshal([]byte(col.String), dest)
}
