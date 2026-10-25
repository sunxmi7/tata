package tata

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdminGrantDiamondsAndFreeCount(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})
	_, uid := registerUser(t, ts, store, "grant")
	var operatorID int64
	if err := store.QueryRow(t.Context(), `SELECT id FROM operators WHERE username='admin'`).Scan(&operatorID); err != nil {
		t.Fatal(err)
	}
	beforeReward := rewardRemaining(t, store, uid)

	msg, _ := call(t, ts, ro.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "diamonds": 5, "free_count": 1, "note": "只读不能写", "reason": "admin_grant",
	})
	if !strings.Contains(msg, "只读") {
		t.Fatalf("readonly grant: %s", msg)
	}
	view := listOne(t, ts, ro.access, fmt.Sprintf("/api/admin/v1/grants?user_id=%d", uid))
	if view["can_write"] != false || view["diamond_bucket_default"] != "pack" || view["writes_iap_orders"] != false {
		t.Fatalf("readonly grant list: %#v", view)
	}
	if msg, raw := call(t, ts, ro.access, http.MethodGet, "/api/admin/v1/ledger", nil); msg != "" {
		t.Fatalf("readonly ledger: %s %s", msg, raw)
	}
	if msg, raw := call(t, ts, ro.access, http.MethodGet, "/api/admin/v1/audit", nil); msg != "" {
		t.Fatalf("readonly audit: %s %s", msg, raw)
	}

	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "diamonds": 5, "free_count": 2,
	})
	if !strings.Contains(msg, "原因") {
		t.Fatalf("missing note: %s", msg)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "diamonds": 1, "note": "错类型", "reason": "iap_pack_grant",
	})
	if !strings.Contains(msg, "admin_grant") {
		t.Fatalf("reason enum: %s", msg)
	}

	granted := create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "diamonds": 5, "free_count": 2, "note": "到账失败补发", "reason": "admin_grant",
	})
	if granted["operator_id"].(float64) != float64(operatorID) || granted["resumes_paywall"] != false || granted["writes_iap_orders"] != false {
		t.Fatalf("grant meta: %#v", granted)
	}
	if granted["pack_diamonds"].(float64) != 5 || granted["subscription_diamonds"].(float64) != 0 || granted["free_count_remaining"].(float64) != 2 {
		t.Fatalf("balances: %#v", granted)
	}
	if granted["registration_reward_remaining"].(float64) != float64(beforeReward) {
		t.Fatalf("registration reward rewritten: %#v", granted)
	}
	items := granted["grants"].([]any)
	if len(items) != 2 {
		t.Fatalf("grant rows: %#v", items)
	}
	var diamondLedgerID, countLedgerID float64
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["reason"] != "admin_grant" || item["operator_id"].(float64) != float64(operatorID) || item["created_at"] == "" {
			t.Fatalf("grant item: %#v", item)
		}
		switch item["kind"] {
		case "diamonds":
			if item["bucket"] != "pack" || item["ledger_bucket"] != "consumable" || item["amount"].(float64) != 5 {
				t.Fatalf("diamond grant: %#v", item)
			}
			diamondLedgerID = item["diamond_ledger_id"].(float64)
		case "free_count":
			if item["amount"].(float64) != 2 || item["bucket"] != "" {
				t.Fatalf("free grant: %#v", item)
			}
			countLedgerID = item["count_ledger_id"].(float64)
		default:
			t.Fatalf("kind: %#v", item)
		}
	}
	var bucket, reason string
	var delta, after int
	var orderID sql.NullInt64
	var grantID int64
	if err := store.QueryRow(t.Context(), `SELECT bucket, delta, balance_after, reason, order_id, grant_id FROM diamond_ledger WHERE id=$1`, int64(diamondLedgerID)).
		Scan(&bucket, &delta, &after, &reason, &orderID, &grantID); err != nil {
		t.Fatal(err)
	}
	if bucket != "consumable" || delta != 5 || after != 5 || reason != "admin_grant" || orderID.Valid || grantID == 0 {
		t.Fatalf("diamond ledger bucket=%s delta=%d after=%d reason=%s order=%v grant=%d", bucket, delta, after, reason, orderID, grantID)
	}
	var ent, result string
	var readingID sql.NullInt64
	var countAmount int
	if err := store.QueryRow(t.Context(), `SELECT entitlement, amount, reason, result, reading_id, grant_id FROM ledger WHERE id=$1`, int64(countLedgerID)).
		Scan(&ent, &countAmount, &reason, &result, &readingID, &grantID); err != nil {
		t.Fatal(err)
	}
	if ent != "free_count" || countAmount != 2 || reason != "admin_grant" || result != "granted" || readingID.Valid || grantID == 0 {
		t.Fatalf("count ledger ent=%s amount=%d reason=%s result=%s reading=%v", ent, countAmount, reason, result, readingID)
	}

	subGrant := create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "diamonds": 3, "diamond_bucket": "subscription", "note": "补到订阅桶",
	})
	if subGrant["subscription_diamonds"].(float64) != 3 || subGrant["pack_diamonds"].(float64) != 5 {
		t.Fatalf("subscription add: %#v", subGrant)
	}
	subItem := subGrant["grants"].([]any)[0].(map[string]any)
	if subItem["kind"] != "diamonds" || subItem["bucket"] != "subscription" || subItem["ledger_bucket"] != "subscription" {
		t.Fatalf("subscription item: %#v", subItem)
	}

	listed := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/grants?user_id=%d", uid))
	if len(listed["grants"].([]any)) != 3 {
		t.Fatalf("listed grants: %#v", listed["grants"])
	}
	audits := list(t, ts, ro.access, "/api/admin/v1/audit")
	foundDiamond, foundFree := false, false
	for _, row := range audits {
		if row["action"] != "admin_grant" || row["operator_id"].(float64) != float64(operatorID) {
			continue
		}
		var detail map[string]any
		if err := json.Unmarshal([]byte(row["detail"].(string)), &detail); err != nil {
			t.Fatal(err)
		}
		if detail["reason"] != "admin_grant" || detail["user_id"].(float64) != float64(uid) || detail["iap_order_rewritten"] != false {
			continue
		}
		if detail["kind"] == "diamonds" && detail["diamond_ledger_id"] != nil {
			foundDiamond = true
		}
		if detail["kind"] == "free_count" && detail["count_ledger_id"] != nil {
			foundFree = true
		}
	}
	if !foundDiamond || !foundFree {
		t.Fatal("audit missing grant correlation")
	}
	ledgers := list(t, ts, admin.access, "/api/admin/v1/ledger")
	seenFree := false
	for _, row := range ledgers {
		if row["user_id"].(float64) == float64(uid) && row["reason"] == "admin_grant" && row["entitlement"] == "free_count" {
			if row["reading_id"] != nil || row["grant_id"] == nil {
				t.Fatalf("free ledger link: %#v", row)
			}
			seenFree = true
		}
	}
	if !seenFree {
		t.Fatal("free count ledger not listed")
	}
}

func TestAdminGrantLeavesIAPOrderAndPaywall(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 2)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	token, uid := registerUser(t, ts, store, "grant-iap")
	grant := 4
	product := fmt.Sprintf("test.grant.month.%d", time.Now().UnixNano())
	insertIAPSKU(t, store, "month_subscription", "subscription", "app_store", product, &grant)
	txn := fmt.Sprintf("txn-grant-%d", time.Now().UnixNano())
	expires := time.Now().Add(30 * 24 * time.Hour).UTC()
	out := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, "orig-"+txn, "receipt-"+txn, &expires))
	orderID := int64(out["order_id"].(float64))
	beforeOrder := orderFingerprint(t, store, orderID)
	beforeLedger := orderLedgerFingerprint(t, store, orderID)
	var beforeEnd sql.NullTime
	var beforeSub int
	if err := store.QueryRow(t.Context(), `SELECT subscription_diamonds, subscription_period_end FROM diamond_balances WHERE user_id=$1`, uid).Scan(&beforeSub, &beforeEnd); err != nil {
		t.Fatal(err)
	}
	var subStatus string
	var subExp time.Time
	if err := store.QueryRow(t.Context(), `SELECT status, expires_at FROM subscriptions WHERE user_id=$1 AND last_order_id=$2`, uid, orderID).Scan(&subStatus, &subExp); err != nil {
		t.Fatal(err)
	}

	created := create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "diamonds": 7, "note": "次卡桶独立加账",
	})
	if created["pack_diamonds"].(float64) != 7 || created["subscription_diamonds"].(float64) != float64(beforeSub) {
		t.Fatalf("pack add touched subscription balance: %#v", created)
	}
	subAdded := create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "diamonds": 2, "diamond_bucket": "subscription", "note": "订阅桶加账不覆盖周期",
	})
	if subAdded["subscription_diamonds"].(float64) != float64(beforeSub+2) || subAdded["pack_diamonds"].(float64) != 7 {
		t.Fatalf("subscription grant replaced the bucket: %#v", subAdded)
	}
	if tasks, ok := subAdded["resumed_tasks"].([]any); !ok || len(tasks) != 0 {
		t.Fatalf("grant resumed tasks: %#v", subAdded["resumed_tasks"])
	}
	if orderFingerprint(t, store, orderID) != beforeOrder {
		t.Fatalf("iap order changed\nbefore %s\nafter  %s", beforeOrder, orderFingerprint(t, store, orderID))
	}
	if orderLedgerFingerprint(t, store, orderID) != beforeLedger {
		t.Fatalf("order diamond ledger rewritten\nbefore %s\nafter  %s", beforeLedger, orderLedgerFingerprint(t, store, orderID))
	}
	var afterEnd sql.NullTime
	if err := store.QueryRow(t.Context(), `SELECT subscription_period_end FROM diamond_balances WHERE user_id=$1`, uid).Scan(&afterEnd); err != nil {
		t.Fatal(err)
	}
	if beforeEnd.Valid != afterEnd.Valid || (beforeEnd.Valid && !beforeEnd.Time.Equal(afterEnd.Time)) {
		t.Fatalf("period end changed: before %v after %v", beforeEnd, afterEnd)
	}
	var afterStatus string
	var afterExp time.Time
	if err := store.QueryRow(t.Context(), `SELECT status, expires_at FROM subscriptions WHERE user_id=$1 AND last_order_id=$2`, uid, orderID).Scan(&afterStatus, &afterExp); err != nil {
		t.Fatal(err)
	}
	if afterStatus != subStatus || !afterExp.Equal(subExp) {
		t.Fatalf("subscription row changed: %s %s", afterStatus, afterExp)
	}

	exhaustFree(t, ts, token)
	setDiamonds(t, store, uid, 0, 0)
	blocked := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "补发不应续跑", "type": "normal"})
	if blocked["paywall"] != true || blocked["generated"] != false {
		t.Fatalf("expected paywall: %#v", blocked)
	}
	readingID := asID(blocked["id"])
	var readingStatus, payStatus string
	if err := store.QueryRow(t.Context(), `SELECT status FROM readings WHERE id=$1`, readingID).Scan(&readingStatus); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT status FROM quota_paywall WHERE reading_id=$1 AND charge_key='play'`, readingID).Scan(&payStatus); err != nil {
		t.Fatal(err)
	}
	if payStatus != paywallPending {
		t.Fatalf("paywall status %s", payStatus)
	}
	held := create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "diamonds": 9, "free_count": 1, "note": "补发不续跑被打断的任务",
	})
	if tasks, ok := held["resumed_tasks"].([]any); !ok || len(tasks) != 0 {
		t.Fatalf("grant resumed the paywall: %#v", held["resumed_tasks"])
	}
	if held["pack_diamonds"].(float64) != 9 || held["free_count_remaining"].(float64) != 1 {
		t.Fatalf("held grant balances: %#v", held)
	}
	var stillReading, stillPay string
	if err := store.QueryRow(t.Context(), `SELECT status FROM readings WHERE id=$1`, readingID).Scan(&stillReading); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT status FROM quota_paywall WHERE reading_id=$1 AND charge_key='play'`, readingID).Scan(&stillPay); err != nil {
		t.Fatal(err)
	}
	if stillReading != readingStatus || stillPay != payStatus {
		t.Fatalf("paywall moved: reading %s->%s pay %s->%s", readingStatus, stillReading, payStatus, stillPay)
	}
	if n := chargeCount(t, store, readingID); n != 0 {
		t.Fatalf("grant charged the held reading: %d", n)
	}
	if orderFingerprint(t, store, orderID) != beforeOrder {
		t.Fatalf("paywall grant rewrote iap order\nbefore %s\nafter  %s", beforeOrder, orderFingerprint(t, store, orderID))
	}
	var orderRows int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE order_id=$1`, orderID).Scan(&orderRows); err != nil {
		t.Fatal(err)
	}
	if orderRows != 1 {
		t.Fatalf("order ledger rows %d", orderRows)
	}
}

func TestAdminGrantFreeCountSpendsBeforeDiamonds(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 3)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	token, uid := registerUser(t, ts, store, "grant-spend")
	exhaustFree(t, ts, token)
	setDiamonds(t, store, uid, 4, 6)
	create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "free_count": 1, "note": "补一次免费",
	})
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "先用补发次数", "type": "normal"})
	if reading["entitlement"] != entAdminFree || reading["paywall"] == true {
		t.Fatalf("spend grant: %#v", reading)
	}
	sub, pack := diamondBalances(t, store, uid)
	if sub != 4 || pack != 6 {
		t.Fatalf("diamonds spent instead of the grant: sub=%d pack=%d", sub, pack)
	}
	var left int
	if err := store.QueryRow(t.Context(), `SELECT admin_grant_remaining FROM rewards WHERE user_id=$1`, uid).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("grant remaining %d", left)
	}
	if rewardRemaining(t, store, uid) != 0 {
		t.Fatal("grant restored registration reward")
	}
}

func TestMergeFoldsDiamondBalances(t *testing.T) {
	store, ts := newMergeServer(t)
	stamp := time.Now().UnixNano()
	emailA := fmt.Sprintf("diamond-merge-a-%d@tata.local", stamp)
	emailB := fmt.Sprintf("diamond-merge-b-%d@tata.local", stamp)
	tokenA := register(t, ts, emailA)
	_ = register(t, ts, emailB)
	idA := userIDByEmail(t, store, emailA)
	idB := userIDByEmail(t, store, emailB)
	openID := fmt.Sprintf("wx-diamond-%d", stamp)
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET wechat_openid=$1 WHERE id=$2`, openID, idB); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(40 * 24 * time.Hour).UTC().Truncate(time.Microsecond)
	sooner := time.Now().Add(10 * 24 * time.Hour).UTC().Truncate(time.Microsecond)
	if _, err := store.Exec(t.Context(), `INSERT INTO diamond_balances(user_id, subscription_diamonds, pack_diamonds, subscription_period_end, subscription_period_key)
		VALUES ($1,4,3,$2,'primary-period'),($3,2,5,$4,'secondary-period')`, idA, later, idB, sooner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO diamond_ledger(user_id, bucket, delta, balance_after, reason) VALUES
		($1,'subscription',4,4,'iap_period_refresh'),($2,'consumable',5,5,'iap_pack_grant')`, idA, idB); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `UPDATE rewards SET admin_grant_remaining=2 WHERE user_id=$1`, idA); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `UPDATE rewards SET admin_grant_remaining=3 WHERE user_id=$1`, idB); err != nil {
		t.Fatal(err)
	}
	var orderID int64
	if err := store.QueryRow(t.Context(), `INSERT INTO iap_orders(user_id, platform, store_transaction_id, verify_status, credit_status, source, credited_diamonds, ledger_bucket)
		VALUES ($1,'app_store',$2,'verified','credited','verify',5,'consumable') RETURNING id`, idB, fmt.Sprintf("merge-order-%d", stamp)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `UPDATE diamond_ledger SET order_id=$1 WHERE user_id=$2`, orderID, idB); err != nil {
		t.Fatal(err)
	}
	beforeOrder := orderFingerprint(t, store, orderID)

	msg, raw := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"openid": openID})
	conflict := decodeConflict(t, msg, raw)
	merged := create(t, ts, tokenA, "/api/app/v1/auth/merge-confirm", map[string]any{"merge_request_id": conflict.MergeRequestID, "confirm": true})
	if merged["merged"] != true || merged["reward_remaining"].(float64) != 2 {
		t.Fatalf("merge: %#v", merged)
	}
	var sub, pack int
	var end time.Time
	var key string
	if err := store.QueryRow(t.Context(), `SELECT subscription_diamonds, pack_diamonds, subscription_period_end, subscription_period_key FROM diamond_balances WHERE user_id=$1`, idA).
		Scan(&sub, &pack, &end, &key); err != nil {
		t.Fatal(err)
	}
	if sub != 6 || pack != 8 || !end.Equal(later) || key != "primary-period" {
		t.Fatalf("folded diamonds sub=%d pack=%d end=%s key=%s", sub, pack, end, key)
	}
	var secondaryBal int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_balances WHERE user_id=$1`, idB).Scan(&secondaryBal); err != nil {
		t.Fatal(err)
	}
	if secondaryBal != 0 {
		t.Fatalf("secondary balance remains: %d", secondaryBal)
	}
	var ledgerOnPrimary int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE user_id=$1`, idA).Scan(&ledgerOnPrimary); err != nil {
		t.Fatal(err)
	}
	if ledgerOnPrimary != 2 {
		t.Fatalf("diamond ledger rows on primary: %d", ledgerOnPrimary)
	}
	var linkedOrder int64
	var linkedReason string
	if err := store.QueryRow(t.Context(), `SELECT order_id, reason FROM diamond_ledger WHERE user_id=$1 AND bucket='consumable'`, idA).Scan(&linkedOrder, &linkedReason); err != nil {
		t.Fatal(err)
	}
	if linkedOrder != orderID || linkedReason != "iap_pack_grant" {
		t.Fatalf("ledger order link changed: order=%d reason=%s", linkedOrder, linkedReason)
	}
	if orderFingerprint(t, store, orderID) != beforeOrder {
		t.Fatalf("merge rewrote iap order\nbefore %s\nafter  %s", beforeOrder, orderFingerprint(t, store, orderID))
	}
	var adminLeft int
	if err := store.QueryRow(t.Context(), `SELECT admin_grant_remaining FROM rewards WHERE user_id=$1`, idA).Scan(&adminLeft); err != nil {
		t.Fatal(err)
	}
	if adminLeft != 5 {
		t.Fatalf("admin free grants not folded: %d", adminLeft)
	}
	var secondaryReward int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM rewards WHERE user_id=$1`, idB).Scan(&secondaryReward); err != nil {
		t.Fatal(err)
	}
	if secondaryReward != 0 {
		t.Fatal("secondary rewards remain")
	}
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": idB, "diamonds": 1, "note": "不能补给被合并账号",
	})
	if !strings.Contains(msg, "主账号") {
		t.Fatalf("grant to merged user: %s", msg)
	}
}

func orderFingerprint(t *testing.T, store *Store, orderID int64) string {
	t.Helper()
	var snap string
	err := store.QueryRow(t.Context(), `SELECT concat_ws('|',
		verify_status, credit_status, COALESCE(credited_diamonds::text,''), store_transaction_id,
		ledger_bucket, COALESCE(subscription_id::text,''), COALESCE(period_end::text,''),
		COALESCE(sku_code,''), source, updated_at::text, user_id::text)
		FROM iap_orders WHERE id=$1`, orderID).Scan(&snap)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func orderLedgerFingerprint(t *testing.T, store *Store, orderID int64) string {
	t.Helper()
	var snap string
	err := store.QueryRow(t.Context(), `SELECT concat_ws('|', id::text, bucket, delta::text, balance_after::text, reason, COALESCE(store_transaction_id,''), user_id::text)
		FROM diamond_ledger WHERE order_id=$1 ORDER BY id`, orderID).Scan(&snap)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}
