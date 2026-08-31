package api

import (
	"html/template"
	"io"
	"net/http"
	"path"
	"strings"

	"haovpn/web"
)

// loadTemplates 从 embed 加载 WebUI 模板。
func loadTemplates() *template.Template {
	t := template.New("web")
	tmpl := template.Must(t.ParseFS(web.FS, "templates/*.html"))
	return tmpl
}

var pageTemplates = loadTemplates()

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "模板渲染失败", http.StatusInternalServerError)
	}
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login.html", nil)
}

func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.render(w, "index.html", nil)
}

func (s *Server) handleUsersPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "user_list.html", nil)
}

// handlePeersPage 托管路由 / 互访白名单页（对齐 ZeroTier Managed Routes）。
func (s *Server) handlePeersPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "peer_routes.html", nil)
}

func (s *Server) handleConnectionsPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "connection_detail.html", nil)
}

func (s *Server) handleAuditPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "audit_log.html", nil)
}

func (s *Server) handleToolsPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "tools.html", nil)
}

// handleStatic 提供嵌入的静态资源（CSS/JS/图标）。
//
// 防御：对 URL 后缀 path.Clean，拒绝「..」穿越与绝对路径，再打开 embed.FS。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Path
	if len(raw) <= len("/static/") {
		http.NotFound(w, r)
		return
	}
	rel := path.Clean("/" + raw[len("/static/"):])
	// Clean 后仍须落在「相对单段路径」：禁止回到根外或带 ..
	if rel == "/" || strings.Contains(rel, "..") || strings.HasPrefix(rel, "/../") {
		http.NotFound(w, r)
		return
	}
	// 去掉 Clean 产生的前导 /
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || strings.Contains(rel, "..") {
		http.NotFound(w, r)
		return
	}
	name := "static/" + rel
	f, err := web.FS.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	if name == "static/style.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	_, _ = io.Copy(w, f)
}
