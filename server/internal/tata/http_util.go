package tata

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type bizError struct {
	msg  string
	code string // 稳定错误码（error_code）。空表示尚未归类的业务错误，仅凭中文 message。
}

func (e *bizError) Error() string { return e.msg }

func biz(msg string) error { return &bizError{msg: msg} }

// bizCode 带稳定错误码的业务错误，供 App 端按码分支，而不是匹配中文 message。
func bizCode(code, msg string) error { return &bizError{msg: msg, code: code} }

// displayLabelTataCoin is the UI name for diamonds. The ledger unit stays diamond.
const displayLabelTataCoin = "塔塔币"

type authFail struct{}

func (authFail) Error() string { return "unauthorized" }

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": data, "message": ""})
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeErrCode(w, status, "", msg)
}

// writeErrCode 带 error_code 的错误包络。error_code 为稳定分类码，message 仍为中文原因。
func writeErrCode(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"code": 1, "data": nil, "message": msg, "error_code": code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return bizCode("invalid_request_body", "请求字段不正确")
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, bizCode("invalid_path_id", "id 不正确")
	}
	return id, nil
}

type App struct {
	Store     *Store
	JWTSecret []byte
	Redis     redisLocker
	KV        kvStore
	Mail      Mailer
	Upload    *Uploader
	Push      *Pusher
}

type redisLocker interface {
	Lock(key string, ttl time.Duration) (bool, error)
	Unlock(key string) error
}

func (a *App) signAccess(aud string, sub int64, anon, role string) (string, error) {
	claims := jwt.MapClaims{
		"aud":  aud,
		"sub":  sub,
		"anon": anon,
		"role": role,
		"exp":  time.Now().Add(2 * time.Hour).Unix(),
		"typ":  "access",
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.JWTSecret)
}

func (a *App) parseAccess(r *http.Request, aud string) (sub int64, anon, role string, err error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return 0, "", "", authFail{}
	}
	tok, err := jwt.Parse(strings.TrimPrefix(h, "Bearer "), func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("alg")
		}
		return a.JWTSecret, nil
	})
	if err != nil || !tok.Valid {
		return 0, "", "", authFail{}
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok || claims["aud"] != aud || claims["typ"] != "access" {
		return 0, "", "", authFail{}
	}
	switch v := claims["sub"].(type) {
	case float64:
		sub = int64(v)
	default:
		return 0, "", "", authFail{}
	}
	if s, ok := claims["anon"].(string); ok {
		anon = s
	}
	if s, ok := claims["role"].(string); ok {
		role = s
	}
	return sub, anon, role, nil
}

func (a *App) issueRefresh(ctx context.Context, aud string, sub int64) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	_, err := a.Store.Exec(ctx, `INSERT INTO refresh_tokens(id, audience, subject_id, expires_at) VALUES ($1,$2,$3,$4)`,
		id, aud, sub, time.Now().Add(7*24*time.Hour))
	return id, err
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
