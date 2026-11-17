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
	"net/url"
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
	prof, err := c.ExchangeCode(context.Background(), " code-1 ")
	if err != nil {
		t.Fatal(err)
	}
	if prof.ID != "uid-1" || gotAppID != "wx" || gotSecret != "secret" || gotCode != "code-1" {
		t.Fatalf("exchange: id=%s appid=%s secret=%s code=%s", prof.ID, gotAppID, gotSecret, gotCode)
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
	prof, err := c.ExchangeCode(context.Background(), "ok")
	if err != nil || prof.ID != "oid-only" {
		t.Fatalf("openid fallback: %s %v", prof.ID, err)
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

func TestWeChatExchangeUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") != "" {
			if r.URL.Query().Get("access_token") != "tok" || r.URL.Query().Get("openid") != "oid-1" {
				t.Fatalf("userinfo query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"nickname":   "塔塔用户",
				"headimgurl": "https://thirdwx.qlogo.cn/mmopen/demo/132",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok", "openid": "oid-1", "unionid": "uid-1",
		})
	}))
	t.Cleanup(srv.Close)
	c := NewWeChatClient(WeChatConfig{Enabled: true, AppID: "wx", AppSecret: "secret"})
	c.tokenURL = srv.URL
	c.userInfoURL = srv.URL
	prof, err := c.ExchangeCode(context.Background(), "code-1")
	if err != nil {
		t.Fatal(err)
	}
	if prof.ID != "uid-1" || prof.Nickname != "塔塔用户" || !strings.Contains(prof.Avatar, "thirdwx.qlogo.cn") {
		t.Fatalf("profile: %+v", prof)
	}
}

func TestWeChatUserInfoFailureStillReturnsID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") != "" {
			_, _ = w.Write([]byte(`{"errcode":"40001","errmsg":"invalid credential"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "openid": "oid-2"})
	}))
	t.Cleanup(srv.Close)
	c := NewWeChatClient(WeChatConfig{Enabled: true, AppID: "wx", AppSecret: "secret"})
	c.tokenURL = srv.URL
	c.userInfoURL = srv.URL
	prof, err := c.ExchangeCode(context.Background(), "code-2")
	if err != nil || prof.ID != "oid-2" || prof.Nickname != "" {
		t.Fatalf("fallback: %+v %v", prof, err)
	}
}

type stubWeChat struct {
	nickname string
	avatar   string
}

func (s stubWeChat) Ready() bool { return true }

func (s stubWeChat) ExchangeCode(_ context.Context, code string) (wechatProfile, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return wechatProfile{}, biz("微信授权码不能为空")
	}
	return wechatProfile{ID: code, Nickname: s.nickname, Avatar: s.avatar}, nil
}

var png1x1 = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func TestWeChatLoginFillsEmptyProfile(t *testing.T) {
	img := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png1x1)
	}))
	t.Cleanup(img.Close)
	wechatAvatarTestHook = func(u *url.URL) bool { return u.Hostname() == "127.0.0.1" }
	t.Cleanup(func() { wechatAvatarTestHook = nil })

	store, _ := newMergeServer(t)
	app := testApp(store)
	app.WeChat = stubWeChat{nickname: "微信昵称", avatar: img.URL + "/head.png"}
	app.Upload = NewUploader(UploadConfig{Driver: "local", LocalDir: t.TempDir()})
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)

	code := fmt.Sprintf("wx-profile-%d", time.Now().UnixNano())
	data := create(t, ts, "", "/api/app/v1/auth/wechat", map[string]string{"code": code})
	token := data["accessToken"].(string)
	profile := listOne(t, ts, token, "/api/app/v1/profile")
	if profile["name"] != "微信昵称" {
		t.Fatalf("name: %#v", profile["name"])
	}
	avatar, _ := profile["avatar_url"].(string)
	if !strings.Contains(avatar, "/uploads/misc/") || !strings.HasSuffix(avatar, ".png") {
		t.Fatalf("avatar: %#v", profile["avatar_url"])
	}

	must(t, ts, token, http.MethodPut, "/api/app/v1/profile", map[string]any{"name": "自己改的"})
	create(t, ts, "", "/api/app/v1/auth/wechat", map[string]string{"code": code})
	again := listOne(t, ts, token, "/api/app/v1/profile")
	if again["name"] != "自己改的" {
		t.Fatalf("overwrote name: %#v", again["name"])
	}
}

func TestWeChatLoginClientProfileFallback(t *testing.T) {
	img := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("missing user-agent")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png1x1)
	}))
	t.Cleanup(img.Close)
	wechatAvatarTestHook = func(u *url.URL) bool { return u.Hostname() == "127.0.0.1" }
	t.Cleanup(func() { wechatAvatarTestHook = nil })

	store, _ := newMergeServer(t)
	app := testApp(store)
	app.WeChat = stubWeChat{}
	app.Upload = NewUploader(UploadConfig{Driver: "local", LocalDir: t.TempDir()})
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)

	code := fmt.Sprintf("wx-fallback-%d", time.Now().UnixNano())
	data := create(t, ts, "", "/api/app/v1/auth/wechat", map[string]any{
		"code": code, "nickname": "客户端昵称", "headimgurl": img.URL + "/head/132",
	})
	profile := listOne(t, ts, data["accessToken"].(string), "/api/app/v1/profile")
	if profile["name"] != "客户端昵称" {
		t.Fatalf("name: %#v", profile["name"])
	}
	avatar, _ := profile["avatar_url"].(string)
	if !strings.Contains(avatar, "/uploads/misc/") || !strings.HasSuffix(avatar, ".png") {
		t.Fatalf("avatar: %#v", profile["avatar_url"])
	}
}

func TestWeChatBindFillsEmptyProfile(t *testing.T) {
	store, _ := newMergeServer(t)
	app := testApp(store)
	app.WeChat = stubWeChat{nickname: "绑定来的昵称"}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	token := register(t, ts, uniqueEmail("wx-bind-name"))
	before := listOne(t, ts, token, "/api/app/v1/profile")
	if name, _ := before["name"].(string); strings.TrimSpace(name) != "" {
		t.Fatalf("expected empty name: %#v", before["name"])
	}
	must(t, ts, token, http.MethodPost, "/api/app/v1/auth/bind-wechat", map[string]string{
		"code": fmt.Sprintf("wx-bind-name-%d", time.Now().UnixNano()),
	})
	after := listOne(t, ts, token, "/api/app/v1/profile")
	if after["name"] != "绑定来的昵称" {
		t.Fatalf("bind name: %#v", after["name"])
	}
}

func TestWeChatAvatarHostRejected(t *testing.T) {
	u, ok := wechatAvatarURLOK("https://evil.example/avatar.png")
	if ok {
		t.Fatalf("allowed arbitrary host: %v", u)
	}
	u, ok = wechatAvatarURLOK("https://thirdwx.qlogo.cn/mmopen/x/132")
	if !ok || u.Host != "thirdwx.qlogo.cn" || !strings.HasSuffix(u.Path, "/0") {
		t.Fatalf("qlogo: %v %v", u, ok)
	}
	u, ok = wechatAvatarURLOK("https://mmbiz.qpic.cn/mmbiz/abc/132")
	if !ok || u.Host != "mmbiz.qpic.cn" {
		t.Fatalf("qpic: %v %v", u, ok)
	}
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
