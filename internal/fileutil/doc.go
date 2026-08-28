// Package fileutil 提供与业务无关的文件系统小工具。
//
// 上游：config、logger、security、credentials、tunnel、wintundll、platform 等写文件或解析 exe 目录。
// 下游：仅标准库 os/path/filepath。
// 能力：EnsureParentDir、WriteFileAtomic（敏感配置/密钥原子写）、ExecutableDir。
// 不变量：EnsureParentDir 对空路径或已存在目录返回 nil；WriteFileAtomic 失败不留下半截目标文件。
package fileutil
