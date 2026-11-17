package tata

import (
	"context"
	"encoding/json"
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
	token, uid := registerUser(t, ts, store, "priority")

	firstID := chargeFreePlay(t, ts, store, token, "先扣今日免费")
	if playEntitlement(t, store, firstID) != entDaily {
		t.Fatalf("daily first %s", playEntitlement(t, store, firstID))
	}
	if n := quotaUseCount(t, store, uid); n != 1 {
		t.Fatalf("daily flags %d", n)
	}

	if _, err := store.Exec(t.Context(), `INSERT INTO rewards(user_id, granted_at, expires_at, invite_remaining) VALUES ($1, now(), now(), 3)
		ON CONFLICT (user_id) DO UPDATE SET invite_remaining=3`, uid); err != nil {
		t.Fatal(err)
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
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(third["id"])), map[string]any{"spread_id": spreadID})
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/shuffle", asID(third["id"])), map[string]any{})
	if !strings.Contains(msg, "开通会员") {
		t.Fatalf("free paywall: %s", msg)
	}

	setMember(t, store, uid, "month")
	setDiamondCost(t, store, 3)
	setQuota(t, store, uid, 10)
	memberPlay := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "会员扣额度", "type": "normal"})
	if memberPlay["entitlement"] != entQuota || memberPlay["paywall"] == true {
		t.Fatalf("member quota: %#v", memberPlay)
	}
	if got := quotaOf(t, store, uid); got != 7 {
		t.Fatalf("quota after spend %d", got)
	}
	setQuota(t, store, uid, 2)
	poor := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "额度不足", "type": "normal"})
	if poor["paywall"] != true || poor["quota_outcome"] != quotaPaywall {
		t.Fatalf("member paywall: %#v", poor)
	}
	prompt, _ := poor["quota_prompt"].(string)
	if !strings.Contains(prompt, "额度不足") || strings.Contains(prompt, "算力") {
		t.Fatalf("paywall prompt: %s", prompt)
	}
	if got := quotaOf(t, store, uid); got != 2 {
		t.Fatalf("paywall spent quota %d", got)
	}
}

func TestDiamondCostUnconfigured(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, uid := registerUser(t, ts, store, "unconfig")
	setMember(t, store, uid, "month")
	setQuota(t, store, uid, 9)
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "未配置不能猜额度", "type": "normal"})
	if reading["quota_outcome"] != quotaUnconfig || reading["diamond_cost_unconfigured"] != true || reading["generated"] != false {
		t.Fatalf("unconfigured: %#v", reading)
	}
	prompt, _ := reading["quota_prompt"].(string)
	if !strings.Contains(prompt, "未配置") || strings.Contains(prompt, "算力") {
		t.Fatalf("prompt: %s", prompt)
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
	setMember(t, store, uid, "month")
	setQuota(t, store, uid, 0)
	spreadID := ensureTimeFlow(t, store)

	cancelled := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "取消的任务", "type": "normal"})
	cancelID := asID(cancelled["id"])
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
	if kept["paywall"] != true {
		t.Fatalf("expected paywall before ritual: %#v", kept)
	}
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", keptID), map[string]any{"spread_id": spreadID})
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/shuffle", keptID), map[string]any{})
	if !strings.Contains(msg, "额度不足") || strings.Contains(msg, "算力") {
		t.Fatalf("shuffle paywall: %s", msg)
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
	if !strings.Contains(failMsg, "未绑定") && !strings.Contains(failMsg, "未重新扣") {
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
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM diamond_ledger WHERE user_id=$1 AND reason='quota_spend'`, uid).Scan(&spends); err != nil {
		t.Fatal(err)
	}
	if spends != 1 {
		t.Fatalf("diamond spends %d", spends)
	}
	if got := quotaOf(t, store, uid); got != 2 {
		t.Fatalf("failure refunded or retried %d", got)
	}
}

func TestFollowupReentersSharedQuota(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	setDiamondCost(t, store, 2)
	token, uid := registerUser(t, ts, store, "follow")
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
	if body["entitlement"] != entQuota || body["charged"] != true {
		t.Fatalf("followup should charge quota: %#v", body)
	}
	if n := followupCharges(t, store, id); n != 1 {
		t.Fatalf("followup charges %d", n)
	}
	msg, raw = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "失败后重试", "continuation": true})
	if msg != "" {
		t.Fatalf("retry followup: %s %s", msg, raw)
	}
	if n := followupCharges(t, store, id); n != 1 {
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
	if body["entitlement"] != entQuota || body["charged"] != true {
		t.Fatalf("second followup entitlement: %#v", body)
	}
	if n := followupCharges(t, store, id); n != 2 {
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
	if n := followupCharges(t, store, id); n != 2 {
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
	if !strings.Contains(msg, "开通会员") {
		t.Fatalf("trial followup should need membership: %s %s", msg, raw)
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
	anon := create(t, ts, "", "/api/app/v1/auth/anonymous", map[string]any{})
	anonToken := anon["accessToken"].(string)
	anonID := anon["anonymous_id"].(string)
	pending := create(t, ts, anonToken, "/api/app/v1/readings", map[string]any{"question": "匿名先问", "type": "normal"})
	if pending["quota_outcome"] != quotaAnon || pending["reward_consumed"] == true || pending["quota_consumed"] == true {
		t.Fatalf("anonymous precharge: %#v", pending)
	}
	if n := chargeCount(t, store, asID(pending["id"])); n != 0 {
		t.Fatalf("anonymous charge rows %d", n)
	}
	email := fmt.Sprintf("anon-login-%d@tata.local", time.Now().UnixNano())
	code := sendEmailCode(t, ts, "", email, "login")
	registered := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{"email": email, "code": code, "anonymous_id": anonID})
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
	msg, raw := call(t, ts, token, http.MethodPost, httpPath(id, "shuffle"), map[string]any{})
	if !strings.Contains(msg, "开通会员") {
		t.Fatalf("explore free paywall: %s %s", msg, raw)
	}
	if n := chargeCount(t, store, id); n != 0 {
		t.Fatalf("explore charged free user %d", n)
	}
	_ = uid
}

func TestDailyExistingResultSkipsCharge(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, _ := registerUser(t, ts, store, "daily")
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

	other, _ := registerUser(t, ts, store, "tomorrow")
	msg, raw = call(t, ts, other, http.MethodPost, "/api/app/v1/daily", map[string]any{"target": "tomorrow"})
	if msg != "" {
		t.Fatalf("tomorrow: %s %s", msg, raw)
	}
	tomorrow := decodeData(t, raw)
	if tomorrow["question"] != "明日运势" {
		t.Fatalf("tomorrow reading: %#v", tomorrow)
	}
	tomorrowID := asID(tomorrow["id"])
	if playEntitlement(t, store, tomorrowID) != entDaily {
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
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/quota-params", map[string]any{"daily_free_count": 2, "daily_slot_count": 2})
	params := listOne(t, ts, admin.access, "/api/admin/v1/quota-params")
	if params["daily_free_count"] != float64(2) || params["daily_slot_count"] != float64(2) {
		t.Fatalf("admin counts: %#v", params)
	}

	token, uid := registerUser(t, ts, store, "dailycfg")
	quota := listOne(t, ts, token, "/api/app/v1/quota")
	if quota["daily_free_count"] != float64(2) || quota["daily_slot_count"] != float64(2) || quota["daily_free_remaining"].(float64) != 2 {
		t.Fatalf("app quota counts: %#v", quota)
	}

	firstID := chargeFreePlay(t, ts, store, token, "今日免费1")
	if playEntitlement(t, store, firstID) != entDaily {
		t.Fatalf("daily 1 %s", playEntitlement(t, store, firstID))
	}
	secondID := chargeFreePlay(t, ts, store, token, "今日免费2")
	if playEntitlement(t, store, secondID) != entDaily {
		t.Fatalf("daily 2 %s", playEntitlement(t, store, secondID))
	}
	if n := quotaUseCount(t, store, uid); n != 2 {
		t.Fatalf("daily uses %d", n)
	}
	third := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "免费用完", "type": "normal"})
	spreadID := ensureTimeFlow(t, store)
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", asID(third["id"])), map[string]any{"spread_id": spreadID})
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/shuffle", asID(third["id"])), map[string]any{})
	if !strings.Contains(msg, "开通会员") {
		t.Fatalf("after daily cap: %s %#v", msg, third)
	}

	slotToken, _ := registerUser(t, ts, store, "dailyslot")
	msg, raw := call(t, ts, slotToken, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if msg != "" {
		t.Fatalf("first daily slot: %s %s", msg, raw)
	}
	firstDaily := decodeData(t, raw)
	msg, raw = call(t, ts, slotToken, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if msg != "" {
		t.Fatalf("second daily slot: %s %s", msg, raw)
	}
	secondDaily := decodeData(t, raw)
	if asID(firstDaily["id"]) == asID(secondDaily["id"]) {
		t.Fatalf("expected a new daily draw: %#v %#v", firstDaily, secondDaily)
	}
	msg, raw = call(t, ts, slotToken, http.MethodPost, "/api/app/v1/daily", map[string]any{})
	if !strings.Contains(msg, "已占用") {
		t.Fatalf("third daily slot: %s %s", msg, raw)
	}
	reused := decodeData(t, raw)
	reading, _ := reused["reading"].(map[string]any)
	if asID(reading["id"]) != asID(secondDaily["id"]) {
		t.Fatalf("reuse latest: %#v", reused)
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
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET diamond_cost=NULL WHERE id=1`)
	})
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET diamond_cost=NULL WHERE id=1`); err != nil {
		t.Fatal(err)
	}
}

func setDiamondCost(t *testing.T, store *Store, cost int) {
	t.Helper()
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET diamond_cost=$1 WHERE id=1`, cost); err != nil {
		t.Fatal(err)
	}
}

func setDiamonds(t *testing.T, store *Store, uid int64, sub, pack int) {
	t.Helper()
	setQuota(t, store, uid, sub+pack)
}

func setQuota(t *testing.T, store *Store, uid int64, n int) {
	t.Helper()
	if _, err := store.Exec(t.Context(), `INSERT INTO diamond_balances(user_id, quota_balance)
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
	err := store.QueryRow(t.Context(), `SELECT quota_balance FROM diamond_balances WHERE user_id=$1`, uid).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

func chargeFreePlay(t *testing.T, ts *httptest.Server, store *Store, token, question string) int64 {
	t.Helper()
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": question, "type": "normal"})
	id := asID(reading["id"])
	spreadID := ensureTimeFlow(t, store)
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", id), map[string]any{"spread_id": spreadID})
	msg, raw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/shuffle", id), map[string]any{})
	if msg != "" {
		t.Fatalf("shuffle %s: %s %s", question, msg, raw)
	}
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

func exhaustFree(t *testing.T, ts *httptest.Server, token string) {
	t.Helper()
	create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "占位", "type": "normal"})
}

func exhaustFreePlay(t *testing.T, ts *httptest.Server, store *Store, token string) {
	t.Helper()
	chargeFreePlay(t, ts, store, token, "用掉今日免费")
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
