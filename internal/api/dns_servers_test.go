package api_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

// TestDNSServersAPICRUD 托管 DNS API：创建、排除、备注不 pending、删。
func TestDNSServersAPICRUD(t *testing.T) {
	ts, client, cookies, csrf, store := newTestAPI(t)
	defer ts.Close()
	defer store.Close()

	u1 := createAPITestVPNUser(t, store, "dns1", "10.88.0.21")
	u2 := createAPITestVPNUser(t, store, "dns2", "10.88.0.22")

	code, raw := apiJSON(t, client, http.MethodPost, ts.URL+"/api/v1/dns-servers", cookies, csrf, map[string]any{
		"dns_ip": "10.10.6.51", "remark": "公司", "apply_all": true,
	})
	if code != http.StatusOK {
		t.Fatalf("create: %d %s", code, raw)
	}
	var created map[string]any
	_ = json.Unmarshal(raw, &created)
	if created["pending_apply"] != true {
		t.Fatalf("create should pending: %v", created)
	}
	item := created["item"].(map[string]any)
	id := int64(item["id"].(float64))

	code, raw = apiJSON(t, client, http.MethodPut, ts.URL+"/api/v1/dns-servers/"+strconv.FormatInt(id, 10)+"/excludes", cookies, csrf, map[string]any{
		"exclude_user_ids": []int64{u1},
	})
	if code != http.StatusOK {
		t.Fatalf("excludes: %d %s", code, raw)
	}

	code, raw = apiJSON(t, client, http.MethodPut, ts.URL+"/api/v1/dns-servers/"+strconv.FormatInt(id, 10)+"/remark", cookies, csrf, map[string]any{
		"remark": "新备注",
	})
	if code != http.StatusOK {
		t.Fatalf("remark: %d %s", code, raw)
	}
	var remarkResp map[string]any
	_ = json.Unmarshal(raw, &remarkResp)
	if remarkResp["pending_apply"] == true {
		t.Fatal("备注不得 pending_apply")
	}

	code, raw = apiJSON(t, client, http.MethodPost, ts.URL+"/api/v1/dns-servers", cookies, csrf, map[string]any{
		"dns_ip": "10.20.0.1", "member_user_ids": []int64{u2},
	})
	if code != http.StatusOK {
		t.Fatalf("create specific: %d %s", code, raw)
	}
	_ = json.Unmarshal(raw, &created)
	manID := int64(created["item"].(map[string]any)["id"].(float64))

	code, raw = apiJSON(t, client, http.MethodDelete, ts.URL+"/api/v1/dns-servers/"+strconv.FormatInt(manID, 10), cookies, csrf, nil)
	if code != http.StatusOK {
		t.Fatalf("delete: %d %s", code, raw)
	}
	d, _ := store.GetDNSServer(manID)
	if d != nil {
		t.Fatal("deleted row should be gone")
	}

	// config 禁改 members：先 seed
	if _, _, _, err := store.SyncConfigDNSServers([]string{"192.168.3.1"}); err != nil {
		t.Fatal(err)
	}
	list, _ := store.ListDNSServers()
	var cfgID int64
	for _, row := range list {
		if row.DNSIP == "192.168.3.1" {
			cfgID = row.ID
		}
	}
	code, raw = apiJSON(t, client, http.MethodPut, ts.URL+"/api/v1/dns-servers/"+strconv.FormatInt(cfgID, 10)+"/members", cookies, csrf, map[string]any{
		"member_user_ids": []int64{u1},
	})
	if code == http.StatusOK {
		t.Fatalf("config members should fail: %s", raw)
	}
}
