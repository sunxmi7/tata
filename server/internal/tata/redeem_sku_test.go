package tata

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestRedeemCodesByIAPSKU(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	token, uid := registerUser(t, ts, store, "redeem-sku")

	// membership month
	month := create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"sku_code": "sub_month", "count": 1, "max_redemptions": 1,
	})
	monthCode := month["items"].([]any)[0].(map[string]any)["code"].(string)
	if month["items"].([]any)[0].(map[string]any)["kind"] != "month_subscription" {
		t.Fatalf("month create: %#v", month)
	}
	got := create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": monthCode})
	if got["sku_code"] != "sub_month" || got["kind"] != "month_subscription" || got["membership_held"] != false {
		t.Fatalf("month redeem: %#v", got)
	}
	member, err := hasActiveMembership(t.Context(), store, uid)
	if err != nil || !member {
		t.Fatalf("membership after month: member=%v err=%v", member, err)
	}

	// stack second month → held/pending like IAP
	month2 := create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"sku_code": "sub_month", "max_redemptions": 1,
	})
	month2Code := month2["items"].([]any)[0].(map[string]any)["code"].(string)
	stacked := create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": month2Code})
	if stacked["membership_held"] != true {
		t.Fatalf("stack: %#v", stacked)
	}

	// quota pack requires membership
	pack := create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"sku_code": "pack_quota_20", "max_redemptions": 1,
	})
	packCode := pack["items"].([]any)[0].(map[string]any)["code"].(string)
	if pack["items"].([]any)[0].(map[string]any)["amount"].(float64) != 50 {
		t.Fatalf("pack amount snapshot: %#v", pack)
	}
	before := quotaOf(t, store, uid)
	packOut := create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": packCode})
	if packOut["amount"].(float64) != 50 || packOut["kind"] != "quota_pack" {
		t.Fatalf("pack redeem: %#v", packOut)
	}
	if quotaOf(t, store, uid) != before+50 {
		t.Fatalf("pack balance %d -> %d", before, quotaOf(t, store, uid))
	}

	// yearly unlock → current Beijing year
	yearCodeBody := create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"sku_code": "unlock_yearly", "max_redemptions": 1,
	})
	yearCode := yearCodeBody["items"].([]any)[0].(map[string]any)["code"].(string)
	yearOut := create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": yearCode})
	wantYear := float64(beijingNow().Year())
	if yearOut["year"] != wantYear || yearOut["unlocked"] != true {
		t.Fatalf("yearly: %#v want year=%v", yearOut, wantYear)
	}
	var unlocked bool
	if err := store.QueryRow(t.Context(), `SELECT unlocked FROM yearly_reports WHERE user_id=$1 AND year=$2`, uid, beijingNow().Year()).Scan(&unlocked); err != nil || !unlocked {
		t.Fatalf("yearly row: unlocked=%v err=%v", unlocked, err)
	}

	// deck unlock
	deck := create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"sku_code": "deck_year", "max_redemptions": 1,
	})
	deckCode := deck["items"].([]any)[0].(map[string]any)["code"].(string)
	deckOut := create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": deckCode})
	if deckOut["kind"] != "deck_year" {
		t.Fatalf("deck: %#v", deckOut)
	}
	var unlocks int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM user_deck_unlocks WHERE user_id=$1`, uid).Scan(&unlocks); err != nil || unlocks < 1 {
		t.Fatalf("deck unlocks=%d err=%v", unlocks, err)
	}

	// bare quota still works for ops
	bare := fmt.Sprintf("BARE%d", time.Now().UnixNano()%100000000)
	create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"kind": "quota", "amount": 7, "benefit_name": "运营补额", "code": bare, "max_redemptions": 1,
	})
	other, otherUID := registerUser(t, ts, store, "redeem-bare")
	bareOut := create(t, ts, other, "/api/app/v1/redeem", map[string]any{"code": bare})
	if bareOut["kind"] != "quota" || bareOut["amount"].(float64) != 7 {
		t.Fatalf("bare: %#v", bareOut)
	}
	if quotaOf(t, store, otherUID) != 7 {
		t.Fatalf("bare balance")
	}

	// non-member cannot redeem pack
	pack2 := create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"sku_code": "pack_quota_20", "max_redemptions": 1,
	})
	pack2Code := pack2["items"].([]any)[0].(map[string]any)["code"].(string)
	if msg, _ := call(t, ts, other, http.MethodPost, "/api/app/v1/redeem", map[string]any{"code": pack2Code}); msg != "仅会员可兑换额度补充包" {
		t.Fatalf("pack gate: %s", msg)
	}
}
