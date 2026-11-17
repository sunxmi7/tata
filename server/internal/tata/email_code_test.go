package tata

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmailCodeUnconfigured(t *testing.T) {
	store, _ := newMergeServer(t)
	app := &App{Store: store, JWTSecret: []byte("test-secret")}
	ts := httptest.NewServer(app.Handler())
	t.Cleanup(ts.Close)
	email := uniqueEmail("nocfg")
	msg, _ := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/email/code", map[string]string{"email": email, "scene": "login"})
	if !strings.Contains(msg, "邮件服务未配置") {
		t.Fatalf("unconfigured: %s", msg)
	}
}

func TestEmailOTPLoginNeedCode(t *testing.T) {
	_, ts := newMergeServer(t)
	email := uniqueEmail("needcode")
	msg, _ := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/login", map[string]string{"email": email})
	if !strings.Contains(msg, "邮箱和验证码必填") {
		t.Fatalf("login without code: %s", msg)
	}
	msg, _ = call(t, ts, "", http.MethodPost, "/api/app/v1/auth/login", map[string]string{"email": email, "code": "000000"})
	if !strings.Contains(msg, "验证码不正确或已过期") {
		t.Fatalf("login bad code: %s", msg)
	}
	msg, _ = call(t, ts, "", http.MethodPost, "/api/app/v1/auth/login", map[string]string{"email": email, "password": "demo1234", "code": "000000"})
	if !strings.Contains(msg, "请求字段不正确") {
		t.Fatalf("login extra password: %s", msg)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/app/v1/auth/register", strings.NewReader(`{"email":"`+email+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("register status %d", res.StatusCode)
	}
	code := sendEmailCode(t, ts, "", email, "login")
	data := create(t, ts, "", "/api/app/v1/auth/login", map[string]string{"email": email, "code": code})
	if data["accessToken"] == nil || data["login"] != "email" {
		t.Fatalf("first login: %#v", data)
	}
	if data["accessToken"] == nil {
		t.Fatalf("first login: %#v", data)
	}
	msg, _ = call(t, ts, "", http.MethodPost, "/api/app/v1/auth/login", map[string]string{"email": email, "code": code})
	if !strings.Contains(msg, "验证码不正确或已过期") {
		t.Fatalf("consumed code: %s", msg)
	}
}

func TestEmailCodeCooldownAndExisting(t *testing.T) {
	_, ts := newMergeServer(t)
	email := uniqueEmail("cool")
	_ = sendEmailCode(t, ts, "", email, "login")
	msg, _ := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/email/code", map[string]string{"email": email, "scene": "login"})
	if !strings.Contains(msg, "发送过于频繁") {
		t.Fatalf("cooldown: %s", msg)
	}
	msg, _ = call(t, ts, "", http.MethodPost, "/api/app/v1/auth/email/code", map[string]string{"email": email, "scene": "register"})
	if !strings.Contains(msg, "验证码场景不正确") {
		t.Fatalf("register scene: %s", msg)
	}
	msg, raw := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/email/code", map[string]string{"email": email, "scene": "bind"})
	if msg != "401" && !strings.Contains(string(raw), "未登录或令牌无效") {
		t.Fatalf("bind without token: %s %s", msg, raw)
	}
}

func TestSMTPMailerNotReady(t *testing.T) {
	m := NewSMTPMailer(MailConfig{Enabled: true})
	if m.Ready() {
		t.Fatal("empty host should not be ready")
	}
	if err := m.SendCode("a@tata.local", "123456", "login"); err == nil || !strings.Contains(err.Error(), "邮件服务未配置") {
		t.Fatalf("send: %v", err)
	}
	off := NewSMTPMailer(MailConfig{})
	if off.Ready() {
		t.Fatal("disabled should not be ready")
	}
}

func TestEmailCodeCopy(t *testing.T) {
	subject, body := emailCodeCopy("login", "123456")
	if subject != "【塔塔】邮箱验证码" {
		t.Fatalf("subject: %s", subject)
	}
	for _, want := range []string{"您好", "登录", "123456", "5 分钟", "若非您本人操作", "塔塔"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if _, body := emailCodeCopy("bind", "000002"); !strings.Contains(body, "绑定邮箱") {
		t.Fatalf("bind copy: %s", body)
	}
}

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@tata.local", prefix, time.Now().UnixNano())
}
