// Package web 通过 go:embed 将 WebUI 静态资源打入服务端二进制。
package web

import "embed"

// Templates 嵌入 HTML 模板（极简 WebUI，无外部 CDN）。
//
//go:embed templates/*.html static/*
var FS embed.FS
