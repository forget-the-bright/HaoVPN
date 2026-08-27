package api

import (
	"mime"
	"net/http"
	"strings"
)

// maxMultipartMem WebUI FormData / 浏览器 multipart 表单内存上限（与 net/http 默认一致）。
const maxMultipartMem = 10 << 20

// parseRequestForm 解析 POST 表单。Go 1.26 起 ParseForm 不再处理 multipart/form-data，
// 浏览器 fetch(FormData) 与 curl -F 均走 multipart，须显式 ParseMultipartForm。
func parseRequestForm(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return r.ParseForm()
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return err
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		return r.ParseMultipartForm(maxMultipartMem)
	}
	return r.ParseForm()
}
