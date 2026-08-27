// Package fileutil 提供与业务无关的文件系统小工具。
//
// 上游：serverapp、logger、security、singleinstance 等需在写文件前创建父目录。
// 下游：仅标准库 os/path/filepath。
// 不变量：EnsureParentDir 对空路径或已存在目录返回 nil。
package fileutil
