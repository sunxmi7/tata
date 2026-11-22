package tata

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type deckRow struct {
	Code           string
	Name           string
	Thumbnail      string
	BackURL        string
	Enabled        bool
	Sort           int
	Description    string
	Tags           string
	UnlockRequired bool
}

type deckFaceIn struct {
	CardID   int    `json:"card_id"`
	ImageURL string `json:"image_url"`
}

func deckPreviewURL(thumb string) string {
	if strings.TrimSpace(thumb) == "" {
		return defaultCardThumbnailPath
	}
	return strings.TrimSpace(thumb)
}

// spreadDiagramURL is the admin preview. An empty spread diagram is not a card back.
func spreadDiagramURL(thumb string) any {
	thumb = strings.TrimSpace(thumb)
	if thumb == "" {
		return nil
	}
	return thumb
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
	q := `SELECT code, name, thumbnail, back_url, enabled, sort_order, description, tags, unlock_required FROM decks`
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
		if err := rows.Scan(&d.Code, &d.Name, &d.Thumbnail, &d.BackURL, &d.Enabled, &d.Sort, &d.Description, &d.Tags, &d.UnlockRequired); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) loadDeckFaces(ctx context.Context, code string) (map[int]string, error) {
	out := map[int]string{}
	code = strings.TrimSpace(code)
	if code == "" {
		return out, nil
	}
	rows, err := s.Query(ctx, `SELECT card_id, image_url FROM deck_faces WHERE deck_code=$1 AND image_url<>''`, code)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			return nil, err
		}
		out[id] = strings.TrimSpace(url)
	}
	return out, rows.Err()
}

func (s *Store) loadCustomDeckFaces(ctx context.Context, deckID int64) (map[int]string, error) {
	out := map[int]string{}
	rows, err := s.Query(ctx, `SELECT card_id, image_url FROM custom_deck_faces WHERE deck_id=$1 AND image_url<>''`, deckID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var url string
		if err := rows.Scan(&id, &url); err != nil {
			return nil, err
		}
		out[id] = strings.TrimSpace(url)
	}
	return out, rows.Err()
}

func (s *Store) deckFaceCounts(ctx context.Context) (map[string]int, error) {
	out := map[string]int{}
	rows, err := s.Query(ctx, `SELECT deck_code, count(*) FROM deck_faces WHERE image_url<>'' GROUP BY deck_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var n int
		if err := rows.Scan(&code, &n); err != nil {
			return nil, err
		}
		out[code] = n
	}
	return out, rows.Err()
}

func (s *Store) replaceDeckFaces(ctx context.Context, code string, faces []deckFaceIn) error {
	if faces == nil {
		return nil
	}
	seen := map[int]bool{}
	for _, f := range faces {
		if f.CardID < 0 || f.CardID > 77 {
			return biz("牌序号不正确")
		}
		if seen[f.CardID] {
			return biz("牌序号不正确")
		}
		seen[f.CardID] = true
		if err := optionalAssetURL(strings.TrimSpace(f.ImageURL)); err != nil {
			return err
		}
	}
	if _, err := s.Exec(ctx, `DELETE FROM deck_faces WHERE deck_code=$1`, code); err != nil {
		return err
	}
	for _, f := range faces {
		url := strings.TrimSpace(f.ImageURL)
		if url == "" {
			continue
		}
		if _, err := s.Exec(ctx, `INSERT INTO deck_faces(deck_code, card_id, image_url) VALUES ($1,$2,$3)`,
			code, f.CardID, url); err != nil {
			return err
		}
	}
	return nil
}

func applyDeckFacesToCfg(cfg map[string]string, cards []cardRow, faces map[int]string) {
	if len(faces) == 0 {
		return
	}
	for _, c := range cards {
		if u := strings.TrimSpace(faces[c.ID]); u != "" {
			cfg[cardThumbnailKey(c.Name)] = u
		}
	}
}

func (d deckRow) public(r *http.Request) map[string]any {
	name, desc := d.Name, d.Description
	if r != nil {
		name = T(r.Context(), d.Name)
		if d.Description != "" {
			desc = T(r.Context(), d.Description)
		}
	}
	return map[string]any{
		"code": d.Code, "name": name, "thumbnail": absoluteAssetURL(r, deckPreviewURL(d.Thumbnail)),
		"cover_url": absoluteAssetURL(r, deckPreviewURL(d.Thumbnail)),
		"back_url":  emptyAsNil(absoluteAssetURL(r, strings.TrimSpace(d.BackURL))),
		"available": d.Enabled, "description": desc, "tags": parseDeckTags(d.Tags), "unlock_required": d.UnlockRequired,
	}
}

func (d deckRow) admin(faceCount int, faces []map[string]any) map[string]any {
	cover := strings.TrimSpace(d.Thumbnail)
	back := strings.TrimSpace(d.BackURL)
	return map[string]any{
		"code": d.Code, "name": d.Name,
		"thumbnail": nullableConfig(cover), "cover_url": nullableConfig(cover),
		"preview": deckPreviewURL(cover), "back_url": nullableConfig(back),
		"back_preview": emptyAsNil(back),
		"face_count": faceCount, "card_count": 78, "faces": faces,
		"enabled": d.Enabled, "sort_order": d.Sort,
		"description": d.Description, "tags": parseDeckTags(d.Tags),
	}
}

func publicDeckList(r *http.Request, rows []deckRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, d := range rows {
		out = append(out, d.public(r))
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

func (a *App) resolveUserDeck(ctx context.Context, uid int64, r *http.Request) (string, []map[string]any, map[string]any, error) {
	available, err := a.Store.listDecks(ctx, true)
	if err != nil {
		return "", nil, nil, err
	}
	if len(available) == 0 {
		available = []deckRow{{Code: "rider-waite", Name: "莱德韦特", Enabled: true}}
	}
	unlocks, err := a.loadDeckUnlocks(ctx, uid)
	if err != nil {
		return "", nil, nil, err
	}
	usable := make([]deckRow, 0, len(available))
	for _, d := range available {
		if unlocks.allows(d.Code, d.UnlockRequired) {
			usable = append(usable, d)
		}
	}
	available = usable
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
	var custom map[string]any
	if customID, err := a.userCustomDeckID(ctx, uid); err != nil {
		return "", nil, nil, err
	} else if customID > 0 {
		d, err := a.loadCustomDeck(ctx, uid, customID)
		if err == nil && d.Status == "saved" {
			code = customDeckCode(customID)
			custom = map[string]any{"id": d.ID, "name": d.Name, "code": code, "status": d.Status}
		}
	}
	return code, publicDeckList(r, available), custom, nil
}

func (a *App) adminDeckItems(r *http.Request) ([]map[string]any, error) {
	// Backfill rider-waite only when that set still has no faces (old DBs / first boot).
	_ = a.Store.ensureRiderWaiteFaces(r.Context())
	rows, err := a.Store.listDecks(r.Context(), false)
	if err != nil {
		return nil, err
	}
	counts, err := a.Store.deckFaceCounts(r.Context())
	if err != nil {
		return nil, err
	}
	cards, err := a.Store.listCards(r.Context())
	if err != nil {
		return nil, err
	}
	list := make([]map[string]any, 0, len(rows))
	for _, d := range rows {
		facesMap, err := a.Store.loadDeckFaces(r.Context(), d.Code)
		if err != nil {
			return nil, err
		}
		faces := make([]map[string]any, 0, len(cards))
		previews := make([]string, 0, 6)
		for _, c := range cards {
			url := strings.TrimSpace(facesMap[c.ID])
			abs := absoluteAssetURL(r, url)
			faces = append(faces, map[string]any{
				"card_id": c.ID, "card_name": c.Name, "image_url": nullableConfig(abs),
			})
			if abs != "" && len(previews) < 6 {
				previews = append(previews, abs)
			}
		}
		item := d.admin(counts[d.Code], faces)
		item["cover_url"] = nullableConfig(absoluteAssetURL(r, strings.TrimSpace(d.Thumbnail)))
		item["thumbnail"] = item["cover_url"]
		item["back_url"] = nullableConfig(absoluteAssetURL(r, strings.TrimSpace(d.BackURL)))
		item["face_previews"] = previews
		list = append(list, item)
	}
	return list, nil
}
