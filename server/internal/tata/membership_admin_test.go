package tata

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdminGrantMembershipCreatesAndExtends(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})
	_, uid := registerUser(t, ts, store, "member")

	msg, _ := call(t, ts, ro.access, http.MethodPost, "/api/admin/v1/memberships", map[string]any{
		"user_id": uid, "note": "只读不能写",
	})
	if !strings.Contains(msg, "只读") {
		t.Fatalf("readonly grant: %s", msg)
	}
	view := listOne(t, ts, ro.access, fmt.Sprintf("/api/admin/v1/memberships?user_id=%d", uid))
	if view["can_write"] != false || view["writes_iap_orders"] != false || view["sku_kind_default"] != membershipKindMonth {
		t.Fatalf("readonly list: %#v", view)
	}

	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/memberships", map[string]any{
		"user_id": uid,
	})
	if !strings.Contains(msg, "原因") {
		t.Fatalf("missing note: %s", msg)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/memberships", map[string]any{
		"user_id": uid, "sku_kind": "week", "note": "错误档位",
	})
	if !strings.Contains(msg, "月卡") {
		t.Fatalf("bad kind: %s", msg)
	}

	granted := create(t, ts, admin.access, "/api/admin/v1/memberships", map[string]any{
		"user_id": uid, "sku_kind": "month", "note": "测试开通月卡",
	})
	if granted["extended"] != false || granted["writes_iap_orders"] != false || granted["duration_days"].(float64) != membershipDaysMonth {
		t.Fatalf("create meta: %#v", granted)
	}
	row := granted["membership"].(map[string]any)
	id := int64(row["id"].(float64))
	if row["sku_kind"] != membershipKindMonth || row["platform"] != membershipPlatform || row["active"] != true || row["status"] != "active" {
		t.Fatalf("created row: %#v", row)
	}
	firstExp := mustParseTime(t, row["expires_at"])
	assertNear(t, firstExp, time.Now().UTC().AddDate(0, 0, membershipDaysMonth), 2*time.Minute)
	assertActiveCount(t, store, uid, 1)
	assertOrderCount(t, store, uid, 0)

	extended := create(t, ts, admin.access, "/api/admin/v1/memberships", map[string]any{
		"user_id": uid, "note": "再延长一个月",
	})
	if extended["extended"] != true {
		t.Fatalf("expected extend: %#v", extended)
	}
	again := extended["membership"].(map[string]any)
	if int64(again["id"].(float64)) != id || again["sku_kind"] != membershipKindMonth {
		t.Fatalf("extend kept a second row: %#v", again)
	}
	secondExp := mustParseTime(t, again["expires_at"])
	assertNear(t, secondExp, firstExp.AddDate(0, 0, membershipDaysMonth), 2*time.Minute)
	assertActiveCount(t, store, uid, 1)

	listed := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/memberships?user_id=%d", uid))
	items := listed["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["active"] != true {
		t.Fatalf("list: %#v", listed)
	}

	upgraded := create(t, ts, admin.access, "/api/admin/v1/memberships", map[string]any{
		"user_id": uid, "sku_kind": "year", "note": "升成年卡",
	})
	yearRow := upgraded["membership"].(map[string]any)
	if int64(yearRow["id"].(float64)) != id || yearRow["sku_kind"] != membershipKindYear || upgraded["duration_days"].(float64) != membershipDaysYear {
		t.Fatalf("year upgrade: %#v", upgraded)
	}
	assertNear(t, mustParseTime(t, yearRow["expires_at"]), secondExp.AddDate(0, 0, membershipDaysYear), 2*time.Minute)
	assertOrderCount(t, store, uid, 0)

	var audits int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE action='admin_membership' AND object_id=$1`, fmtID(id)).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 3 {
		t.Fatalf("audits: %d", audits)
	}

	_, storeUser := registerUser(t, ts, store, "store")
	var storeSub int64
	storeExp := time.Now().UTC().AddDate(0, 0, 10)
	if err := store.QueryRow(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at, platform, merge_state)
		VALUES ($1,'month','active',$2,$3,'apple','') RETURNING id`, storeUser, time.Now().UTC(), storeExp).Scan(&storeSub); err != nil {
		t.Fatal(err)
	}
	kept := create(t, ts, admin.access, "/api/admin/v1/memberships", map[string]any{
		"user_id": storeUser, "sku_kind": "month", "note": "延长商店订阅",
	})
	keptRow := kept["membership"].(map[string]any)
	if int64(keptRow["id"].(float64)) != storeSub || keptRow["platform"] != "apple" || kept["extended"] != true {
		t.Fatalf("store row rewritten: %#v", kept)
	}
	assertActiveCount(t, store, storeUser, 1)
	assertOrderCount(t, store, storeUser, 0)

	if _, err := store.Exec(t.Context(), `UPDATE app_users SET cancelled_at=now() WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/memberships", map[string]any{
		"user_id": uid, "note": "注销后",
	})
	if !strings.Contains(msg, "注销") {
		t.Fatalf("cancelled: %s", msg)
	}
}

func assertActiveCount(t *testing.T, store *Store, uid int64, want int) {
	t.Helper()
	var n int
	err := store.QueryRow(t.Context(), `SELECT count(*) FROM subscriptions
		WHERE user_id=$1 AND status='active' AND expires_at > now()
		  AND merge_state NOT IN ('superseded','expired')`, uid).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("active subscriptions: got %d want %d", n, want)
	}
}

func assertOrderCount(t *testing.T, store *Store, uid int64, want int) {
	t.Helper()
	var n int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM iap_orders WHERE user_id=$1`, uid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("iap orders: got %d want %d", n, want)
	}
}

func mustParseTime(t *testing.T, raw any) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, fmt.Sprint(raw))
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, fmt.Sprint(raw))
	}
	if err != nil {
		t.Fatalf("time %v: %v", raw, err)
	}
	return parsed
}

func assertNear(t *testing.T, got, want time.Time, slack time.Duration) {
	t.Helper()
	delta := got.Sub(want)
	if delta < 0 {
		delta = -delta
	}
	if delta > slack {
		t.Fatalf("time %s, want %s (±%s)", got, want, slack)
	}
}
