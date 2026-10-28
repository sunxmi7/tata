package tata

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

const helpQueryMax = 64

const (
	campaignEmptyTitle = "暂时没有可参与的活动"
	campaignEmptyHint  = "稍后再来看看"
	campaignFootnote   = "活动规则与奖励以活动详情页为准"
	campaignEntryOpen  = "查看活动"
	campaignEntryEnded = "活动已结束"
	campaignBadgeEnded = "已结束"

	redeemEmptyTitle  = "暂无兑换记录"
	redeemPendingNote = "记录字段待产品确认。成功权益以实际到账结果为准。"

	participationEmptyTitle  = "暂无参与记录"
	participationPendingNote = "列表字段与空状态待产品确认。此处不展示伪造记录。"
	rewardEmptyTitle         = "暂无奖励记录"
	rewardPendingNote        = "奖励发放与记录字段待产品确认。集合页不写未确认的奖励承诺。"

	helpSearchEmptyTitle = "暂时没有找到相关问题"
	helpSearchEmptyHint  = "换个关键词，或向我们反馈"
	helpBodyPending      = "待确认"
)

func (a *App) appRedeemRecords(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能查看兑换记录")
	}
	rows, err := a.Store.Query(r.Context(), `SELECT r.id, r.created_at, c.code, c.benefit_name, c.kind, c.amount
		FROM redeem_redemptions r
		JOIN redeem_codes c ON c.id = r.code_id
		WHERE r.user_id=$1
		ORDER BY r.id DESC
		LIMIT 100`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var created sql.NullTime
		var code, benefit, kind string
		var amount int
		if err := rows.Scan(&id, &created, &code, &benefit, &kind, &amount); err != nil {
			return err
		}
		unit := "diamond"
		var label any = displayLabelTataCoin
		if kind == "free_count" {
			unit = "count"
			label = nil
		}
		var at any
		if created.Valid {
			at = created.Time
		}
		items = append(items, map[string]any{
			"id": id, "created_at": at, "code": code, "benefit_name": benefit,
			"kind": kind, "amount": amount, "unit": unit, "display_label": label,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{
		"items": items, "empty_title": redeemEmptyTitle,
		"placeholder_note": redeemPendingNote, "fields_confirmed": false,
	})
	return nil
}

func (a *App) appCampaigns(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT campaign_id, title, subtitle, badge, status
		FROM campaigns WHERE enabled=true ORDER BY sort_order, campaign_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, title, subtitle, badge, status string
		if err := rows.Scan(&id, &title, &subtitle, &badge, &status); err != nil {
			return err
		}
		items = append(items, campaignCard(id, title, subtitle, badge, status))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{
		"items":       items,
		"empty_title": campaignEmptyTitle, "empty_hint": campaignEmptyHint,
		"footnote": campaignFootnote,
	})
	return nil
}

func campaignCard(id, title, subtitle, badge, status string) map[string]any {
	entry := campaignEntryOpen
	entryEnabled := true
	if status == "ended" {
		badge = campaignBadgeEnded
		entry = campaignEntryEnded
		entryEnabled = false
	}
	return map[string]any{
		"id": id, "title": title, "subtitle": subtitle, "status": status,
		"badge": badge, "entry_label": entry, "entry_enabled": entryEnabled,
	}
}

func (a *App) appCampaign(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if !validContentID(id) {
		return biz("活动不存在")
	}
	var title, subtitle, detailTitle, detailBody, detailStatus, status string
	var enabled bool
	err := a.Store.QueryRow(r.Context(), `SELECT title, subtitle, detail_title, detail_body, detail_status, status, enabled
		FROM campaigns WHERE campaign_id=$1`, id).Scan(&title, &subtitle, &detailTitle, &detailBody, &detailStatus, &status, &enabled)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !enabled) {
		return biz("活动不存在")
	}
	if err != nil {
		return err
	}
	writeOK(w, map[string]any{
		"id": id, "title": detailTitle, "list_title": title, "subtitle": subtitle,
		"status": status, "body": detailBody, "body_status": detailStatus,
		"detail_delivered": detailStatus == "ready",
	})
	return nil
}

func (a *App) appCampaignParticipations(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能查看参与记录")
	}
	writeOK(w, placeholderList(participationEmptyTitle, participationPendingNote))
	return nil
}

func (a *App) appCampaignRewards(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能查看奖励记录")
	}
	writeOK(w, placeholderList(rewardEmptyTitle, rewardPendingNote))
	return nil
}

func placeholderList(emptyTitle, note string) map[string]any {
	return map[string]any{
		"items": []map[string]any{}, "empty_title": emptyTitle,
		"placeholder_note": note, "fields_confirmed": false,
	}
}

func (a *App) appHelpCategories(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT category_id, title, articles_status, empty_note
		FROM help_categories ORDER BY sort_order, category_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	categories := []map[string]any{}
	for rows.Next() {
		var id, title, status, note string
		if err := rows.Scan(&id, &title, &status, &note); err != nil {
			return err
		}
		categories = append(categories, map[string]any{
			"id": id, "title": title, "articles_status": status, "empty_note": note,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{"categories": categories})
	return nil
}

func (a *App) appHelpArticles(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	category := strings.TrimSpace(r.URL.Query().Get("category"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > helpQueryMax {
		return biz("搜索关键词过长")
	}
	if category != "" && !validContentID(category) {
		return biz("问题分类不正确")
	}
	var categoryNote string
	if category != "" {
		var status, note string
		err := a.Store.QueryRow(r.Context(), `SELECT articles_status, empty_note FROM help_categories WHERE category_id=$1`, category).Scan(&status, &note)
		if errors.Is(err, sql.ErrNoRows) {
			return biz("问题分类不正确")
		}
		if err != nil {
			return err
		}
		if q == "" && status == "pending" {
			categoryNote = note
		}
	}
	rows, err := a.Store.Query(r.Context(), `SELECT a.article_id, a.title, c.category_id, c.title
		FROM help_articles a
		JOIN help_categories c ON c.category_id = a.category_id
		WHERE ($1 = '' OR a.category_id = $1)
		  AND ($2 = '' OR strpos(lower(a.title), lower($2)) > 0)
		ORDER BY c.sort_order, a.sort_order, a.article_id`, category, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, title, categoryID, categoryTitle string
		if err := rows.Scan(&id, &title, &categoryID, &categoryTitle); err != nil {
			return err
		}
		items = append(items, map[string]any{
			"id": id, "title": title, "category_id": categoryID, "category_title": categoryTitle,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	emptyTitle, emptyHint := "", ""
	if q != "" && len(items) == 0 {
		emptyTitle = helpSearchEmptyTitle
		emptyHint = helpSearchEmptyHint
		categoryNote = ""
	}
	if q == "" && len(items) > 0 {
		categoryNote = ""
	}
	writeOK(w, map[string]any{
		"items": items, "query": q,
		"empty_title": emptyTitle, "empty_hint": emptyHint,
		"category_note": categoryNote,
	})
	return nil
}

func (a *App) appHelpArticle(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if !validContentID(id) {
		return biz("问题不存在")
	}
	var title, body, bodyStatus, categoryID, categoryTitle string
	err := a.Store.QueryRow(r.Context(), `SELECT a.title, a.body, a.body_status, c.category_id, c.title
		FROM help_articles a
		JOIN help_categories c ON c.category_id = a.category_id
		WHERE a.article_id=$1`, id).Scan(&title, &body, &bodyStatus, &categoryID, &categoryTitle)
	if errors.Is(err, sql.ErrNoRows) {
		return biz("问题不存在")
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		body = helpBodyPending
		bodyStatus = "pending"
	}
	writeOK(w, map[string]any{
		"id": id, "title": title, "category_id": categoryID, "category_title": categoryTitle,
		"body": body, "body_status": bodyStatus,
	})
	return nil
}

func validContentID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}
