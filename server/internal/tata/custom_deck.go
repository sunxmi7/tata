package tata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	customDeckNameMax = 20
	customDeckLimit   = 10
	customDeckUpload  = "upload"
	customDeckPrefix  = "custom:"
	defaultCustomName = "我的自定义牌组"
)

type customDeckRow struct {
	ID         int64
	UserID     int64
	Name       string
	Status     string
	FaceSource string
	BackSource string
	BackURL    string
}

func (a *App) appListCustomDecks(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT d.id, d.name, d.status, d.face_source, d.back_source, d.back_url,
		(SELECT count(*) FROM custom_deck_faces f WHERE f.deck_id=d.id AND f.image_url<>'')
		FROM custom_decks d WHERE d.user_id=$1 ORDER BY d.id DESC`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	inUse, err := a.userCustomDeckID(r.Context(), uid)
	if err != nil {
		return err
	}
	items := []map[string]any{}
	for rows.Next() {
		var d customDeckRow
		var uploaded int
		if err := rows.Scan(&d.ID, &d.Name, &d.Status, &d.FaceSource, &d.BackSource, &d.BackURL, &uploaded); err != nil {
			return err
		}
		d.UserID = uid
		items = append(items, a.customDeckSummary(r, d, uploaded, inUse))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{"items": items, "limit": customDeckLimit})
	return nil
}

func (a *App) appCreateCustomDeck(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	var req customDeckBody
	if err := decode(r, &req); err != nil {
		return err
	}
	var n int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM custom_decks WHERE user_id=$1`, uid).Scan(&n); err != nil {
		return err
	}
	if n >= customDeckLimit {
		return biz("自定义牌组数量已达上限")
	}
	rawName := ""
	if req.Name != nil {
		rawName = *req.Name
	}
	name, err := normalizeCustomDeckName(rawName, defaultCustomName)
	if err != nil {
		return err
	}
	face, back, backURL, err := a.normalizeCustomSources(r.Context(), req.FaceSource, req.BackSource, req.BackURL)
	if err != nil {
		return err
	}
	status := "draft"
	if req.Status == "saved" {
		status = "saved"
	}
	var id int64
	if err := a.Store.QueryRow(r.Context(), `INSERT INTO custom_decks(user_id, name, status, face_source, back_source, back_url)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, uid, name, status, face, back, backURL).Scan(&id); err != nil {
		return err
	}
	if err := a.replaceCustomFaces(r.Context(), id, req.Faces); err != nil {
		return err
	}
	view, err := a.loadCustomDeckView(r, uid, id)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appGetCustomDeck(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	view, err := a.loadCustomDeckView(r, uid, id)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appPutCustomDeck(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	cur, err := a.loadCustomDeck(r.Context(), uid, id)
	if err != nil {
		return err
	}
	var req customDeckBody
	if err := decode(r, &req); err != nil {
		return err
	}
	name := cur.Name
	if req.Name != nil {
		name, err = normalizeCustomDeckName(*req.Name, cur.Name)
		if err != nil {
			return err
		}
	}
	face := cur.FaceSource
	if req.FaceSource != nil {
		face = strings.TrimSpace(*req.FaceSource)
	}
	back := cur.BackSource
	if req.BackSource != nil {
		back = strings.TrimSpace(*req.BackSource)
	}
	backURL := cur.BackURL
	if req.BackURL != nil {
		backURL = strings.TrimSpace(*req.BackURL)
	}
	face, back, backURL, err = a.normalizeCustomSources(r.Context(), strPtr(face), strPtr(back), strPtr(backURL))
	if err != nil {
		return err
	}
	status := cur.Status
	if req.Status == "saved" || req.Status == "draft" {
		status = req.Status
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE custom_decks SET name=$1, status=$2, face_source=$3, back_source=$4, back_url=$5, updated_at=now()
		WHERE id=$6 AND user_id=$7`, name, status, face, back, backURL, id, uid); err != nil {
		return err
	}
	if req.Faces != nil {
		if err := a.replaceCustomFaces(r.Context(), id, req.Faces); err != nil {
			return err
		}
	}
	view, err := a.loadCustomDeckView(r, uid, id)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appUseCustomDeck(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	d, err := a.loadCustomDeck(r.Context(), uid, id)
	if err != nil {
		return err
	}
	if d.Status != "saved" {
		return biz("请先保存牌组")
	}
	if err := a.setUserDeck(r.Context(), uid, d.fallbackOfficial(), id); err != nil {
		return err
	}
	view, err := a.loadCustomDeckView(r, uid, id)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

type customDeckBody struct {
	Name       *string        `json:"name"`
	Status     string         `json:"status"`
	FaceSource *string        `json:"face_source"`
	BackSource *string        `json:"back_source"`
	BackURL    *string        `json:"back_url"`
	Faces      []customFaceIn `json:"faces"`
}

type customFaceIn struct {
	CardID   int    `json:"card_id"`
	ImageURL string `json:"image_url"`
}

func strPtr(s string) *string { return &s }

func normalizeCustomDeckName(raw, fallback string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return fallback, nil
	}
	if runeLen(name) > customDeckNameMax {
		return "", biz("牌组名称过长")
	}
	return name, nil
}

func (a *App) normalizeCustomSources(ctx context.Context, face, back, backURL *string) (string, string, string, error) {
	fs := ""
	if face != nil {
		fs = strings.TrimSpace(*face)
	}
	bs := ""
	if back != nil {
		bs = strings.TrimSpace(*back)
	}
	url := ""
	if backURL != nil {
		url = strings.TrimSpace(*backURL)
	}
	if err := a.validCustomSource(ctx, fs); err != nil {
		return "", "", "", err
	}
	if err := a.validCustomSource(ctx, bs); err != nil {
		return "", "", "", biz("牌背来源不正确")
	}
	if err := optionalAssetURL(url); err != nil {
		return "", "", "", err
	}
	return fs, bs, url, nil
}

func (a *App) validCustomSource(ctx context.Context, source string) error {
	source = strings.TrimSpace(source)
	if source == "" || source == customDeckUpload {
		return nil
	}
	if _, ok, err := a.enabledDeck(ctx, source); err != nil {
		return err
	} else if !ok {
		return biz("牌面来源不正确")
	}
	return nil
}

func (a *App) replaceCustomFaces(ctx context.Context, deckID int64, faces []customFaceIn) error {
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
	if _, err := a.Store.Exec(ctx, `DELETE FROM custom_deck_faces WHERE deck_id=$1`, deckID); err != nil {
		return err
	}
	for _, f := range faces {
		url := strings.TrimSpace(f.ImageURL)
		if url == "" {
			continue
		}
		if _, err := a.Store.Exec(ctx, `INSERT INTO custom_deck_faces(deck_id, card_id, image_url) VALUES ($1,$2,$3)`,
			deckID, f.CardID, url); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) loadCustomDeck(ctx context.Context, uid, id int64) (customDeckRow, error) {
	var d customDeckRow
	err := a.Store.QueryRow(ctx, `SELECT id, user_id, name, status, face_source, back_source, back_url
		FROM custom_decks WHERE id=$1 AND user_id=$2`, id, uid).Scan(
		&d.ID, &d.UserID, &d.Name, &d.Status, &d.FaceSource, &d.BackSource, &d.BackURL)
	if errors.Is(err, sql.ErrNoRows) {
		return customDeckRow{}, biz("自定义牌组不存在")
	}
	return d, err
}

func (a *App) loadCustomDeckView(r *http.Request, uid, id int64) (map[string]any, error) {
	d, err := a.loadCustomDeck(r.Context(), uid, id)
	if err != nil {
		return nil, err
	}
	inUse, err := a.userCustomDeckID(r.Context(), uid)
	if err != nil {
		return nil, err
	}
	faces, uploaded, err := a.customFaceItems(r, d)
	if err != nil {
		return nil, err
	}
	out := a.customDeckSummary(r, d, uploaded, inUse)
	out["faces"] = faces
	return out, nil
}

func (a *App) customFaceItems(r *http.Request, d customDeckRow) ([]map[string]any, int, error) {
	cfg := a.cardConfig(r.Context())
	uploads := map[int]string{}
	rows, err := a.Store.Query(r.Context(), `SELECT card_id, image_url FROM custom_deck_faces WHERE deck_id=$1`, d.ID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var cardID int
		var url string
		if err := rows.Scan(&cardID, &url); err != nil {
			return nil, 0, err
		}
		if strings.TrimSpace(url) != "" {
			uploads[cardID] = strings.TrimSpace(url)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	cards, err := a.Store.listCards(r.Context())
	if err != nil {
		return nil, 0, err
	}
	out := make([]map[string]any, 0, len(cards))
	uploaded := 0
	for _, c := range cards {
		item := a.cardPublic(r, c, cfg)
		item["name"] = T(r.Context(), c.Name)
		item["card_name"] = item["name"]
		item["index_label"] = fmt.Sprintf("%02d", c.ID)
		if d.FaceSource == customDeckUpload {
			if u, ok := uploads[c.ID]; ok {
				item["image_url"] = absoluteAssetURL(r, u)
				item["card_thumbnail"] = absoluteAssetURL(r, u)
				item["uploaded"] = true
				uploaded++
			} else {
				item["image_url"] = nil
				item["uploaded"] = false
			}
		} else {
			item["image_url"] = item["card_thumbnail"]
			item["uploaded"] = false
		}
		out = append(out, item)
	}
	if d.FaceSource != customDeckUpload && d.FaceSource != "" {
		uploaded = len(out)
	}
	return out, uploaded, nil
}

func (a *App) customDeckSummary(r *http.Request, d customDeckRow, uploaded int, inUse int64) map[string]any {
	return map[string]any{
		"id": d.ID, "name": d.Name, "status": d.Status,
		"face_source": d.FaceSource, "back_source": d.BackSource,
		"back_url":       emptyAsNil(absoluteAssetURL(r, d.BackURL)),
		"uploaded_count": uploaded, "card_count": 78,
		"in_use": inUse == d.ID, "code": customDeckCode(d.ID),
	}
}

func (d customDeckRow) fallbackOfficial() string {
	if d.FaceSource != "" && d.FaceSource != customDeckUpload {
		return d.FaceSource
	}
	return "rider-waite"
}

func customDeckCode(id int64) string {
	return customDeckPrefix + strconv.FormatInt(id, 10)
}

func parseCustomDeckCode(code string) (int64, bool) {
	code = strings.TrimSpace(code)
	if !strings.HasPrefix(code, customDeckPrefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(code, customDeckPrefix), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (a *App) userCustomDeckID(ctx context.Context, uid int64) (int64, error) {
	var id sql.NullInt64
	err := a.Store.QueryRow(ctx, `SELECT custom_deck_id FROM app_users WHERE id=$1`, uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !id.Valid {
		return 0, nil
	}
	return id.Int64, nil
}

func (a *App) setUserDeck(ctx context.Context, uid int64, official string, customID int64) error {
	if official == "" {
		official = "rider-waite"
	}
	var custom any
	if customID > 0 {
		custom = customID
	}
	_, err := a.Store.Exec(ctx, `UPDATE app_users SET deck_code=$1, custom_deck_id=$2 WHERE id=$3`, official, custom, uid)
	return err
}

func (a *App) appListCards(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	deckCode := strings.TrimSpace(r.URL.Query().Get("deck"))
	if deckCode == "" {
		code, _, _, err := a.resolveUserDeck(r.Context(), uid, r)
		if err != nil {
			return err
		}
		deckCode = code
	}
	if customID, ok := parseCustomDeckCode(deckCode); ok {
		view, err := a.loadCustomDeckView(r, uid, customID)
		if err != nil {
			return err
		}
		faces, _ := view["faces"].([]map[string]any)
		writeOK(w, a.cardsCatalog(r, view, faces))
		return nil
	}
	d, ok, err := a.enabledDeck(r.Context(), deckCode)
	if err != nil {
		return err
	}
	if !ok {
		return biz("该牌面风格尚未开放")
	}
	cfg := a.cardConfig(r.Context())
	rows, err := a.Store.listCards(r.Context())
	if err != nil {
		return err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		item := a.cardPublic(r, c, cfg)
		item["name"] = T(r.Context(), c.Name)
		item["card_name"] = item["name"]
		item["index_label"] = fmt.Sprintf("%02d", c.ID)
		items = append(items, item)
	}
	pub := d.public(r)
	meta := map[string]any{
		"code": deckCode, "name": pub["name"], "status": "saved", "in_use": false,
		"face_source": deckCode, "uploaded_count": len(items), "card_count": 78,
		"description": pub["description"], "tags": pub["tags"], "thumbnail": pub["thumbnail"],
	}
	current, _, _, err := a.resolveUserDeck(r.Context(), uid, r)
	if err != nil {
		return err
	}
	meta["in_use"] = current == deckCode
	writeOK(w, a.cardsCatalog(r, meta, items))
	return nil
}

func (a *App) cardsCatalog(r *http.Request, meta map[string]any, items []map[string]any) map[string]any {
	groups := []map[string]any{
		{"key": "major", "title": T(r.Context(), "大阿尔卡纳"), "items": []map[string]any{}},
		{"key": "wands", "title": T(r.Context(), "权杖"), "items": []map[string]any{}},
		{"key": "cups", "title": T(r.Context(), "圣杯"), "items": []map[string]any{}},
		{"key": "swords", "title": T(r.Context(), "宝剑"), "items": []map[string]any{}},
		{"key": "pentacles", "title": T(r.Context(), "星币"), "items": []map[string]any{}},
	}
	index := map[string]int{"major": 0, "权杖": 1, "圣杯": 2, "宝剑": 3, "星币": 4}
	for _, item := range items {
		slot := 0
		if arcana, _ := item["arcana"].(string); arcana == "major" {
			slot = 0
		} else if suit, ok := item["suit"].(string); ok {
			if i, found := index[suit]; found {
				slot = i
			}
		}
		g := groups[slot]
		list, _ := g["items"].([]map[string]any)
		g["items"] = append(list, item)
		groups[slot] = g
	}
	return map[string]any{"deck": meta, "groups": groups, "items": items}
}
