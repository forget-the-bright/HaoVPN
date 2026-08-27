// Package version 保存构建时注入的版本元数据（-version 与 API /system/info 共用）。
//
// 版本号唯一来源为根目录 VERSION 文件；本包仅读取 ldflags 或 fallback。
package version
