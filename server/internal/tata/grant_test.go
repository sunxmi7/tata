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
	_ = rewardRemaining(t, store, uid)

	msg, _ := call(t, ts, ro.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "quota": 5, "free_count": 1, "note": "只读不能写", "reason": "admin_grant",
	})
	if !strings.Contains(msg, "只读") {
		t.Fatalf("readonly grant: %s", msg)
	}
	view := listOne(t, ts, ro.access, fmt.Sprintf("/api/admin/v1/grants?user_id=%d", uid))
	if view["can_write"] != false || view["quota_bucket_default"] != "pack" || view["writes_iap_orders"] != false {
		t.Fatalf("readonly grant list: %#v", view)
	}
	if msg, raw := call(t, ts, ro.access, http.MethodGet, "/api/admin/v1/ledger", nil); msg != "" {
		t.Fatalf("readonly ledger: %s %s", msg, raw)
	}
	if msg, raw := call(t, ts, ro.access, http.MethodGet, "/api/admin/v1/audit", nil); msg != "" {
		t.Fatalf("readonly audit: %s %s", msg, raw)
	}

	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "quota": 5, "quota_bucket": "pack",
	})
	if !strings.Contains(msg, "原因") {
		t.Fatalf("missing note: %s", msg)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "quota": 1, "quota_bucket": "pack", "note": "错类型", "reason": "iap_pack_grant",
	})
	if !strings.Contains(msg, "admin_grant") {
		t.Fatalf("reason enum: %s", msg)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "quota": 1, "free_count": 2, "quota_bucket": "pack", "note": "不能补免费次数",
	})
	if !strings.Contains(msg, "只补会员额度") {
		t.Fatalf("free count grant: %s", msg)
	}

	granted := create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "quota": 5, "quota_bucket": "pack", "note": "到账失败补发", "reason": "admin_grant",
	})
	if granted["operator_id"].(float64) != float64(operatorID) || granted["resumes_paywall"] != false || granted["writes_iap_orders"] != false {
		t.Fatalf("grant meta: %#v", granted)
	}
	if granted["quota_remaining"].(float64) != 5 || granted["free_count_remaining"].(float64) != 0 {
		t.Fatalf("balances: %#v", granted)
	}
	items := granted["grants"].([]any)
	if len(items) != 1 {
		t.Fatalf("grant rows: %#v", items)
	}
	var diamondLedgerID float64
	item := items[0].(map[string]any)
	if item["reason"] != "admin_grant" || item["operator_id"].(float64) != float64(operatorID) || item["created_at"] == "" {
		t.Fatalf("grant item: %#v", item)
	}
	if item["kind"] != "quota" || item["bucket"] != "pack" || item["ledger_bucket"] != iapBucketQuota || item["amount"].(float64) != 5 {
		t.Fatalf("quota grant: %#v", item)
	}
	diamondLedgerID = item["quota_ledger_id"].(float64)
	var lotSource string
	if err := store.QueryRow(t.Context(), `SELECT source FROM quota_lots WHERE user_id=$1 AND reason='admin_grant'`, uid).Scan(&lotSource); err != nil {
		t.Fatal(err)
	}
	if lotSource != "pack" {
		t.Fatalf("lot source %s", lotSource)
	}
	var bucket, reason string
	var delta, after int
	var orderID sql.NullInt64
	var grantID int64
	if err := store.QueryRow(t.Context(), `SELECT bucket, delta, balance_after, reason, order_id, grant_id FROM quota_ledger WHERE id=$1`, int64(diamondLedgerID)).
		Scan(&bucket, &delta, &after, &reason, &orderID, &grantID); err != nil {
		t.Fatal(err)
	}
	if bucket != iapBucketQuota || delta != 5 || after != 5 || reason != "admin_grant" || orderID.Valid || grantID == 0 {
		t.Fatalf("quota ledger bucket=%s delta=%d after=%d reason=%s order=%v grant=%d", bucket, delta, after, reason, orderID, grantID)
	}
	subGrant := create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "quota": 3, "quota_bucket": "subscription", "note": "补到订阅桶",
	})
	if subGrant["quota_remaining"].(float64) != 8 {
		t.Fatalf("quota add: %#v", subGrant)
	}
	subItem := subGrant["grants"].([]any)[0].(map[string]any)
	if subItem["kind"] != "quota" || subItem["bucket"] != "subscription" || subItem["ledger_bucket"] != iapBucketQuota {
		t.Fatalf("quota item: %#v", subItem)
	}
	if err := store.QueryRow(t.Context(), `SELECT source FROM quota_lots WHERE user_id=$1 AND source='subscription'`, uid).Scan(&lotSource); err != nil {
		t.Fatal(err)
	}
	if lotSource != "subscription" {
		t.Fatalf("subscription lot %s", lotSource)
	}

	listed := listOne(t, ts, admin.access, fmt.Sprintf("/api/admin/v1/grants?user_id=%d", uid))
	if len(listed["grants"].([]any)) != 2 {
		t.Fatalf("listed grants: %#v", listed["grants"])
	}
	audits := list(t, ts, ro.access, "/api/admin/v1/audit")
	foundDiamond := false
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
		if detail["kind"] == "quota" && detail["quota_ledger_id"] != nil {
			foundDiamond = true
		}
		if detail["kind"] == "free_count" {
			t.Fatalf("free count audit: %#v", detail)
		}
	}
	if !foundDiamond {
		t.Fatal("audit missing grant correlation")
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
	var beforeQuota int
	if err := store.QueryRow(t.Context(), `SELECT quota_balance FROM quota_balances WHERE user_id=$1`, uid).Scan(&beforeQuota); err != nil {
		t.Fatal(err)
	}
	var subStatus string
	var subExp time.Time
	if err := store.QueryRow(t.Context(), `SELECT status, expires_at FROM subscriptions WHERE user_id=$1 AND last_order_id=$2`, uid, orderID).Scan(&subStatus, &subExp); err != nil {
		t.Fatal(err)
	}

	created := create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "quota": 7, "quota_bucket": "pack", "note": "次卡桶独立加账",
	})
	if created["quota_remaining"].(float64) != float64(beforeQuota+7) {
		t.Fatalf("quota add: %#v", created)
	}
	subAdded := create(t, ts, admin.access, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "quota": 2, "quota_bucket": "subscription", "note": "统一额度加账",
	})
	if subAdded["quota_remaining"].(float64) != float64(beforeQuota+9) {
		t.Fatalf("quota grant: %#v", subAdded)
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
	var afterStatus string
	var afterExp time.Time
	if err := store.QueryRow(t.Context(), `SELECT status, expires_at FROM subscriptions WHERE user_id=$1 AND last_order_id=$2`, uid, orderID).Scan(&afterStatus, &afterExp); err != nil {
		t.Fatal(err)
	}
	if afterStatus != subStatus || !afterExp.Equal(subExp) {
		t.Fatalf("subscription row changed: %s %s", afterStatus, afterExp)
	}

	exhaustFree(t, ts, token)
	setDiamonds(t, store, uid, 1, 0)
	blocked := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "补发不应续跑", "type": "normal"})
	spreadID := ensureTimeFlow(t, store)
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(blocked["id"])), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	if !strings.Contains(msg, "额度不足") || blocked["generated"] != false {
		t.Fatalf("expected paywall: %s %#v", msg, blocked)
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
		"user_id": uid, "quota": 9, "quota_bucket": "pack", "note": "补发不续跑被打断的任务",
	})
	if tasks, ok := held["resumed_tasks"].([]any); !ok || len(tasks) != 0 {
		t.Fatalf("grant resumed the paywall: %#v", held["resumed_tasks"])
	}
	if held["quota_remaining"].(float64) != 10 || held["free_count_remaining"].(float64) != 0 {
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
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE order_id=$1`, orderID).Scan(&orderRows); err != nil {
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
	exhaustFreePlay(t, ts, store, token)
	setQuota(t, store, uid, 10)
	msg, _ := call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": uid, "free_count": 1, "note": "补一次免费",
	})
	if !strings.Contains(msg, "只补会员额度") {
		t.Fatalf("free count grant: %s", msg)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO rewards(user_id, granted_at, expires_at, remaining, admin_grant_remaining)
		VALUES ($1, now(), now(), 0, 1)
		ON CONFLICT (user_id) DO UPDATE SET admin_grant_remaining=1`, uid); err != nil {
		t.Fatal(err)
	}
	rid := chargeFreePlay(t, ts, store, token, "先用补发次数")
	if playEntitlement(t, store, rid) != entAdminFree {
		t.Fatalf("spend grant: %s", playEntitlement(t, store, rid))
	}
	if quotaOf(t, store, uid) != 10 {
		t.Fatalf("quota spent instead of the grant: %d", quotaOf(t, store, uid))
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
	if _, err := store.Exec(t.Context(), `INSERT INTO quota_balances(user_id, quota_balance)
		VALUES ($1,7),($2,7)
		ON CONFLICT (user_id) DO UPDATE SET quota_balance=EXCLUDED.quota_balance, updated_at=now()`, idA, idB); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO quota_ledger(user_id, bucket, delta, balance_after, reason) VALUES
		($1,'quota',4,4,'iap_period_refresh'),($2,'quota',5,5,'iap_pack_grant')`, idA, idB); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO rewards(user_id, granted_at, expires_at, remaining, admin_grant_remaining)
		VALUES ($1, now(), now(), 0, 2)
		ON CONFLICT (user_id) DO UPDATE SET admin_grant_remaining=2`, idA); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO rewards(user_id, granted_at, expires_at, remaining, admin_grant_remaining)
		VALUES ($1, now(), now(), 0, 3)
		ON CONFLICT (user_id) DO UPDATE SET admin_grant_remaining=3`, idB); err != nil {
		t.Fatal(err)
	}
	var orderID int64
	if err := store.QueryRow(t.Context(), `INSERT INTO iap_orders(user_id, platform, store_transaction_id, verify_status, credit_status, source, credited_quota, sku_kind)
		VALUES ($1,'app_store',$2,'verified','credited','verify',5,'quota_pack') RETURNING id`, idB, fmt.Sprintf("merge-order-%d", stamp)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `UPDATE quota_ledger SET order_id=$1 WHERE user_id=$2`, orderID, idB); err != nil {
		t.Fatal(err)
	}
	beforeOrder := orderFingerprint(t, store, orderID)

	msg, raw := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"code": openID})
	if !strings.Contains(msg, "已绑定其他账号") {
		t.Fatalf("bind should not merge: %s %s", msg, raw)
	}
	var quota int
	if err := store.QueryRow(t.Context(), `SELECT quota_balance FROM quota_balances WHERE user_id=$1`, idA).Scan(&quota); err != nil {
		t.Fatal(err)
	}
	if quota != 7 {
		t.Fatalf("primary quota moved: %d", quota)
	}
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET merged_into=$1, merged_at=now() WHERE id=$2`, idA, idB); err != nil {
		t.Fatal(err)
	}
	var secondaryBal int
	if err := store.QueryRow(t.Context(), `SELECT quota_balance FROM quota_balances WHERE user_id=$1`, idB).Scan(&secondaryBal); err != nil {
		t.Fatal(err)
	}
	if secondaryBal != 7 {
		t.Fatalf("secondary quota moved: %d", secondaryBal)
	}
	if orderFingerprint(t, store, orderID) != beforeOrder {
		t.Fatalf("bind rewrote iap order\nbefore %s\nafter  %s", beforeOrder, orderFingerprint(t, store, orderID))
	}
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/grants", map[string]any{
		"user_id": idB, "quota": 1, "quota_bucket": "pack", "note": "不能补给被合并账号",
	})
	if !strings.Contains(msg, "主账号") {
		t.Fatalf("grant to merged user: %s", msg)
	}
}

func orderFingerprint(t *testing.T, store *Store, orderID int64) string {
	t.Helper()
	var snap string
	err := store.QueryRow(t.Context(), `SELECT concat_ws('|',
		verify_status, credit_status, COALESCE(credited_quota::text,''), store_transaction_id,
		sku_kind, COALESCE(subscription_id::text,''), COALESCE(period_end::text,''),
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
		FROM quota_ledger WHERE order_id=$1 ORDER BY id`, orderID).Scan(&snap)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}
