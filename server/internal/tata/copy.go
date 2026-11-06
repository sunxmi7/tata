package tata

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const thumbnailTemplateKey = "card.thumbnail_template"

var yearlySectionDefs = []struct{ key, title string }{
	{"theme", "年度主题"},
	{"domain", "领域启示"},
	{"rhythm", "节奏变化"},
	{"action", "行动建议"},
}

func copyStatus(body string) string {
	if strings.TrimSpace(body) == "" || strings.TrimSpace(body) == helpBodyPending {
		return "pending"
	}
	return "ready"
}

func yearlyConfigKey(year int, section string) string {
	return "yearly." + strconv.Itoa(year) + "." + section
}

func cardYiKey(name string) string {
	return "card." + name + ".yi"
}

func cardThumbnailKey(name string) string {
	return "card." + name + ".thumbnail"
}

func nullableConfig(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

func defaultCardThumbnail(cfg map[string]string) string {
	if v := strings.TrimSpace(cfg[thumbnailDefaultKey]); v != "" {
		return v
	}
	return defaultCardThumbnailPath
}

func resolveCardThumbnail(cfg map[string]string, name string) any {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if v := strings.TrimSpace(cfg[cardThumbnailKey(name)]); v != "" {
		return v
	}
	tpl := strings.TrimSpace(cfg[thumbnailTemplateKey])
	if tpl != "" {
		if card, ok := cardByName(name); ok {
			out := strings.ReplaceAll(tpl, "{id}", strconv.Itoa(card.ID))
			out = strings.ReplaceAll(out, "{name}", url.PathEscape(name))
			if strings.TrimSpace(out) != "" {
				return out
			}
		}
	}
	return defaultCardThumbnail(cfg)
}

func (s *Store) putConfig(ctx context.Context, key, value string) error {
	_, err := s.Exec(ctx, `INSERT INTO app_config(key, value, updated_at) VALUES ($1,$2,now())
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()`, key, strings.TrimSpace(value))
	return err
}

func (s *Store) configMap(ctx context.Context, prefix string) (map[string]string, error) {
	rows, err := s.Query(ctx, `SELECT key, value FROM app_config WHERE key LIKE $1`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = strings.TrimSpace(value)
	}
	return out, rows.Err()
}

func (a *App) loadYearlySections(ctx context.Context, year int) ([]map[string]any, bool, error) {
	cfg, err := a.Store.configMap(ctx, "yearly."+strconv.Itoa(year)+".")
	if err != nil {
		return nil, false, err
	}
	allReady := true
	out := make([]map[string]any, 0, len(yearlySectionDefs))
	for _, def := range yearlySectionDefs {
		body := cfg[yearlyConfigKey(year, def.key)]
		status := copyStatus(body)
		if status != "ready" {
			allReady = false
			body = ""
		}
		out = append(out, map[string]any{
			"key": def.key, "title": def.title, "body": body, "content_status": status,
		})
	}
	return out, allReady, nil
}

func (a *App) cardConfig(ctx context.Context) map[string]string {
	cfg, err := a.Store.configMap(ctx, "card.")
	if err != nil {
		return map[string]string{}
	}
	return cfg
}

func (a *App) applyCardThumbnails(ctx context.Context, items ...map[string]any) {
	cfg := a.cardConfig(ctx)
	for _, item := range items {
		if item == nil {
			continue
		}
		name, _ := item["card_name"].(string)
		if name == "" {
			name = firstCardName(item["cards"])
		}
		item["card_thumbnail"] = resolveCardThumbnail(cfg, name)
	}
}

func (a *App) attachDailyHome(ctx context.Context, item map[string]any, target, business string) {
	name := firstCardName(item["cards"])
	cfg := a.cardConfig(ctx)
	attachDailyHome(item, target, business, nullableConfig(cfg[cardYiKey(name)]), resolveCardThumbnail(cfg, name))
}

func (a *App) adminGetCopy(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	kind := strings.TrimSpace(r.PathValue("kind"))
	switch kind {
	case "yearly":
		year, err := copyYear(r)
		if err != nil {
			return err
		}
		sections, ready, err := a.loadYearlySections(r.Context(), year)
		if err != nil {
			return err
		}
		status := "pending"
		if ready {
			status = "ready"
		}
		writeOK(w, map[string]any{"kind": kind, "year": year, "sections": sections, "content_status": status})
		return nil
	case "cards":
		cfg, err := a.Store.configMap(r.Context(), "card.")
		if err != nil {
			return err
		}
		list := []map[string]any{}
		for _, card := range deck() {
			list = append(list, map[string]any{
				"card_id":        card.ID,
				"card_name":      card.Name,
				"yi":             nullableConfig(cfg[cardYiKey(card.Name)]),
				"thumbnail":      nullableConfig(cfg[cardThumbnailKey(card.Name)]),
				"card_thumbnail": resolveCardThumbnail(cfg, card.Name),
			})
		}
		writeOK(w, map[string]any{
			"kind":               kind,
			"thumbnail_template": nullableConfig(cfg[thumbnailTemplateKey]),
			"thumbnail_default":  nullableConfig(cfg[thumbnailDefaultKey]),
			"items":              list,
		})
		return nil
	case "help":
		rows, err := a.Store.Query(r.Context(), `SELECT article_id, category_id, title, body, body_status, sort_order
			FROM help_articles ORDER BY category_id, sort_order, article_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []map[string]any{}
		for rows.Next() {
			var id, cat, title, body, status string
			var order int
			if err := rows.Scan(&id, &cat, &title, &body, &status, &order); err != nil {
				return err
			}
			list = append(list, map[string]any{
				"article_id": id, "category_id": cat, "title": title, "body": body, "body_status": status, "sort_order": order,
			})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		writeOK(w, map[string]any{"kind": kind, "items": list})
		return nil
	case "campaigns":
		rows, err := a.Store.Query(r.Context(), `SELECT campaign_id, title, subtitle, badge, detail_title, detail_body, detail_status, status, enabled, sort_order
			FROM campaigns ORDER BY sort_order, campaign_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		list := []map[string]any{}
		for rows.Next() {
			var id, title, subtitle, badge, detailTitle, detailBody, detailStatus, status string
			var enabled bool
			var order int
			if err := rows.Scan(&id, &title, &subtitle, &badge, &detailTitle, &detailBody, &detailStatus, &status, &enabled, &order); err != nil {
				return err
			}
			list = append(list, map[string]any{
				"campaign_id": id, "title": title, "subtitle": subtitle, "badge": badge,
				"detail_title": detailTitle, "detail_body": detailBody, "detail_status": detailStatus,
				"status": status, "enabled": enabled, "sort_order": order,
			})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		writeOK(w, map[string]any{"kind": kind, "items": list})
		return nil
	case "decks":
		rows, err := a.Store.listDecks(r.Context(), false)
		if err != nil {
			return err
		}
		list := make([]map[string]any, 0, len(rows))
		for _, d := range rows {
			list = append(list, d.admin())
		}
		writeOK(w, map[string]any{"kind": kind, "items": list})
		return nil
	default:
		return biz("文案类型不正确")
	}
}

func (a *App) adminPutCopy(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	kind := strings.TrimSpace(r.PathValue("kind"))
	switch kind {
	case "yearly":
		var req struct {
			Year     int `json:"year"`
			Sections []struct {
				Key  string `json:"key"`
				Body string `json:"body"`
			} `json:"sections"`
		}
		if err := decode(r, &req); err != nil {
			return err
		}
		if req.Year < 2000 || req.Year > 2100 {
			return biz("年份不正确")
		}
		allowed := map[string]bool{}
		for _, def := range yearlySectionDefs {
			allowed[def.key] = true
		}
		if len(req.Sections) == 0 {
			return biz("请填写年运段落")
		}
		for _, sec := range req.Sections {
			if !allowed[sec.Key] {
				return biz("年运段落不正确")
			}
			if err := a.Store.putConfig(r.Context(), yearlyConfigKey(req.Year, sec.Key), sec.Body); err != nil {
				return err
			}
		}
		a.Store.audit(r.Context(), act.id, "update", "copy", kind, strconv.Itoa(req.Year))
		sections, ready, err := a.loadYearlySections(r.Context(), req.Year)
		if err != nil {
			return err
		}
		status := "pending"
		if ready {
			status = "ready"
		}
		writeOK(w, map[string]any{"kind": kind, "year": req.Year, "sections": sections, "content_status": status})
		return nil
	case "cards":
		var req struct {
			CardName          string  `json:"card_name"`
			Yi                *string `json:"yi"`
			Thumbnail         *string `json:"thumbnail"`
			ThumbnailTemplate *string `json:"thumbnail_template"`
			ThumbnailDefault  *string `json:"thumbnail_default"`
		}
		if err := decode(r, &req); err != nil {
			return err
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
		name := strings.TrimSpace(req.CardName)
		if name == "" {
			if req.ThumbnailTemplate == nil && req.ThumbnailDefault == nil {
				return biz("牌名不正确")
			}
			a.Store.audit(r.Context(), act.id, "update", "copy", kind, "card_assets")
			break
		}
		if !knownCardName(name) {
			return biz("牌名不正确")
		}
		if req.Yi != nil {
			if err := a.Store.putConfig(r.Context(), cardYiKey(name), *req.Yi); err != nil {
				return err
			}
		}
		if req.Thumbnail != nil {
			if err := a.Store.putConfig(r.Context(), cardThumbnailKey(name), *req.Thumbnail); err != nil {
				return err
			}
		}
		a.Store.audit(r.Context(), act.id, "update", "copy", kind, name)
	case "help":
		var req struct {
			ArticleID string `json:"article_id"`
			Title     string `json:"title"`
			Body      string `json:"body"`
		}
		if err := decode(r, &req); err != nil {
			return err
		}
		if !validContentID(req.ArticleID) {
			return biz("问题不存在")
		}
		body := strings.TrimSpace(req.Body)
		status := copyStatus(body)
		if status != "ready" {
			body = helpBodyPending
			status = "pending"
		}
		title := strings.TrimSpace(req.Title)
		var tag tagResult
		if title == "" {
			tag, err = a.Store.Exec(r.Context(), `UPDATE help_articles SET body=$1, body_status=$2 WHERE article_id=$3`,
				body, status, req.ArticleID)
		} else {
			tag, err = a.Store.Exec(r.Context(), `UPDATE help_articles SET title=$1, body=$2, body_status=$3 WHERE article_id=$4`,
				title, body, status, req.ArticleID)
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return biz("问题不存在")
		}
		a.Store.audit(r.Context(), act.id, "update", "copy", kind, req.ArticleID)
	case "campaigns":
		var req struct {
			CampaignID  string `json:"campaign_id"`
			Title       string `json:"title"`
			Subtitle    string `json:"subtitle"`
			Badge       string `json:"badge"`
			DetailTitle string `json:"detail_title"`
			DetailBody  string `json:"detail_body"`
			Status      string `json:"status"`
			Enabled     *bool  `json:"enabled"`
		}
		if err := decode(r, &req); err != nil {
			return err
		}
		if !validContentID(req.CampaignID) {
			return biz("活动不存在")
		}
		var curTitle, curSub, curBadge, curDetailTitle, curDetailBody, curStatus string
		var curEnabled bool
		err := a.Store.QueryRow(r.Context(), `SELECT title, subtitle, badge, detail_title, detail_body, status, enabled
			FROM campaigns WHERE campaign_id=$1`, req.CampaignID).Scan(&curTitle, &curSub, &curBadge, &curDetailTitle, &curDetailBody, &curStatus, &curEnabled)
		if errors.Is(err, sql.ErrNoRows) {
			return biz("活动不存在")
		}
		if err != nil {
			return err
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = curTitle
		}
		subtitle := req.Subtitle
		if subtitle == "" {
			subtitle = curSub
		}
		badge := req.Badge
		if badge == "" {
			badge = curBadge
		}
		detailTitle := strings.TrimSpace(req.DetailTitle)
		if detailTitle == "" {
			detailTitle = curDetailTitle
		}
		detailBody := strings.TrimSpace(req.DetailBody)
		if req.DetailBody == "" {
			detailBody = curDetailBody
		}
		status := strings.TrimSpace(req.Status)
		if status == "" {
			status = curStatus
		}
		if status != "ongoing" && status != "ended" {
			return biz("活动状态不正确")
		}
		enabled := curEnabled
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		detailStatus := copyStatus(detailBody)
		if _, err := a.Store.Exec(r.Context(), `UPDATE campaigns SET title=$1, subtitle=$2, badge=$3, detail_title=$4, detail_body=$5, detail_status=$6, status=$7, enabled=$8
			WHERE campaign_id=$9`,
			title, subtitle, badge, detailTitle, detailBody, detailStatus, status, enabled, req.CampaignID); err != nil {
			return err
		}
		a.Store.audit(r.Context(), act.id, "update", "copy", kind, req.CampaignID)
	case "decks":
		var req struct {
			Code      string  `json:"code"`
			Name      string  `json:"name"`
			Thumbnail *string `json:"thumbnail"`
			Enabled   *bool   `json:"enabled"`
			SortOrder *int    `json:"sort_order"`
		}
		if err := decode(r, &req); err != nil {
			return err
		}
		code := strings.ToLower(strings.TrimSpace(req.Code))
		if !validContentID(code) {
			return biz("牌面编码不正确")
		}
		var curName, curThumb string
		var curEnabled bool
		var curSort int
		err := a.Store.QueryRow(r.Context(), `SELECT name, thumbnail, enabled, sort_order FROM decks WHERE code=$1`, code).
			Scan(&curName, &curThumb, &curEnabled, &curSort)
		exists := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = curName
		}
		if name == "" {
			return biz("请填写牌面名称")
		}
		thumb := curThumb
		if req.Thumbnail != nil {
			thumb = strings.TrimSpace(*req.Thumbnail)
		}
		enabled := true
		if exists {
			enabled = curEnabled
		}
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		sortOrder := curSort
		if req.SortOrder != nil {
			sortOrder = *req.SortOrder
		}
		if !enabled {
			var n int
			if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM decks WHERE enabled=true AND code<>$1`, code).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return biz("至少保留一套启用的牌面风格")
			}
		}
		if exists {
			if _, err := a.Store.Exec(r.Context(), `UPDATE decks SET name=$1, thumbnail=$2, enabled=$3, sort_order=$4, updated_at=now() WHERE code=$5`,
				name, thumb, enabled, sortOrder, code); err != nil {
				return err
			}
		} else {
			if _, err := a.Store.Exec(r.Context(), `INSERT INTO decks(code, name, thumbnail, enabled, sort_order) VALUES ($1,$2,$3,$4,$5)`,
				code, name, thumb, enabled, sortOrder); err != nil {
				return err
			}
		}
		a.Store.audit(r.Context(), act.id, "update", "copy", kind, code)
	default:
		return biz("文案类型不正确")
	}
	return a.adminGetCopy(w, r)
}

func copyYear(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("year"))
	if raw == "" {
		return time.Now().In(time.FixedZone("CST", 8*3600)).Year(), nil
	}
	year, err := strconv.Atoi(raw)
	if err != nil || year < 2000 || year > 2100 {
		return 0, biz("年份不正确")
	}
	return year, nil
}
