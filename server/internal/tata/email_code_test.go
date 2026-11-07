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
	msg, _ := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/email/code", map[string]string{"email": email, "scene": "register"})
	if !strings.Contains(msg, "邮件服务未配置") {
		t.Fatalf("unconfigured: %s", msg)
	}
}

func TestEmailRegisterAndLoginNeedCode(t *testing.T) {
	_, ts := newMergeServer(t)
	email := uniqueEmail("needcode")
	msg, _ := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/register", map[string]string{"email": email, "password": "demo1234"})
	if !strings.Contains(msg, "验证码必填") {
		t.Fatalf("register without code: %s", msg)
	}
	msg, _ = call(t, ts, "", http.MethodPost, "/api/app/v1/auth/register", map[string]string{"email": email, "password": "demo1234", "code": "000000"})
	if !strings.Contains(msg, "验证码不正确或已过期") {
		t.Fatalf("register bad code: %s", msg)
	}
	code := sendEmailCode(t, ts, "", email, "register")
	token := create(t, ts, "", "/api/app/v1/auth/register", map[string]string{"email": email, "password": "demo1234", "code": code})["accessToken"].(string)
	if token == "" {
		t.Fatal("register token empty")
	}
	msg, _ = call(t, ts, "", http.MethodPost, "/api/app/v1/auth/login", map[string]string{"email": email, "password": "demo1234"})
	if !strings.Contains(msg, "验证码必填") {
		t.Fatalf("login without code: %s", msg)
	}
	loginCode := sendEmailCode(t, ts, "", email, "login")
	data := create(t, ts, "", "/api/app/v1/auth/login", map[string]string{"email": email, "password": "demo1234", "code": loginCode})
	if data["accessToken"] == nil {
		t.Fatalf("login: %#v", data)
	}
}

func TestEmailCodeCooldownAndExisting(t *testing.T) {
	_, ts := newMergeServer(t)
	email := uniqueEmail("cool")
	_ = sendEmailCode(t, ts, "", email, "register")
	msg, _ := call(t, ts, "", http.MethodPost, "/api/app/v1/auth/email/code", map[string]string{"email": email, "scene": "register"})
	if !strings.Contains(msg, "发送过于频繁") {
		t.Fatalf("cooldown: %s", msg)
	}
	msg, _ = call(t, ts, "", http.MethodPost, "/api/app/v1/auth/email/code", map[string]string{"email": email, "scene": "login"})
	if !strings.Contains(msg, "该邮箱尚未注册") {
		t.Fatalf("login scene before register: %s", msg)
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
	for _, want := range []string{"您好", "登录账号", "123456", "5 分钟", "若非您本人操作", "塔塔"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if _, body := emailCodeCopy("register", "000001"); !strings.Contains(body, "注册账号") {
		t.Fatalf("register copy: %s", body)
	}
	if _, body := emailCodeCopy("bind", "000002"); !strings.Contains(body, "绑定邮箱") {
		t.Fatalf("bind copy: %s", body)
	}
}

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@tata.local", prefix, time.Now().UnixNano())
}
