package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"haovpn/internal/auth"
	"haovpn/internal/persist"
)

// createAPITestVPNUser 在测试库中创建带公钥的 VPN 账号。
func createAPITestVPNUser(t *testing.T, store *persist.Store, name, ip string) int64 {
	t.Helper()
	hash, err := auth.HashPassword("Pass12345!")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateVPNAccount(persist.User{
		Username: name, PasswordHash: hash, PublicKey: "pk-" + name, PrivateKeyEnc: "sk",
		VPNIP: ip, AllowedIPs: []string{"10.88.0.0/24"}, IPMode: persist.IPModeFixed, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// apiJSON 带 Cookie + CSRF 发 JSON 请求并返回状态与 body。
func apiJSON(t *testing.T, client *http.Client, method, url string, cookies []*http.Cookie, csrf string, body any) (int, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// TestPeerRouteMembersNarrowMarksRemovedDirty 成员收窄时被移除账号须进入 dirty（旧∪新）。
func TestPeerRouteMembersNarrowMarksRemovedDirty(t *testing.T) {
	ts, client, cookies, csrf, store := newTestAPI(t)
	defer ts.Close()
	defer store.Close()

	via := createAPITestVPNUser(t, store, "via1", "10.88.0.10")
	u1 := createAPITestVPNUser(t, store, "acc1", "10.88.0.11")
	u2 := createAPITestVPNUser(t, store, "acc2", "10.88.0.12")

	code, raw := apiJSON(t, client, http.MethodPost, ts.URL+"/api/v1/peer-routes", cookies, csrf, map[string]any{
		"dest_cidr": "192.168.50.0/24", "via_user_id": via, "member_user_ids": []int64{u1, u2},
	})
	if code != http.StatusOK {
		t.Fatalf("create route: %d %s", code, raw)
	}
	var created map[string]any
	_ = json.Unmarshal(raw, &created)
	item, _ := created["item"].(map[string]any)
	routeID := int64(item["id"].(float64))

	// 收窄为仅 u1：u2 须仍在 dirty
	code, raw = apiJSON(t, client, http.MethodPut, ts.URL+"/api/v1/peer-routes/"+strconv.FormatInt(routeID, 10)+"/members", cookies, csrf, map[string]any{
		"member_user_ids": []int64{u1},
	})
	if code != http.StatusOK {
		t.Fatalf("replace members: %d %s", code, raw)
	}

	code, raw = apiJSON(t, client, http.MethodGet, ts.URL+"/api/v1/peers/apply", cookies, "", nil)
	if code != http.StatusOK {
		t.Fatalf("apply status: %d %s", code, raw)
	}
	var st struct {
		Pending bool    `json:"pending_apply"`
		All     bool    `json:"all"`
		UserIDs []int64 `json:"user_ids"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	if !st.Pending {
		t.Fatal("收窄后应 pending_apply")
	}
	seen := map[int64]bool{}
	for _, id := range st.UserIDs {
		seen[id] = true
	}
	if !seen[u1] || !seen[u2] {
		t.Fatalf("dirty 须含 u1 与被移除的 u2, got %v", st.UserIDs)
	}
}

// TestPeerRouteAllToSpecificMarksDirtyAll all→指定须 mark dirty all。
func TestPeerRouteAllToSpecificMarksDirtyAll(t *testing.T) {
	ts, client, cookies, csrf, store := newTestAPI(t)
	defer ts.Close()
	defer store.Close()

	via := createAPITestVPNUser(t, store, "via2", "10.88.0.20")
	u1 := createAPITestVPNUser(t, store, "acc3", "10.88.0.21")

	code, raw := apiJSON(t, client, http.MethodPost, ts.URL+"/api/v1/peer-routes", cookies, csrf, map[string]any{
		"dest_cidr": "192.168.51.0/24", "via_user_id": via, "apply_all": true,
	})
	if code != http.StatusOK {
		t.Fatalf("create: %d %s", code, raw)
	}
	var created map[string]any
	_ = json.Unmarshal(raw, &created)
	item, _ := created["item"].(map[string]any)
	routeID := int64(item["id"].(float64))

	// 清空先前 dirty（创建时已 dirtyAll），再测 all→specific
	code, raw = apiJSON(t, client, http.MethodPost, ts.URL+"/api/v1/peers/apply", cookies, csrf, map[string]any{
		"force_all": true,
	})
	if code != http.StatusOK {
		t.Fatalf("apply clear: %d %s", code, raw)
	}

	code, raw = apiJSON(t, client, http.MethodPut, ts.URL+"/api/v1/peer-routes/"+strconv.FormatInt(routeID, 10)+"/members", cookies, csrf, map[string]any{
		"member_user_ids": []int64{u1},
	})
	if code != http.StatusOK {
		t.Fatalf("narrow from all: %d %s", code, raw)
	}
	code, raw = apiJSON(t, client, http.MethodGet, ts.URL+"/api/v1/peers/apply", cookies, "", nil)
	if code != http.StatusOK {
		t.Fatalf("status: %d %s", code, raw)
	}
	var st struct {
		Pending bool `json:"pending_apply"`
		All     bool `json:"all"`
	}
	_ = json.Unmarshal(raw, &st)
	if !st.Pending || !st.All {
		t.Fatalf("all→指定须 dirtyAll pending=%v all=%v", st.Pending, st.All)
	}
}

// TestPeersApplyKeepsFailedDirty Increment 失败时不清脏（用非法 id 模拟：对已删账号）。
// 此处验证成功路径清空 done、且响应含 kicked。
func TestPeersApplyClearsOnlyDone(t *testing.T) {
	ts, client, cookies, csrf, store := newTestAPI(t)
	defer ts.Close()
	defer store.Close()

	via := createAPITestVPNUser(t, store, "via3", "10.88.0.30")
	u1 := createAPITestVPNUser(t, store, "acc4", "10.88.0.31")
	code, raw := apiJSON(t, client, http.MethodPost, ts.URL+"/api/v1/peer-routes", cookies, csrf, map[string]any{
		"dest_cidr": "192.168.52.0/24", "via_user_id": via, "member_user_ids": []int64{u1},
	})
	if code != http.StatusOK {
		t.Fatalf("create: %d %s", code, raw)
	}
	code, raw = apiJSON(t, client, http.MethodPost, ts.URL+"/api/v1/peers/apply", cookies, csrf, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("apply: %d %s", code, raw)
	}
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out["ok"] != true {
		t.Fatalf("want ok, got %v", out)
	}
	if int(out["kicked"].(float64)) < 1 {
		t.Fatalf("want kicked>=1, got %v", out)
	}
	code, raw = apiJSON(t, client, http.MethodGet, ts.URL+"/api/v1/peers/apply", cookies, "", nil)
	var st struct {
		Pending bool `json:"pending_apply"`
	}
	_ = json.Unmarshal(raw, &st)
	if st.Pending {
		t.Fatal("全部成功后不应再 pending")
	}
}

// TestCreatePeerRouteRejectsNonVPNVia via 无公钥须 400。
func TestCreatePeerRouteRejectsNonVPNVia(t *testing.T) {
	ts, client, cookies, csrf, store := newTestAPI(t)
	defer ts.Close()
	defer store.Close()

	// admin 通常无 VPN 公钥
	admin, err := store.GetUserByUsername("admin")
	if err != nil || admin == nil {
		t.Fatal(err)
	}
	code, raw := apiJSON(t, client, http.MethodPost, ts.URL+"/api/v1/peer-routes", cookies, csrf, map[string]any{
		"dest_cidr": "192.168.53.0/24", "via_user_id": admin.ID, "apply_all": true,
	})
	if code != http.StatusBadRequest {
		t.Fatalf("非 VPN via 应 400, got %d %s", code, raw)
	}
}
