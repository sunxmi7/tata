package tata

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type cardRow struct {
	ID         int
	Name       string
	NameZhHant string
	NameEN     string
	NameJA     string
	NameKO     string
	Arcana     string
	Suit       string
	Rank       string
	Yi         string
	Thumbnail  string
	Enabled    bool
	SortOrder  int
}

func classifyCard(id int, name string) (arcana, suit, rank string) {
	if id >= 0 && id < 22 {
		return "major", "", ""
	}
	for _, s := range suits {
		if strings.HasPrefix(name, s) {
			return "minor", s, strings.TrimPrefix(name, s)
		}
	}
	return "minor", "", ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func (s *Store) listCards(ctx context.Context) ([]cardRow, error) {
	rows, err := s.Query(ctx, `SELECT id, name, name_zh_hant, name_en, name_ja, name_ko, arcana, suit, rank, yi, thumbnail, enabled, sort_order
		FROM cards ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []cardRow{}
	for rows.Next() {
		var c cardRow
		if err := rows.Scan(&c.ID, &c.Name, &c.NameZhHant, &c.NameEN, &c.NameJA, &c.NameKO, &c.Arcana, &c.Suit, &c.Rank, &c.Yi, &c.Thumbnail, &c.Enabled, &c.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) getCard(ctx context.Context, id int) (cardRow, error) {
	var c cardRow
	err := s.QueryRow(ctx, `SELECT id, name, name_zh_hant, name_en, name_ja, name_ko, arcana, suit, rank, yi, thumbnail, enabled, sort_order
		FROM cards WHERE id=$1`, id).Scan(&c.ID, &c.Name, &c.NameZhHant, &c.NameEN, &c.NameJA, &c.NameKO, &c.Arcana, &c.Suit, &c.Rank, &c.Yi, &c.Thumbnail, &c.Enabled, &c.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return cardRow{}, biz("牌不存在")
	}
	return c, err
}

func (s *Store) seedCards(ctx context.Context) error {
	data := loadSeedContent()
	byName := map[string]seedCard{}
	for _, c := range data.Cards {
		byName[c.Name] = c
	}
	cfg, err := s.configMap(ctx, "card.")
	if err != nil {
		return err
	}
	for _, def := range deck() {
		sc := byName[def.Name]
		arcana, suit, rank := classifyCard(def.ID, def.Name)
		thumb := firstNonEmpty(cfg[cardThumbnailKey(def.Name)])
		if thumb == "" && strings.TrimSpace(sc.File) != "" {
			thumb = "/uploads/cards/" + sc.File
		}
		yi := firstNonEmpty(cfg[cardYiKey(def.Name)])
		en := firstNonEmpty(cfg["card."+def.Name+".name_en"], sc.EN)
		ja := firstNonEmpty(cfg["card."+def.Name+".name_ja"], sc.JA)
		ko := firstNonEmpty(cfg["card."+def.Name+".name_ko"], sc.KO)
		hant := firstNonEmpty(cfg["card."+def.Name+".name_zh_hant"], catalogGet("zh-Hant", def.Name), def.Name)
		if _, err := s.Exec(ctx, `INSERT INTO cards(id, name, name_zh_hant, name_en, name_ja, name_ko, arcana, suit, rank, yi, thumbnail, enabled, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,true,$1)
			ON CONFLICT (id) DO UPDATE SET
				name_zh_hant = CASE WHEN cards.name_zh_hant = '' THEN EXCLUDED.name_zh_hant ELSE cards.name_zh_hant END,
				name_en = CASE WHEN cards.name_en = '' THEN EXCLUDED.name_en ELSE cards.name_en END,
				name_ja = CASE WHEN cards.name_ja = '' THEN EXCLUDED.name_ja ELSE cards.name_ja END,
				name_ko = CASE WHEN cards.name_ko = '' THEN EXCLUDED.name_ko ELSE cards.name_ko END,
				arcana = CASE WHEN cards.arcana = '' THEN EXCLUDED.arcana ELSE cards.arcana END,
				suit = CASE WHEN cards.suit = '' THEN EXCLUDED.suit ELSE cards.suit END,
				rank = CASE WHEN cards.rank = '' THEN EXCLUDED.rank ELSE cards.rank END,
				yi = CASE WHEN cards.yi = '' THEN EXCLUDED.yi ELSE cards.yi END,
				thumbnail = CASE WHEN cards.thumbnail = '' THEN EXCLUDED.thumbnail ELSE cards.thumbnail END`,
			def.ID, def.Name, hant, en, ja, ko, arcana, suit, rank, yi, thumb); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) patchCardAssets(ctx context.Context, name string, yi, thumb *string) error {
	name = strings.TrimSpace(name)
	var id int
	err := s.QueryRow(ctx, `SELECT id FROM cards WHERE name=$1`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		def, ok := cardByName(name)
		if !ok {
			return biz("牌名不正确")
		}
		arcana, suit, rank := classifyCard(def.ID, def.Name)
		y, t := "", ""
		if yi != nil {
			y = strings.TrimSpace(*yi)
		}
		if thumb != nil {
			t = strings.TrimSpace(*thumb)
		}
		_, err = s.Exec(ctx, `INSERT INTO cards(id, name, arcana, suit, rank, yi, thumbnail, enabled, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,true,$1)`, def.ID, def.Name, arcana, suit, rank, y, t)
		return err
	}
	if err != nil {
		return err
	}
	if yi != nil {
		if _, err := s.Exec(ctx, `UPDATE cards SET yi=$1, updated_at=now() WHERE id=$2`, strings.TrimSpace(*yi), id); err != nil {
			return err
		}
	}
	if thumb != nil {
		if _, err := s.Exec(ctx, `UPDATE cards SET thumbnail=$1, updated_at=now() WHERE id=$2`, strings.TrimSpace(*thumb), id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) nextCardID(ctx context.Context) (int, error) {
	var n sql.NullInt64
	if err := s.QueryRow(ctx, `SELECT MAX(id) FROM cards`).Scan(&n); err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return int(n.Int64) + 1, nil
}

func cardUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

func (a *App) cardConfig(ctx context.Context) map[string]string {
	cfg, err := a.Store.configMap(ctx, "card.")
	if err != nil {
		cfg = map[string]string{}
	}
	rows, err := a.Store.listCards(ctx)
	if err != nil {
		return cfg
	}
	for _, c := range rows {
		cfg[cardYiKey(c.Name)] = strings.TrimSpace(c.Yi)
		cfg[cardThumbnailKey(c.Name)] = strings.TrimSpace(c.Thumbnail)
		if v := strings.TrimSpace(c.NameEN); v != "" {
			cfg["card."+c.Name+".name_en"] = v
		}
		if v := strings.TrimSpace(c.NameJA); v != "" {
			cfg["card."+c.Name+".name_ja"] = v
		}
		if v := strings.TrimSpace(c.NameKO); v != "" {
			cfg["card."+c.Name+".name_ko"] = v
		}
		if v := strings.TrimSpace(c.NameZhHant); v != "" {
			cfg["card."+c.Name+".name_zh_hant"] = v
		}
	}
	return cfg
}

func (a *App) cardPublic(r *http.Request, c cardRow, cfg map[string]string) map[string]any {
	merged := map[string]string{}
	for k, v := range cfg {
		merged[k] = v
	}
	if strings.TrimSpace(c.Thumbnail) != "" {
		merged[cardThumbnailKey(c.Name)] = c.Thumbnail
	}
	return map[string]any{
		"id":             c.ID,
		"card_id":        c.ID,
		"name":           c.Name,
		"card_name":      c.Name,
		"name_zh_hant":   c.NameZhHant,
		"name_en":        c.NameEN,
		"name_ja":        c.NameJA,
		"name_ko":        c.NameKO,
		"arcana":         c.Arcana,
		"suit":           nullableConfig(c.Suit),
		"rank":           nullableConfig(c.Rank),
		"yi":             nullableConfig(c.Yi),
		"thumbnail":      nullableConfig(c.Thumbnail),
		"card_thumbnail": absoluteAssetAny(r, resolveCardThumbnail(merged, c.Name)),
		"enabled":        c.Enabled,
		"sort_order":     c.SortOrder,
	}
}

func (a *App) writeCardsPayload(w http.ResponseWriter, r *http.Request) error {
	cfg, err := a.Store.configMap(r.Context(), "card.")
	if err != nil {
		return err
	}
	rows, err := a.Store.listCards(r.Context())
	if err != nil {
		return err
	}
	list := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		list = append(list, a.cardPublic(r, c, cfg))
	}
	writeOK(w, map[string]any{
		"items":              list,
		"thumbnail_template": nullableConfig(cfg[thumbnailTemplateKey]),
		"thumbnail_default":  nullableConfig(cfg[thumbnailDefaultKey]),
	})
	return nil
}

func (a *App) adminListCards(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	return a.writeCardsPayload(w, r)
}

type cardBody struct {
	Name       string  `json:"name"`
	NameZhHant *string `json:"name_zh_hant"`
	NameEN     *string `json:"name_en"`
	NameJA     *string `json:"name_ja"`
	NameKO     *string `json:"name_ko"`
	Arcana     *string `json:"arcana"`
	Suit       *string `json:"suit"`
	Rank       *string `json:"rank"`
	Yi         *string `json:"yi"`
	Thumbnail  *string `json:"thumbnail"`
	Enabled    *bool   `json:"enabled"`
	SortOrder  *int    `json:"sort_order"`
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func (a *App) adminCreateCard(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req cardBody
	if err := decode(r, &req); err != nil {
		return err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return biz("请填写牌名")
	}
	id, err := a.Store.nextCardID(r.Context())
	if err != nil {
		return err
	}
	if id < 78 {
		id = 78
	}
	arcana, suit, rank := classifyCard(id, name)
	if req.Arcana != nil {
		arcana = strings.TrimSpace(*req.Arcana)
	}
	if req.Suit != nil {
		suit = strings.TrimSpace(*req.Suit)
	}
	if req.Rank != nil {
		rank = strings.TrimSpace(*req.Rank)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sortOrder := id
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO cards(id, name, name_zh_hant, name_en, name_ja, name_ko, arcana, suit, rank, yi, thumbnail, enabled, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		id, name, derefStr(req.NameZhHant), derefStr(req.NameEN), derefStr(req.NameJA), derefStr(req.NameKO),
		arcana, suit, rank, derefStr(req.Yi), derefStr(req.Thumbnail), enabled, sortOrder); err != nil {
		if cardUniqueErr(err) {
			return biz("牌名已存在")
		}
		return err
	}
	a.Store.audit(r.Context(), act.id, "create", "card", strconv.Itoa(id), name)
	writeOK(w, map[string]any{"id": id})
	return nil
}

func pathCardID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 0 {
		return 0, biz("牌不存在")
	}
	return id, nil
}

func (a *App) adminUpdateCard(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathCardID(r)
	if err != nil {
		return err
	}
	cur, err := a.Store.getCard(r.Context(), id)
	if err != nil {
		return err
	}
	var req cardBody
	if err := decode(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Name) != "" && strings.TrimSpace(req.Name) != cur.Name {
		return biz("不能修改牌名")
	}
	hant, en, ja, ko := cur.NameZhHant, cur.NameEN, cur.NameJA, cur.NameKO
	if req.NameZhHant != nil {
		hant = strings.TrimSpace(*req.NameZhHant)
	}
	if req.NameEN != nil {
		en = strings.TrimSpace(*req.NameEN)
	}
	if req.NameJA != nil {
		ja = strings.TrimSpace(*req.NameJA)
	}
	if req.NameKO != nil {
		ko = strings.TrimSpace(*req.NameKO)
	}
	arcana, suit, rank := cur.Arcana, cur.Suit, cur.Rank
	if req.Arcana != nil {
		arcana = strings.TrimSpace(*req.Arcana)
	}
	if req.Suit != nil {
		suit = strings.TrimSpace(*req.Suit)
	}
	if req.Rank != nil {
		rank = strings.TrimSpace(*req.Rank)
	}
	yi, thumb := cur.Yi, cur.Thumbnail
	if req.Yi != nil {
		yi = strings.TrimSpace(*req.Yi)
	}
	if req.Thumbnail != nil {
		thumb = strings.TrimSpace(*req.Thumbnail)
	}
	enabled := cur.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sortOrder := cur.SortOrder
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE cards SET name_zh_hant=$1, name_en=$2, name_ja=$3, name_ko=$4, arcana=$5, suit=$6, rank=$7, yi=$8, thumbnail=$9, enabled=$10, sort_order=$11, updated_at=now() WHERE id=$12`,
		hant, en, ja, ko, arcana, suit, rank, yi, thumb, enabled, sortOrder, id); err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "update", "card", strconv.Itoa(id), cur.Name)
	writeOK(w, true)
	return nil
}

func (a *App) adminPutCardSettings(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req struct {
		ThumbnailTemplate *string `json:"thumbnail_template"`
		ThumbnailDefault  *string `json:"thumbnail_default"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.ThumbnailTemplate == nil && req.ThumbnailDefault == nil {
		return biz("请填写牌图设置")
	}
	if req.ThumbnailTemplate != nil {
		if err := a.Store.putConfig(r.Context(), thumbnailTemplateKey, *req.ThumbnailTemplate); err != nil {
			return err
		}
	}
	if req.ThumbnailDefault != nil {
		if err := a.Store.putConfig(r.Context(), thumbnailDefaultKey, *req.ThumbnailDefault); err != nil {
			return err
		}
	}
	a.Store.audit(r.Context(), act.id, "update", "card", "settings", "card_assets")
	return a.writeCardsPayload(w, r)
}
