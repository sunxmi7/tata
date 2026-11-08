package tata

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Sign in with Apple 只走 iOS。aud 为 iOS Bundle ID。安卓没有苹果登录。
const appleJWKSURL = "https://appleid.apple.com/auth/keys"
const appleIssuer = "https://appleid.apple.com"

type AppleConfig struct {
	Enabled  bool
	ClientID string
}

type appleVerifier interface {
	Ready() bool
	VerifyIdentityToken(ctx context.Context, identityToken string) (string, error)
}

type appleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type AppleVerifier struct {
	cfg     AppleConfig
	http    *http.Client
	jwksURL string

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	keysExp time.Time
}

func NewAppleVerifier(cfg AppleConfig) *AppleVerifier {
	return &AppleVerifier{
		cfg:     cfg,
		http:    &http.Client{Timeout: 10 * time.Second},
		jwksURL: appleJWKSURL,
	}
}

func (v *AppleVerifier) Ready() bool {
	if v == nil || !v.cfg.Enabled {
		return false
	}
	return strings.TrimSpace(v.cfg.ClientID) != ""
}

func (v *AppleVerifier) VerifyIdentityToken(ctx context.Context, identityToken string) (string, error) {
	if v == nil || !v.Ready() {
		return "", biz("苹果登录未配置")
	}
	identityToken = strings.TrimSpace(identityToken)
	if identityToken == "" {
		return "", biz("苹果身份令牌不能为空")
	}
	aud := strings.TrimSpace(v.cfg.ClientID)
	claims := &jwt.RegisteredClaims{}
	tok, err := jwt.ParseWithClaims(identityToken, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("alg")
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("kid")
		}
		return v.keyForKid(ctx, kid)
	}, jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}), jwt.WithAudience(aud), jwt.WithIssuer(appleIssuer), jwt.WithLeeway(60*time.Second), jwt.WithExpirationRequired())
	if err != nil || !tok.Valid {
		return "", biz("苹果身份令牌无效")
	}
	sub := strings.TrimSpace(claims.Subject)
	if sub == "" {
		return "", biz("苹果身份令牌无效")
	}
	return sub, nil
}

func (v *AppleVerifier) keyForKid(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	if key, ok := v.keys[kid]; ok && time.Now().Before(v.keysExp) {
		v.mu.Unlock()
		return key, nil
	}
	v.mu.Unlock()
	keys, err := v.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	v.keys = keys
	v.keysExp = time.Now().Add(10 * time.Minute)
	key, ok := v.keys[kid]
	v.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown kid")
	}
	return key, nil
}

func (v *AppleVerifier) fetchJWKS(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return nil, biz("苹果身份校验失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, biz("苹果身份校验失败")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, biz("苹果身份校验失败")
	}
	var doc struct {
		Keys []appleJWK `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, biz("苹果身份校验失败")
	}
	out := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			continue
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, biz("苹果身份校验失败")
	}
	return out, nil
}

func rsaPublicKeyFromJWK(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e == 0 {
		return nil, fmt.Errorf("jwk e")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func (a *App) appleIdentity(ctx context.Context, identityToken string) (string, error) {
	if a == nil || a.Apple == nil || !a.Apple.Ready() {
		return "", biz("苹果登录未配置")
	}
	return a.Apple.VerifyIdentityToken(ctx, identityToken)
}
