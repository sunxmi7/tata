package tata

import (
	"fmt"
	"strconv"
	"testing"
)

func TestAdminEntitlementLists(t *testing.T) {
	store, ts := newMergeServer(t)
	_, uid := registerUser(t, ts, store, "entlist")
	seedGift(t, store, uid, 1)
	if _, err := store.Exec(t.Context(), `INSERT INTO quota_lots(user_id, source, total, remaining, expires_at, reason)
		VALUES ($1,'subscription',10,10, now() - interval '1 minute','period'),
		       ($1,'pack',50,40, now() + interval '10 days','pack')`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO yearly_reports(user_id, year, unlocked, unlock_source, unlocked_at, report_status)
		VALUES ($1, 2026, TRUE, 'purchase', now(), 'pending')`, uid); err != nil {
		t.Fatal(err)
	}
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})

	gifts := listOne(t, ts, admin.access, "/api/admin/v1/registration-gifts")
	if !listHasUser(t, gifts, uid) {
		t.Fatalf("gifts: %#v", gifts)
	}
	lots := listOne(t, ts, admin.access, "/api/admin/v1/quota-lots")
	items, _ := lots["items"].([]any)
	var sawSub, sawPack bool
	for _, raw := range items {
		row, _ := raw.(map[string]any)
		if row["user_id"] != float64(uid) {
			continue
		}
		if row["source"] == "subscription" && row["expired"] != true {
			t.Fatalf("expired subscription still live: %#v", row)
		}
		if row["source"] == "subscription" {
			sawSub = true
		}
		if row["source"] == "pack" && row["remaining"] == float64(40) && row["expired"] == false {
			sawPack = true
		}
	}
	if !sawSub || !sawPack {
		t.Fatalf("lots: %#v", lots)
	}
	years := listOne(t, ts, admin.access, "/api/admin/v1/yearly-qualifications")
	foundYear := false
	for _, raw := range years["items"].([]any) {
		row, _ := raw.(map[string]any)
		if row["user_id"] == float64(uid) && row["year"] == float64(2026) && row["unlock_source"] == "purchase" && row["unlocked"] == true {
			foundYear = true
		}
	}
	if !foundYear {
		t.Fatalf("yearly: %#v", years)
	}
}

func TestAdminEntitlementsFilterByUser(t *testing.T) {
	store, ts := newMergeServer(t)
	_, left := registerUser(t, ts, store, "entleft")
	_, right := registerUser(t, ts, store, "entright")
	seedGift(t, store, left, 1)
	seedGift(t, store, right, 2)
	if _, err := store.Exec(t.Context(), `INSERT INTO quota_lots(user_id, source, total, remaining, reason)
		VALUES ($1,'pack',4,4,'pack')`, left); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO yearly_reports(user_id, year, unlocked, unlock_source, unlocked_at, report_status)
		VALUES ($1, 2026, TRUE, 'membership', now(), 'pending')`, left); err != nil {
		t.Fatal(err)
	}
	var rel int64
	if err := store.QueryRow(t.Context(), `INSERT INTO invite_rewards(inviter_id, invitee_id, invite_code, amount)
		VALUES ($1,$2,$3,3) RETURNING id`, left, right, strconv.FormatInt(left, 10)).Scan(&rel); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO invite_grants(user_id, relationship_id, role, total, remaining, expires_at)
		VALUES ($1,$3,'inviter',3,3, now() + interval '1 year'),
		       ($2,$3,'invitee',3,3, now() + interval '1 year')`, left, right, rel); err != nil {
		t.Fatal(err)
	}
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	path := func(kind string) string {
		return fmt.Sprintf("/api/admin/v1/%s?user_id=%d", kind, left)
	}
	gifts := listOne(t, ts, admin.access, path("registration-gifts"))
	items, _ := gifts["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["user_id"] != float64(left) {
		t.Fatalf("gift filter: %#v", gifts)
	}
	lots := listOne(t, ts, admin.access, path("quota-lots"))
	lotItems, _ := lots["items"].([]any)
	if len(lotItems) != 1 || lotItems[0].(map[string]any)["source"] != "pack" {
		t.Fatalf("lot filter: %#v", lots)
	}
	years := listOne(t, ts, admin.access, path("yearly-qualifications"))
	yearItems, _ := years["items"].([]any)
	if len(yearItems) != 1 || yearItems[0].(map[string]any)["unlock_source"] != "membership" {
		t.Fatalf("year filter: %#v", years)
	}
	invites := listOne(t, ts, admin.access, path("invites"))
	inviteItems, _ := invites["items"].([]any)
	if len(inviteItems) != 1 {
		t.Fatalf("invite filter: %#v", invites)
	}
	row := inviteItems[0].(map[string]any)
	if row["inviter_id"] != float64(left) || row["invitee_id"] != float64(right) || row["inviter_remaining"] != float64(3) {
		t.Fatalf("invite row: %#v", row)
	}
	other := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/invites?user_id=%d", right))
	if len(other["items"].([]any)) != 1 {
		t.Fatalf("invitee side: %#v", other)
	}
	msg, _ := call(t, ts, admin.access, "GET", "/api/admin/v1/quota-lots?user_id=0", nil)
	if msg != "用户 id 不正确" {
		t.Fatalf("bad user: %s", msg)
	}
}

func listHasUser(t *testing.T, page map[string]any, uid int64) bool {
	t.Helper()
	items, _ := page["items"].([]any)
	for _, raw := range items {
		row, _ := raw.(map[string]any)
		if row["user_id"] == float64(uid) && row["remaining"] == float64(1) && row["granted_at"] != nil && row["expires_at"] != nil {
			return true
		}
	}
	return false
}
