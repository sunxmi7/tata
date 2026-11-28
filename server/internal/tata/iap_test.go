package tata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIAPSeedPricesAndAppleProductIDs(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})
	raw := get(t, ts, admin.access, "/api/admin/v1/iap/skus")
	body := string(raw)
	for _, banned := range []string{"18.80", "¥98", "98/年"} {
		if strings.Contains(body, banned) {
			t.Fatalf("seed exposed a competitor price %s: %s", banned, body)
		}
	}
	type spec struct {
		kind, apple string
		price       float64
		grant       any
	}
	want := map[string]spec{
		"sub_month":      {"month_subscription", "tata.membership.month", 3900, nil},
		"sub_year":       {"year_subscription", "tata.membership.year", 19800, nil},
		"pack_quota_20":  {"quota_pack", "tata.quota.20", 2000, float64(50)},
		"pack_quota_60":  {"quota_pack", "tata.quota.60", 6000, float64(160)},
		"pack_quota_100": {"quota_pack", "tata.quota.100", 10000, float64(300)},
		"unlock_yearly":  {"yearly_unlock", "tata.unlock.yearly", 2880, nil},
	}
	found := map[string]bool{}
	skus := list(t, ts, ro.access, "/api/admin/v1/iap/skus")
	for _, row := range skus {
		code, _ := row["code"].(string)
		spec, ok := want[code]
		if !ok {
			continue
		}
		found[code] = true
		if row["kind"] != spec.kind {
			t.Fatalf("%s mapping: %#v", code, row)
		}
		if row["price_amount_minor"].(float64) != spec.price || row["price_currency"] != "CNY" {
			t.Fatalf("%s price: %#v", code, row)
		}
		if row["app_store_product_id"] != spec.apple {
			t.Fatalf("%s apple id: %#v", code, row)
		}
		if row["quota_grant"] != spec.grant {
			t.Fatalf("%s grant: %#v", code, row)
		}
	}
	if len(found) != len(want) {
		t.Fatalf("priced slots: %#v", found)
	}
	cfg := listOne(t, ts, admin.access, "/api/admin/v1/iap/config")
	if cfg["restore_hook_enabled"] != true || cfg["restore_required_first_pack"] != false {
		t.Fatalf("restore switch: %#v", cfg)
	}
}

func TestIAPVerifyCreditsDiamondsAndIgnoresFreeQuota(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	email := fmt.Sprintf("iap-credit-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	uid := userIDByEmail(t, store, email)
	grant := 4
	product := fmt.Sprintf("test.month.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "month_subscription", "subscription", "app_store", product, &grant)
	txn := fmt.Sprintf("txn-credit-%d", time.Now().UnixNano())
	expires := time.Now().Add(30 * 24 * time.Hour).UTC()
	out := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, "orig-"+txn, "receipt-"+txn, &expires))
	if out["idempotent"] != false || out["verify_status"] != "verified" || out["credit_status"] != "credited" || out["unit"] != "quota" {
		t.Fatalf("verify: %#v", out)
	}
	if out["sku_kind"] != "month_subscription" || out["credited_quota"].(float64) != float64(grant) {
		t.Fatalf("credit: %#v", out)
	}
	if quotaNum(out) != float64(grant) {
		t.Fatalf("balances: %#v", out)
	}
	sub := out["subscription"].(map[string]any)
	if sub["status"] != "active" || sub["sku_kind"] != "month" || sub["expires_at"] == "" || sub["auto_renew"] != nil {
		t.Fatalf("subscription: %#v", sub)
	}
	if out["period_end"] == nil || out["period_end"] == "" {
		t.Fatal("missing validity")
	}
	assertFreeQuotaUntouched(t, store, uid)
	orders := list(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/iap/orders?user_id=%d", uid))
	if len(orders) != 1 || orders[0]["verify_status"] != "verified" || orders[0]["credit_status"] != "credited" || orders[0]["period_end"] == nil {
		t.Fatalf("admin order: %#v", orders)
	}
	if orders[0]["subscription_status"] != "active" || orders[0]["user_id"].(float64) != float64(uid) {
		t.Fatalf("admin subscription state: %#v", orders[0])
	}
}

func TestIAPEntitlementsMatchesQuotaForMember(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	token, uid := registerUser(t, ts, store, "iap-ent")
	month := create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"sku_code": "sub_month", "count": 1, "max_redemptions": 1,
	})
	code := month["items"].([]any)[0].(map[string]any)["code"].(string)
	redeemed := create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": code})
	if redeemed["kind"] != "month_subscription" {
		t.Fatalf("redeem: %#v", redeemed)
	}
	quota := listOne(t, ts, token, "/api/app/v1/quota")
	ent := listOne(t, ts, token, "/api/app/v1/iap/entitlements")
	if quota["is_member"] != true || ent["is_member"] != true {
		t.Fatalf("member flags quota=%#v ent=%#v uid=%d", quota, ent, uid)
	}
	qm, _ := quota["membership"].(map[string]any)
	em, _ := ent["membership"].(map[string]any)
	es, _ := ent["subscription"].(map[string]any)
	if qm == nil || em == nil || es == nil {
		t.Fatalf("membership missing quota=%#v ent=%#v", quota, ent)
	}
	for _, key := range []string{"sku_kind", "status", "expires_at", "type_label"} {
		if qm[key] != em[key] || em[key] != es[key] {
			t.Fatalf("%s mismatch quota=%#v ent.membership=%#v ent.subscription=%#v", key, qm, em, es)
		}
	}
	if em["sku_kind"] != "month" || em["status"] != "active" || em["type_label"] != "月度会员" {
		t.Fatalf("month membership: %#v", em)
	}
	if quota["quota_remaining"] != ent["quota_remaining"] || quota["balance"] != ent["balance"] {
		t.Fatalf("balance mismatch quota=%#v ent=%#v", quota, ent)
	}
}

func TestIAPDuplicateTransactionDoesNotDoubleCredit(t *testing.T) {
	store, ts := newMergeServer(t)
	email := fmt.Sprintf("iap-idem-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	uid := userIDByEmail(t, store, email)
	grant := 4
	product := fmt.Sprintf("test.idem.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "month_subscription", "subscription", "app_store", product, &grant)
	txn := fmt.Sprintf("txn-idem-%d", time.Now().UnixNano())
	expires := time.Now().Add(15 * 24 * time.Hour).UTC()
	body := iapBody("app_store", product, txn, txn, "receipt-idem", &expires)
	first := create(t, ts, token, "/api/app/v1/iap/verify", body)
	second := create(t, ts, token, "/api/app/v1/iap/verify", body)
	if first["idempotent"] != false || second["idempotent"] != true {
		t.Fatalf("idempotent flags: %#v %#v", first["idempotent"], second["idempotent"])
	}
	if second["credited_quota"].(float64) != float64(grant) || quotaNum(second) != float64(grant) {
		t.Fatalf("second credit changed balance: %#v", second)
	}
	var orders, ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM iap_orders WHERE store_transaction_id=$1`, txn).Scan(&orders); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE user_id=$1`, uid).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if orders != 1 || ledger != 1 {
		t.Fatalf("orders=%d ledger=%d", orders, ledger)
	}
	assertFreeQuotaUntouched(t, store, uid)
}

func TestIAPSubscriptionRefreshAndPackStaySeparate(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	email := fmt.Sprintf("iap-split-%d@tata.local", time.Now().UnixNano())
	otherEmail := fmt.Sprintf("iap-split-other-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	_ = register(t, ts, otherEmail)
	uid := userIDByEmail(t, store, email)
	other := userIDByEmail(t, store, otherEmail)
	subGrant, packGrant := 4, 2
	monthProduct := fmt.Sprintf("test.split.month.%d", time.Now().UnixNano())
	packProduct := fmt.Sprintf("test.split.pack.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "month_subscription", "subscription", "app_store", monthProduct, &subGrant)
	insertIAPSKUWithReadings(t, store, packProduct, packGrant)

	expires := time.Now().Add(20 * 24 * time.Hour).UTC()
	orig := fmt.Sprintf("orig-split-%d", time.Now().UnixNano())
	firstTxn := orig + "-1"
	first := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", monthProduct, firstTxn, orig, "receipt-sub-1", &expires))
	if quotaNum(first) != 4 {
		t.Fatalf("first grant: %#v", first)
	}
	if _, err := store.Exec(t.Context(), `UPDATE quota_balances SET quota_balance=1 WHERE user_id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(40 * 24 * time.Hour).UTC()
	secondTxn := orig + "-2"
	refreshed := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", monthProduct, secondTxn, orig, "receipt-sub-2", &later))
	if refreshed["credit_status"] != "deferred" || quotaNum(refreshed) != 1 {
		t.Fatalf("early renewal should wait: %#v", refreshed)
	}
	if refreshed["sku_kind"] != "month_subscription" || refreshed["credited_quota"] != nil {
		t.Fatalf("refresh credit: %#v", refreshed)
	}
	var subRows int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM subscriptions WHERE user_id=$1 AND platform='app_store' AND store_original_transaction_id=$2`, uid, orig).Scan(&subRows); err != nil {
		t.Fatal(err)
	}
	if subRows != 1 {
		t.Fatalf("renewal created another subscription row: %d", subRows)
	}

	packTxn := fmt.Sprintf("txn-pack-%d", time.Now().UnixNano())
	pack := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", packProduct, packTxn, packTxn, "receipt-pack", nil))
	if pack["sku_kind"] != "quota_pack" || pack["credited_quota"].(float64) != float64(packGrant) {
		t.Fatalf("pack credit: %#v", pack)
	}
	if quotaNum(pack) != float64(1+packGrant) {
		t.Fatalf("pack quota: %#v", pack)
	}
	if pack["subscription"] != nil || pack["period_end"] != nil {
		t.Fatalf("pack should not gain a subscription term: %#v", pack)
	}
	again := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", packProduct, packTxn, packTxn, "receipt-pack", nil))
	if again["idempotent"] != true || quotaNum(again) != float64(1+packGrant) {
		t.Fatalf("pack replay: %#v", again)
	}

	var quotaLedger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE user_id=$1 AND bucket='quota'`, uid).Scan(&quotaLedger); err != nil {
		t.Fatal(err)
	}
	if quotaLedger != 2 {
		t.Fatalf("ledger rows quota=%d", quotaLedger)
	}
	view := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/iap/quota?user_id=%d", uid))
	if quotaNum(view) != float64(1+packGrant) {
		t.Fatalf("admin quota: %#v", view)
	}
	buckets := map[string]bool{}
	for _, item := range view["ledger"].([]any) {
		row := item.(map[string]any)
		buckets[row["bucket"].(string)] = true
	}
	if !buckets["quota"] {
		t.Fatalf("admin ledger buckets: %#v", view["ledger"])
	}
	orders := list(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/iap/orders?user_id=%d&verify_status=verified", uid))
	if len(orders) != 3 {
		t.Fatalf("orders: %#v", orders)
	}
	seen := map[string]bool{}
	for _, row := range orders {
		if row["user_id"].(float64) != float64(uid) || row["verify_status"] != "verified" {
			t.Fatalf("order filter: %#v", row)
		}
		seen[row["sku_kind"].(string)] = true
	}
	if !seen["month_subscription"] || !seen["quota_pack"] {
		t.Fatalf("order kinds: %#v", orders)
	}
	others := list(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/iap/orders?user_id=%d", other))
	for _, row := range others {
		if row["store_transaction_id"] == packTxn || row["store_transaction_id"] == firstTxn {
			t.Fatalf("other user saw order: %#v", row)
		}
	}
	assertFreeQuotaUntouched(t, store, uid)
}

func TestPrepaidPeriodCreditsWhenCurrentEnds(t *testing.T) {
	store, ts := newMergeServer(t)
	email := fmt.Sprintf("iap-hold-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	uid := userIDByEmail(t, store, email)
	grant := 4
	product := fmt.Sprintf("test.hold.month.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "month_subscription", "subscription", "app_store", product, &grant)
	firstEnd := time.Now().Add(20 * 24 * time.Hour).UTC()
	orig := fmt.Sprintf("orig-hold-%d", time.Now().UnixNano())
	first := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, orig+"-1", orig, "receipt-hold-1", &firstEnd))
	if quotaNum(first) != float64(grant) {
		t.Fatalf("first grant: %#v", first)
	}
	var expires time.Time
	if err := store.QueryRow(t.Context(), `SELECT expires_at FROM subscriptions WHERE user_id=$1`, uid).Scan(&expires); err != nil {
		t.Fatal(err)
	}
	nextEnd := time.Now().Add(40 * 24 * time.Hour).UTC()
	held := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, orig+"-2", orig, "receipt-hold-2", &nextEnd))
	if held["credit_status"] != "deferred" || quotaNum(held) != float64(grant) {
		t.Fatalf("held: %#v", held)
	}
	var still, pending time.Time
	if err := store.QueryRow(t.Context(), `SELECT expires_at, pending_expires_at FROM subscriptions WHERE user_id=$1`, uid).Scan(&still, &pending); err != nil {
		t.Fatal(err)
	}
	if !still.Equal(expires) || !pending.After(still) {
		t.Fatalf("expires moved early: current %s pending %s", still, pending)
	}
	if _, err := store.Exec(t.Context(), `UPDATE subscriptions SET expires_at=now() - interval '1 minute' WHERE user_id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	if err := expireQuotaLots(t.Context(), store, uid); err != nil {
		t.Fatal(err)
	}
	var balance int
	var rolled time.Time
	var pendingLeft *time.Time
	if err := store.QueryRow(t.Context(), `SELECT quota_balance FROM quota_balances WHERE user_id=$1`, uid).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT expires_at, pending_expires_at FROM subscriptions WHERE user_id=$1`, uid).Scan(&rolled, &pendingLeft); err != nil {
		t.Fatal(err)
	}
	if balance != grant || pendingLeft != nil || !rolled.After(time.Now()) {
		t.Fatalf("rolled balance=%d expires=%s pending=%v", balance, rolled, pendingLeft)
	}
	if err := expireQuotaLots(t.Context(), store, uid); err != nil {
		t.Fatal(err)
	}
	var again int
	if err := store.QueryRow(t.Context(), `SELECT quota_balance FROM quota_balances WHERE user_id=$1`, uid).Scan(&again); err != nil {
		t.Fatal(err)
	}
	if again != grant {
		t.Fatalf("second roll credited again: %d", again)
	}
}

func TestIAPYearSubscriptionRefreshesSameBucket(t *testing.T) {
	store, ts := newMergeServer(t)
	email := fmt.Sprintf("iap-year-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	uid := userIDByEmail(t, store, email)
	grant := 6
	product := fmt.Sprintf("test.year.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "year_subscription", "subscription", "app_store", product, &grant)
	expires := time.Now().Add(300 * 24 * time.Hour).UTC()
	txn := fmt.Sprintf("txn-year-%d", time.Now().UnixNano())
	out := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, txn, "receipt-year", &expires))
	if out["sku_kind"] != "year_subscription" || quotaNum(out) != float64(grant) {
		t.Fatalf("year credit: %#v", out)
	}
	sub := out["subscription"].(map[string]any)
	if sub["sku_kind"] != "year" || sub["status"] != "active" {
		t.Fatalf("year subscription: %#v", sub)
	}
	var kind string
	if err := store.QueryRow(t.Context(), `SELECT sku_kind FROM subscriptions WHERE user_id=$1 AND store_original_transaction_id=$2`, uid, txn).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "year" {
		t.Fatalf("stored kind %s", kind)
	}
	assertFreeQuotaUntouched(t, store, uid)
}

func TestIAPRestoreHookUpdatesSubscriptionAndDiamonds(t *testing.T) {
	store, ts := newMergeServer(t)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE iap_settings SET restore_hook_enabled=TRUE, restore_required_first_pack=FALSE WHERE id=1`)
	})
	email := fmt.Sprintf("iap-restore-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	uid := userIDByEmail(t, store, email)
	grant := 4
	product := fmt.Sprintf("test.restore.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "month_subscription", "subscription", "app_store", product, &grant)
	if _, err := store.Exec(t.Context(), `UPDATE iap_settings SET restore_hook_enabled=FALSE WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	blockedTxn := fmt.Sprintf("txn-blocked-%d", time.Now().UnixNano())
	expires := time.Now().Add(25 * 24 * time.Hour).UTC()
	msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/iap/restore", map[string]any{
		"platform":  "app_store",
		"purchases": []map[string]any{iapBody("app_store", product, blockedTxn, blockedTxn, "receipt-blocked", &expires)},
	})
	if !strings.Contains(msg, "恢复购买钩子已关闭") {
		t.Fatalf("disabled hook: %s", msg)
	}
	var blocked int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM iap_orders WHERE store_transaction_id=$1`, blockedTxn).Scan(&blocked); err != nil {
		t.Fatal(err)
	}
	if blocked != 0 {
		t.Fatal("disabled restore wrote an order")
	}
	verifyTxn := fmt.Sprintf("txn-while-hook-off-%d", time.Now().UnixNano())
	verified := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, verifyTxn, verifyTxn, "receipt-verify", &expires))
	if verified["credit_status"] != "credited" || quotaNum(verified) != float64(grant) {
		t.Fatalf("verify must stay available when restore is off: %#v", verified)
	}
	if _, err := store.Exec(t.Context(), `UPDATE iap_settings SET restore_hook_enabled=TRUE WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	restoreTxn := fmt.Sprintf("txn-restore-%d", time.Now().UnixNano())
	restored := create(t, ts, token, "/api/app/v1/iap/restore", map[string]any{
		"platform":  "app_store",
		"purchases": []map[string]any{iapBody("", product, restoreTxn, restoreTxn, "receipt-restore", &expires)},
	})
	if restored["restore_hook_enabled"] != true || restored["restore_required_first_pack"] != false {
		t.Fatalf("restore flags: %#v", restored)
	}
	results := restored["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results: %#v", restored)
	}
	first := results[0].(map[string]any)
	if first["verify_status"] != "verified" || first["source"] != "restore" || first["idempotent"] != false {
		t.Fatalf("restore result: %#v", first)
	}
	if quotaNum(first) != float64(grant) {
		t.Fatalf("restore balances: %#v", first)
	}
	sub := first["subscription"].(map[string]any)
	if sub["status"] != "active" {
		t.Fatalf("restore subscription: %#v", sub)
	}
	var source string
	if err := store.QueryRow(t.Context(), `SELECT source FROM iap_orders WHERE store_transaction_id=$1`, restoreTxn).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "restore" {
		t.Fatalf("source %s", source)
	}
	again := create(t, ts, token, "/api/app/v1/iap/restore", map[string]any{
		"platform":  "app_store",
		"purchases": []map[string]any{iapBody("app_store", product, restoreTxn, restoreTxn, "receipt-restore", &expires)},
	})
	replay := again["results"].([]any)[0].(map[string]any)
	if replay["idempotent"] != true || quotaNum(replay) != float64(grant) {
		t.Fatalf("restore replay: %#v", replay)
	}
	var ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE user_id=$1 AND store_transaction_id=$2`, uid, restoreTxn).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 1 {
		t.Fatalf("restore ledger %d", ledger)
	}
	assertFreeQuotaUntouched(t, store, uid)
}

func TestIAPNullGrantDoesNotInventDiamonds(t *testing.T) {
	store, ts := newMergeServer(t)
	email := fmt.Sprintf("iap-empty-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	uid := userIDByEmail(t, store, email)
	product := fmt.Sprintf("test.empty.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "month_subscription", "subscription", "app_store", product, nil)
	txn := fmt.Sprintf("txn-empty-%d", time.Now().UnixNano())
	expires := time.Now().Add(10 * 24 * time.Hour).UTC()
	out := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, txn, "receipt-empty", &expires))
	if out["verify_status"] != "verified" || out["credit_status"] != "amount_unconfigured" || out["credited_quota"] != nil {
		t.Fatalf("unconfigured grant: %#v", out)
	}
	if quotaNum(out) != 0 {
		t.Fatalf("invented quota: %#v", out)
	}
	sub := out["subscription"].(map[string]any)
	if sub["status"] != "active" {
		t.Fatalf("subscription should still update: %#v", sub)
	}
	var ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE user_id=$1`, uid).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 0 {
		t.Fatalf("ledger rows %d", ledger)
	}
	laterGrant := 4
	if _, err := store.Exec(t.Context(), `UPDATE iap_skus SET quota_grant=$1 WHERE app_store_product_id=$2`, laterGrant, product); err != nil {
		t.Fatal(err)
	}
	replay := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, txn, "receipt-empty", &expires))
	if replay["idempotent"] != true || replay["credit_status"] != "amount_unconfigured" || quotaNum(replay) != 0 {
		t.Fatalf("later grant must not rewrite a verified order: %#v", replay)
	}
	assertFreeQuotaUntouched(t, store, uid)
}

func TestIAPTransactionCannotCreditSecondUser(t *testing.T) {
	store, ts := newMergeServer(t)
	stamp := time.Now().UnixNano()
	tokenA := register(t, ts, fmt.Sprintf("iap-a-%d@tata.local", stamp))
	tokenB := register(t, ts, fmt.Sprintf("iap-b-%d@tata.local", stamp))
	idA := userIDByEmail(t, store, fmt.Sprintf("iap-a-%d@tata.local", stamp))
	grant := 4
	product := fmt.Sprintf("test.bind.%d", stamp)
	insertIAPSKU(t, store, "quota_pack", "", "play", product, &grant)
	txn := fmt.Sprintf("txn-bind-%d", stamp)
	body := iapBody("play", product, txn, txn, "receipt-bind", nil)
	first := create(t, ts, tokenA, "/api/app/v1/iap/verify", body)
	if first["verify_status"] != "failed" || !strings.Contains(fmt.Sprint(first["verify_message"]), "仅会员") {
		t.Fatalf("non-member pack: %#v", first)
	}
	msg, _ := call(t, ts, tokenB, http.MethodPost, "/api/app/v1/iap/verify", body)
	if !strings.Contains(msg, "交易号已使用") {
		t.Fatalf("second user: %s", msg)
	}
	var ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE store_transaction_id=$1`, txn).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 0 {
		t.Fatalf("offline pack wrote ledger %d", ledger)
	}
	if n := quotaOf(t, store, idA); n != 0 {
		t.Fatalf("first balance changed: %d", n)
	}
}

func TestIAPVerifyRejectsUnmappedAndMissingExpiry(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	email := fmt.Sprintf("iap-bad-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	uid := userIDByEmail(t, store, email)
	unknownTxn := fmt.Sprintf("txn-unknown-%d", time.Now().UnixNano())
	unknown := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", "missing.product", unknownTxn, unknownTxn, "receipt-unknown", nil))
	if unknown["verify_status"] != "failed" || !strings.Contains(unknown["verify_message"].(string), "未映射") {
		t.Fatalf("unknown product: %#v", unknown)
	}
	msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/iap/verify", iapBody("app_store", "missing.product", unknownTxn, unknownTxn, "", nil))
	if !strings.Contains(msg, "票据为空") {
		t.Fatalf("empty receipt on replay should fail before rewrite: %s", msg)
	}
	grant := 4
	product := fmt.Sprintf("test.noexp.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "month_subscription", "subscription", "app_store", product, &grant)
	noExpTxn := fmt.Sprintf("txn-noexp-%d", time.Now().UnixNano())
	noExp := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, noExpTxn, noExpTxn, "receipt-noexp", nil))
	if noExp["verify_status"] != "failed" || !strings.Contains(noExp["verify_message"].(string), "有效期") {
		t.Fatalf("missing expiry: %#v", noExp)
	}
	var subs, quota int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM subscriptions WHERE user_id=$1 AND store_original_transaction_id=$2`, uid, noExpTxn).Scan(&subs); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE user_id=$1`, uid).Scan(&quota); err != nil {
		t.Fatal(err)
	}
	if subs != 0 || quota != 0 {
		t.Fatalf("failed verify wrote entitlement subs=%d quota=%d", subs, quota)
	}
	orders := list(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/iap/orders?user_id=%d&verify_status=failed", uid))
	if len(orders) < 2 {
		t.Fatalf("failed orders not queryable: %#v", orders)
	}
	past := time.Now().Add(-time.Hour).UTC()
	expiredTxn := fmt.Sprintf("txn-expired-%d", time.Now().UnixNano())
	expired := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, expiredTxn, expiredTxn, "receipt-expired", &past))
	if expired["verify_status"] != "verified" || expired["credit_status"] != "skipped_expired" || quotaNum(expired) != 0 {
		t.Fatalf("expired: %#v", expired)
	}
	sub := expired["subscription"].(map[string]any)
	if sub["status"] != "expired" {
		t.Fatalf("expired status: %#v", sub)
	}
	assertFreeQuotaUntouched(t, store, uid)
}

func TestIAPConcurrentVerifyCreditsOnce(t *testing.T) {
	store, ts := newMergeServer(t)
	email := fmt.Sprintf("iap-race-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	uid := userIDByEmail(t, store, email)
	grant := 4
	product := fmt.Sprintf("test.race.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "month_subscription", "subscription", "app_store", product, &grant)
	txn := fmt.Sprintf("txn-race-%d", time.Now().UnixNano())
	expires := time.Now().Add(20 * 24 * time.Hour).UTC()
	body := iapBody("app_store", product, txn, txn, "receipt-race", &expires)
	var wg sync.WaitGroup
	type result struct {
		code int
		data map[string]any
		msg  string
	}
	out := make([]result, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, data, msg := postIAP(ts, token, "/api/app/v1/iap/verify", body)
			out[i] = result{code: code, data: data, msg: msg}
		}(i)
	}
	wg.Wait()
	credited := 0
	for _, item := range out {
		if item.msg != "" || item.code != 0 {
			t.Fatalf("concurrent verify: %+v", item)
		}
		if item.data["idempotent"] != true {
			credited++
		}
		if quotaNum(item.data) != float64(grant) {
			t.Fatalf("balance: %#v", item.data)
		}
	}
	if credited != 1 {
		t.Fatalf("expected one fresh credit, got %d (%#v %#v)", credited, out[0].data["idempotent"], out[1].data["idempotent"])
	}
	var ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE user_id=$1 AND store_transaction_id=$2`, uid, txn).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 1 {
		t.Fatalf("ledger %d", ledger)
	}
}

var iapSKUSeq atomic.Int64

func insertIAPSKU(t *testing.T, store *Store, kind, bucket, platform, productID string, grant *int) {
	t.Helper()
	code := fmt.Sprintf("iap-test-%d-%d", time.Now().UnixNano(), iapSKUSeq.Add(1))
	var appID, playID any
	switch platform {
	case "app_store":
		appID = productID
	case "play":
		playID = productID
	default:
		t.Fatalf("platform %s", platform)
	}
	var amount any
	if grant != nil {
		amount = *grant
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO iap_skus(code, kind, display_name, app_store_product_id, play_product_id, quota_grant)
		VALUES ($1,$2,'测试商品位',$3,$4,$5)`, code, kind, appID, playID, amount); err != nil {
		t.Fatal(err)
	}
}

func iapBody(platform, product, txn, original, receipt string, expires *time.Time) map[string]any {
	body := map[string]any{
		"platform":                platform,
		"product_id":              product,
		"transaction_id":          txn,
		"original_transaction_id": original,
		"receipt":                 receipt,
	}
	if expires != nil {
		body["expires_at"] = expires.UTC().Format(time.RFC3339)
	}
	return body
}

func quotaNum(m map[string]any) float64 {
	if m == nil {
		return 0
	}
	if v, ok := m["quota_remaining"].(float64); ok {
		return v
	}
	if v, ok := m["balance"].(float64); ok {
		return v
	}
	return 0
}

func assertFreeQuotaUntouched(t *testing.T, store *Store, uid int64) {
	t.Helper()
	var reward, ledgerN, quotaN int
	err := store.QueryRow(t.Context(), `SELECT remaining FROM rewards WHERE user_id=$1`, uid).Scan(&reward)
	if err != nil {
		reward = 0
	}
	if reward != 0 {
		t.Fatalf("registration reward remaining %d", reward)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM ledger WHERE user_id=$1`, uid).Scan(&ledgerN); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_use WHERE user_id=$1`, uid).Scan(&quotaN); err != nil {
		t.Fatal(err)
	}
	if ledgerN != 0 || quotaN != 0 {
		t.Fatalf("IAP touched free pools ledger=%d quota=%d", ledgerN, quotaN)
	}
}

func TestAdminIAPSKUCreateByKindAndPrice(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})
	token, uid := registerUser(t, ts, store, "skuadmin")
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE iap_skus SET enabled=FALSE WHERE code LIKE $1`, "%"+suffix)
	})
	price := int64(3900)
	grant := 60
	month := create(t, ts, admin.access, "/api/admin/v1/iap/skus", map[string]any{
		"code": "month_" + suffix, "kind": "month_subscription", "display_name": "月卡测试",
		"price_amount_minor": price, "price_currency": "CNY", "period_count": 1,
	})
	if month["kind"] != "month_subscription" || month["price_amount_minor"].(float64) != 3900 {
		t.Fatalf("month sku: %#v", month)
	}
	pack := create(t, ts, admin.access, "/api/admin/v1/iap/skus", map[string]any{
		"code": "pack_" + suffix, "kind": "quota_pack", "display_name": "60 测试",
		"price_amount_minor": int64(600), "quota_grant": grant,
	})
	if pack["kind"] != "quota_pack" || pack["quota_grant"].(float64) != 60 || pack["price_currency"] != "CNY" {
		t.Fatalf("pack sku: %#v", pack)
	}
	unlock := create(t, ts, admin.access, "/api/admin/v1/iap/skus", map[string]any{
		"code": "yearunlock_" + suffix, "kind": "yearly_unlock", "display_name": "年运测试",
		"price_amount_minor": int64(2880),
	})
	if unlock["kind"] != "yearly_unlock" || unlock["quota_grant"] != nil {
		t.Fatalf("unlock sku: %#v", unlock)
	}
	deck := create(t, ts, admin.access, "/api/admin/v1/iap/skus", map[string]any{
		"code": "deck_" + suffix, "kind": "deck_single", "display_name": "单套测试",
		"price_amount_minor": int64(660),
	})
	if deck["kind"] != "deck_single" || deck["quota_grant"] != nil {
		t.Fatalf("deck sku: %#v", deck)
	}
	msg, _ := call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/iap/skus", map[string]any{
		"code": "baddeck_" + suffix, "kind": "deck_year", "display_name": "坏牌组", "quota_grant": 10,
	})
	if !strings.Contains(msg, "不入账额度") {
		t.Fatalf("deck grant: %s", msg)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/iap/skus", map[string]any{
		"code": "badpack_" + suffix, "kind": "coin_pack", "display_name": "坏币包",
	})
	if !strings.Contains(msg, "商品类型不正确") {
		t.Fatalf("removed kind: %s", msg)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/iap/skus", map[string]any{
		"code": "bad_" + suffix, "kind": "yearly_unlock", "display_name": "坏年运", "quota_grant": 10,
	})
	if !strings.Contains(msg, "不入账额度") {
		t.Fatalf("yearly grant: %s", msg)
	}
	msg, _ = call(t, ts, ro.access, http.MethodPost, "/api/admin/v1/iap/skus", map[string]any{
		"code": "ro_" + suffix, "kind": "month_subscription", "display_name": "只读",
	})
	if !strings.Contains(msg, "只读") {
		t.Fatalf("readonly create: %s", msg)
	}
	filtered := list(t, ts, token, "/api/app/v1/iap/skus?kind=yearly_unlock")
	foundUnlock := false
	for _, row := range filtered {
		if row["code"] == "yearunlock_"+suffix {
			foundUnlock = true
		}
	}
	if !foundUnlock {
		t.Fatalf("app kind filter: %#v", filtered)
	}
	timesList := list(t, ts, token, "/api/app/v1/iap/skus?kind=quota_pack")
	for _, row := range timesList {
		if row["code"] == "pack_"+suffix {
			t.Fatalf("free user saw reading pack: %#v", timesList)
		}
	}
	msg, _ = call(t, ts, token, http.MethodPost, "/api/app/v1/yearly-reports/2026/unlock", map[string]any{})
	if !strings.Contains(msg, "内购") {
		t.Fatalf("unlock with priced sku: %s", msg)
	}
	setMember(t, store, uid, "month")
	timesList = list(t, ts, token, "/api/app/v1/iap/skus?kind=quota_pack")
	foundTimes := false
	for _, row := range timesList {
		if row["code"] == "pack_"+suffix {
			foundTimes = true
			if row["quota_grant"].(float64) != 60 {
				t.Fatalf("app reading pack: %#v", row)
			}
		}
	}
	if !foundTimes {
		t.Fatalf("app reading pack filter: %#v", timesList)
	}
	yearView := listOne(t, ts, token, "/api/app/v1/yearly-reports/2026")
	if yearView["unlock_price_minor"].(float64) != 2880 || yearView["unlock_price_currency"] != "CNY" || yearView["standalone_purchase"] != false || yearView["unlocked"] != true {
		t.Fatalf("yearly price from sku: %#v", yearView)
	}
	memberSKUs := list(t, ts, token, "/api/app/v1/iap/skus?kind=yearly_unlock")
	if len(memberSKUs) != 0 {
		t.Fatalf("member saw yearly unlock: %#v", memberSKUs)
	}
	opened := create(t, ts, token, "/api/app/v1/yearly-reports/2026/unlock", map[string]any{})
	if opened["unlocked"] != true {
		t.Fatalf("member unlock: %#v", opened)
	}
	id := asID(month["id"])
	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/iap/skus/%d", id), map[string]any{
		"display_name": "月卡改价", "price_amount_minor": int64(4900), "price_currency": "CNY", "enabled": true,
	})
	skus := list(t, ts, admin.access, "/api/admin/v1/iap/skus?kind=month_subscription")
	found := false
	for _, row := range skus {
		if asID(row["id"]) == id {
			found = true
			if row["display_name"] != "月卡改价" || row["price_amount_minor"].(float64) != 4900 {
				t.Fatalf("updated: %#v", row)
			}
		}
	}
	if !found {
		t.Fatalf("updated month missing: %#v", skus)
	}
}

func TestIAPReadingPackCreditsFreeCounts(t *testing.T) {
	store, ts := newMergeServer(t)
	email := fmt.Sprintf("iap-times-%d@tata.local", time.Now().UnixNano())
	token := register(t, ts, email)
	uid := userIDByEmail(t, store, email)
	grant := 5
	product := fmt.Sprintf("test.times.%d", time.Now().UnixNano())
	insertIAPSKUWithReadings(t, store, product, grant)
	txn := fmt.Sprintf("txn-times-%d", time.Now().UnixNano())
	blocked := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, "orig-"+txn, "receipt-"+txn, nil))
	if blocked["verify_status"] != "failed" || !strings.Contains(fmt.Sprint(blocked["verify_message"]), "仅会员") {
		t.Fatalf("non-member pack: %#v", blocked)
	}
	setMember(t, store, uid, "month")
	txnOK := fmt.Sprintf("txn-times-ok-%d", time.Now().UnixNano())
	out := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txnOK, "orig-"+txnOK, "receipt-"+txnOK, nil))
	if out["idempotent"] != false || out["verify_status"] != "verified" || out["credit_status"] != "credited" {
		t.Fatalf("verify: %#v", out)
	}
	if out["sku_kind"] != "quota_pack" || out["credited_quota"].(float64) != 5 {
		t.Fatalf("credit: %#v", out)
	}
	if quotaNum(out) != float64(grant) {
		t.Fatalf("quota: %#v", out)
	}
	if quotaOf(t, store, uid) != grant {
		t.Fatalf("quota remaining %d", quotaOf(t, store, uid))
	}
	var ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE user_id=$1 AND bucket='quota'`, uid).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 1 {
		t.Fatalf("quota ledger %d", ledger)
	}
	blank := fmt.Sprintf("test.times.blank.%d", time.Now().UnixNano())
	insertIAPSKUWithReadings(t, store, blank, 0)
	txn2 := fmt.Sprintf("txn-times-blank-%d", time.Now().UnixNano())
	unconfig := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", blank, txn2, "orig-"+txn2, "receipt-"+txn2, nil))
	if unconfig["credit_status"] != "amount_unconfigured" || unconfig["credited_quota"] != nil {
		t.Fatalf("unconfigured: %#v", unconfig)
	}
}

func insertIAPSKUWithReadings(t *testing.T, store *Store, productID string, grant int) {
	t.Helper()
	code := fmt.Sprintf("iap-times-%d-%d", time.Now().UnixNano(), iapSKUSeq.Add(1))
	var amount any
	if grant > 0 {
		amount = grant
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO iap_skus(code, kind, display_name, app_store_product_id, quota_grant)
		VALUES ($1,'quota_pack','额度补充测试',$2,$3)`, code, productID, amount); err != nil {
		t.Fatal(err)
	}
}

func postIAP(ts *httptest.Server, token, path string, body any) (int, map[string]any, string) {
	rawBody, _ := json.Marshal(body)
	if rawApp, ok := liveTestApps.Load(ts.URL); ok {
		if _, raw, handled := dispatchRetired(rawApp.(*App), http.MethodPost, path, rawBody, token); handled {
			var env struct {
				Code    int            `json:"code"`
				Message string         `json:"message"`
				Data    map[string]any `json:"data"`
			}
			if err := json.Unmarshal(raw, &env); err != nil {
				return 1, nil, err.Error()
			}
			return env.Code, env.Data, env.Message
		}
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(rawBody))
	if err != nil {
		return 1, nil, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1, nil, err.Error()
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var env struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return 1, nil, err.Error()
	}
	return env.Code, env.Data, env.Message
}

func TestSchemaCommentsMapPriceKinds(t *testing.T) {
	store, _ := newMergeServer(t)
	var table, kind, grant string
	if err := store.QueryRow(t.Context(), `SELECT obj_description('iap_skus'::regclass)`).Scan(&table); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(table, "会员") || !strings.Contains(table, "牌组") || !strings.Contains(table, "年运") {
		t.Fatalf("iap_skus table comment: %s", table)
	}
	if err := store.QueryRow(t.Context(), `
		SELECT col_description('iap_skus'::regclass, attnum) FROM pg_attribute
		WHERE attrelid='iap_skus'::regclass AND attname='kind'`).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(kind, "quota_pack") || !strings.Contains(kind, "deck_year") || strings.Contains(kind, "consumable_pack") {
		t.Fatalf("iap_skus.kind comment: %s", kind)
	}
	if err := store.QueryRow(t.Context(), `
		SELECT col_description('iap_skus'::regclass, attnum) FROM pg_attribute
		WHERE attrelid='iap_skus'::regclass AND attname='quota_grant'`).Scan(&grant); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(grant, "额度") {
		t.Fatalf("quota_grant comment: %s", grant)
	}
	var gone bool
	if err := store.QueryRow(t.Context(), `SELECT to_regclass('public.deck_products') IS NULL`).Scan(&gone); err != nil {
		t.Fatal(err)
	}
	if !gone {
		t.Fatal("deck_products still exists")
	}
}
