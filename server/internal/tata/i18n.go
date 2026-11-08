package tata

import (
	"context"
	"encoding/json"
	"io/fs"
	"path"
	"strings"

	"embed"
)

//go:embed i18n/*.json
var i18nFS embed.FS

var i18nCatalogs map[string]map[string]string

func init() {
	i18nCatalogs = map[string]map[string]string{}
	entries, err := fs.ReadDir(i18nFS, "i18n")
	if err != nil {
		panic("i18n: " + err.Error())
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		lang := strings.TrimSuffix(e.Name(), ".json")
		raw, err := i18nFS.ReadFile(path.Join("i18n", e.Name()))
		if err != nil {
			panic("i18n " + lang + ": " + err.Error())
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			panic("i18n " + lang + ": " + err.Error())
		}
		i18nCatalogs[lang] = m
	}
}

// T looks up msgid in the request language catalog.
// zh-Hans uses the Chinese source string. Other languages fall back to zh-Hans, then msgid.
func T(ctx context.Context, msgid string) string {
	return TL(AppLanguageFromContext(ctx), msgid)
}

// TL looks up msgid for an explicit language tag.
func TL(lang, msgid string) string {
	msgid = strings.TrimSpace(msgid)
	if msgid == "" {
		return ""
	}
	if lang == "" {
		lang = defaultAppLanguage
	}
	if s := catalogGet(lang, msgid); s != "" {
		return s
	}
	if lang != defaultAppLanguage {
		if s := catalogGet(defaultAppLanguage, msgid); s != "" {
			return s
		}
	}
	return msgid
}

func catalogGet(lang, msgid string) string {
	m := i18nCatalogs[lang]
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[msgid])
}

func localizeString(ctx context.Context, v any) any {
	s, ok := v.(string)
	if !ok || s == "" {
		return v
	}
	return T(ctx, s)
}

func localizeReading(ctx context.Context, item map[string]any) {
	if item == nil {
		return
	}
	for _, key := range []string{
		"progress", "status_label", "type_label", "orientation_label",
		"fail_reason", "quota_prompt", "card_name",
	} {
		switch v := item[key].(type) {
		case string:
			if v != "" {
				item[key] = T(ctx, v)
			}
		case *string:
			if v != nil && *v != "" {
				s := T(ctx, *v)
				item[key] = s
			}
		}
	}
	if raw, ok := item["cards"].(json.RawMessage); ok {
		item["cards"] = localizeJSONFields(ctx, raw, "name", "position_name")
	}
}

func localizeJSONFields(ctx context.Context, raw json.RawMessage, fields ...string) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return raw
	}
	walkLocalizeFields(ctx, v, fields)
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

func walkLocalizeFields(ctx context.Context, v any, fields []string) {
	switch t := v.(type) {
	case []any:
		for _, x := range t {
			walkLocalizeFields(ctx, x, fields)
		}
	case map[string]any:
		for _, f := range fields {
			if s, ok := t[f].(string); ok && s != "" {
				t[f] = T(ctx, s)
			}
		}
	}
}

func localizeMapStrings(ctx context.Context, item map[string]any, keys ...string) {
	if item == nil {
		return
	}
	for _, key := range keys {
		if s, ok := item[key].(string); ok && s != "" {
			item[key] = T(ctx, s)
		}
	}
}
