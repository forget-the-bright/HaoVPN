package api

import (
	"html/template"
	"io"
	"net/http"
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
	if _, ok := s.sessionFromRequest(r); !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	s.render(w, "index.html", nil)
}

func (s *Server) handleUsersPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "user_list.html", nil)
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

// handleStatic 提供嵌入的静态资源（CSS）。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if len(path) <= len("/static/") {
		http.NotFound(w, r)
		return
	}
	name := "static/" + path[len("/static/"):]
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
