package tata

import (
	"context"
	"encoding/json"
	"strings"
)

type deckRow struct {
	Code        string
	Name        string
	Thumbnail   string
	Enabled     bool
	Sort        int
	Description string
	Tags        string
}

func deckPreviewURL(thumb string) string {
	if strings.TrimSpace(thumb) == "" {
		return defaultCardThumbnailPath
	}
	return strings.TrimSpace(thumb)
}

func parseDeckTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []string{}
	}
	var tags []string
	if json.Unmarshal([]byte(raw), &tags) != nil {
		return []string{}
	}
	return normalizeDeckTagList(tags)
}

func encodeDeckTags(tags []string) string {
	tags = normalizeDeckTagList(tags)
	if len(tags) == 0 {
		return ""
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return ""
	}
	return string(b)
}

func normalizeDeckDescription(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > 200 {
		return "", biz("牌组介绍过长")
	}
	return s, nil
}

func normalizeDeckTags(tags []string) ([]string, error) {
	out := normalizeDeckTagList(tags)
	if len(out) > 8 {
		return nil, biz("标签过多")
	}
	for _, t := range out {
		if len([]rune(t)) > 16 {
			return nil, biz("标签过长")
		}
	}
	return out, nil
}

func normalizeDeckTagList(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func (s *Store) listDecks(ctx context.Context, onlyEnabled bool) ([]deckRow, error) {
	q := `SELECT code, name, thumbnail, enabled, sort_order, description, tags FROM decks`
	if onlyEnabled {
		q += ` WHERE enabled=true`
	}
	q += ` ORDER BY sort_order, code`
	rows, err := s.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []deckRow{}
	for rows.Next() {
		var d deckRow
		if err := rows.Scan(&d.Code, &d.Name, &d.Thumbnail, &d.Enabled, &d.Sort, &d.Description, &d.Tags); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (d deckRow) public() map[string]any {
	return map[string]any{
		"code": d.Code, "name": d.Name, "thumbnail": deckPreviewURL(d.Thumbnail), "available": d.Enabled,
		"description": d.Description, "tags": parseDeckTags(d.Tags),
	}
}

func (d deckRow) admin() map[string]any {
	return map[string]any{
		"code": d.Code, "name": d.Name, "thumbnail": nullableConfig(d.Thumbnail),
		"preview": deckPreviewURL(d.Thumbnail), "enabled": d.Enabled, "sort_order": d.Sort,
		"description": d.Description, "tags": parseDeckTags(d.Tags),
	}
}

func publicDeckList(rows []deckRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, d := range rows {
		out = append(out, d.public())
	}
	return out
}

func (a *App) enabledDeck(ctx context.Context, code string) (deckRow, bool, error) {
	code = strings.TrimSpace(code)
	rows, err := a.Store.listDecks(ctx, true)
	if err != nil {
		return deckRow{}, false, err
	}
	for _, d := range rows {
		if d.Code == code {
			return d, true, nil
		}
	}
	return deckRow{}, false, nil
}

func (a *App) resolveUserDeck(ctx context.Context, uid int64) (string, []map[string]any, error) {
	available, err := a.Store.listDecks(ctx, true)
	if err != nil {
		return "", nil, err
	}
	if len(available) == 0 {
		available = []deckRow{{Code: "rider-waite", Name: "莱德韦特", Enabled: true}}
	}
	code := "rider-waite"
	_ = a.Store.QueryRow(ctx, `SELECT deck_code FROM app_users WHERE id=$1`, uid).Scan(&code)
	code = strings.TrimSpace(code)
	ok := false
	for _, d := range available {
		if d.Code == code {
			ok = true
			break
		}
	}
	if !ok {
		code = available[0].Code
	}
	return code, publicDeckList(available), nil
}
