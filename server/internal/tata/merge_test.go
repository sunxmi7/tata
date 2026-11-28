package tata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDecideSubscriptions(t *testing.T) {
	now := time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC)
	sooner := now.Add(10 * 24 * time.Hour)
	later := now.Add(30 * 24 * time.Hour)
	picked := decideSubscriptions([]subscriptionRow{
		{ID: 1, Status: "active", ExpiresAt: sooner},
		{ID: 2, Status: "active", ExpiresAt: later},
	}, now)
	if picked.Prompt || picked.Resolution != "selected" || picked.WinnerID != 2 || len(picked.Superseded) != 1 || picked.Superseded[0] != 1 {
		t.Fatalf("longest: %+v", picked)
	}
	tie := decideSubscriptions([]subscriptionRow{
		{ID: 3, Status: "active", ExpiresAt: later},
		{ID: 4, Status: "active", ExpiresAt: later},
		{ID: 5, Status: "active", ExpiresAt: sooner},
	}, now)
	if !tie.Prompt || tie.Resolution != "prompt_required" || tie.WinnerID != 0 || len(tie.PromptIDs) != 2 || len(tie.Superseded) != 1 {
		t.Fatalf("tie: %+v", tie)
	}
	none := decideSubscriptions([]subscriptionRow{{ID: 6, Status: "active", ExpiresAt: now.Add(-time.Hour)}}, now)
	if none.Prompt || none.Resolution != "none" {
		t.Fatalf("expired: %+v", none)
	}
	closed := decideSubscriptions([]subscriptionRow{{ID: 7, Status: "expired", ExpiresAt: later}}, now)
	if closed.Resolution != "none" || closed.Prompt {
		t.Fatalf("non-active: %+v", closed)
	}
}

func TestAccountsStayIndependent(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	stamp := time.Now().UnixNano()
	emailA := fmt.Sprintf("merge-a-%d@tata.local", stamp)
	emailB := fmt.Sprintf("merge-b-%d@tata.local", stamp)
	tokenA := register(t, ts, emailA)
	tokenB := register(t, ts, emailB)
	idA := userIDByEmail(t, store, emailA)
	idB := userIDByEmail(t, store, emailB)
	openID := fmt.Sprintf("wx-merge-%d", stamp)
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET wechat_openid=$1 WHERE id=$2`, openID, idB); err != nil {
		t.Fatal(err)
	}
	readingB := insertReadingWithMessage(t, store, idB, "问题乙", "消息乙")
	setQuota(t, store, idA, 7)
	setQuota(t, store, idB, 4)

	msg, raw := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"code": openID})
	if !strings.Contains(msg, "已绑定其他账号") {
		t.Fatalf("bind occupied: %s %s", msg, raw)
	}
	var requests int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM account_merge_requests WHERE primary_user_id=$1 OR secondary_user_id=$2`, idA, idB).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatalf("merge request created: %d", requests)
	}
	assertNotMerged(t, store, idA, idB, readingB)
	if quotaOf(t, store, idA) != 7 || quotaOf(t, store, idB) != 4 {
		t.Fatalf("balances moved: %d %d", quotaOf(t, store, idA), quotaOf(t, store, idB))
	}
	msg, _ = call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/merge-confirm", map[string]any{"merge_request_id": 1, "confirm": true})
	if !strings.Contains(msg, "不提供账号合并") {
		t.Fatalf("confirm: %s", msg)
	}
	msg, _ = call(t, ts, admin.access, http.MethodGet, "/api/admin/v1/account-merges", nil)
	if !strings.Contains(msg, "不提供账号合并") {
		t.Fatalf("admin list: %s", msg)
	}
	if msg, _ = call(t, ts, tokenB, http.MethodGet, "/api/app/v1/profile", nil); msg != "" {
		t.Fatalf("other account locked: %s", msg)
	}
	var still *string
	if err := store.QueryRow(t.Context(), `SELECT wechat_openid FROM app_users WHERE id=$1`, idB).Scan(&still); err != nil {
		t.Fatal(err)
	}
	if still == nil || *still != openID {
		t.Fatalf("identity moved: %v", still)
	}
}

func TestOccupiedIdentityDoesNotMoveDailyOrSubscription(t *testing.T) {
	store, ts := newMergeServer(t)
	stamp := time.Now().UnixNano()
	emailA := fmt.Sprintf("daily-a-%d@tata.local", stamp)
	emailB := fmt.Sprintf("daily-b-%d@tata.local", stamp)
	tokenA := register(t, ts, emailA)
	_ = register(t, ts, emailB)
	idA := userIDByEmail(t, store, emailA)
	idB := userIDByEmail(t, store, emailB)
	openID := fmt.Sprintf("wx-daily-%d", stamp)
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET wechat_openid=$1 WHERE id=$2`, openID, idB); err != nil {
		t.Fatal(err)
	}
	cycle := cycleOf(time.Now())
	readingB := insertReadingWithMessage(t, store, idB, "乙已用每日", "乙消息")
	if _, err := store.Exec(t.Context(), `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3)`, idB, cycle, readingB); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(20 * 24 * time.Hour).UTC()
	if _, err := store.Exec(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at) VALUES ($1,'month','active',now(),$2),($3,'year','active',now(),$2)`, idA, exp, idB); err != nil {
		t.Fatal(err)
	}
	msg, _ := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"code": openID})
	if !strings.Contains(msg, "已绑定其他账号") {
		t.Fatalf("bind: %s", msg)
	}
	var owner int64
	if err := store.QueryRow(t.Context(), `SELECT user_id FROM quota_use WHERE reading_id=$1`, readingB).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != idB {
		t.Fatalf("daily use moved to %d", owner)
	}
	var subsA, subsB int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM subscriptions WHERE user_id=$1 AND status='active'`, idA).Scan(&subsA); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM subscriptions WHERE user_id=$1 AND status='active'`, idB).Scan(&subsB); err != nil {
		t.Fatal(err)
	}
	if subsA != 1 || subsB != 1 {
		t.Fatalf("subscriptions moved: %d %d", subsA, subsB)
	}
}

func TestBindConflictDoesNotSilentlyMerge(t *testing.T) {
	store, ts := newMergeServer(t)
	stamp := time.Now().UnixNano()
	cases := []struct {
		kind  string
		path  string
		field string
		value string
	}{
		{kind: "apple", path: "/api/app/v1/auth/bind-apple", field: "identity_token", value: fmt.Sprintf("apple-%d", stamp)},
		{kind: "wechat", path: "/api/app/v1/auth/bind-wechat", field: "code", value: fmt.Sprintf("wx-silent-%d", stamp)},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			emailA := fmt.Sprintf("%s-a-%d@tata.local", tc.kind, stamp)
			emailB := fmt.Sprintf("%s-b-%d@tata.local", tc.kind, stamp)
			tokenA := register(t, ts, emailA)
			_ = register(t, ts, emailB)
			idA := userIDByEmail(t, store, emailA)
			idB := userIDByEmail(t, store, emailB)
			col := identityColumns[tc.kind]
			if _, err := store.Exec(t.Context(), fmt.Sprintf(`UPDATE app_users SET %s=$1 WHERE id=$2`, col), tc.value, idB); err != nil {
				t.Fatal(err)
			}
			readingB := insertReadingWithMessage(t, store, idB, tc.kind, tc.kind+"-msg")
			msg, raw := call(t, ts, tokenA, http.MethodPost, tc.path, map[string]string{tc.field: tc.value})
			if !strings.Contains(msg, "已绑定其他账号") {
				t.Fatalf("%s bind: %s %s", tc.kind, msg, raw)
			}
			assertNotMerged(t, store, idA, idB, readingB)
		})
	}

	t.Run("email", func(t *testing.T) {
		wechatA := fmt.Sprintf("wx-email-host-%d", stamp)
		emailB := fmt.Sprintf("email-slot-%d@tata.local", stamp)
		var idA int64
		if err := store.QueryRow(t.Context(), `INSERT INTO app_users(wechat_openid) VALUES ($1) RETURNING id`, wechatA).Scan(&idA); err != nil {
			t.Fatal(err)
		}
		tokenA, err := (&App{Store: store, JWTSecret: []byte("test-secret")}).signAccess("app", idA, "")
		if err != nil {
			t.Fatal(err)
		}
		_ = register(t, ts, emailB)
		idB := userIDByEmail(t, store, emailB)
		readingB := insertReadingWithMessage(t, store, idB, "邮箱冲突", "邮箱消息")
		bindCode := sendEmailCode(t, ts, tokenA, emailB, "bind")
		msg, raw := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-email", map[string]string{"email": emailB, "code": bindCode})
		if !strings.Contains(msg, "已绑定其他账号") {
			t.Fatalf("email bind: %s %s", msg, raw)
		}
		assertNotMerged(t, store, idA, idB, readingB)
		var got *string
		if err := store.QueryRow(t.Context(), `SELECT email FROM app_users WHERE id=$1`, idA).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("email moved: %v", got)
		}
	})
}

func TestBindDoesNotRebind(t *testing.T) {
	store, ts := newMergeServer(t)
	stamp := time.Now().UnixNano()
	emailA := fmt.Sprintf("rebind-a-%d@tata.local", stamp)
	emailB := fmt.Sprintf("rebind-b-%d@tata.local", stamp)
	tokenA := register(t, ts, emailA)
	_ = register(t, ts, emailB)
	idA := userIDByEmail(t, store, emailA)
	idB := userIDByEmail(t, store, emailB)
	keep := fmt.Sprintf("wx-keep-%d", stamp)
	other := fmt.Sprintf("wx-other-%d", stamp)
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET wechat_openid=$1 WHERE id=$2`, keep, idA); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET wechat_openid=$1 WHERE id=$2`, other, idB); err != nil {
		t.Fatal(err)
	}
	readingB := insertReadingWithMessage(t, store, idB, "换绑", "不应合并")
	msg, _ := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"code": other})
	if !strings.Contains(msg, "换绑未开放") {
		t.Fatalf("rebind message: %s", msg)
	}
	assertNotMerged(t, store, idA, idB, readingB)
	var still *string
	if err := store.QueryRow(t.Context(), `SELECT wechat_openid FROM app_users WHERE id=$1`, idA).Scan(&still); err != nil {
		t.Fatal(err)
	}
	if still == nil || *still != keep {
		t.Fatalf("wechat overwritten: %v", still)
	}
}

func newMergeApp(t *testing.T) (*Store, *App, *httptest.Server) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://tata:tata@127.0.0.1:54329/tata?sslmode=disable"
	}
	store, err := Open(t.Context(), dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	protectLiveAIConfig(t, store)
	app := testApp(store)
	ts := httptest.NewServer(app.Handler())
	rememberTestApp(ts.URL, app, t)
	t.Cleanup(ts.Close)
	return store, app, ts
}

func newMergeServer(t *testing.T) (*Store, *httptest.Server) {
	store, _, ts := newMergeApp(t)
	return store, ts
}

func absAsset(ts *httptest.Server, path string) string {
	return strings.TrimRight(ts.URL, "/") + path
}

func userIDByEmail(t *testing.T, store *Store, email string) int64 {
	t.Helper()
	var id int64
	if err := store.QueryRow(t.Context(), `SELECT id FROM app_users WHERE lower(email)=lower($1)`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertReadingWithMessage(t *testing.T, store *Store, uid int64, question, content string) int64 {
	t.Helper()
	var id int64
	if err := store.QueryRow(t.Context(), `INSERT INTO readings(user_id, type, question, status) VALUES ($1,'normal',$2,'QUESTION_CREATED') RETURNING id`, uid, question).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO reading_messages(reading_id, turn_id, seq, role, content) VALUES ($1,1,0,'user',$2)`, id, content); err != nil {
		t.Fatal(err)
	}
	return id
}

type conflictBody struct {
	Conflict        bool    `json:"conflict"`
	Merged          bool    `json:"merged"`
	ConfirmRequired bool    `json:"confirm_required"`
	MergeRequestID  float64 `json:"merge_request_id"`
	PrimaryUserID   float64 `json:"primary_user_id"`
	SecondaryUserID float64 `json:"secondary_user_id"`
}

func decodeConflict(t *testing.T, msg string, raw []byte) conflictBody {
	t.Helper()
	if !strings.Contains(msg, "需要确认") {
		t.Fatalf("expected confirm message, got %s raw %s", msg, raw)
	}
	var env struct {
		Code int          `json:"code"`
		Data conflictBody `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.Code == 0 || !env.Data.Conflict || env.Data.Merged {
		t.Fatalf("silent or success bind: %+v", env)
	}
	return env.Data
}

func assertNotMerged(t *testing.T, store *Store, idA, idB, readingB int64) {
	t.Helper()
	var owner int64
	if err := store.QueryRow(t.Context(), `SELECT user_id FROM readings WHERE id=$1`, readingB).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != idB {
		t.Fatalf("reading moved without confirm: owner %d", owner)
	}
	var merged *int64
	if err := store.QueryRow(t.Context(), `SELECT merged_into FROM app_users WHERE id=$1`, idB).Scan(&merged); err != nil {
		t.Fatal(err)
	}
	if merged != nil {
		t.Fatal("secondary merged without confirm")
	}
	var rewardB int
	err := store.QueryRow(t.Context(), `SELECT remaining FROM rewards WHERE user_id=$1`, idB).Scan(&rewardB)
	if err != nil {
		rewardB = 0
	}
	if rewardB != 0 {
		t.Fatalf("secondary registration reward should stay unused: %d", rewardB)
	}
	var audits int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE action='account_merge' AND operator_id=$1 AND detail LIKE $2`, idA, fmt.Sprintf("%%\"secondary_user_id\":%d%%", idB)).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Fatal("merge audit exists before confirm")
	}
	var requests int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM account_merge_requests WHERE primary_user_id=$1 OR secondary_user_id=$1 OR primary_user_id=$2 OR secondary_user_id=$2`, idA, idB).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatalf("bind created a merge request: %d", requests)
	}
}

func assertMergeOutcome(t *testing.T, merged map[string]any, idA, idB int64, reward, dailyRemaining, dailyUsed int, subPrompt bool) {
	t.Helper()
	if merged["merged"] != true {
		t.Fatalf("not merged: %#v", merged)
	}
	before := merged["before"].(map[string]any)
	after := merged["after"].(map[string]any)
	if before["primary_user_id"].(float64) != float64(idA) || before["secondary_user_id"].(float64) != float64(idB) {
		t.Fatalf("before ids: %#v", before)
	}
	if after["surviving_user_id"].(float64) != float64(idA) || after["primary_user_id"].(float64) != float64(idA) || after["secondary_user_id"].(float64) != float64(idB) {
		t.Fatalf("after ids: %#v", after)
	}
	if merged["confirm_at"] == nil || merged["confirm_at"] == "" {
		t.Fatal("missing confirm_at")
	}
	checks := map[string]float64{
		"reward_remaining":     float64(reward),
		"daily_free_remaining": float64(dailyRemaining),
		"daily_used_flags":     float64(dailyUsed),
	}
	for key, want := range checks {
		got, ok := merged[key].(float64)
		if !ok || got != want {
			t.Fatalf("%s=%v want %v", key, merged[key], want)
		}
	}
	if merged["subscription_prompt_required"] != subPrompt {
		t.Fatalf("subscription prompt=%v", merged["subscription_prompt_required"])
	}
}
