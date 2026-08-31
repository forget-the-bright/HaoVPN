package api_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"haovpn/internal/api"
	"haovpn/internal/audit"
	"haovpn/internal/auth"
	"haovpn/internal/ippool"
	"haovpn/internal/persist"
	"haovpn/internal/sessionmgr"
	"haovpn/web"
)

// 模板文件名 → 页脚本（相对 static/）；共用逻辑在 app.js。
var webUIPageScripts = map[string]string{
	"login.html":             "login.js",
	"index.html":             "index.js",
	"user_list.html":         "user_list.js",
	"peer_routes.html":       "peer_routes.js",
	"security_probe.html":    "security_probe.js",
	"tools.html":             "tools.js",
	"audit_log.html":         "audit_log.js",
	"connection_detail.html": "connection_detail.js",
}

// 登录后烟测：路径 → 期望的页脚本 URL 片段。
var webUIAuthPages = []struct {
	path   string
	script string
}{
	{"/", "/static/index.js"},
	{"/users", "/static/user_list.js"},
	{"/peers", "/static/peer_routes.js"},
	{"/security", "/static/security_probe.js"},
	{"/tools", "/static/tools.js"},
	{"/audit", "/static/audit_log.js"},
	{"/connections", "/static/connection_detail.js"},
}

// HTML 内联事件属性（CSP script-src 'self' 会拦截）。
var htmlInlineHandlerRE = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)

// type="button" … id="foo"（属性顺序可乱，用两次匹配简化）。
var htmlButtonIDRE = regexp.MustCompile(`(?is)<button\b[^>]*\bid\s*=\s*["']([^"']+)["'][^>]*>`)
var htmlButtonTypeButtonRE = regexp.MustCompile(`(?i)\btype\s*=\s*["']button["']`)

// TestEmbeddedTemplatesHaveFavicon 各管理页 head 须引用 /static/ favicon，避免页签默认地球图标。
func TestEmbeddedTemplatesHaveFavicon(t *testing.T) {
	entries, err := fs.ReadDir(web.FS, "templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		b, err := web.FS.ReadFile("templates/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		html := string(b)
		if !strings.Contains(html, `rel="icon"`) || !strings.Contains(html, "/static/favicon") {
			t.Errorf("templates/%s 缺少 favicon link（rel=\"icon\" + /static/favicon）", e.Name())
		}
	}
}

// TestLoginPageLoadsExternalLoginScript 登录页引用 static/login.js；CSP script-src 不得含 unsafe-inline。
func TestLoginPageLoadsExternalLoginScript(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "csp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme12")
	cfg := testServerCfg()
	pool, _ := ippool.New(cfg.VPN.Subnet)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "pk")

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "loginForm") || !strings.Contains(body, "/static/login.js") {
		t.Fatal("登录页应含表单并引用 /static/login.js")
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src") {
		t.Fatalf("CSP 须含 script-src: %q", csp)
	}
	// 只检查 script-src 段：不得含 unsafe-inline（style-src 仍可保留）
	scriptPart := csp
	if idx := strings.Index(csp, "script-src"); idx >= 0 {
		scriptPart = csp[idx:]
		if end := strings.Index(scriptPart, ";"); end >= 0 {
			scriptPart = scriptPart[:end]
		}
	}
	if strings.Contains(scriptPart, "'unsafe-inline'") {
		t.Fatalf("CSP script-src 不得含 unsafe-inline: %q", csp)
	}
}

// TestIndexPageLoadsExternalIndexScript 仪表盘引用 /static/index.js，且无内联 <script>。
func TestIndexPageLoadsExternalIndexScript(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "csp-index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme12")
	cfg := testServerCfg()
	pool, _ := ippool.New(cfg.VPN.Subnet)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "pk")

	cookies := loginTestAdmin(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "/static/index.js") {
		t.Fatal("仪表盘应引用 /static/index.js")
	}
	if strings.Contains(body, "<script>") {
		t.Fatal("仪表盘不应再含内联 <script> 块")
	}
}

// TestEmbeddedTemplatesNoInlineEventHandlers CSP 拦截 HTML 内联事件（onclick/onchange/…）。
func TestEmbeddedTemplatesNoInlineEventHandlers(t *testing.T) {
	entries, err := fs.ReadDir(web.FS, "templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		b, err := web.FS.ReadFile("templates/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if htmlInlineHandlerRE.Match(b) {
			t.Errorf("templates/%s 含内联 on*= 事件属性，CSP script-src 'self' 会拦截", e.Name())
		}
	}
}

// TestEmbeddedStaticJSNoOnclickHTMLLiteral 禁止在 static JS 中用 innerHTML 拼 onclick=（行/块注释除外）。
// data-action 委托绑定（如 unban-ip、ban-event-ip）允许。
func TestEmbeddedStaticJSNoOnclickHTMLLiteral(t *testing.T) {
	entries, err := fs.ReadDir(web.FS, "static")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		b, err := web.FS.ReadFile("static/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		code := stripJSComments(string(b))
		if strings.Contains(code, "onclick=") {
			t.Errorf("static/%s 非注释代码含 onclick= 字面量（勿在 HTML 字符串里写内联事件）", e.Name())
		}
	}
}

// TestWebUIButtonIDsBoundInPageScript type=button 且有 id 的控件须在页脚本或 app.js 中出现该 id。
func TestWebUIButtonIDsBoundInPageScript(t *testing.T) {
	appJS, err := web.FS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	appSrc := string(appJS)

	for tmpl, pageJS := range webUIPageScripts {
		html, err := web.FS.ReadFile("templates/" + tmpl)
		if err != nil {
			t.Fatalf("%s: %v", tmpl, err)
		}
		pageSrc := ""
		if pageJS != "" {
			b, err := web.FS.ReadFile("static/" + pageJS)
			if err != nil {
				t.Fatalf("%s → %s: %v", tmpl, pageJS, err)
			}
			pageSrc = string(b)
		}
		combined := pageSrc + "\n" + appSrc

		// 带 id 的 button；再确认含 type=button（排除 submit）
		for _, m := range htmlButtonIDRE.FindAllSubmatch(html, -1) {
			tag := string(m[0])
			id := string(m[1])
			if !htmlButtonTypeButtonRE.MatchString(tag) {
				continue
			}
			if !strings.Contains(combined, id) {
				t.Errorf("%s: button id=%q 未在 %s 或 app.js 中出现（易成哑按钮）", tmpl, id, pageJS)
			}
		}

		if tmpl == "login.html" {
			continue
		}
		if !strings.Contains(string(html), `data-action="logout"`) {
			t.Errorf("%s: 管理页须用 data-action=\"logout\"（禁止 onclick=HaoVPN.logout）", tmpl)
		}
	}
	if !strings.Contains(appSrc, `data-action="logout"`) {
		t.Fatal("app.js 须绑定 data-action=\"logout\"")
	}
}

// TestWebUIAuthPagesExternalScripts 登录后各管理页引用外置脚本且无内联 onclick=。
func TestWebUIAuthPagesExternalScripts(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "csp-pages.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme12")
	cfg := testServerCfg()
	pool, _ := ippool.New(cfg.VPN.Subnet)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "pk")

	cookies := loginTestAdmin(t, srv)
	for _, p := range webUIAuthPages {
		req := httptest.NewRequest(http.MethodGet, p.path, nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status=%d body=%s", p.path, w.Code, w.Body.String())
			continue
		}
		body := w.Body.String()
		if !strings.Contains(body, p.script) {
			t.Errorf("%s: 应引用 %s", p.path, p.script)
		}
		if !strings.Contains(body, "/static/app.js") {
			t.Errorf("%s: 应引用 /static/app.js", p.path)
		}
		if htmlInlineHandlerRE.MatchString(body) {
			t.Errorf("%s: 响应含内联 on*= 事件属性", p.path)
		}
	}
}

// TestSecurityPageExternalScript 探针页引用 security_probe.js，且含 btnBanIP。
func TestSecurityPageExternalScript(t *testing.T) {
	dir := t.TempDir()
	store, err := persist.Open(filepath.Join(dir, "csp-sec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authSvc := auth.New(store, 5, 60, 3600)
	_ = ensureTestAdmin(store, authSvc, "admin", "changeme12")
	cfg := testServerCfg()
	pool, _ := ippool.New(cfg.VPN.Subnet)
	srv := api.NewServer(cfg, store, authSvc, audit.New(store), sessionmgr.New(store), testVPNService(store, pool, cfg), nil, time.Now(), "pk")

	cookies := loginTestAdmin(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/security", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "/static/security_probe.js") {
		t.Fatal("探针页应引用 /static/security_probe.js")
	}
	if htmlInlineHandlerRE.MatchString(body) {
		t.Fatal("探针页不得含内联 on*= 事件")
	}
	if !strings.Contains(body, `id="btnBanIP"`) {
		t.Fatal("探针页应有 btnBanIP，由 JS 绑定封禁")
	}
	if !strings.Contains(body, `id="banDurationPreset"`) {
		t.Fatal("探针页应有 banDurationPreset 封禁时长选择")
	}
}

// loginTestAdmin 以测试管理员登录，返回 Session Cookie。
func loginTestAdmin(t *testing.T, srv *api.Server) []*http.Cookie {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/login", strings.NewReader("username=admin&password=changeme12"))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(loginW, loginReq)
	if loginW.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginW.Code, loginW.Body.String())
	}
	return loginW.Result().Cookies()
}

// stripJSComments 去掉行注释与块注释（不解析正则字面量；足以扫 HTML 的 onclick= 字面量）。
// 注意：勿用简易状态机把 /"/g 的引号当成字符串起点，否则后续 // 注释会漏删。
func stripJSComments(src string) string {
	// 块注释
	src = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "\n")
	var out []string
	for _, line := range strings.Split(src, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "//") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
