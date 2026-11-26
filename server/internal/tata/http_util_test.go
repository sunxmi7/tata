package tata

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAccessTokenTTL(t *testing.T) {
	a := &App{JWTSecret: []byte("test-secret")}
	now := time.Now()
	appTok, err := a.signAccess("app", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	adminTok, err := a.signAccess("admin", 2, "admin")
	if err != nil {
		t.Fatal(err)
	}
	appExp := tokenExp(t, appTok, a.JWTSecret)
	adminExp := tokenExp(t, adminTok, a.JWTSecret)
	if d := appExp.Sub(now); d < 6*24*time.Hour || d > 8*24*time.Hour {
		t.Fatalf("app access ttl: %v", d)
	}
	if d := adminExp.Sub(now); d < time.Hour || d > 3*time.Hour {
		t.Fatalf("admin access ttl: %v", d)
	}
}

func tokenExp(t *testing.T, raw string, secret []byte) time.Time {
	t.Helper()
	tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil || !tok.Valid {
		t.Fatalf("parse: %v", err)
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims")
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		t.Fatalf("exp: %v", err)
	}
	return exp.Time
}
