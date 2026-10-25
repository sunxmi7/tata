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

func TestMergeConfirmAddsBalancesAndKeepsHistory(t *testing.T) {
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
	phone := fmt.Sprintf("13%09d", stamp%1_000_000_000)
	apple := fmt.Sprintf("apple-merge-%d", stamp)
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET wechat_openid=$1, phone=$2, apple_sub=$3 WHERE id=$4`, openID, phone, apple, idB); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `INSERT INTO paid_pools(user_id, subscription_remaining, pack_remaining) VALUES ($1,2,5),($2,3,7)`, idA, idB); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(30 * 24 * time.Hour).UTC()
	sooner := time.Now().Add(10 * 24 * time.Hour).UTC()
	var subA, subB int64
	if err := store.QueryRow(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at) VALUES ($1,'year','active',now(),$2) RETURNING id`, idA, later).Scan(&subA); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at) VALUES ($1,'month','active',now(),$2) RETURNING id`, idB, sooner).Scan(&subB); err != nil {
		t.Fatal(err)
	}
	readingA := insertReadingWithMessage(t, store, idA, "问题甲", "消息甲")
	readingB := insertReadingWithMessage(t, store, idB, "问题乙", "消息乙")
	if _, err := store.Exec(t.Context(), `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'registration_reward',1,'甲侧流水','consumed'),($3,$4,'daily_free',1,'乙侧流水','consumed')`, idA, readingA, idB, readingB); err != nil {
		t.Fatal(err)
	}

	msg, raw := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]any{"openid": openID, "confirm": true})
	if !strings.Contains(msg, "请求字段不正确") {
		t.Fatalf("confirm flag on bind must not be accepted: %s %s", msg, raw)
	}
	assertNotMerged(t, store, idA, idB, readingB)

	msg, raw = call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"openid": openID})
	conflict := decodeConflict(t, msg, raw)
	if conflict.MergeRequestID == 0 || conflict.PrimaryUserID != float64(idA) || conflict.SecondaryUserID != float64(idB) || conflict.Merged || !conflict.ConfirmRequired {
		t.Fatalf("conflict payload: %+v", conflict)
	}
	msg, raw = call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"openid": openID})
	again := decodeConflict(t, msg, raw)
	if again.MergeRequestID != conflict.MergeRequestID {
		t.Fatalf("second conflict created another request: %v vs %v", again.MergeRequestID, conflict.MergeRequestID)
	}
	var pending int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM account_merge_requests WHERE id=$1 AND status='pending_confirm'`, int64(conflict.MergeRequestID)).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("pending request: %d %v", pending, err)
	}
	assertNotMerged(t, store, idA, idB, readingB)
	var mergeAudits int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE action='account_merge' AND detail LIKE $1`, fmt.Sprintf("%%\"secondary_user_id\":%d%%", idB)).Scan(&mergeAudits); err != nil {
		t.Fatal(err)
	}
	if mergeAudits != 0 {
		t.Fatal("conflict wrote a merge audit")
	}

	msg, _ = call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/merge-confirm", map[string]any{"merge_request_id": conflict.MergeRequestID, "confirm": false})
	if !strings.Contains(msg, "未确认") {
		t.Fatalf("false confirm: %s", msg)
	}
	assertNotMerged(t, store, idA, idB, readingB)

	msg, _ = call(t, ts, tokenB, http.MethodPost, "/api/app/v1/auth/merge-confirm", map[string]any{"merge_request_id": conflict.MergeRequestID, "confirm": true})
	if !strings.Contains(msg, "只能确认自己发起的合并") {
		t.Fatalf("secondary confirm: %s", msg)
	}
	assertNotMerged(t, store, idA, idB, readingB)

	merged := create(t, ts, tokenA, "/api/app/v1/auth/merge-confirm", map[string]any{"merge_request_id": conflict.MergeRequestID, "confirm": true})
	assertMergeOutcome(t, merged, idA, idB, 2, 2, 0, 5, 12, false)
	if merged["identity_prompt_required"] != true {
		t.Fatalf("both emails should be retained for a client prompt: %#v", merged["retained_identities"])
	}
	againMerged := create(t, ts, tokenA, "/api/app/v1/auth/merge-confirm", map[string]any{"merge_request_id": conflict.MergeRequestID, "confirm": true})
	if againMerged["reward_remaining"].(float64) != 2 || againMerged["paid_subscription_remaining"].(float64) != 5 {
		t.Fatalf("second confirm changed balances: %#v", againMerged)
	}
	must(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"openid": openID})
	var rewardLeft int
	if err := store.QueryRow(t.Context(), `SELECT remaining FROM rewards WHERE user_id=$1`, idA).Scan(&rewardLeft); err != nil {
		t.Fatal(err)
	}
	if rewardLeft != 2 {
		t.Fatalf("rebind/repeat bind changed reward remaining: %d", rewardLeft)
	}

	var ownerA, ownerB int64
	if err := store.QueryRow(t.Context(), `SELECT user_id FROM readings WHERE id=$1`, readingA).Scan(&ownerA); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT user_id FROM readings WHERE id=$1`, readingB).Scan(&ownerB); err != nil {
		t.Fatal(err)
	}
	if ownerA != idA || ownerB != idA {
		t.Fatalf("readings owners %d %d, want primary %d", ownerA, ownerB, idA)
	}
	var secondaryReadings int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM readings WHERE user_id=$1`, idB).Scan(&secondaryReadings); err != nil {
		t.Fatal(err)
	}
	if secondaryReadings != 0 {
		t.Fatalf("secondary still owns readings: %d", secondaryReadings)
	}
	messages := list(t, ts, tokenA, fmt.Sprintf("/api/app/v1/readings/%d/messages", readingB))
	if len(messages) != 1 || messages[0]["content"] != "消息乙" {
		t.Fatalf("secondary messages not readable by primary: %#v", messages)
	}
	var messageCount int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM reading_messages WHERE reading_id IN ($1,$2)`, readingA, readingB).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if messageCount != 2 {
		t.Fatalf("messages dropped: %d", messageCount)
	}
	var ledgerOnPrimary int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM ledger WHERE user_id=$1 AND reading_id IN ($2,$3)`, idA, readingA, readingB).Scan(&ledgerOnPrimary); err != nil {
		t.Fatal(err)
	}
	if ledgerOnPrimary != 2 {
		t.Fatalf("ledger rows not both on primary: %d", ledgerOnPrimary)
	}

	var emailANow, emailBNow, wechatA, phoneA, appleA *string
	if err := store.QueryRow(t.Context(), `SELECT email, wechat_openid, phone, apple_sub FROM app_users WHERE id=$1`, idA).Scan(&emailANow, &wechatA, &phoneA, &appleA); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT email FROM app_users WHERE id=$1`, idB).Scan(&emailBNow); err != nil {
		t.Fatal(err)
	}
	if emailANow == nil || *emailANow != emailA || emailBNow != nil {
		t.Fatalf("emails after merge primary=%v secondary=%v", emailANow, emailBNow)
	}
	if wechatA == nil || *wechatA != openID || phoneA == nil || *phoneA != phone || appleA == nil || *appleA != apple {
		t.Fatalf("non-conflicting identities not moved: wechat=%v phone=%v apple=%v", wechatA, phoneA, appleA)
	}
	var retained string
	if err := store.QueryRow(t.Context(), `SELECT value FROM retained_identities WHERE merge_request_id=$1 AND kind='email'`, int64(conflict.MergeRequestID)).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != emailB {
		t.Fatalf("retained email %s", retained)
	}

	var stateA, stateB string
	var subUsers int
	if err := store.QueryRow(t.Context(), `SELECT merge_state FROM subscriptions WHERE id=$1`, subA).Scan(&stateA); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT merge_state FROM subscriptions WHERE id=$1`, subB).Scan(&stateB); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM subscriptions WHERE id IN ($1,$2) AND user_id=$3`, subA, subB, idA).Scan(&subUsers); err != nil {
		t.Fatal(err)
	}
	if stateA != "selected" || stateB != "superseded" || subUsers != 2 {
		t.Fatalf("subscription states %s %s owned %d", stateA, stateB, subUsers)
	}

	quota := listOne(t, ts, tokenA, "/api/app/v1/quota")
	if quota["reward_remaining"].(float64) != 2 || quota["daily_free_remaining"].(float64) != 2 || quota["daily_used_flags"].(float64) != 0 {
		t.Fatalf("quota after merge: %#v", quota)
	}
	if quota["paid_subscription_remaining"].(float64) != 5 || quota["paid_pack_remaining"].(float64) != 12 || quota["subscription_prompt_required"] != false {
		t.Fatalf("paid/subscription quota: %#v", quota)
	}
	create(t, ts, tokenA, "/api/app/v1/readings", map[string]any{"question": "合并后再问", "type": "normal"})
	quota = listOne(t, ts, tokenA, "/api/app/v1/quota")
	if quota["reward_remaining"].(float64) != 1 {
		t.Fatalf("summed reward was not spendable, remaining=%v", quota["reward_remaining"])
	}

	var action, detail, actorKind string
	var operatorID int64
	if err := store.QueryRow(t.Context(), `SELECT operator_id, action, detail, actor_kind FROM audit_log WHERE action='account_merge' AND detail LIKE $1 ORDER BY id DESC LIMIT 1`, fmt.Sprintf("%%\"secondary_user_id\":%d%%", idB)).Scan(&operatorID, &action, &detail, &actorKind); err != nil {
		t.Fatal(err)
	}
	if operatorID != idA || actorKind != "app_user" || !strings.Contains(detail, `"confirm_at"`) || !strings.Contains(detail, fmt.Sprintf(`"primary_user_id":%d`, idA)) || !strings.Contains(detail, `"merge_result"`) {
		t.Fatalf("audit operator=%d kind=%s detail=%s", operatorID, actorKind, detail)
	}
	audits := list(t, ts, admin.access, "/api/admin/v1/audit")
	seenAudit := false
	for _, row := range audits {
		if row["action"] == "account_merge" && strings.Contains(fmt.Sprint(row["detail"]), fmt.Sprintf(`"secondary_user_id":%d`, idB)) {
			seenAudit = true
		}
	}
	if !seenAudit {
		t.Fatal("admin audit list missing merge")
	}
	merges := list(t, ts, admin.access, "/api/admin/v1/account-merges")
	seenMerge := false
	for _, row := range merges {
		if row["status"] == "merged" && row["primary_user_id"].(float64) == float64(idA) && row["secondary_user_id"].(float64) == float64(idB) {
			if row["confirm_at"] == nil || !strings.Contains(fmt.Sprint(row["result"]), `"reward_remaining":2`) {
				t.Fatalf("admin merge row missing confirm/result: %#v", row)
			}
			seenMerge = true
		}
	}
	if !seenMerge {
		t.Fatal("admin account-merges missing row")
	}
	if msg, _ := call(t, ts, tokenB, http.MethodGet, "/api/app/v1/profile", nil); msg != "401" {
		t.Fatalf("merged-away token still works: %s", msg)
	}
}

func TestMergeDailyFlagsSum(t *testing.T) {
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
	msg, raw := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"openid": openID})
	conflict := decodeConflict(t, msg, raw)
	merged := create(t, ts, tokenA, "/api/app/v1/auth/merge-confirm", map[string]any{"merge_request_id": conflict.MergeRequestID, "confirm": true})
	if merged["daily_used_flags"].(float64) != 1 || merged["daily_free_remaining"].(float64) != 1 {
		t.Fatalf("daily flags not summed: %#v", merged)
	}
	var uses int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM daily_free_uses WHERE source_user_id=$1 AND reading_id=$2 AND kind='quota_use' AND user_id=$3`, idB, readingB, idA).Scan(&uses); err != nil {
		t.Fatal(err)
	}
	if uses != 1 {
		t.Fatalf("used daily flag reading dropped: %d", uses)
	}
	quota := listOne(t, ts, tokenA, "/api/app/v1/quota")
	if quota["daily_free_used"] != false || quota["daily_free_remaining"].(float64) != 1 || quota["daily_used_flags"].(float64) != 1 {
		t.Fatalf("quota hid the unused side: %#v", quota)
	}
}

func TestMergeSubscriptionTiePrompts(t *testing.T) {
	store, ts := newMergeServer(t)
	stamp := time.Now().UnixNano()
	emailA := fmt.Sprintf("sub-a-%d@tata.local", stamp)
	emailB := fmt.Sprintf("sub-b-%d@tata.local", stamp)
	tokenA := register(t, ts, emailA)
	_ = register(t, ts, emailB)
	idA := userIDByEmail(t, store, emailA)
	idB := userIDByEmail(t, store, emailB)
	openID := fmt.Sprintf("wx-sub-%d", stamp)
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET wechat_openid=$1 WHERE id=$2`, openID, idB); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(20 * 24 * time.Hour).UTC().Truncate(time.Microsecond)
	var subA, subB int64
	if err := store.QueryRow(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at) VALUES ($1,'month','active',now(),$2) RETURNING id`, idA, exp).Scan(&subA); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `INSERT INTO subscriptions(user_id, sku_kind, status, starts_at, expires_at) VALUES ($1,'year','active',now(),$2) RETURNING id`, idB, exp).Scan(&subB); err != nil {
		t.Fatal(err)
	}
	msg, raw := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"openid": openID})
	conflict := decodeConflict(t, msg, raw)
	merged := create(t, ts, tokenA, "/api/app/v1/auth/merge-confirm", map[string]any{"merge_request_id": conflict.MergeRequestID, "confirm": true})
	if merged["subscription_prompt_required"] != true || merged["subscription_resolution"] != "prompt_required" {
		t.Fatalf("tie should prompt: %#v", merged)
	}
	var n int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM subscriptions WHERE id IN ($1,$2) AND user_id=$3 AND merge_state='prompt_required'`, subA, subB, idA).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("tied subscriptions not both kept for prompt: %d", n)
	}
	var flagged bool
	if err := store.QueryRow(t.Context(), `SELECT subscription_prompt_required FROM app_users WHERE id=$1`, idA).Scan(&flagged); err != nil {
		t.Fatal(err)
	}
	if !flagged {
		t.Fatal("prompt flag not stored")
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
		{kind: "apple", path: "/api/app/v1/auth/bind-apple", field: "sub", value: fmt.Sprintf("apple-%d", stamp)},
		{kind: "phone", path: "/api/app/v1/auth/bind-phone", field: "phone", value: fmt.Sprintf("15%09d", stamp%1_000_000_000)},
		{kind: "wechat", path: "/api/app/v1/auth/bind-wechat", field: "openid", value: fmt.Sprintf("wx-silent-%d", stamp)},
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
			conflict := decodeConflict(t, msg, raw)
			if conflict.Merged || !conflict.ConfirmRequired || conflict.SecondaryUserID != float64(idB) {
				t.Fatalf("%s conflict: %+v", tc.kind, conflict)
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
		tokenA, err := (&App{Store: store, JWTSecret: []byte("test-secret")}).signAccess("app", idA, "", "")
		if err != nil {
			t.Fatal(err)
		}
		_ = register(t, ts, emailB)
		idB := userIDByEmail(t, store, emailB)
		readingB := insertReadingWithMessage(t, store, idB, "邮箱冲突", "邮箱消息")
		msg, raw := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-email", map[string]string{"email": emailB, "password": "new-pass-123"})
		conflict := decodeConflict(t, msg, raw)
		if conflict.Merged || conflict.SecondaryUserID != float64(idB) {
			t.Fatalf("email conflict: %+v", conflict)
		}
		assertNotMerged(t, store, idA, idB, readingB)
		merged := create(t, ts, tokenA, "/api/app/v1/auth/merge-confirm", map[string]any{"merge_request_id": conflict.MergeRequestID, "confirm": true})
		if merged["merged"] != true {
			t.Fatalf("email merge: %#v", merged)
		}
		var got *string
		if err := store.QueryRow(t.Context(), `SELECT email FROM app_users WHERE id=$1`, idA).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got == nil || !strings.EqualFold(*got, emailB) {
			t.Fatalf("email not moved: %v", got)
		}
		loginMsg, _ := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/login", map[string]string{"email": emailB, "password": "new-pass-123"})
		if loginMsg != "" {
			t.Fatalf("moved email login: %s", loginMsg)
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
	msg, _ := call(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"openid": other})
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

func newMergeServer(t *testing.T) (*Store, *httptest.Server) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://tata:tata@127.0.0.1:54329/tata?sslmode=disable"
	}
	store, err := Open(t.Context(), dsn, []byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{Store: store, JWTSecret: []byte("test-secret")}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	return store, ts
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
	if err := store.QueryRow(t.Context(), `SELECT remaining FROM rewards WHERE user_id=$1`, idB).Scan(&rewardB); err != nil {
		t.Fatal(err)
	}
	if rewardB != 1 {
		t.Fatalf("secondary reward changed before confirm: %d", rewardB)
	}
	var audits int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM audit_log WHERE action='account_merge' AND operator_id=$1 AND detail LIKE $2`, idA, fmt.Sprintf("%%\"secondary_user_id\":%d%%", idB)).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Fatal("merge audit exists before confirm")
	}
}

func assertMergeOutcome(t *testing.T, merged map[string]any, idA, idB int64, reward, dailyRemaining, dailyUsed, subRemain, packRemain int, subPrompt bool) {
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
		"reward_remaining":            float64(reward),
		"daily_free_remaining":        float64(dailyRemaining),
		"daily_used_flags":            float64(dailyUsed),
		"paid_subscription_remaining": float64(subRemain),
		"paid_pack_remaining":         float64(packRemain),
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
