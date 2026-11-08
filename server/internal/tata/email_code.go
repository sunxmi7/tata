package tata

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
)

const (
	emailCodeTTL      = 5 * time.Minute
	emailCodeCooldown = 60 * time.Second
	emailCodeHourly   = 10
	emailCodeTries    = 5
)

func (a *App) kv() kvStore {
	if a != nil && a.KV != nil {
		return a.KV
	}
	return nil
}

func normalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func validLoginEmail(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " <>") {
		return false
	}
	return validOptionalEmail(raw)
}

func (a *App) appSendEmailCode(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Email string `json:"email"`
		Scene string `json:"scene"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	email := normalizeEmail(req.Email)
	if !validLoginEmail(email) {
		return biz("邮箱不正确")
	}
	scene := strings.TrimSpace(req.Scene)
	switch scene {
	case "register", "bind":
	default:
		return biz("验证码场景不正确")
	}
	if scene == "bind" {
		uid, _, err := a.appActor(r)
		if err != nil {
			return err
		}
		if uid == 0 {
			return authFail{}
		}
	}
	if scene == "register" {
		var n int
		if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM app_users WHERE lower(email)=lower($1) AND cancelled_at IS NULL AND merged_into IS NULL`, email).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return biz("邮箱已注册")
		}
	}
	store := a.kv()
	if store == nil {
		return biz("邮件服务未配置")
	}
	if a.Mail == nil || !a.Mail.Ready() {
		return biz("邮件服务未配置")
	}
	hourKey := "tata:mail:hour:" + email
	n, err := store.IncrExpire(hourKey, time.Hour)
	if err != nil {
		return err
	}
	if n > emailCodeHourly {
		return biz("发送过于频繁")
	}
	cdKey := "tata:mail:cd:" + scene + ":" + email
	ok, err := store.SetNX(cdKey, "1", emailCodeCooldown)
	if err != nil {
		return err
	}
	if !ok {
		return biz("发送过于频繁")
	}
	code, err := randomDigits(6)
	if err != nil {
		_ = store.Del(cdKey)
		return err
	}
	if err := a.Mail.SendCode(email, code, scene); err != nil {
		_ = store.Del(cdKey)
		if _, ok := err.(*bizError); ok {
			return err
		}
		return fmt.Errorf("send email code: %w", err)
	}
	if err := store.Set("tata:mail:code:"+scene+":"+email, code, emailCodeTTL); err != nil {
		return err
	}
	_ = store.Del("tata:mail:try:" + scene + ":" + email)
	writeOK(w, map[string]any{"expires_in": int(emailCodeTTL.Seconds())})
	return nil
}

func (a *App) matchEmailCode(ctx context.Context, scene, email, code string) error {
	email = normalizeEmail(email)
	code = strings.TrimSpace(code)
	if code == "" {
		return biz("验证码必填")
	}
	store := a.kv()
	if store == nil {
		return biz("验证码不正确或已过期")
	}
	want, ok, err := store.Get("tata:mail:code:" + scene + ":" + email)
	if err != nil {
		return err
	}
	if !ok || !codeEqual(want, code) {
		n, incrErr := store.IncrExpire("tata:mail:try:"+scene+":"+email, emailCodeTTL)
		if incrErr != nil {
			return incrErr
		}
		if n >= emailCodeTries {
			_ = store.Del("tata:mail:code:" + scene + ":" + email)
			_ = store.Del("tata:mail:try:" + scene + ":" + email)
		}
		return biz("验证码不正确或已过期")
	}
	return nil
}

func (a *App) consumeEmailCode(ctx context.Context, scene, email string) {
	email = normalizeEmail(email)
	store := a.kv()
	if store == nil {
		return
	}
	_ = store.Del("tata:mail:code:" + scene + ":" + email)
	_ = store.Del("tata:mail:try:" + scene + ":" + email)
}

func codeEqual(want, got string) bool {
	if len(want) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

func randomDigits(n int) (string, error) {
	max := big.NewInt(1)
	for i := 0; i < n; i++ {
		max.Mul(max, big.NewInt(10))
	}
	v, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", n, v.Int64()), nil
}
