package version

import "fmt"

var (
	// Version 构建时由脚本从根目录 VERSION 文件注入；未注入时为 "dev"。
	Version = "dev"
	// Commit 构建时写入的短 git commit hash；未知构建为 "unknown"。
	Commit = "unknown"
	// BuildTime UTC 构建时间戳字符串；未知构建为 "unknown"。
	BuildTime = "unknown"
)

// String 返回面向 CLI/日志的一行版本摘要。
//
// 参数：无；使用包级 Version/Commit/BuildTime。
// 返回：形如「haovpn x.y.z (commit abc, built …)」的字符串。
// 副作用：无。
func String() string {
	return fmt.Sprintf("haovpn %s (commit %s, built %s)", Version, Commit, BuildTime)
}

// Info 返回结构化构建元数据，供 API /health 或 GUI 关于页。
//
// 参数：无。
// 返回：含 version、commit、build_time 键的 map（值为字符串）。
// 副作用：无；每次调用新建 map。
func Info() map[string]string {
	return map[string]string{
		"version":    Version,
		"commit":     Commit,
		"build_time": BuildTime,
	}
}
