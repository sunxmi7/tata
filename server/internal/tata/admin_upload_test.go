package tata

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func TestAdminUpload(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})

	msg, _ := call(t, ts, ro.access, http.MethodPost, "/api/admin/v1/uploads", map[string]any{
		"filename": "dot.png", "content_type": "image/png", "content_base64": tinyPNG,
	})
	if !strings.Contains(msg, "只读") {
		t.Fatalf("readonly upload: %s", msg)
	}

	created := create(t, ts, admin.access, "/api/admin/v1/uploads", map[string]any{
		"filename": "dot.png", "content_type": "image/png", "content_base64": tinyPNG,
	})
	url, _ := created["url"].(string)
	if !strings.Contains(url, "/uploads/misc/") || !strings.HasSuffix(url, ".png") {
		t.Fatalf("upload url: %#v", created)
	}
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("fetch upload: %d", res.StatusCode)
	}

	png, err := base64.StdEncoding.DecodeString(tinyPNG)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", "dot.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/admin/v1/uploads", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+admin.access)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != 0 {
		t.Fatalf("multipart: %s %s", env.Message, raw)
	}
	partURL, _ := env.Data["url"].(string)
	if !strings.Contains(partURL, "/uploads/misc/") {
		t.Fatalf("multipart url: %#v", env.Data)
	}

	spread := create(t, ts, admin.access, "/api/admin/v1/uploads?folder=spreads", map[string]any{
		"filename": "dot.png", "content_type": "image/png", "content_base64": tinyPNG,
	})
	spreadURL, _ := spread["url"].(string)
	if !strings.Contains(spreadURL, "/uploads/spreads/") {
		t.Fatalf("spread folder: %#v", spread)
	}
	spreadRes, err := http.Get(spreadURL)
	if err != nil {
		t.Fatal(err)
	}
	defer spreadRes.Body.Close()
	if spreadRes.StatusCode != http.StatusOK {
		t.Fatalf("fetch spread: %d", spreadRes.StatusCode)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/uploads?folder=../etc", map[string]any{
		"filename": "dot.png", "content_type": "image/png", "content_base64": tinyPNG,
	})
	if !strings.Contains(msg, "上传目录不正确") {
		t.Fatalf("bad folder: %s", msg)
	}
}
