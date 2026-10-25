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
	setDiamondCost(t, store, 3)
	setDiamonds(t, store, uid, 5, 3)

	first := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "先扣注册奖励", "type": "normal"})
	if first["entitlement"] != entReward || first["reward_consumed"] != true || first["quota_consumed"] == true || first["paywall"] == true {
		t.Fatalf("reward first: %#v", first)
	}
	if got := rewardRemaining(t, store, uid); got != 0 {
		t.Fatalf("reward remaining %d", got)
	}
	if n := quotaUseCount(t, store, uid); n != 0 {
		t.Fatalf("daily used on reward charge: %d", n)
	}
	sub, pack := diamondBalances(t, store, uid)
	if sub != 5 || pack != 3 {
		t.Fatalf("diamonds moved on free charge: sub=%d pack=%d", sub, pack)
	}
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/use-reward", asID(first["id"])), map[string]any{})
	msg, raw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/confirm-quota", asID(first["id"])), map[string]any{"consume": true})
	if msg != "" || strings.Contains(string(raw), "未闭合") {
		t.Fatalf("confirm after auto charge: %s %s", msg, raw)
	}
	if n := ledgerCount(t, store, asID(first["id"])); n != 1 {
		t.Fatalf("manual path double charged ledger=%d", n)
	}
	if n := quotaUseCount(t, store, uid); n != 0 {
		t.Fatalf("confirm consumed daily: %d", n)
	}

	second := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "再扣今日免费", "type": "normal"})
	if second["entitlement"] != entDaily || second["quota_consumed"] != true {
		t.Fatalf("daily second: %#v", second)
	}
	if n := quotaUseCount(t, store, uid); n != 1 {
		t.Fatalf("daily flags %d", n)
	}
	sub, pack = diamondBalances(t, store, uid)
	if sub != 5 || pack != 3 {
		t.Fatalf("diamonds moved on daily charge: sub=%d pack=%d", sub, pack)
	}

	third := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "扣点数包钻石", "type": "normal"})
	if third["entitlement"] != entPack || third["paywall"] == true {
		t.Fatalf("pack third: %#v", third)
	}
	sub, pack = diamondBalances(t, store, uid)
	if pack != 0 || sub != 5 {
		t.Fatalf("pack-first spend: sub=%d pack=%d", sub, pack)
	}
	setDiamonds(t, store, uid, 5, 2)
	fourth := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "再扣会员钻石", "type": "normal"})
	if fourth["entitlement"] != entSub {
		t.Fatalf("subscription fourth: %#v", fourth)
	}
	sub, pack = diamondBalances(t, store, uid)
	if pack != 2 || sub != 2 {
		t.Fatalf("subscription spend: sub=%d pack=%d", sub, pack)
	}
	fifth := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "都不足", "type": "normal"})
	if fifth["paywall"] != true || fifth["quota_outcome"] != quotaPaywall || fifth["generated"] != false {
		t.Fatalf("paywall: %#v", fifth)
	}
	prompt, _ := fifth["quota_prompt"].(string)
	if strings.Contains(prompt, "算力") || strings.Contains(prompt, "未闭合") || !strings.Contains(prompt, "钻石") {
		t.Fatalf("paywall prompt: %s", prompt)
	}
	sub, pack = diamondBalances(t, store, uid)
	if pack != 2 || sub != 2 {
		t.Fatalf("paywall debited diamonds: sub=%d pack=%d", sub, pack)
	}
	if n := chargeCount(t, store, asID(fifth["id"])); n != 0 {
		t.Fatalf("paywall wrote a charge: %d", n)
	}
}

func TestDiamondCostUnconfigured(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, uid := registerUser(t, ts, store, "unconfig")
	exhaustFree(t, ts, token)
	setDiamonds(t, store, uid, 4, 7)
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "未配置不能猜钻", "type": "normal"})
	if reading["quota_outcome"] != quotaUnconfig || reading["diamond_cost_unconfigured"] != true || reading["generated"] != false {
		t.Fatalf("unconfigured: %#v", reading)
	}
	prompt, _ := reading["quota_prompt"].(string)
	if !strings.Contains(prompt, "未配置") || !strings.Contains(prompt, "已用完") || strings.Contains(prompt, "算力") {
		t.Fatalf("prompt: %s", prompt)
	}
	sub, pack := diamondBalances(t, store, uid)
	if sub != 4 || pack != 7 {
		t.Fatalf("unconfigured debited: sub=%d pack=%d", sub, pack)
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
	exhaustFree(t, ts, token)
	setDiamonds(t, store, uid, 0, 0)
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
	if !strings.Contains(msg, "钻石不足") || strings.Contains(msg, "算力") {
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
	insertIAPSKU(t, store, "consumable_pack", "consumable", "app_store", product, &grant)
	txn := fmt.Sprintf("txn-resume-%d", time.Now().UnixNano())
	out := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, txn, "receipt-"+txn, nil))
	tasks, _ := out["resumed_tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("resume hook: %#v", out["resumed_tasks"])
	}
	resumed := tasks[0].(map[string]any)
	if asID(resumed["reading_id"]) != keptID || resumed["resume_action"] != resumeShuffle || resumed["entitlement"] != entPack {
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
	sub, pack := diamondBalances(t, store, uid)
	if pack != 2 || sub != 0 {
		t.Fatalf("resume spend: sub=%d pack=%d", sub, pack)
	}

	again := create(t, ts, token, "/api/app/v1/iap/verify", iapBody("app_store", product, txn, txn, "receipt-"+txn, nil))
	if again["idempotent"] != true {
		t.Fatalf("verify replay: %#v", again)
	}
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/use-reward", keptID), map[string]any{})
	sub, pack = diamondBalances(t, store, uid)
	if pack != 2 || sub != 0 {
		t.Fatalf("replay recharged: sub=%d pack=%d", sub, pack)
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
	sub, pack = diamondBalances(t, store, uid)
	if pack != 2 || sub != 0 {
		t.Fatalf("failure refunded or retried: sub=%d pack=%d", sub, pack)
	}
}

func TestFollowupReentersSharedQuota(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, uid := registerUser(t, ts, store, "follow")
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "原问题", "type": "normal"})
	id := asID(reading["id"])
	completeReading(t, store, id)
	msg, raw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "延续一下", "continuation": true})
	if msg != "" {
		t.Fatalf("followup: %s %s", msg, raw)
	}
	body := decodeData(t, raw)
	if body["entitlement"] != entDaily || body["charged"] != true {
		t.Fatalf("followup should charge daily: %#v", body)
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
	setDiamondCost(t, store, 2)
	setDiamonds(t, store, uid, 9, 6)
	msg, raw = call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "再追问", "continuation": true})
	if msg != "" {
		t.Fatalf("second followup: %s %s", msg, raw)
	}
	body = decodeData(t, raw)
	if body["entitlement"] != entPack || body["charged"] != true {
		t.Fatalf("second followup entitlement: %#v", body)
	}
	sub, pack := diamondBalances(t, store, uid)
	if pack != 4 || sub != 9 {
		t.Fatalf("followup diamond order: sub=%d pack=%d", sub, pack)
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

func TestAITrialThenFormalQuota(t *testing.T) {
	store, ts := newMergeServer(t)
	resetDiamondCost(t, store)
	token, uid := registerUser(t, ts, store, "trial")
	tarotistID := firstTarotist(t, ts, token)
	first := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "试用开场", "type": "tarotist", "tarotist_id": tarotistID})
	if first["entitlement"] != entTrial || first["reward_consumed"] == true {
		t.Fatalf("trial open: %#v", first)
	}
	if got := rewardRemaining(t, store, uid); got != 1 {
		t.Fatalf("trial consumed reward %d", got)
	}
	if n := quotaUseCount(t, store, uid); n != 0 {
		t.Fatalf("trial consumed daily %d", n)
	}
	id := asID(first["id"])
	completeReading(t, store, id)
	msg, raw := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", id), map[string]any{"text": "试用里的追问", "continuation": true})
	if msg != "" || strings.Contains(string(raw), "试用已用完") {
		t.Fatalf("trial followup: %s %s", msg, raw)
	}
	body := decodeData(t, raw)
	if body["charged"] == true || body["entitlement"] != entTrial {
		t.Fatalf("trial followup charged: %#v", body)
	}
	if n := followupCharges(t, store, id); n != 0 {
		t.Fatalf("trial followup charge rows %d", n)
	}
	if got := rewardRemaining(t, store, uid); got != 1 {
		t.Fatalf("trial followup took reward %d", got)
	}
	second := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "试用后的正式占卜", "type": "tarotist", "tarotist_id": tarotistID})
	if second["entitlement"] != entReward || second["reward_consumed"] != true {
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
	registered := create(t, ts, "", "/api/app/v1/auth/register", map[string]any{"email": email, "password": "demo1234", "anonymous_id": anonID})
	if registered["reward_consumed"] != true {
		t.Fatalf("login before answer did not charge: %#v", registered)
	}
	var ent string
	if err := store.QueryRow(t.Context(), `SELECT entitlement FROM quota_charges WHERE reading_id=$1 AND charge_key='play'`, asID(pending["id"])).Scan(&ent); err != nil {
		t.Fatal(err)
	}
	if ent != entReward {
		t.Fatalf("settled entitlement %s", ent)
	}

	_, wxRaw := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/wechat", map[string]any{"openid": fmt.Sprintf("wx-quota-%d", time.Now().UnixNano())})
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
	if !strings.Contains(string(quotaRaw), "次数") || !strings.Contains(string(quotaRaw), "钻石") {
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
	if got := rewardRemaining(t, store, uid); got != 1 {
		t.Fatalf("reward %d", got)
	}
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", id), map[string]any{"spread_id": spreadID})
	shuffled := create(t, ts, token, httpPath(id, "shuffle"), map[string]any{})
	if shuffled["status"] != "SHUFFLING" {
		t.Fatalf("shuffle: %#v", shuffled)
	}
	var ent string
	if err := store.QueryRow(t.Context(), `SELECT entitlement FROM quota_charges WHERE reading_id=$1 AND charge_key='play'`, id).Scan(&ent); err != nil {
		t.Fatal(err)
	}
	if ent != entReward {
		t.Fatalf("explore entitlement %s", ent)
	}
	msg, _ := call(t, ts, token, http.MethodPost, httpPath(id, "shuffle"), map[string]any{})
	if msg == "" {
		t.Fatal("second shuffle should not replay the ritual")
	}
	if got := rewardRemaining(t, store, uid); got != 0 {
		t.Fatalf("retry refunded or skipped reward %d", got)
	}
	if n := chargeCount(t, store, id); n != 1 {
		t.Fatalf("explore charges %d", n)
	}
	msg, _ = call(t, ts, token, http.MethodPost, httpPath(id, "interpret"), map[string]any{})
	if !strings.Contains(msg, "请先填写问题") {
		t.Fatalf("explore question gate: %s", msg)
	}
	must(t, ts, token, http.MethodPut, httpPath(id, "question"), map[string]any{"question": "牌已经抽了再问"})
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

	other, otherID := registerUser(t, ts, store, "tomorrow")
	exhaustFree(t, ts, other)
	setDiamondCost(t, store, 2)
	setDiamonds(t, store, otherID, 1, 2)
	msg, raw = call(t, ts, other, http.MethodPost, "/api/app/v1/daily", map[string]any{"target": "tomorrow"})
	if msg != "" {
		t.Fatalf("tomorrow: %s %s", msg, raw)
	}
	tomorrow := decodeData(t, raw)
	if tomorrow["question"] != "明日运势" {
		t.Fatalf("tomorrow reading: %#v", tomorrow)
	}
	sub, pack := diamondBalances(t, store, otherID)
	if pack != 0 || sub != 1 {
		t.Fatalf("tomorrow should spend pack first: sub=%d pack=%d", sub, pack)
	}
	tomorrowID := asID(tomorrow["id"])
	msg, _ = call(t, ts, other, http.MethodPost, "/api/app/v1/daily", map[string]any{"target": "tomorrow"})
	if !strings.Contains(msg, "不重复扣") {
		t.Fatalf("tomorrow reuse: %s", msg)
	}
	sub, pack = diamondBalances(t, store, otherID)
	if pack != 0 || sub != 1 || chargeCount(t, store, tomorrowID) != 1 {
		t.Fatalf("tomorrow recharged sub=%d pack=%d", sub, pack)
	}
}

func registerUser(t *testing.T, ts *httptest.Server, store *Store, prefix string) (string, int64) {
	t.Helper()
	email := fmt.Sprintf("%s-%d@tata.local", prefix, time.Now().UnixNano())
	token := register(t, ts, email)
	return token, userIDByEmail(t, store, email)
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
	if _, err := store.Exec(t.Context(), `INSERT INTO diamond_balances(user_id, subscription_diamonds, pack_diamonds)
		VALUES ($1,$2,$3)
		ON CONFLICT (user_id) DO UPDATE SET subscription_diamonds=$2, pack_diamonds=$3, updated_at=now()`, uid, sub, pack); err != nil {
		t.Fatal(err)
	}
}

func diamondBalances(t *testing.T, store *Store, uid int64) (int, int) {
	t.Helper()
	var sub, pack int
	err := store.QueryRow(t.Context(), `SELECT subscription_diamonds, pack_diamonds FROM diamond_balances WHERE user_id=$1`, uid).Scan(&sub, &pack)
	if err != nil {
		t.Fatal(err)
	}
	return sub, pack
}

func rewardRemaining(t *testing.T, store *Store, uid int64) int {
	t.Helper()
	var n int
	if err := store.QueryRow(t.Context(), `SELECT remaining FROM rewards WHERE user_id=$1`, uid).Scan(&n); err != nil {
		t.Fatal(err)
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
	create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "用掉注册奖励", "type": "normal"})
	create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "用掉今日免费", "type": "normal"})
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
