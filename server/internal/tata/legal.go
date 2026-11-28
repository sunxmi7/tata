package tata

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"embed"
)

//go:embed legal/*.md
var legalFS embed.FS

const (
	legalUserAgreement = "用户协议"
	legalPrivacyPolicy = "隐私政策"
	legalDocAgreement  = "user_agreement"
	legalDocPrivacy    = "privacy_policy"
)

var legalBodies = map[string]map[string]string{} // kind -> lang -> body

func init() {
	entries, err := fs.ReadDir(legalFS, "legal")
	if err != nil {
		panic("legal: " + err.Error())
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		kind, lang, ok := strings.Cut(name, ".")
		if !ok || (kind != legalDocAgreement && kind != legalDocPrivacy) {
			panic("legal: unexpected file " + e.Name())
		}
		if _, ok := supportedAppLanguages[lang]; !ok {
			panic("legal: unsupported language in " + e.Name())
		}
		raw, err := legalFS.ReadFile(path.Join("legal", e.Name()))
		if err != nil {
			panic("legal " + e.Name() + ": " + err.Error())
		}
		body := strings.TrimSpace(string(raw))
		if body == "" {
			panic("legal: empty " + e.Name())
		}
		if legalBodies[kind] == nil {
			legalBodies[kind] = map[string]string{}
		}
		legalBodies[kind][lang] = body
	}
	for _, kind := range []string{legalDocAgreement, legalDocPrivacy} {
		for lang := range supportedAppLanguages {
			if strings.TrimSpace(legalBodies[kind][lang]) == "" {
				panic("legal: missing " + kind + "." + lang + ".md")
			}
		}
	}
}

func legalBody(kind, lang string) string {
	if body := legalBodies[kind][lang]; body != "" {
		return body
	}
	return legalBodies[kind][defaultAppLanguage]
}

func (a *App) appLegal(w http.ResponseWriter, r *http.Request) error {
	lang := AppLanguageFromContext(r.Context())
	writeOK(w, map[string]any{
		"user_agreement": legalDoc(r, legalUserAgreement, legalBody(legalDocAgreement, lang)),
		"privacy_policy": legalDoc(r, legalPrivacyPolicy, legalBody(legalDocPrivacy, lang)),
	})
	return nil
}

func legalDoc(r *http.Request, title, body string) map[string]any {
	return map[string]any{
		"title": T(r.Context(), title), "body": body, "body_status": "ready",
	}
}

func (a *App) copyLegal(w http.ResponseWriter, r *http.Request) error {
	lang := strings.TrimSpace(r.URL.Query().Get("lang"))
	if lang == "" {
		lang = AppLanguageFromContext(r.Context())
	} else if _, ok := supportedAppLanguages[lang]; !ok {
		lang = defaultAppLanguage
	}
	writeOK(w, map[string]any{
		"kind": "legal",
		"lang": lang,
		"user_agreement": map[string]any{
			"body": legalBody(legalDocAgreement, lang), "body_status": "ready",
		},
		"privacy_policy": map[string]any{
			"body": legalBody(legalDocPrivacy, lang), "body_status": "ready",
		},
	})
	return nil
}
