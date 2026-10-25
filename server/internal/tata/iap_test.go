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

func TestIAPSeedSlotsLeavePriceAndGrantEmpty(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})
	raw := get(t, ts, admin.access, "/api/admin/v1/iap/skus")
	body := string(raw)
	for _, banned := range []string{"18.80", "28.80", "¥98", "98/年"} {
		if strings.Contains(body, banned) {
			t.Fatalf("seed exposed a competitor price %s: %s", banned, body)
		}
	}
	skus := list(t, ts, ro.access, "/api/admin/v1/iap/skus")
	want := map[string][2]string{
		"sub_month":       {"month_subscription", "subscription"},
		"sub_year":        {"year_subscription", "subscription"},
		"pack_consumable": {"consumable_pack", "consumable"},
	}
	found := map[string]bool{}
	for _, row := range skus {
		code, _ := row["code"].(string)
		spec, ok := want[code]
		if !ok {
			continue
		}
		found[code] = true
		if row["kind"] != spec[0] || row["ledger_bucket"] != spec[1] {
			t.Fatalf("%s mapping: %#v", code, row)
		}
		for _, key := range []string{"price_amount_minor", "price_currency", "diamond_grant", "app_store_product_id", "play_product_id", "period_count"} {
			if row[key] != nil {
				t.Fatalf("%s %s should stay empty, got %#v", code, key, row[key])
			}
		}
	}
	if len(found) != len(want) {
		t.Fatalf("reserved slots: %#v", found)
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
	if out["idempotent"] != false || out["verify_status"] != "verified" || out["credit_status"] != "credited" || out["unit"] != "diamond" {
		t.Fatalf("verify: %#v", out)
	}
	if out["ledger_bucket"] != "subscription" || out["credited_diamonds"].(float64) != float64(grant) {
		t.Fatalf("credit: %#v", out)
	}
	if out["subscription_diamonds"].(float64) != float64(grant) || out["pack_diamonds"].(float64) != 0 {
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
	if second["credited_diamonds"].(float64) != float64(grant) || second["subscription_diamonds"].(float64) != float64(grant) {
		t.Fatalf("second credit changed balance: %#v", second)
	}
	var orders, ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM iap_orders WHERE store_transaction_id=$1`, txn).Scan(&orders); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE user_id=$1`, uid).Scan(&ledger); err != nil {
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
	insertIAPSKU(t, store, "consumable_pack", "consumable", "play", packProduct, &packGrant)

	expires := time.Now().Add(20 * 24 * time.Hour).UTC()
	orig := fmt.Sprintf("orig-split-%d", time.Now().UnixNano())
	firstTxn := orig + "-1"
	first := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", monthProduct, firstTxn, orig, "receipt-sub-1", &expires))
	if first["subscription_diamonds"].(float64) != 4 || first["pack_diamonds"].(float64) != 0 {
		t.Fatalf("first grant: %#v", first)
	}
	if _, err := store.Exec(t.Context(), `UPDATE diamond_balances SET subscription_diamonds=1 WHERE user_id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(40 * 24 * time.Hour).UTC()
	secondTxn := orig + "-2"
	refreshed := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", monthProduct, secondTxn, orig, "receipt-sub-2", &later))
	if refreshed["subscription_diamonds"].(float64) != float64(subGrant) || refreshed["pack_diamonds"].(float64) != 0 {
		t.Fatalf("refresh should replace the subscription bucket, not add: %#v", refreshed)
	}
	if refreshed["ledger_bucket"] != "subscription" || refreshed["credited_diamonds"].(float64) != float64(subGrant) {
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
	pack := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("play", packProduct, packTxn, packTxn, "receipt-pack", nil))
	if pack["ledger_bucket"] != "consumable" || pack["credited_diamonds"].(float64) != float64(packGrant) {
		t.Fatalf("pack credit: %#v", pack)
	}
	if pack["pack_diamonds"].(float64) != float64(packGrant) || pack["subscription_diamonds"].(float64) != float64(subGrant) {
		t.Fatalf("buckets mixed: %#v", pack)
	}
	if pack["subscription"] != nil || pack["period_end"] != nil {
		t.Fatalf("consumable should not gain a subscription term: %#v", pack)
	}
	again := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("play", packProduct, packTxn, packTxn, "receipt-pack", nil))
	if again["idempotent"] != true || again["pack_diamonds"].(float64) != float64(packGrant) {
		t.Fatalf("pack replay: %#v", again)
	}

	var subLedger, packLedger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE user_id=$1 AND bucket='subscription'`, uid).Scan(&subLedger); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE user_id=$1 AND bucket='consumable'`, uid).Scan(&packLedger); err != nil {
		t.Fatal(err)
	}
	if subLedger != 2 || packLedger != 1 {
		t.Fatalf("ledger rows subscription=%d pack=%d", subLedger, packLedger)
	}
	view := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/iap/diamonds?user_id=%d", uid))
	if view["subscription_diamonds"].(float64) != float64(subGrant) || view["pack_diamonds"].(float64) != float64(packGrant) {
		t.Fatalf("admin diamonds: %#v", view)
	}
	buckets := map[string]bool{}
	for _, item := range view["ledger"].([]any) {
		row := item.(map[string]any)
		buckets[row["bucket"].(string)] = true
	}
	if !buckets["subscription"] || !buckets["consumable"] {
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
		seen[row["ledger_bucket"].(string)] = true
	}
	if !seen["subscription"] || !seen["consumable"] {
		t.Fatalf("order buckets: %#v", orders)
	}
	others := list(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/iap/orders?user_id=%d", other))
	for _, row := range others {
		if row["store_transaction_id"] == packTxn || row["store_transaction_id"] == firstTxn {
			t.Fatalf("other user saw order: %#v", row)
		}
	}
	assertFreeQuotaUntouched(t, store, uid)
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
	if out["ledger_bucket"] != "subscription" || out["pack_diamonds"].(float64) != 0 || out["subscription_diamonds"].(float64) != float64(grant) {
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
	if verified["credit_status"] != "credited" || verified["subscription_diamonds"].(float64) != float64(grant) {
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
	if first["subscription_diamonds"].(float64) != float64(grant) || first["pack_diamonds"].(float64) != 0 {
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
	if replay["idempotent"] != true || replay["subscription_diamonds"].(float64) != float64(grant) {
		t.Fatalf("restore replay: %#v", replay)
	}
	var ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE user_id=$1 AND store_transaction_id=$2`, uid, restoreTxn).Scan(&ledger); err != nil {
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
	if out["verify_status"] != "verified" || out["credit_status"] != "amount_unconfigured" || out["credited_diamonds"] != nil {
		t.Fatalf("unconfigured grant: %#v", out)
	}
	if out["subscription_diamonds"].(float64) != 0 || out["pack_diamonds"].(float64) != 0 {
		t.Fatalf("invented diamonds: %#v", out)
	}
	sub := out["subscription"].(map[string]any)
	if sub["status"] != "active" {
		t.Fatalf("subscription should still update: %#v", sub)
	}
	var ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE user_id=$1`, uid).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 0 {
		t.Fatalf("ledger rows %d", ledger)
	}
	laterGrant := 4
	if _, err := store.Exec(t.Context(), `UPDATE iap_skus SET diamond_grant=$1 WHERE app_store_product_id=$2`, laterGrant, product); err != nil {
		t.Fatal(err)
	}
	replay := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, txn, "receipt-empty", &expires))
	if replay["idempotent"] != true || replay["credit_status"] != "amount_unconfigured" || replay["subscription_diamonds"].(float64) != 0 {
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
	insertIAPSKU(t, store, "consumable_pack", "consumable", "play", product, &grant)
	txn := fmt.Sprintf("txn-bind-%d", stamp)
	body := iapBody("play", product, txn, txn, "receipt-bind", nil)
	first := create(t, ts, tokenA, "/api/app/v1/iap/verify", body)
	if first["pack_diamonds"].(float64) != float64(grant) {
		t.Fatalf("first user: %#v", first)
	}
	msg, _ := call(t, ts, tokenB, http.MethodPost, "/api/app/v1/iap/verify", body)
	if !strings.Contains(msg, "交易号已使用") {
		t.Fatalf("second user: %s", msg)
	}
	var ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE store_transaction_id=$1`, txn).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 1 {
		t.Fatalf("ledger %d", ledger)
	}
	var packA int
	if err := store.QueryRow(t.Context(), `SELECT pack_diamonds FROM diamond_balances WHERE user_id=$1`, idA).Scan(&packA); err != nil {
		t.Fatal(err)
	}
	if packA != grant {
		t.Fatalf("first balance changed: %d", packA)
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
	var subs, diamonds int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM subscriptions WHERE user_id=$1 AND store_original_transaction_id=$2`, uid, noExpTxn).Scan(&subs); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE user_id=$1`, uid).Scan(&diamonds); err != nil {
		t.Fatal(err)
	}
	if subs != 0 || diamonds != 0 {
		t.Fatalf("failed verify wrote entitlement subs=%d diamonds=%d", subs, diamonds)
	}
	orders := list(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/iap/orders?user_id=%d&verify_status=failed", uid))
	if len(orders) < 2 {
		t.Fatalf("failed orders not queryable: %#v", orders)
	}
	past := time.Now().Add(-time.Hour).UTC()
	expiredTxn := fmt.Sprintf("txn-expired-%d", time.Now().UnixNano())
	expired := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, expiredTxn, expiredTxn, "receipt-expired", &past))
	if expired["verify_status"] != "verified" || expired["credit_status"] != "skipped_expired" || expired["subscription_diamonds"].(float64) != 0 {
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
	insertIAPSKU(t, store, "consumable_pack", "consumable", "app_store", product, &grant)
	txn := fmt.Sprintf("txn-race-%d", time.Now().UnixNano())
	body := iapBody("app_store", product, txn, txn, "receipt-race", nil)
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
		if item.data["pack_diamonds"].(float64) != float64(grant) {
			t.Fatalf("balance: %#v", item.data)
		}
	}
	if credited != 1 {
		t.Fatalf("expected one fresh credit, got %d (%#v %#v)", credited, out[0].data["idempotent"], out[1].data["idempotent"])
	}
	var ledger int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE user_id=$1 AND store_transaction_id=$2`, uid, txn).Scan(&ledger); err != nil {
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
	if _, err := store.Exec(t.Context(), `INSERT INTO iap_skus(code, kind, ledger_bucket, display_name, app_store_product_id, play_product_id, diamond_grant)
		VALUES ($1,$2,$3,'测试商品位',$4,$5,$6)`, code, kind, bucket, appID, playID, amount); err != nil {
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

func assertFreeQuotaUntouched(t *testing.T, store *Store, uid int64) {
	t.Helper()
	var reward, ledgerN, poolN, quotaN int
	if err := store.QueryRow(t.Context(), `SELECT remaining FROM rewards WHERE user_id=$1`, uid).Scan(&reward); err != nil {
		t.Fatal(err)
	}
	if reward != 1 {
		t.Fatalf("free reward remaining %d", reward)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM ledger WHERE user_id=$1`, uid).Scan(&ledgerN); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM paid_pools WHERE user_id=$1`, uid).Scan(&poolN); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_use WHERE user_id=$1`, uid).Scan(&quotaN); err != nil {
		t.Fatal(err)
	}
	if ledgerN != 0 || poolN != 0 || quotaN != 0 {
		t.Fatalf("IAP touched free/legacy pools ledger=%d paid_pools=%d quota=%d", ledgerN, poolN, quotaN)
	}
}

func postIAP(ts *httptest.Server, token, path string, body any) (int, map[string]any, string) {
	rawBody, _ := json.Marshal(body)
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
