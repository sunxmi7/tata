package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSharedQuotaPriority(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET registration_grant_delay_minutes=NULL, registration_spread_ids='[]' WHERE id=1`)
	})
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET registration_grant_delay_minutes=NULL, registration_spread_ids='[]' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	token, uid := registerUser(t, ts, store, "priority")
	allowGiftSpread(t, store)
	// This case starts from invite only; clear any pending registration gift row.
	if _, err := store.Exec(t.Context(), `DELETE FROM registration_gifts WHERE user_id=$1`, uid); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Exec(t.Context(), `INSERT INTO rewards(user_id, granted_at, expires_at, invite_remaining) VALUES ($1, now(), now(), 3)
		ON CONFLICT (user_id) DO UPDATE SET invite_remaining=3`, uid); err != nil {
		t.Fatal(err)
	}
	firstID := chargeFreePlay(t, ts, store, token, "先扣邀请")
	if playEntitlement(t, store, firstID) != entInvite {
		t.Fatalf("invite first %s", playEntitlement(t, store, firstID))
	}
	secondID := chargeFreePlay(t, ts, store, token, "再扣邀请次数")
	if playEntitlement(t, store, secondID) != entInvite {
		t.Fatalf("invite second %s", playEntitlement(t, store, secondID))
	}
	if _, err := store.Exec(t.Context(), `UPDATE rewards SET invite_remaining=0 WHERE user_id=$1`, uid); err != nil {
		t.Fatal(err)
	}

	third := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "免费用完", "type": "normal"})
	if third["quota_outcome"] != quotaDeferred {
		t.Fatalf("deferred create: %#v", third)
	}
	spreadID := ensureTimeFlow(t, store)
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(third["id"])), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/interpret", asID(third["id"])), map[string]any{})
	if !strings.Contains(msg, "开通会员") {
		t.Fatalf("free paywall: %s", msg)
	}

	setMember(t, store, uid, "month")
	setDiamondCost(t, store, 3)
	setQuota(t, store, uid, 10)
	memberPlay := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "会员扣额度", "type": "normal"})
	if memberPlay["quota_outcome"] != quotaDeferred {
		t.Fatalf("member create defers: %#v", memberPlay)
	}
	memberID := asID(memberPlay["id"])
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", memberID), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	if playEntitlement(t, store, memberID) != entQuota {
		t.Fatalf("member quota %s", playEntitlement(t, store, memberID))
	}
	if got := quotaOf(t, store, uid); got != 7 {
		t.Fatalf("quota after spend %d", got)
	}
	setQuota(t, store, uid, 2)
	poor := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "额度不足", "type": "normal"})
	msg, _ = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(poor["id"])), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	if !strings.Contains(msg, "额度不足") || strings.Contains(msg, "算力") {
		t.Fatalf("paywall prompt: %s", msg)
	}
	if got := quotaOf(t, store, uid); got != 2 {
		t.Fatalf("paywall spent quota %d", got)
	}
}

func TestDiamondCostUnconfigured(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, uid := registerUser(t, ts, store, "unconfig")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	setMember(t, store, uid, "month")
	setQuota(t, store, uid, 9)
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "未配置不能猜额度", "type": "normal"})
	spreadID := ensureTimeFlow(t, store)
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(reading["id"])), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	if !strings.Contains(msg, "未配置") || strings.Contains(msg, "算力") {
		t.Fatalf("prompt: %s", msg)
	}
	if got := quotaOf(t, store, uid); got != 9 {
		t.Fatalf("unconfigured debited %d", got)
	}
	if n := chargeCount(t, store, asID(reading["id"])); n != 0 {
		t.Fatalf("unconfigured charge %d", n)
	}
}

func TestPaywallResumeAfterCredit(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 2)
	token, uid := registerUser(t, ts, store, "resume")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	setMember(t, store, uid, "month")
	setQuota(t, store, uid, 1)
	spreadID := ensureTimeFlow(t, store)

	cancelled := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "取消的任务", "type": "normal"})
	cancelID := asID(cancelled["id"])
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", cancelID), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	if !strings.Contains(msg, "额度不足") {
		t.Fatalf("cancel setup: %s", msg)
	}
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/paywall", cancelID), map[string]any{"action": "cancel"})
	var payStatus string
	if err := store.QueryRow(t.Context(), `SELECT status FROM quota_paywall WHERE reading_id=$1 AND charge_key='play'`, cancelID).Scan(&payStatus); err != nil {
		t.Fatal(err)
	}
	if payStatus != paywallCancel {
		t.Fatalf("cancel status %s", payStatus)
	}

	kept := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "待续跑", "type": "normal"})
	keptID := asID(kept["id"])
	msg, _ = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", keptID), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	if !strings.Contains(msg, "额度不足") || strings.Contains(msg, "算力") {
		t.Fatalf("spread paywall: %s", msg)
	}
	var status string
	if err := store.QueryRow(t.Context(), `SELECT status FROM readings WHERE id=$1`, keptID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "SPREAD_SELECTED" {
		t.Fatalf("shuffle advanced without quota: %s", status)
	}

	grant := 4
	product := fmt.Sprintf("test.pack.resume.%d", time.Now().UnixNano())
	insertIAPSKUWithReadings(t, store, product, grant)
	txn := fmt.Sprintf("txn-resume-%d", time.Now().UnixNano())
	out := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, txn, "receipt-"+txn, nil))
	tasks, _ := out["resumed_tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("resume hook: %#v", out["resumed_tasks"])
	}
	resumed := tasks[0].(map[string]any)
	if asID(resumed["reading_id"]) != keptID || resumed["resume_action"] != resumeShuffle || resumed["entitlement"] != entQuota {
		t.Fatalf("resumed task: %#v", resumed)
	}
	if err := store.QueryRow(t.Context(), `SELECT status FROM readings WHERE id=$1`, keptID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "SHUFFLING" {
		t.Fatalf("paid task not continued: %s", status)
	}
	if err := store.QueryRow(t.Context(), `SELECT status FROM readings WHERE id=$1`, cancelID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status == "SHUFFLING" || status == "COMPLETED" {
		t.Fatalf("cancelled task continued: %s", status)
	}
	if n := chargeCount(t, store, cancelID); n != 0 {
		t.Fatalf("cancelled reading charged: %d", n)
	}
	if got := quotaOf(t, store, uid); got != 2 {
		t.Fatalf("resume spend quota %d", got)
	}

	again := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, txn, "receipt-"+txn, nil))
	if again["idempotent"] != true {
		t.Fatalf("verify replay: %#v", again)
	}
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/use-reward", keptID), map[string]any{})
	if got := quotaOf(t, store, uid); got != 2 {
		t.Fatalf("replay recharged %d", got)
	}
	if _, err := store.Exec(t.Context(), `UPDATE readings SET status='CARDS_FIXED', cards=$2 WHERE id=$1`, keptID, `[{"name":"愚者"}]`); err != nil {
		t.Fatal(err)
	}
	failMsg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/interpret", keptID), map[string]any{})
	if failMsg != "" && !strings.Contains(failMsg, "未绑定") && !strings.Contains(failMsg, "未重新扣") {
		t.Fatalf("failed interpret: %s", failMsg)
	}
	failMsg, _ = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/interpret", keptID), map[string]any{})
	if strings.Contains(failMsg, "算力") {
		t.Fatalf("retry exposed 算力: %s", failMsg)
	}
	if n := chargeCount(t, store, keptID); n != 1 {
		t.Fatalf("retry recharged: %d", n)
	}
	var spends int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE user_id=$1 AND reason='quota_spend'`, uid).Scan(&spends); err != nil {
		t.Fatal(err)
	}
	if spends != 1 {
		t.Fatalf("diamond spends %d", spends)
	}
	if got := quotaOf(t, store, uid); got != 2 {
		t.Fatalf("failure refunded or retried %d", got)
	}
}

func TestNumerologyDrawCanUseGift(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET registration_spread_ids='[]' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	token, uid := registerUser(t, ts, store, "numgift")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	seedGift(t, store, uid, 1)
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "灵数也能用赠送", "type": "normal"})
	id := asID(reading["id"])
	spreadID := ensureTimeFlow(t, store)
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", id), map[string]any{"spread_id": spreadID, "draw_mode": "numerology"})
	if n := chargeCount(t, store, id); n != 1 {
		t.Fatalf("numerology spread charge %d", n)
	}
	if _, err := store.Exec(t.Context(), `UPDATE readings SET status='CARDS_FIXED', cards='[{"name":"愚者"}]' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, _ = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/interpret", id), map[string]any{})
	if n := chargeCount(t, store, id); n != 1 {
		t.Fatalf("numerology charged again %d", n)
	}
	if playEntitlement(t, store, id) != entReward {
		t.Fatalf("numerology entitlement %s", playEntitlement(t, store, id))
	}
	var left int
	if err := store.QueryRow(t.Context(), `SELECT remaining FROM registration_gifts WHERE user_id=$1`, uid).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("gift remaining %d", left)
	}
}

func TestFollowupReentersSharedQuota(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 2)
	token, uid := registerUser(t, ts, store, "follow")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	setMember(t, store, uid, "month")
	setQuota(t, store, uid, 20)
	if _, err := store.Exec(t.Context(), `DELETE FROM bindings WHERE task='tarotist'`); err != nil {
		t.Fatal(err)
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "原问题", "type": "normal"})
	id := asID(reading["id"])
	completeReading(t, store, id)
	msg, raw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "延续一下", "continuation": true})
	if msg != "" {
		t.Fatalf("followup: %s %s", msg, raw)
	}
	body := decodeData(t, raw)
	if body["charged"] == true || body["entitlement"] == entQuota {
		t.Fatalf("followup spent quota: %#v", body)
	}
	if got := quotaOf(t, store, uid); got != 20 {
		t.Fatalf("followup changed quota %d", got)
	}
	if n := followupCharges(t, store, id); n != 0 {
		t.Fatalf("followup charges %d", n)
	}
	msg, raw = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "失败后重试", "continuation": true})
	if msg != "" {
		t.Fatalf("retry followup: %s %s", msg, raw)
	}
	if n := followupCharges(t, store, id); n != 0 {
		t.Fatalf("retry recharged followup: %d", n)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO reading_messages(reading_id, turn_id, seq, role, content)
		SELECT $1, turn_id, 1, 'assistant', '已答' FROM reading_messages WHERE reading_id=$1 AND role='user' ORDER BY turn_id DESC LIMIT 1`, id); err != nil {
		t.Fatal(err)
	}
	msg, raw = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "再追问", "continuation": true})
	if msg != "" {
		t.Fatalf("second followup: %s %s", msg, raw)
	}
	body = decodeData(t, raw)
	if body["charged"] == true || quotaOf(t, store, uid) != 20 {
		t.Fatalf("second followup spent quota: %#v balance=%d", body, quotaOf(t, store, uid))
	}
	if n := followupCharges(t, store, id); n != 0 {
		t.Fatalf("followup charges %d", n)
	}
	msg, raw = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "这是新问题", "continuation": false})
	if msg != "" {
		t.Fatalf("new question: %s %s", msg, raw)
	}
	body = decodeData(t, raw)
	if body["new_reading"] != true {
		t.Fatalf("new reading flag: %#v", body)
	}
	created, _ := body["reading"].(map[string]any)
	if created == nil || asID(created["id"]) == id {
		t.Fatalf("new question reused the record: %#v", body)
	}
	if n := followupCharges(t, store, id); n != 0 {
		t.Fatalf("new question charged the old thread: %d", n)
	}
}

func TestFollowupLimitConfigurable(t *testing.T) {
	store, ts := newMergeServer(t)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET followup_limit=5 WHERE id=1`)
	})
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	params := listOne(t, ts, admin.access, "/api/admin/v1/quota-params")
	if params["followup_limit"].(float64) != 5 || params["followup_limit_editable"] != true {
		t.Fatalf("seed followup limit: %#v", params)
	}
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{"followup_limit": 1})
	token, uid := registerUser(t, ts, store, "followcap")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	setMember(t, store, uid, "month")
	setDiamondCost(t, store, 1)
	setQuota(t, store, uid, 20)
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "限次追问", "type": "normal"})
	id := asID(reading["id"])
	completeReading(t, store, id)
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "第一次", "continuation": true})
	if msg != "" {
		t.Fatalf("first followup: %s", msg)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO reading_messages(reading_id, turn_id, seq, role, content)
		SELECT $1, turn_id, 1, 'assistant', '已答' FROM reading_messages WHERE reading_id=$1 AND role='user' ORDER BY turn_id DESC LIMIT 1`, id); err != nil {
		t.Fatal(err)
	}
	msg, _ = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "第二次", "continuation": true})
	if !strings.Contains(msg, "上限") {
		t.Fatalf("limit: %s", msg)
	}
	quota := listOne(t, ts, token, "/api/app/v1/quota")
	if quota["followup_limit"].(float64) != 1 {
		t.Fatalf("app quota limit: %#v", quota)
	}
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{"followup_limit": nil})
	params = listOne(t, ts, admin.access, "/api/admin/v1/quota-params")
	if params["followup_limit"] != nil {
		t.Fatalf("cleared limit: %#v", params)
	}
}

func TestAITrialThenFormalQuota(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, uid := registerUser(t, ts, store, "trial")
	tarotistID := firstTarotist(t, ts, token)
	first := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "试用开场", "type": "tarotist", "tarotist_id": tarotistID})
	if first["entitlement"] != entTrial || first["reward_consumed"] == true {
		t.Fatalf("trial open: %#v", first)
	}
	if n := quotaUseCount(t, store, uid); n != 0 {
		t.Fatalf("trial consumed daily %d", n)
	}
	id := asID(first["id"])
	completeReading(t, store, id)
	msg, raw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "试用里的追问", "continuation": true})
	if !strings.Contains(msg, "不能追问") {
		t.Fatalf("trial followup should be closed: %s %s", msg, raw)
	}
	if n := followupCharges(t, store, id); n != 0 {
		t.Fatalf("trial followup charge rows %d", n)
	}
	second := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "试用后的正式占卜", "type": "tarotist", "tarotist_id": tarotistID})
	if second["paywall"] != true {
		t.Fatalf("formal path after trial: %#v", second)
	}
	if strings.Contains(fmt.Sprint(second), "试用已用完") || strings.Contains(fmt.Sprint(second), "算力") {
		t.Fatalf("formal path wording: %#v", second)
	}
}

func TestQuotaWordingAndAnonymousTiming(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	deviceID := fmt.Sprintf("device-quota-%d", time.Now().UnixNano())
	pending := create(t, ts, "", "/api/app/v1/readings", map[string]any{"question": "匿名先问", "type": "normal", "device_id": deviceID})
	if pending["quota_outcome"] != quotaAnon || pending["reward_consumed"] == true || pending["quota_consumed"] == true {
		t.Fatalf("anonymous precharge: %#v", pending)
	}
	if n := chargeCount(t, store, asID(pending["id"])); n != 0 {
		t.Fatalf("anonymous charge rows %d", n)
	}
	email := fmt.Sprintf("anon-login-%d@tata.local", time.Now().UnixNano())
	code := sendEmailCode(t, ts, "", email, "login")
	registered := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{"email": email, "code": code, "device_id": deviceID})
	if registered["accessToken"] == nil {
		t.Fatalf("login: %#v", registered)
	}

	_, wxRaw := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/wechat", map[string]any{"code": fmt.Sprintf("wx-quota-%d", time.Now().UnixNano())})
	if strings.Contains(string(wxRaw), "未闭合") || strings.Contains(string(wxRaw), "算力") {
		t.Fatalf("wechat wording: %s", wxRaw)
	}
	token := registered["accessToken"].(string)
	_, rewardRaw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/use-reward", asID(pending["id"])), map[string]any{})
	quotaRaw := get(t, ts, token, "/api/app/v1/quota")
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	adminRaw := get(t, ts, admin.access, "/api/admin/v1/quota-params")
	for _, raw := range []string{string(wxRaw), string(rewardRaw), string(quotaRaw), string(adminRaw)} {
		if strings.Contains(raw, "未闭合") || strings.Contains(raw, "算力") {
			t.Fatalf("user-visible wording: %s", raw)
		}
	}
	if !strings.Contains(string(quotaRaw), "次数") || !strings.Contains(string(quotaRaw), "额度") {
		t.Fatalf("quota labels: %s", quotaRaw)
	}
}

func TestExploreChargesBeforeRitual(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, uid := registerUser(t, ts, store, "explore")
	spreadID := ensureTimeFlow(t, store)
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"type": "explore", "source": "explore"})
	id := asID(reading["id"])
	if reading["quota_outcome"] != quotaDeferred || reading["reward_consumed"] == true {
		t.Fatalf("explore charged at create: %#v", reading)
	}
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", id), map[string]any{"spread_id": spreadID})
	clientCards := `[{"position":1,"position_name":"主题","name":"愚者","orientation":"upright","card_id":0}]`
	if _, err := store.Exec(t.Context(), `UPDATE readings SET cards=$2, status='CARDS_FIXED' WHERE id=$1`, id, clientCards); err != nil {
		t.Fatal(err)
	}
	if n := chargeCount(t, store, id); n != 0 {
		t.Fatalf("explore charged before interpret %d", n)
	}
	if _, err := store.Exec(t.Context(), `UPDATE readings SET question='探索一下' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	msg, raw := call(t, ts, token, http.MethodPost, httpPath(id, "interpret"), map[string]any{})
	if !strings.Contains(msg, "开通会员") {
		t.Fatalf("explore interpret paywall: %s %s", msg, raw)
	}
	var kept string
	if err := store.QueryRow(t.Context(), `SELECT cards FROM readings WHERE id=$1`, id).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept == "" {
		t.Fatal("draw was cleared")
	}
	_ = uid
}

func TestDailyAcceptsClientCard(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, uid := registerUser(t, ts, store, "dailycard")
	seedInvite(t, store, uid, 1)
	msg, raw := call(t, ts, token, http.MethodPost, "/api/app/v1/daily", map[string]any{
		"target": "today",
		"cards": []map[string]any{
			{"name": "女皇", "orientation": "reversed", "position": 1, "position_name": "今日", "card_id": 3},
		},
	})
	if msg != "" {
		t.Fatalf("daily client card: %s %s", msg, raw)
	}
	item := decodeData(t, raw)
	if item["card_name"] != "女皇" {
		t.Fatalf("card_name: %#v", item["card_name"])
	}
	var cardsRaw string
	if err := store.QueryRow(t.Context(), `SELECT cards FROM readings WHERE id=$1`, asID(item["id"])).Scan(&cardsRaw); err != nil {
		t.Fatal(err)
	}
	var cards []drawnCard
	if err := json.Unmarshal([]byte(cardsRaw), &cards); err != nil || len(cards) != 1 {
		t.Fatalf("stored cards: %s err=%v", cardsRaw, err)
	}
	if cards[0].Name != "女皇" || cards[0].Orientation != "reversed" {
		t.Fatalf("stored card: %#v", cards[0])
	}
	msg, _ = call(t, ts, token, http.MethodPost, "/api/app/v1/daily", map[string]any{
		"target": "today",
		"cards": []map[string]any{
			{"name": "愚者", "orientation": "upright"},
			{"name": "魔术师", "orientation": "upright"},
		},
	})
	if !strings.Contains(msg, "一张") {
		t.Fatalf("two cards: %s", msg)
	}
}

func TestDailyExistingResultSkipsCharge(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, uid := registerUser(t, ts, store, "daily")
	seedInvite(t, store, uid, 1)
	msg, raw := call(t, ts, token, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if msg != "" {
		t.Fatalf("first daily: %s %s", msg, raw)
	}
	first := decodeData(t, raw)
	firstID := asID(first["id"])
	if n := chargeCount(t, store, firstID); n != 1 {
		t.Fatalf("first daily charges %d", n)
	}
	msg, raw = call(t, ts, token, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if !strings.Contains(msg, "已占用") || strings.Contains(msg, "算力") {
		t.Fatalf("second daily: %s", msg)
	}
	reused := decodeData(t, raw)
	reading, _ := reused["reading"].(map[string]any)
	if reused["charged"] != false || reading == nil || asID(reading["id"]) != firstID {
		t.Fatalf("reuse payload: %#v", reused)
	}
	if n := chargeCount(t, store, firstID); n != 1 {
		t.Fatalf("reuse charged again %d", n)
	}

	other, otherID := registerUser(t, ts, store, "tomorrow")
	seedInvite(t, store, otherID, 1)
	msg, raw = call(t, ts, other, http.MethodPost, "/api/app/v1/daily", map[string]any{"target": "tomorrow"})
	if msg != "" {
		t.Fatalf("tomorrow: %s %s", msg, raw)
	}
	tomorrow := decodeData(t, raw)
	if tomorrow["question"] != "明日运势" {
		t.Fatalf("tomorrow reading: %#v", tomorrow)
	}
	tomorrowID := asID(tomorrow["id"])
	if playEntitlement(t, store, tomorrowID) != entInvite {
		t.Fatalf("tomorrow entitlement %s", playEntitlement(t, store, tomorrowID))
	}
	msg, _ = call(t, ts, other, http.MethodPost, "/api/app/v1/daily", map[string]any{"target": "tomorrow"})
	if !strings.Contains(msg, "不重复扣") && !strings.Contains(msg, "已占用") {
		t.Fatalf("tomorrow reuse: %s", msg)
	}
	if chargeCount(t, store, tomorrowID) != 1 {
		t.Fatalf("tomorrow recharged %d", chargeCount(t, store, tomorrowID))
	}
}

func registerUser(t *testing.T, ts *httptest.Server, store *Store, prefix string) (string, int64) {
	t.Helper()
	email := fmt.Sprintf("%s-%d@tata.local", prefix, time.Now().UnixNano())
	token := register(t, ts, email)
	return token, userIDByEmail(t, store, email)
}

func TestDailyQuotaCountsConfigurable(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	resetDailyCounts(t, store)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	msg, _ := call(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{"daily_free_count": 2, "daily_slot_count": 2})
	if !strings.Contains(msg, "已下线") {
		t.Fatalf("daily free still editable: %s", msg)
	}
	params := listOne(t, ts, admin.access, "/api/admin/v1/quota-params")
	if params["daily_free_editable"] != false || params["daily_slot_editable"] != false {
		t.Fatalf("admin counts: %#v", params)
	}

	token, _ := registerUser(t, ts, store, "dailycfg")
	quota := listOne(t, ts, token, "/api/app/v1/quota")
	if _, ok := quota["daily_free_count"]; ok || quota["priority"] == "今日免费次数 → 邀请奖励次数" {
		t.Fatalf("app quota still offers daily free: %#v", quota)
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "不再扣每日免费", "type": "normal"})
	spreadID := ensureTimeFlow(t, store)
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(reading["id"])), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	var ent string
	err := store.QueryRow(t.Context(), `SELECT entitlement FROM quota_charges WHERE reading_id=$1 AND charge_key='play'`, asID(reading["id"])).Scan(&ent)
	if err == nil && ent == entDaily {
		t.Fatal("daily free still charged")
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}

	slotToken, slotUser := registerUser(t, ts, store, "dailyslot")
	seedInvite(t, store, slotUser, 1)
	msg, raw := call(t, ts, slotToken, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if msg != "" {
		t.Fatalf("first daily slot: %s %s", msg, raw)
	}
	firstDaily := decodeData(t, raw)
	msg, raw = call(t, ts, slotToken, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if !strings.Contains(msg, "已占用") && !strings.Contains(msg, "不重复扣") {
		t.Fatalf("second daily slot: %s %s", msg, raw)
	}
	reused := decodeData(t, raw)
	again, _ := reused["reading"].(map[string]any)
	if asID(again["id"]) != asID(firstDaily["id"]) {
		t.Fatalf("same day should reuse: %#v", reused)
	}
}

func TestTokensPerQuotaConfig(t *testing.T) {
	store, ts := newMergeServer(t)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET tokens_per_quota=NULL WHERE id=1`)
	})
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET tokens_per_quota=NULL WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	params := listOne(t, ts, admin.access, "/api/admin/v1/quota-params")
	if params["tokens_per_quota"] != nil || params["tokens_per_quota_editable"] != true {
		t.Fatalf("default: %#v", params["tokens_per_quota"])
	}
	msg, _ := call(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{"tokens_per_quota": 0})
	if !strings.Contains(msg, "token") {
		t.Fatalf("zero: %s", msg)
	}
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{"tokens_per_quota": 1000})
	params = listOne(t, ts, admin.access, "/api/admin/v1/quota-params")
	if params["tokens_per_quota"] != float64(1000) {
		t.Fatalf("saved: %#v", params["tokens_per_quota"])
	}
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{"tokens_per_quota": nil})
	params = listOne(t, ts, admin.access, "/api/admin/v1/quota-params")
	if params["tokens_per_quota"] != nil {
		t.Fatalf("cleared: %#v", params["tokens_per_quota"])
	}
}

func TestRegistrationGiftConfig(t *testing.T) {
	store, ts := newMergeServer(t)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET registration_grant_delay_minutes=NULL, registration_valid_days=3, registration_grant_count=1, registration_spread_ids='[]' WHERE id=1`)
	})
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	var oneCard, celtic int64
	if err := store.QueryRow(t.Context(), `SELECT id FROM spreads WHERE name='单张牌占卜' AND theme_key=''`).Scan(&oneCard); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT id FROM spreads WHERE name='凯尔特十字牌阵' AND theme_key=''`).Scan(&celtic); err != nil {
		t.Fatal(err)
	}
	msg, _ := call(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{
		"registration_spread_ids": []int64{celtic},
	})
	if !strings.Contains(msg, "白名单") {
		t.Fatalf("wide spread: %s", msg)
	}
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{
		"registration_grant_delay_minutes": 0,
		"registration_valid_days":          1,
		"registration_grant_count":         2,
		"registration_spread_ids":          []int64{oneCard},
	})
	params := listOne(t, ts, admin.access, "/api/admin/v1/quota-params")
	ids, _ := params["registration_spread_ids"].([]any)
	if params["registration_grant_delay_minutes"] != float64(0) || params["registration_valid_days"] != float64(1) || params["registration_grant_count"] != float64(2) || len(ids) != 1 {
		t.Fatalf("params: %#v", params)
	}

	token, uid := registerUser(t, ts, store, "reggift")
	var remaining int
	var granted time.Time
	var expires time.Time
	if err := store.QueryRow(t.Context(), `SELECT remaining, granted_at, expires_at FROM registration_gifts WHERE user_id=$1`, uid).Scan(&remaining, &granted, &expires); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 || expires.Sub(granted) < 23*time.Hour || expires.Sub(granted) > 25*time.Hour {
		t.Fatalf("grant remaining=%d granted=%s expires=%s", remaining, granted, expires)
	}
	quota := listOne(t, ts, token, "/api/app/v1/quota")
	gift, _ := quota["registration_gift"].(map[string]any)
	if gift["remaining"] != float64(2) || gift["total"] != float64(2) || gift["used"] != false {
		t.Fatalf("app gift: %#v", gift)
	}

	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{"registration_grant_delay_minutes": nil})
	_, uid2 := registerUser(t, ts, store, "regwait")
	var granted2 sql.NullTime
	var remaining2 int
	if err := store.QueryRow(t.Context(), `SELECT granted_at, remaining FROM registration_gifts WHERE user_id=$1`, uid2).Scan(&granted2, &remaining2); err != nil {
		t.Fatal(err)
	}
	if granted2.Valid || remaining2 != 0 {
		t.Fatalf("ungranted: valid=%v remaining=%d", granted2.Valid, remaining2)
	}
}

// Pending gift must be granted inside charge/settle, without App calling GET /quota first.
func TestSettleGrantsPendingRegistrationGiftWithoutQuotaAPI(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET registration_grant_delay_minutes=NULL, registration_valid_days=3, registration_grant_count=1, registration_spread_ids='[]' WHERE id=1`)
	})
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET registration_grant_delay_minutes=NULL, registration_spread_ids='[]' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	token, uid := registerUser(t, ts, store, "settlegrant")
	var granted sql.NullTime
	if err := store.QueryRow(t.Context(), `SELECT granted_at FROM registration_gifts WHERE user_id=$1`, uid).Scan(&granted); err != nil {
		t.Fatal(err)
	}
	if granted.Valid {
		t.Fatal("expected ungranted before settle")
	}
	allowGiftSpread(t, store)
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET registration_grant_delay_minutes=0, registration_grant_count=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	id := chargeFreePlay(t, ts, store, token, "结算时补发注册赠送")
	if playEntitlement(t, store, id) != entReward {
		t.Fatalf("entitlement %s", playEntitlement(t, store, id))
	}
	var left int
	var grantedAt sql.NullTime
	if err := store.QueryRow(t.Context(), `SELECT remaining, granted_at FROM registration_gifts WHERE user_id=$1`, uid).Scan(&left, &grantedAt); err != nil {
		t.Fatal(err)
	}
	if !grantedAt.Valid {
		t.Fatal("gift should be granted during charge")
	}
	if left != 0 {
		t.Fatalf("remaining after play %d", left)
	}
}

func resetDailyCounts(t *testing.T, store *Store) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET daily_free_count=1, daily_slot_count=1 WHERE id=1`)
	})
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET daily_free_count=1, daily_slot_count=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
}

func resetDiamondCost(t *testing.T, store *Store) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET quota_cost=NULL WHERE id=1`)
	})
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET quota_cost=NULL WHERE id=1`); err != nil {
		t.Fatal(err)
	}
}

func setDiamondCost(t *testing.T, store *Store, cost int) {
	t.Helper()
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET quota_cost=$1 WHERE id=1`, cost); err != nil {
		t.Fatal(err)
	}
}

func setDiamonds(t *testing.T, store *Store, uid int64, sub, pack int) {
	t.Helper()
	setQuota(t, store, uid, sub+pack)
}

func setQuota(t *testing.T, store *Store, uid int64, n int) {
	t.Helper()
	if _, err := store.Exec(t.Context(), `DELETE FROM quota_lots WHERE user_id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO quota_balances(user_id, quota_balance)
		VALUES ($1,$2)
		ON CONFLICT (user_id) DO UPDATE SET quota_balance=$2, updated_at=now()`, uid, n); err != nil {
		t.Fatal(err)
	}
}

func setMember(t *testing.T, store *Store, uid int64, kind string) {
	t.Helper()
	if kind == "" {
		kind = "month"
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at)
		VALUES ($1,$2,'active',now(), now() + interval '40 days')`, uid, kind); err != nil {
		t.Fatal(err)
	}
}

func quotaOf(t *testing.T, store *Store, uid int64) int {
	t.Helper()
	var n int
	err := store.QueryRow(t.Context(), `SELECT quota_balance FROM quota_balances WHERE user_id=$1`, uid).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func chargeFreePlay(t *testing.T, ts *httptest.Server, store *Store, token, question string) int64 {
	t.Helper()
	me := listOne(t, ts, token, "/api/app/v1/profile")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, asID(me["id"])); err != nil {
		t.Fatal(err)
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": question, "type": "normal"})
	id := asID(reading["id"])
	spreadID := ensureTimeFlow(t, store)
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", id), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	return id
}

func playEntitlement(t *testing.T, store *Store, readingID int64) string {
	t.Helper()
	var ent string
	if err := store.QueryRow(t.Context(), `SELECT entitlement FROM quota_charges WHERE reading_id=$1 AND charge_key='play'`, readingID).Scan(&ent); err != nil {
		t.Fatal(err)
	}
	return ent
}

func rewardRemaining(t *testing.T, store *Store, uid int64) int {
	t.Helper()
	var n int
	err := store.QueryRow(t.Context(), `SELECT remaining FROM rewards WHERE user_id=$1`, uid).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func quotaUseCount(t *testing.T, store *Store, uid int64) int {
	t.Helper()
	var n int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_use WHERE user_id=$1`, uid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ledgerCount(t *testing.T, store *Store, readingID int64) int {
	t.Helper()
	var n int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM ledger WHERE reading_id=$1`, readingID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func chargeCount(t *testing.T, store *Store, readingID int64) int {
	t.Helper()
	var n int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_charges WHERE reading_id=$1`, readingID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func followupCharges(t *testing.T, store *Store, readingID int64) int {
	t.Helper()
	var n int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_charges WHERE reading_id=$1 AND charge_key LIKE 'followup:%'`, readingID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestQuotaLotsSpendSubscriptionThenPackAndExpire(t *testing.T) {
	store, ts := newMergeServer(t)
	_, uid := registerUser(t, ts, store, "lots")
	setMember(t, store, uid, "month")
	tx, err := store.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = creditQuota(t.Context(), tx, uid, 5, nil, "sub", "iap_period_refresh"); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if _, err = creditQuota(t.Context(), tx, uid, 3, nil, "pack", "iap_quota_pack"); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	after, ok, draws, err := spendQuota(t.Context(), tx, uid, 6)
	if err != nil || !ok || after != 2 || len(draws) != 2 || draws[0].Source != "subscription" || draws[0].Remaining != 0 || draws[1].Source != "pack" || draws[1].Remaining != 2 {
		_ = tx.Rollback(t.Context())
		t.Fatalf("spend after=%d ok=%v draws=%+v err=%v", after, ok, draws, err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var subLeft, packLeft int
	if err = store.QueryRow(t.Context(), `SELECT remaining FROM quota_lots WHERE user_id=$1 AND source='subscription'`, uid).Scan(&subLeft); err != nil {
		t.Fatal(err)
	}
	if err = store.QueryRow(t.Context(), `SELECT remaining FROM quota_lots WHERE user_id=$1 AND source='pack'`, uid).Scan(&packLeft); err != nil {
		t.Fatal(err)
	}
	if subLeft != 0 || packLeft != 2 || quotaOf(t, store, uid) != 2 {
		t.Fatalf("lots sub=%d pack=%d balance=%d", subLeft, packLeft, quotaOf(t, store, uid))
	}
	if _, err = store.Exec(t.Context(), `UPDATE quota_lots SET expires_at=now() - interval '1 minute' WHERE user_id=$1 AND source='pack'`, uid); err != nil {
		t.Fatal(err)
	}
	if err = expireQuotaLots(t.Context(), store, uid); err != nil {
		t.Fatal(err)
	}
	if n := quotaOf(t, store, uid); n != 0 {
		t.Fatalf("expired pack still in balance %d", n)
	}
}

func TestLedgerDrawsNameTheLot(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 6)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	token, uid := registerUser(t, ts, store, "ledgerlot")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	setMember(t, store, uid, "month")
	tx, err := store.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = creditQuota(t.Context(), tx, uid, 5, nil, "sub", "iap_period_refresh"); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if _, err = creditQuota(t.Context(), tx, uid, 3, nil, "pack", "iap_quota_pack"); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "跨批次扣额度", "type": "normal"})
	spreadID := ensureTimeFlow(t, store)
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(reading["id"])), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	rows := list(t, ts, admin.access, "/api/admin/v1/ledger")
	var found map[string]any
	for _, row := range rows {
		if asID(row["reading_id"]) == asID(reading["id"]) {
			found = row
		}
	}
	if found == nil {
		t.Fatalf("ledger missing reading: %#v", rows)
	}
	draws, _ := found["draws"].([]any)
	if len(draws) != 2 {
		t.Fatalf("draws: %#v", found)
	}
	first := draws[0].(map[string]any)
	second := draws[1].(map[string]any)
	if first["source"] != "subscription" || first["amount"] != float64(5) || first["remaining_after"] != float64(0) || first["expires_at"] == nil {
		t.Fatalf("subscription draw: %#v", first)
	}
	if second["source"] != "pack" || second["amount"] != float64(1) || second["remaining_after"] != float64(2) || second["expires_at"] == nil {
		t.Fatalf("pack draw: %#v", second)
	}
	if found["reading_status"] == nil || found["reading_status"] == "" {
		t.Fatalf("reading status: %#v", found)
	}
}

func TestMembershipEndDiscardsPool(t *testing.T) {
	store, ts := newMergeServer(t)
	_, uid := registerUser(t, ts, store, "endpool")
	setMember(t, store, uid, "month")
	tx, err := store.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = creditQuota(t.Context(), tx, uid, 5, nil, "sub", "iap_period_refresh"); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if _, err = creditQuota(t.Context(), tx, uid, 3, nil, "pack", "iap_quota_pack"); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Exec(t.Context(), `UPDATE subscriptions SET expires_at=now() - interval '1 minute' WHERE user_id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	if err = expireQuotaLots(t.Context(), store, uid); err != nil {
		t.Fatal(err)
	}
	var left int
	if err = store.QueryRow(t.Context(), `SELECT COALESCE(SUM(remaining),0) FROM quota_lots WHERE user_id=$1`, uid).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 || quotaOf(t, store, uid) != 0 {
		t.Fatalf("ended membership kept pool lots=%d balance=%d", left, quotaOf(t, store, uid))
	}
	setMember(t, store, uid, "month")
	tx, err = store.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	after, err := creditQuota(t.Context(), tx, uid, 4, nil, "next", "iap_period_refresh")
	if err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if after != 4 || quotaOf(t, store, uid) != 4 {
		t.Fatalf("new period carried old quota after=%d balance=%d", after, quotaOf(t, store, uid))
	}
}

func asID(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	default:
		return 0
	}
}

func decodeData(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err, string(raw))
	}
	return env.Data
}

func allowGiftSpread(t *testing.T, store *Store) {
	t.Helper()
	id := ensureTimeFlow(t, store)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET registration_spread_ids='[]' WHERE id=1`)
	})
	raw, err := json.Marshal([]int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET registration_spread_ids=$1 WHERE id=1`, string(raw)); err != nil {
		t.Fatal(err)
	}
}

func seedInvite(t *testing.T, store *Store, uid int64, n int) {
	t.Helper()
	if _, err := store.Exec(t.Context(), `INSERT INTO rewards(user_id, granted_at, expires_at, invite_remaining)
		VALUES ($1, now(), now(), $2)
		ON CONFLICT (user_id) DO UPDATE SET invite_remaining=$2`, uid, n); err != nil {
		t.Fatal(err)
	}
}

func seedGift(t *testing.T, store *Store, uid int64, n int) {
	t.Helper()
	if _, err := store.Exec(t.Context(), `INSERT INTO registration_gifts(user_id, registered_at, granted_at, usable_at, expires_at, total, remaining)
		VALUES ($1, now(), now(), now(), now() + interval '3 days', $2, $2)
		ON CONFLICT (user_id) DO UPDATE SET granted_at=now(), usable_at=now(), expires_at=now() + interval '3 days', total=$2, remaining=$2, used_at=NULL`, uid, n); err != nil {
		t.Fatal(err)
	}
}

func TestGiftBeforeInviteAndShortPool(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 3)
	allowGiftSpread(t, store)
	token, uid := registerUser(t, ts, store, "giftorder")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	seedGift(t, store, uid, 1)
	seedInvite(t, store, uid, 1)

	giftID := chargeFreePlay(t, ts, store, token, "先扣注册赠送")
	if playEntitlement(t, store, giftID) != entReward {
		t.Fatalf("gift first %s", playEntitlement(t, store, giftID))
	}
	inviteID := chargeFreePlay(t, ts, store, token, "再扣邀请")
	if playEntitlement(t, store, inviteID) != entInvite {
		t.Fatalf("invite second %s", playEntitlement(t, store, inviteID))
	}

	shortToken, shortID := registerUser(t, ts, store, "shortpool")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, shortID); err != nil {
		t.Fatal(err)
	}
	seedGift(t, store, shortID, 1)
	setMember(t, store, shortID, "month")
	setQuota(t, store, shortID, 2)
	reading := create(t, ts, shortToken, "/api/app/v1/readings", map[string]any{"question": "额度不够不碰赠送", "type": "normal"})
	spreadID := ensureTimeFlow(t, store)
	msg, _ := call(t, ts, shortToken, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(reading["id"])), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	if !strings.Contains(msg, "额度不足") {
		t.Fatalf("short pool: %s %#v", msg, reading)
	}
	var left int
	if err := store.QueryRow(t.Context(), `SELECT remaining FROM registration_gifts WHERE user_id=$1`, shortID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 || quotaOf(t, store, shortID) != 2 {
		t.Fatalf("short pool consumed gift=%d quota=%d", left, quotaOf(t, store, shortID))
	}

	openToken, openID := registerUser(t, ts, store, "zeropool")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, openID); err != nil {
		t.Fatal(err)
	}
	held := create(t, ts, openToken, "/api/app/v1/readings", map[string]any{"question": "池子为零再用赠送", "type": "normal"})
	setMember(t, store, openID, "month")
	setQuota(t, store, openID, 0)
	seedGift(t, store, openID, 1)
	must(t, ts, openToken, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(held["id"])), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	if playEntitlement(t, store, asID(held["id"])) != entReward {
		t.Fatalf("zero pool entitlement %s", playEntitlement(t, store, asID(held["id"])))
	}
}

func exhaustFree(t *testing.T, ts *httptest.Server, token string) {
	t.Helper()
	create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "占位", "type": "normal"})
}

func TestSpreadCostOverridesGlobalQuota(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 1)
	token, uid := registerUser(t, ts, store, "spreadcost")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET reading_trial_used=true WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	setMember(t, store, uid, "month")
	setQuota(t, store, uid, 20)
	spreadID := ensureTimeFlow(t, store)
	if _, err := store.Exec(t.Context(), `UPDATE spreads SET quota_cost=4 WHERE id=$1`, spreadID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE spreads SET quota_cost=NULL WHERE id=$1`, spreadID)
	})
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "按这套牌阵算", "type": "normal"})
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(reading["id"])), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	var amount int
	if err := store.QueryRow(t.Context(), `SELECT amount FROM quota_charges WHERE reading_id=$1 AND charge_key=$2`, asID(reading["id"]), chargeKeyPlay).Scan(&amount); err != nil {
		t.Fatal(err)
	}
	if amount != 4 {
		t.Fatalf("charged %d", amount)
	}
}

func TestMemberTarotistCanFollowUpAfterTrial(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 1)
	token, uid := registerUser(t, ts, store, "tarotpaid")
	tarotistID := firstTarotist(t, ts, token)
	first := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "试用", "type": "tarotist", "tarotist_id": tarotistID})
	completeReading(t, store, asID(first["id"]))
	setMember(t, store, uid, "month")
	setQuota(t, store, uid, 10)
	second := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "正式", "type": "tarotist", "tarotist_id": tarotistID})
	if second["entitlement"] != entQuota {
		t.Fatalf("paid tarotist: %#v", second)
	}
	id := asID(second["id"])
	completeReading(t, store, id)
	msg, raw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "继续看", "continuation": true})
	if msg != "" {
		t.Fatalf("member followup: %s %s", msg, raw)
	}
}

func exhaustFreePlay(t *testing.T, ts *httptest.Server, store *Store, token string) {
	t.Helper()
}

func ensureTimeFlow(t *testing.T, store *Store) int64 {
	t.Helper()
	if _, err := store.Exec(t.Context(), `UPDATE spreads SET enabled=true WHERE name='时间流'`); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := store.QueryRow(t.Context(), `SELECT id FROM spreads WHERE name='时间流' ORDER BY id LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func completeReading(t *testing.T, store *Store, id int64) {
	t.Helper()
	if _, err := store.Exec(t.Context(), `UPDATE readings SET status='COMPLETED', cards=$2, interpretation='ok' WHERE id=$1`, id, `[{"name":"愚者"}]`); err != nil {
		t.Fatal(err)
	}
}

func firstTarotist(t *testing.T, ts *httptest.Server, token string) int64 {
	t.Helper()
	list := list(t, ts, token, "/api/app/v1/tarotists")
	if len(list) == 0 {
		t.Fatal("no tarotist")
	}
	return asID(list[0]["id"])
}

func httpPath(id int64, action string) string {
	return fmt.Sprintf("/api/app/v1/readings/%d/%s", id, action)
}
