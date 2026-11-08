package tata

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadOSSUnconfigured(t *testing.T) {
	u := NewUploader(UploadConfig{Driver: "oss"})
	_, err := u.Save(&http.Request{}, "dot.png", []byte("x"), "image/png")
	if err == nil || !strings.Contains(err.Error(), "OSS 未配置") {
		t.Fatalf("unconfigured oss: %v", err)
	}

	store, local := newMergeServer(t)
	admin := login(t, local, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	msg, _ := call(t, local, admin.access, http.MethodPost, "/api/admin/v1/uploads", map[string]any{
		"filename": "dot.png", "content_type": "image/png", "content_base64": tinyPNG,
	})
	if msg != "" {
		t.Fatalf("default still local: %s", msg)
	}

	ossApp := &App{Store: store, JWTSecret: []byte("test-secret"), Upload: NewUploader(UploadConfig{Driver: "oss"})}
	ossTS := httptest.NewServer(ossApp.Handler())
	t.Cleanup(ossTS.Close)
	ossAdmin := login(t, ossTS, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	msg, _ = call(t, ossTS, ossAdmin.access, http.MethodPost, "/api/admin/v1/uploads", map[string]any{
		"filename": "dot.png", "content_type": "image/png", "content_base64": tinyPNG,
	})
	if !strings.Contains(msg, "OSS 未配置") {
		t.Fatalf("http oss: %s", msg)
	}
}

func TestUploadOSSPublicURL(t *testing.T) {
	u := NewUploader(UploadConfig{
		Driver: "oss",
		OSS: OSSConfig{
			Endpoint:        "oss-cn-hangzhou.aliyuncs.com",
			Bucket:          "tata",
			AccessKeyId:     "id",
			AccessKeySecret: "secret",
			Prefix:          "uploads",
		},
	})
	var gotKey, gotType string
	u.putOSS = func(key, contentType string, data []byte) error {
		gotKey, gotType = key, contentType
		if len(data) == 0 {
			t.Fatal("empty body")
		}
		return nil
	}
	url, err := u.Save(&http.Request{}, "spreads/ab.png", []byte("img"), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "uploads/spreads/ab.png" || gotType != "image/png" {
		t.Fatalf("put: %s %s", gotKey, gotType)
	}
	if url != "https://tata.oss-cn-hangzhou.aliyuncs.com/uploads/spreads/ab.png" {
		t.Fatalf("url: %s", url)
	}

	u.cfg.OSS.PublicBaseURL = "https://cdn.example.test/"
	url, err = u.Save(&http.Request{}, "spreads/ab.png", []byte("img"), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://cdn.example.test/uploads/spreads/ab.png" {
		t.Fatalf("cdn url: %s", url)
	}
}

func TestUploadDriverInvalid(t *testing.T) {
	u := NewUploader(UploadConfig{Driver: "s3"})
	_, err := u.Save(&http.Request{}, "a.png", []byte("x"), "image/png")
	if err == nil || !strings.Contains(err.Error(), "上传方式不正确") {
		t.Fatalf("invalid driver: %v", err)
	}
}

func TestAbsoluteAssetURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/api/app/v1/spreads", nil)
	req.Host = "127.0.0.1:8080"
	if got := absoluteAssetURL(req, "/uploads/card-default.png"); got != "http://127.0.0.1:8080/uploads/card-default.png" {
		t.Fatalf("relative: %s", got)
	}
	if got := absoluteAssetURL(req, "https://cdn.example.test/a.png"); got != "https://cdn.example.test/a.png" {
		t.Fatalf("cdn: %s", got)
	}
	if got := absoluteAssetURL(req, ""); got != "" {
		t.Fatalf("empty: %s", got)
	}
	if got := absoluteAssetURL(nil, "/uploads/card-default.png"); got != "/uploads/card-default.png" {
		t.Fatalf("nil request: %s", got)
	}
	req.Header.Set("X-Forwarded-Proto", "https")
	if got := absoluteAssetURL(req, "/uploads/card-default.png"); got != "https://127.0.0.1:8080/uploads/card-default.png" {
		t.Fatalf("forwarded: %s", got)
	}
}
