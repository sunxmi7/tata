package tata

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPushAndInbox(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://tata:tata@127.0.0.1:54329/tata?sslmode=disable"
	}
	store, err := Open(t.Context(), dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Skipf("no database: %v", err)
	}
	app := &App{Store: store, JWTSecret: []byte("test-secret")} // Push nil: 个推禁用，仅落站内信
	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	// 隔离：站内信/已读/设备是全局表，清空以让本用例断言绝对数量。
	for _, tbl := range []string{"app_message_reads", "app_messages", "push_devices"} {
		if _, err := store.Exec(t.Context(), "DELETE FROM "+tbl); err != nil {
			t.Fatal(err)
		}
	}

	token, uid := registerUser(t, ts, store, "push")

	// 设备注册 + 幂等 upsert
	must(t, ts, token, http.MethodPost, "/api/app/v1/push/register", map[string]any{"cid": "cid-1", "platform": "ios"})
	must(t, ts, token, http.MethodPost, "/api/app/v1/push/register", map[string]any{"cid": "cid-1", "platform": "android"})
	var devices int
	var platform string
	if err := store.QueryRow(t.Context(), `SELECT count(*), max(platform) FROM push_devices WHERE user_id=$1`, uid).Scan(&devices, &platform); err != nil {
		t.Fatal(err)
	}
	if devices != 1 || platform != "android" {
		t.Fatalf("upsert device: count=%d platform=%s", devices, platform)
	}
	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/push/register", map[string]any{"cid": ""}); msg != "缺少推送 CID" {
		t.Fatalf("empty cid: %s", msg)
	}

	// 管理端群发 → 站内信
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	bc := create(t, ts, admin.access, "/api/admin/v1/push/broadcast", map[string]any{
		"title": "双十一优惠", "body": "塔塔币 5 折", "target": "tata://campaign/1", "audience": "all",
	})
	if bc["push_enabled"] != false {
		t.Fatalf("push should be disabled without getui creds: %#v", bc)
	}
	msgID := bc["id"].(float64)

	inbox := listOne(t, ts, token, "/api/app/v1/messages")
	if inbox["unread_count"].(float64) != 1 {
		t.Fatalf("unread after broadcast: %#v", inbox["unread_count"])
	}
	items := inbox["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("inbox items: %d", len(items))
	}
	first := items[0].(map[string]any)
	if first["title"] != "双十一优惠" || first["read"] != false {
		t.Fatalf("inbox first: %#v", first)
	}

	// 标记已读
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/messages/%.0f/read", msgID), nil)
	inbox = listOne(t, ts, token, "/api/app/v1/messages")
	if inbox["unread_count"].(float64) != 0 {
		t.Fatalf("unread after read: %#v", inbox["unread_count"])
	}
	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/messages/999999/read", nil); msg != "消息不存在" {
		t.Fatalf("read missing: %s", msg)
	}

	// 定向给另一个用户，不应出现在当前用户收件箱
	otherToken, otherUID := registerUser(t, ts, store, "push-other")
	_ = otherToken
	must(t, ts, admin.access, http.MethodPost, "/api/admin/v1/push/broadcast", map[string]any{
		"title": "专属提醒", "audience": "user", "user_id": otherUID,
	})
	inbox = listOne(t, ts, token, "/api/app/v1/messages")
	if inbox["unread_count"].(float64) != 0 {
		t.Fatalf("targeted message leaked to other user: %#v", inbox)
	}

	// 创建 banner 时自动推一条站内信
	nowRFC := "2026-01-01T00:00:00Z"
	endRFC := "2030-01-01T00:00:00Z"
	must(t, ts, admin.access, http.MethodPost, "/api/admin/v1/banners", map[string]any{
		"copy": "新年活动", "start_at": nowRFC, "end_at": endRFC, "priority": 1, "target": "tata://new-year",
		"push": true, "push_title": "新年活动上线",
	})
	inbox = listOne(t, ts, token, "/api/app/v1/messages")
	if inbox["unread_count"].(float64) != 1 {
		t.Fatalf("banner auto-push not delivered: %#v", inbox["unread_count"])
	}
}
