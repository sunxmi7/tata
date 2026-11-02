package tata

import (
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
