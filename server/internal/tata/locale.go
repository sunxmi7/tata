package tata

import (
	"context"
	"net/http"
	"strings"
)

type ctxKey int

const ctxAppLanguage ctxKey = iota

const defaultAppLanguage = "zh-Hans"

// supportedAppLanguages are BCP47 tags the App may send (Accept-Language / X-Language).
var supportedAppLanguages = map[string]struct{}{
	"zh-Hans": {},
	"zh-Hant": {},
	"en":      {},
	"ja":      {},
	"ko":      {},
}

// AppLanguageFromContext returns the normalized language tag for this request (default zh-Hans).
func AppLanguageFromContext(ctx context.Context) string {
	if ctx == nil {
		return defaultAppLanguage
	}
	if v, ok := ctx.Value(ctxAppLanguage).(string); ok && v != "" {
		return v
	}
	return defaultAppLanguage
}

func withAppLanguage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := resolveAppLanguage(r)
		ctx := context.WithValue(r.Context(), ctxAppLanguage, lang)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func resolveAppLanguage(r *http.Request) string {
	if r == nil {
		return defaultAppLanguage
	}
	if x := strings.TrimSpace(r.Header.Get("X-Language")); x != "" {
		if tag := normalizeAppLanguageTag(x); tag != "" {
			return tag
		}
	}
	if al := r.Header.Get("Accept-Language"); al != "" {
		for _, part := range strings.Split(al, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if i := strings.Index(part, ";"); i >= 0 {
				part = strings.TrimSpace(part[:i])
			}
			if tag := normalizeAppLanguageTag(part); tag != "" {
				return tag
			}
		}
	}
	return defaultAppLanguage
}

func normalizeAppLanguageTag(raw string) string {
	tag := strings.TrimSpace(raw)
	if tag == "" {
		return ""
	}
	tag = strings.ReplaceAll(tag, "_", "-")
	lower := strings.ToLower(tag)
	switch lower {
	case "zh-hans", "zh-cn", "zh":
		return "zh-Hans"
	case "zh-hant", "zh-tw", "zh-hk":
		return "zh-Hant"
	case "en", "en-us", "en-gb":
		return "en"
	case "ja", "ja-jp":
		return "ja"
	case "ko", "ko-kr":
		return "ko"
	}
	if _, ok := supportedAppLanguages[tag]; ok {
		return tag
	}
	return ""
}
