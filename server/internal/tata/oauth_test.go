package tata

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestWeChatClientNotReady(t *testing.T) {
	off := NewWeChatClient(WeChatConfig{})
	if off.Ready() {
		t.Fatal("disabled should not be ready")
	}
	empty := NewWeChatClient(WeChatConfig{Enabled: true})
	if empty.Ready() {
		t.Fatal("empty creds should not be ready")
	}
	_, err := empty.ExchangeCode(context.Background(), "code")
	if err == nil || !strings.Contains(err.Error(), "微信登录未配置") {
		t.Fatalf("unconfigured: %v", err)
	}
}

func TestWeChatExchangeUnionID(t *testing.T) {
	var gotAppID, gotSecret, gotCode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAppID = r.URL.Query().Get("appid")
		gotSecret = r.URL.Query().Get("secret")
		gotCode = r.URL.Query().Get("code")
		_ = json.NewEncoder(w).Encode(map[string]any{"openid": "oid-1", "unionid": "uid-1"})
	}))
	t.Cleanup(srv.Close)
	c := NewWeChatClient(WeChatConfig{Enabled: true, AppID: "wx", AppSecret: "secret"})
	c.tokenURL = srv.URL
	id, err := c.ExchangeCode(context.Background(), " code-1 ")
	if err != nil {
		t.Fatal(err)
	}
	if id != "uid-1" || gotAppID != "wx" || gotSecret != "secret" || gotCode != "code-1" {
		t.Fatalf("exchange: id=%s appid=%s secret=%s code=%s", id, gotAppID, gotSecret, gotCode)
	}
}

func TestWeChatExchangeOpenIDAndInvalidCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "bad" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 40029, "errmsg": "invalid code"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"openid": "oid-only"})
	}))
	t.Cleanup(srv.Close)
	c := NewWeChatClient(WeChatConfig{Enabled: true, AppID: "wx", AppSecret: "secret"})
	c.tokenURL = srv.URL
	id, err := c.ExchangeCode(context.Background(), "ok")
	if err != nil || id != "oid-only" {
		t.Fatalf("openid fallback: %s %v", id, err)
	}
	_, err = c.ExchangeCode(context.Background(), "bad")
	if err == nil || !strings.Contains(err.Error(), "微信授权码无效") {
		t.Fatalf("invalid code: %v", err)
	}
	_, err = c.ExchangeCode(context.Background(), "  ")
	if err == nil || !strings.Contains(err.Error(), "微信授权码不能为空") {
		t.Fatalf("empty code: %v", err)
	}
}

func TestAppleVerifierNotReady(t *testing.T) {
	off := NewAppleVerifier(AppleConfig{})
	if off.Ready() {
		t.Fatal("disabled should not be ready")
	}
	empty := NewAppleVerifier(AppleConfig{Enabled: true})
	if empty.Ready() {
		t.Fatal("empty clientId should not be ready")
	}
	_, err := empty.VerifyIdentityToken(context.Background(), "tok")
	if err == nil || !strings.Contains(err.Error(), "苹果登录未配置") {
		t.Fatalf("unconfigured: %v", err)
	}
}

func TestAppleVerifyIdentityToken(t *testing.T) {
	priv, jwks := appleTestKey(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)
	v := NewAppleVerifier(AppleConfig{Enabled: true, ClientID: "com.tata.app"})
	v.jwksURL = srv.URL
	tok := signAppleToken(t, priv, "apple-sub-1", appleIssuer, "com.tata.app")
	sub, err := v.VerifyIdentityToken(context.Background(), tok)
	if err != nil || sub != "apple-sub-1" {
		t.Fatalf("verify: %s %v", sub, err)
	}
	badAud := signAppleToken(t, priv, "apple-sub-1", appleIssuer, "com.other.app")
	_, err = v.VerifyIdentityToken(context.Background(), badAud)
	if err == nil || !strings.Contains(err.Error(), "苹果身份令牌无效") {
		t.Fatalf("bad aud: %v", err)
	}
	_, err = v.VerifyIdentityToken(context.Background(), "not-a-jwt")
	if err == nil || !strings.Contains(err.Error(), "苹果身份令牌无效") {
		t.Fatalf("garbage: %v", err)
	}
}

func TestWechatAppleUnconfiguredHTTP(t *testing.T) {
	store, _ := newMergeServer(t)
	app := &App{Store: store, JWTSecret: []byte("test-secret")}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	msg, _ := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/wechat", map[string]string{"code": "wx-code"})
	if !strings.Contains(msg, "微信登录未配置") {
		t.Fatalf("wechat: %s", msg)
	}
	msg, _ = call(t, ts, "", http.MethodPost, "/api/app/v1/auth/apple", map[string]string{"identity_token": "tok"})
	if !strings.Contains(msg, "苹果登录未配置") {
		t.Fatalf("apple: %s", msg)
	}
}

func TestWechatAppleLoginAndBind(t *testing.T) {
	store, ts := newMergeServer(t)
	wxCode := fmt.Sprintf("wx-login-%d", time.Now().UnixNano())
	data := create(t, ts, "", "/api/app/v1/auth/wechat", map[string]string{"code": wxCode})
	if data["accessToken"] == nil || data["id"] == nil {
		t.Fatalf("wechat login: %#v", data)
	}
	again := create(t, ts, "", "/api/app/v1/auth/wechat", map[string]string{"code": wxCode})
	if again["id"] != data["id"] {
		t.Fatalf("wechat relogin created another user: %#v %#v", data, again)
	}
	var stored string
	if err := store.QueryRow(t.Context(), `SELECT wechat_openid FROM app_users WHERE id=$1`, int64(data["id"].(float64))).Scan(&stored); err != nil || stored != wxCode {
		t.Fatalf("stored wechat: %s %v", stored, err)
	}

	appleTok := fmt.Sprintf("apple-login-%d", time.Now().UnixNano())
	apple := create(t, ts, "", "/api/app/v1/auth/apple", map[string]string{"identity_token": appleTok})
	if apple["login"] != "apple" || apple["accessToken"] == nil {
		t.Fatalf("apple login: %#v", apple)
	}

	email := uniqueEmail("oauth-bind")
	token := register(t, ts, email)
	bindCode := fmt.Sprintf("wx-bind-%d", time.Now().UnixNano())
	must(t, ts, token, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{"code": bindCode})
	must(t, ts, token, http.MethodPost, "/api/app/v1/auth/bind-apple", map[string]string{"identity_token": fmt.Sprintf("apple-bind-%d", time.Now().UnixNano())})
}

func appleTestKey(t *testing.T) (*rsa.PrivateKey, map[string]any) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	n := base64.RawURLEncoding.EncodeToString(priv.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.PublicKey.E)).Bytes())
	return priv, map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA", "kid": "test-kid", "use": "sig", "alg": "RS256", "n": n, "e": e,
		}},
	}
}

func signAppleToken(t *testing.T, priv *rsa.PrivateKey, sub, iss, aud string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Issuer:    iss,
		Subject:   sub,
		Audience:  jwt.ClaimStrings{aud},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	tok.Header["kid"] = "test-kid"
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
