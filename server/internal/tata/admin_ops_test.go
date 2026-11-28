package tata

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestAdminUsersPageAndSearch(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	email := fmt.Sprintf("ops-user-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	_ = token

	page := listOne(t, ts, admin.access, "/api/admin/v1/users?limit=10&offset=0")
	if page["total"] == nil || page["items"] == nil {
		t.Fatalf("page shape: %#v", page)
	}
	items := page["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected users")
	}
	found := list(t, ts, admin.access, "/api/admin/v1/users?q="+email)
	if len(found) != 1 {
		t.Fatalf("search by email: %#v", found)
	}
}

func TestAdminBannerEnabledFilter(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	now := time.Now()
	created := create(t, ts, admin.access, "/api/admin/v1/banners", map[string]any{
		"copy": "关闭弹窗", "start_at": now.Add(-time.Hour).Format(time.RFC3339),
		"end_at": now.Add(time.Hour).Format(time.RFC3339), "priority": 9, "target": "",
		"enabled": false, "frequency": "daily_once",
	})
	id := int64(created["id"].(float64))
	appList := list(t, ts, "", "/api/app/v1/banners")
	for _, b := range appList {
		if int64(b["id"].(float64)) == id {
			t.Fatalf("disabled banner visible: %#v", appList)
		}
	}
	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/banners/%d", id), map[string]any{
		"copy": "关闭弹窗", "start_at": now.Add(-time.Hour).Format(time.RFC3339),
		"end_at": now.Add(time.Hour).Format(time.RFC3339), "priority": 9, "target": "",
		"enabled": true, "frequency": "daily_once",
	})
	appList = list(t, ts, "", "/api/app/v1/banners")
	ok := false
	for _, b := range appList {
		if int64(b["id"].(float64)) == id {
			ok = true
			if b["frequency"] != "daily_once" {
				t.Fatalf("frequency: %#v", b)
			}
		}
	}
	if !ok {
		t.Fatalf("enabled banner missing: %#v", appList)
	}
}

func TestAdminCampaignTimeWindow(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	id := fmt.Sprintf("ops_camp_%d", time.Now().UnixNano())
	pastStart := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	pastEnd := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
	must(t, ts, admin.access, http.MethodPost, "/api/admin/v1/copy/campaigns", map[string]any{
		"campaign_id": id, "title": "过期活动", "detail_title": "过期活动", "detail_body": "正文",
		"status": "ongoing", "enabled": true, "start_at": pastStart, "end_at": pastEnd,
	})
	listBody := listOne(t, ts, "", "/api/app/v1/campaigns")
	for _, raw := range listBody["items"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == id {
			t.Fatalf("expired campaign listed: %#v", item)
		}
	}
	if msg, _ := call(t, ts, "", http.MethodGet, "/api/app/v1/campaigns/"+id, nil); msg != "活动不存在" {
		t.Fatalf("detail outside window: %s", msg)
	}
}

func TestAdminUserStatsDailyAndRegion(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	_ = register(t, ts, fmt.Sprintf("ops-stats-%d@tata.local", time.Now().UnixNano()))
	stats := listOne(t, ts, admin.access, "/api/admin/v1/users/stats?days=7")
	daily, _ := stats["daily_new"].([]any)
	if len(daily) != 7 {
		t.Fatalf("daily_new len=%d %#v", len(daily), stats["daily_new"])
	}
	if stats["region"] == nil {
		t.Fatalf("missing region: %#v", stats)
	}
}

func TestPrivateIPRegionUnknown(t *testing.T) {
	if !isPrivateOrLocalIP("127.0.0.1") || !isPrivateOrLocalIP("192.168.1.1") || !isPrivateOrLocalIP("10.0.0.8") {
		t.Fatal("private detect")
	}
	if isPrivateOrLocalIP("8.8.8.8") {
		t.Fatal("public IP treated private")
	}
}

func TestAdminIAPRevenueShape(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	rev := listOne(t, ts, admin.access, "/api/admin/v1/iap/revenue?grain=day")
	for _, key := range []string{"revenue_minor", "paying_users", "new_users", "pay_rate", "by_sku", "series", "notes"} {
		if rev[key] == nil {
			t.Fatalf("missing %s: %#v", key, rev)
		}
	}
}
