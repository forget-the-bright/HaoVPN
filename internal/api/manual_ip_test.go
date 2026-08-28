package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestCreateAccountManualVPNIP fixed 模式可指定 IP。
func TestCreateAccountManualVPNIP(t *testing.T) {
	ts, client, cookies, csrf, store := newTestAPI(t)
	defer ts.Close()
	defer store.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users",
		strings.NewReader("username=eng_manual&password=SecurePass123!&ip_mode=fixed&vpn_ip=10.88.0.50"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", csrf)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	var out struct {
		VPNIP string `json:"vpn_ip"`
		ID    int64  `json:"id"`
	}
	_ = json.Unmarshal(body, &out)
	if out.VPNIP != "10.88.0.50" {
		t.Fatalf("vpn_ip=%s", out.VPNIP)
	}
	u, _ := store.GetUserByID(out.ID)
	if u.VPNIP != "10.88.0.50" {
		t.Fatalf("db vpn_ip=%s", u.VPNIP)
	}
}

// TestCreateAccountManualIPConflict 冲突 IP 应 400。
func TestCreateAccountManualIPConflict(t *testing.T) {
	ts, client, cookies, csrf, store := newTestAPI(t)
	defer ts.Close()
	defer store.Close()

	mk := func(name, ip string) int {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users",
			strings.NewReader("username="+name+"&password=SecurePass123!&ip_mode=fixed&vpn_ip="+ip))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", csrf)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp, _ := client.Do(req)
		code := resp.StatusCode
		resp.Body.Close()
		return code
	}
	if mk("a1", "10.88.0.60") != 200 {
		t.Fatal("first allocate")
	}
	if mk("a2", "10.88.0.60") != 400 {
		t.Fatal("conflict should 400")
	}
	if mk("a3", "10.88.0.1") != 400 {
		t.Fatal("gateway should 400")
	}
}

// TestDynamicModeRejectsManualIP 动态模式禁止指定 VPN IP。
func TestDynamicModeRejectsManualIP(t *testing.T) {
	ts, client, cookies, csrf, store := newTestAPI(t)
	defer ts.Close()
	defer store.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users",
		strings.NewReader("username=dyn1&password=SecurePass123!&ip_mode=dynamic_session&vpn_ip=10.88.0.70"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", csrf)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, _ := client.Do(req)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", resp.StatusCode, body)
	}
}

// TestPatchChangeFixedVPNIP 改固定 IP 后踢线字段更新。
func TestPatchChangeFixedVPNIP(t *testing.T) {
	ts, client, cookies, csrf, store := newTestAPI(t)
	defer ts.Close()
	defer store.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/users",
		strings.NewReader("username=patchip&password=SecurePass123!&ip_mode=fixed&vpn_ip=10.88.0.80"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", csrf)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, _ := client.Do(req)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var created struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(body, &created)

	patch, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/users/"+itoa(created.ID)+"/vpn",
		strings.NewReader(`{"vpn_ip":"10.88.0.81","ip_mode":"fixed","allowed_ips":["192.168.31.0/24"]}`))
	patch.Header.Set("Content-Type", "application/json")
	patch.Header.Set("X-CSRF-Token", csrf)
	for _, c := range cookies {
		patch.AddCookie(c)
	}
	pr, _ := client.Do(patch)
	pb, _ := io.ReadAll(pr.Body)
	pr.Body.Close()
	if pr.StatusCode != 200 {
		t.Fatalf("%d %s", pr.StatusCode, pb)
	}
	u, _ := store.GetUserByID(created.ID)
	if u.VPNIP != "10.88.0.81" {
		t.Fatalf("vpn_ip=%s", u.VPNIP)
	}
}
