package tata

import (
	"net/http"
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
