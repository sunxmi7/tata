package tata

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveAppLanguage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		header map[string]string
		want   string
	}{
		{"default", nil, "zh-Hans"},
		{"x-language", map[string]string{"X-Language": "ja"}, "ja"},
		{"accept-language", map[string]string{"Accept-Language": "en-US,en;q=0.9"}, "en"},
		{"x-overrides-accept", map[string]string{"X-Language": "ko", "Accept-Language": "en"}, "ko"},
		{"unknown falls back", map[string]string{"Accept-Language": "fr-FR"}, "zh-Hans"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest("GET", "/", nil)
			for k, v := range tc.header {
				r.Header.Set(k, v)
			}
			if got := resolveAppLanguage(r); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestTLCatalog(t *testing.T) {
	t.Parallel()
	if got := TL("zh-Hans", "未登录或令牌无效"); got != "未登录或令牌无效" {
		t.Fatalf("zh-Hans: %q", got)
	}
	if got := TL("en", "未登录或令牌无效"); got != "Not logged in or token invalid" {
		t.Fatalf("en error: %q", got)
	}
	if got := TL("en", "愚者"); got != "The Fool" {
		t.Fatalf("en card: %q", got)
	}
	if got := TL("ja", "女皇"); got != "女帝" {
		t.Fatalf("ja card: %q", got)
	}
	if got := TL("ko", "塔塔币"); got != "타타코인" {
		t.Fatalf("ko coin: %q", got)
	}
	if got := TL("zh-Hant", "登录后才能抽今日一牌"); got == "登录后才能抽今日一牌" || got == "" {
		t.Fatalf("zh-Hant not converted: %q", got)
	}
	if got := TL("fr", "内部错误"); got != "内部错误" {
		t.Fatalf("unknown lang fallback: %q", got)
	}
}

func TestWithAppLanguageContext(t *testing.T) {
	t.Parallel()
	var got string
	h := withAppLanguage(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = AppLanguageFromContext(r.Context())
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "zh-Hant")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != "zh-Hant" {
		t.Fatalf("context lang %q", got)
	}
}

func TestI18nHTTPHeaders(t *testing.T) {
	_, ts := newMergeServer(t)
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/app/v1/profile", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Language", "en")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var env struct {
		Message   string `json:"message"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnauthorized || env.ErrorCode != "unauthorized" {
		t.Fatalf("status=%d code=%s body=%s", res.StatusCode, env.ErrorCode, raw)
	}
	if env.Message != "Not logged in or token invalid" {
		t.Fatalf("en message: %q", env.Message)
	}

	req, err = http.NewRequest(http.MethodGet, ts.URL+"/api/app/v1/preset-questions", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Language", "en")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	var qenv struct {
		Data struct {
			Featured []map[string]any `json:"featured"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &qenv); err != nil {
		t.Fatal(err)
	}
	if len(qenv.Data.Featured) == 0 {
		t.Fatalf("no featured: %s", raw)
	}
	if qenv.Data.Featured[0]["body"] != "What do I most need to pay attention to right now?" {
		t.Fatalf("en question: %#v", qenv.Data.Featured[0])
	}
}
