package timeutil

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	// 嵌入 IANA 时区数据库，避免 Windows 等环境缺少 zoneinfo 导致 Asia/Shanghai 解析失败。
	_ "time/tzdata"
)

// offsetRE 匹配 GMT+8、UTC-5、+08:00、+8、+0800 等固定偏移写法。
var offsetRE = regexp.MustCompile(`(?i)^(?:GMT|UTC)?([+-])(\d{1,2})(?::?(\d{2}))?$`)

// LoadDisplayLocation 解析 WebUI 展示时区配置。
//
// 支持：
//   - 空 / UTC / Etc/UTC → time.UTC
//   - IANA 名：Asia/Shanghai
//   - 固定偏移：GMT+8、UTC+8、+08:00、+8（东八区为正，与日常习惯一致）
//
// 返回：*time.Location；无法识别时 error（中文说明）。
// 关联：config.APISection.DisplayTimezone；仅用于展示，不影响 SQLite/API 存库时区。
func LoadDisplayLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "UTC") || strings.EqualFold(name, "Etc/UTC") || strings.EqualFold(name, "Z") {
		return time.UTC, nil
	}
	if loc, err := time.LoadLocation(name); err == nil {
		return loc, nil
	}
	m := offsetRE.FindStringSubmatch(name)
	if m == nil {
		return nil, fmt.Errorf("无法解析时区 %q（示例：UTC、Asia/Shanghai、GMT+8、+08:00）", name)
	}
	sign := 1
	if m[1] == "-" {
		sign = -1
	}
	hours, _ := strconv.Atoi(m[2])
	mins := 0
	if m[3] != "" {
		mins, _ = strconv.Atoi(m[3])
	}
	if hours > 14 || mins > 59 || (hours == 14 && mins > 0) {
		return nil, fmt.Errorf("时区偏移超出范围: %q", name)
	}
	sec := sign * (hours*3600 + mins*60)
	label := fmt.Sprintf("UTC%s%02d:%02d", m[1], hours, mins)
	return time.FixedZone(label, sec), nil
}

// FormatInDisplay 将时刻格式化为展示字符串（含数值偏移）。
//
// 参数：t — 任意 Location 的时刻；loc — 展示时区（nil 当 UTC）。
// 返回：如「2026-08-30 18:00:00 +08:00」；零值返回空串。
func FormatInDisplay(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return ""
	}
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format("2006-01-02 15:04:05 -07:00")
}

// OffsetLabel 返回 loc 相对 UTC 的简短标签（如 +08:00 / Z）。
func OffsetLabel(loc *time.Location, at time.Time) string {
	if loc == nil {
		return "Z"
	}
	if at.IsZero() {
		at = time.Now()
	}
	_, off := at.In(loc).Zone()
	if off == 0 {
		return "Z"
	}
	sign := "+"
	if off < 0 {
		sign = "-"
		off = -off
	}
	h := off / 3600
	m := (off % 3600) / 60
	return fmt.Sprintf("%s%02d:%02d", sign, h, m)
}
