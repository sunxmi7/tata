package tata

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestCancelDisablesLogin(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "cancel")
	must(t, ts, token, http.MethodPut, "/api/app/v1/profile", map[string]any{"name": "将注销"})
	must(t, ts, token, http.MethodPost, "/api/app/v1/auth/cancel", map[string]any{})

	if msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/profile", nil); msg != "账号已注销" {
		t.Fatalf("token after cancel: %s", msg)
	}

	var cancelledAt any
	if err := store.QueryRow(t.Context(), `SELECT cancelled_at FROM app_users WHERE id=$1`, uid).Scan(&cancelledAt); err != nil {
		t.Fatal(err)
	}
	if cancelledAt == nil {
		t.Fatal("cancelled_at not set")
	}

	var email string
	if err := store.QueryRow(t.Context(), `SELECT COALESCE(email,'') FROM app_users WHERE id=$1`, uid).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "" {
		t.Fatalf("email still bound: %q", email)
	}
}

func TestCancelBlocksSameEmail(t *testing.T) {
	_, app, ts := newMergeApp(t)

	email := uniqueEmail("cancel-same")
	otp := sendEmailCode(t, ts, "", email, "login")
	session := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{"email": email, "code": otp})
	token, _ := session["accessToken"].(string)
	if token == "" {
		t.Fatalf("login: %#v", session)
	}
	must(t, ts, token, http.MethodPost, "/api/app/v1/auth/cancel", map[string]any{})

	_ = app.KV.Del("tata:mail:cd:login:" + strings.ToLower(email))
	otp = sendEmailCode(t, ts, "", email, "login")
	msg, _ := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/login", map[string]any{"email": email, "code": otp})
	if msg != "该邮箱已注销，不能重复注册" {
		t.Fatalf("re-register after cancel: %s", msg)
	}
}

func TestSoftDeleteDeviceSkipsGiftAndInvite(t *testing.T) {
	store, ts := newMergeServer(t)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE quota_settings SET registration_grant_delay_minutes=NULL, registration_valid_days=3, registration_grant_count=1 WHERE id=1`)
	})
	if _, err := store.Exec(t.Context(), `UPDATE quota_settings SET registration_grant_delay_minutes=0, registration_valid_days=3, registration_grant_count=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	login := func(email, device string) (string, int64) {
		t.Helper()
		otp := sendEmailCode(t, ts, "", email, "login")
		session := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{"email": email, "code": otp, "device_id": device})
		token, _ := session["accessToken"].(string)
		if token == "" {
			t.Fatalf("login %s: %#v", email, session)
		}
		return token, asID(session["id"])
	}
	giftRemaining := func(uid int64) (int, bool) {
		t.Helper()
		var remaining int
		var blocked bool
		if err := store.QueryRow(t.Context(), `SELECT remaining, reward_blocked FROM registration_gifts WHERE user_id=$1`, uid).Scan(&remaining, &blocked); err != nil {
			t.Fatal(err)
		}
		return remaining, blocked
	}

	tokenA, uidA := login(uniqueEmail("soft-a"), "device-soft-1")
	if remaining, blocked := giftRemaining(uidA); remaining != 1 || blocked {
		t.Fatalf("first gift remaining=%d blocked=%v", remaining, blocked)
	}
	must(t, ts, tokenA, http.MethodPost, "/api/app/v1/readings", map[string]any{"question": "注销前的记录", "type": "normal"})
	must(t, ts, tokenA, http.MethodPost, "/api/app/v1/auth/cancel", map[string]any{})

	var still int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM app_users WHERE id=$1 AND cancelled_at IS NOT NULL`, uidA).Scan(&still); err != nil {
		t.Fatal(err)
	}
	if still != 1 {
		t.Fatal("cancelled account row was removed")
	}
	var readings int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM readings WHERE user_id=$1`, uidA).Scan(&readings); err != nil {
		t.Fatal(err)
	}
	if readings != 1 {
		t.Fatalf("reading removed on cancel: %d", readings)
	}
	var devices int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM identity_claims WHERE kind='device' AND value='device-soft-1'`).Scan(&devices); err != nil {
		t.Fatal(err)
	}
	if devices != 1 {
		t.Fatal("device was not kept after cancel")
	}

	tokenB, uidB := login(uniqueEmail("soft-b"), "device-soft-1")
	if remaining, blocked := giftRemaining(uidB); remaining != 0 || !blocked {
		t.Fatalf("repeat device gift remaining=%d blocked=%v", remaining, blocked)
	}
	_, inviterID := login(uniqueEmail("soft-inviter"), "device-soft-inviter")
	msg, _ := call(t, ts, tokenB, http.MethodPost, "/api/app/v1/invites/accept", map[string]any{"code": strconv.FormatInt(inviterID, 10)})
	if !strings.Contains(msg, "不能再领邀请奖励") {
		t.Fatalf("invite: %s", msg)
	}

	_, uidC := login(uniqueEmail("soft-c"), "device-soft-2")
	if remaining, blocked := giftRemaining(uidC); remaining != 1 || blocked {
		t.Fatalf("fresh device gift remaining=%d blocked=%v", remaining, blocked)
	}
}
