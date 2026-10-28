package tata

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	shellContentMax   = 500
	redeemReason      = "redeem"
	notifDefaultTime  = "08:00"
	uploadMaxBytes    = 5 << 20
	uploadNamePattern = 32
)

var feedbackCategories = map[string]bool{
	"使用问题": true,
	"内容建议": true,
	"其他":   true,
}

func (a *App) appExploreTopics(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT topic_id, group_key, group_title, title, subtitle, illustration, spread_id, sort_order
		FROM explore_topics ORDER BY sort_order, topic_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type group struct {
		Key    string
		Title  string
		Topics []map[string]any
	}
	order := []string{}
	groups := map[string]*group{}
	for rows.Next() {
		var topicID, groupKey, groupTitle, title, subtitle string
		var illustration sql.NullString
		var spreadID sql.NullInt64
		var sort int
		if err := rows.Scan(&topicID, &groupKey, &groupTitle, &title, &subtitle, &illustration, &spreadID, &sort); err != nil {
			return err
		}
		g, ok := groups[groupKey]
		if !ok {
			g = &group{Key: groupKey, Title: groupTitle, Topics: []map[string]any{}}
			groups[groupKey] = g
			order = append(order, groupKey)
		}
		item := map[string]any{
			"topic_id": topicID, "title": title, "subtitle": subtitle,
			"illustration": nullString(illustration),
			"spread": map[string]any{
				"spread_id": nullInt(spreadID),
				"stub":      !spreadID.Valid,
			},
		}
		g.Topics = append(g.Topics, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		g := groups[key]
		out = append(out, map[string]any{
			"group_key": g.Key, "title": g.Title, "topics": g.Topics,
		})
	}
	writeOK(w, map[string]any{"groups": out})
	return nil
}

func (a *App) appRedeem(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能兑换")
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" || utf8.RuneCountInString(code) > 64 {
		return biz("兑换码无效")
	}
	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if err := lockGrantUser(r.Context(), tx, uid); err != nil {
		return err
	}
	var codeID int64
	var kind, benefit string
	var amount int
	var max sql.NullInt64
	var enabled bool
	var expires sql.NullTime
	err = tx.QueryRow(r.Context(), `SELECT id, kind, amount, benefit_name, max_redemptions, enabled, expires_at
		FROM redeem_codes WHERE code=$1 FOR UPDATE`, code).Scan(&codeID, &kind, &amount, &benefit, &max, &enabled, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return biz("兑换码无效")
	}
	if err != nil {
		return err
	}
	if !enabled {
		return biz("兑换码无效")
	}
	if expires.Valid && !expires.Time.After(time.Now()) {
		return biz("兑换码已过期")
	}
	var usedByUser int
	if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM redeem_redemptions WHERE code_id=$1 AND user_id=$2`, codeID, uid).Scan(&usedByUser); err != nil {
		return err
	}
	if usedByUser > 0 {
		return biz("兑换码已使用")
	}
	if max.Valid {
		var used int
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM redeem_redemptions WHERE code_id=$1`, codeID).Scan(&used); err != nil {
			return err
		}
		if used >= int(max.Int64) {
			return biz("兑换码已使用")
		}
	}
	now := time.Now().UTC()
	var ledgerID any
	pack, sub := 0, 0
	switch kind {
	case "diamonds":
		id, p, s, err := creditRedeemDiamonds(r.Context(), tx, uid, amount, codeID, now)
		if err != nil {
			return err
		}
		ledgerID, pack, sub = id, p, s
	case "free_count":
		if err := creditRedeemFree(r.Context(), tx, uid, amount); err != nil {
			return err
		}
		pack, sub, err = diamondTotals(r.Context(), tx, uid)
		if err != nil {
			return err
		}
	default:
		return biz("兑换码无效")
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO redeem_redemptions(code_id, user_id, diamond_ledger_id, created_at)
		VALUES ($1,$2,$3,$4)`, codeID, uid, ledgerID, now); err != nil {
		return err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	unit := "diamond"
	if kind == "free_count" {
		unit = "count"
	}
	writeOK(w, map[string]any{
		"code": code, "benefit_name": benefit, "kind": kind, "amount": amount,
		"unit": unit, "display_label": displayLabelTataCoin,
		"pack_diamonds": pack, "subscription_diamonds": sub, "balance": pack + sub,
	})
	return nil
}

func creditRedeemDiamonds(ctx context.Context, tx Tx, uid int64, amount int, codeID int64, now time.Time) (int64, int, int, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO diamond_balances(user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return 0, 0, 0, err
	}
	var pack, sub int
	if err := tx.QueryRow(ctx, `SELECT pack_diamonds, subscription_diamonds FROM diamond_balances WHERE user_id=$1 FOR UPDATE`, uid).Scan(&pack, &sub); err != nil {
		return 0, 0, 0, err
	}
	after := pack + amount
	if _, err := tx.Exec(ctx, `UPDATE diamond_balances SET pack_diamonds=$1, updated_at=now() WHERE user_id=$2`, after, uid); err != nil {
		return 0, 0, 0, err
	}
	var ledgerID int64
	txn := "redeem:" + fmtID(codeID) + ":" + fmtID(uid)
	if err := tx.QueryRow(ctx, `INSERT INTO diamond_ledger(user_id, bucket, delta, balance_after, store_transaction_id, reason, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		uid, iapBucketPack, amount, after, txn, redeemReason, now).Scan(&ledgerID); err != nil {
		return 0, 0, 0, err
	}
	return ledgerID, after, sub, nil
}

func creditRedeemFree(ctx context.Context, tx Tx, uid int64, amount int) error {
	if _, err := tx.Exec(ctx, `INSERT INTO rewards(user_id, granted_at, expires_at, remaining, admin_grant_remaining)
		VALUES ($1, now(), now(), 0, 0)
		ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE rewards SET admin_grant_remaining = admin_grant_remaining + $2 WHERE user_id=$1`, uid, amount)
	return err
}

func diamondTotals(ctx context.Context, q rowQuery, uid int64) (int, int, error) {
	var pack, sub int
	err := q.QueryRow(ctx, `SELECT pack_diamonds, subscription_diamonds FROM diamond_balances WHERE user_id=$1`, uid).Scan(&pack, &sub)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return pack, sub, err
}

func (a *App) appWalletLedger(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能查看余额流水")
	}
	pack, sub, err := diamondTotals(r.Context(), a.Store, uid)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT created_at, reason, delta, balance_after, bucket
		FROM diamond_ledger WHERE user_id=$1 ORDER BY id DESC LIMIT 100`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var created time.Time
		var reason, bucket string
		var amount, balance int
		if err := rows.Scan(&created, &reason, &amount, &balance, &bucket); err != nil {
			return err
		}
		items = append(items, map[string]any{
			"time": created, "type": reason, "amount": amount, "balance": balance,
			"note": ledgerNote(reason), "bucket": bucket,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{
		"items": items, "display_label": displayLabelTataCoin, "unit": "diamond",
		"pack_diamonds": pack, "subscription_diamonds": sub, "balance": pack + sub,
	})
	return nil
}

func ledgerNote(reason string) string {
	switch reason {
	case "iap_pack_grant", "iap_restore_pack_grant":
		return "充值入账"
	case "iap_period_refresh", "iap_restore_period_refresh":
		return "会员钻石刷新"
	case "quota_spend":
		return "占卜消耗"
	case grantReason:
		return "人工补发"
	case redeemReason:
		return "兑换码"
	default:
		return reason
	}
}

func (a *App) appUploads(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能上传")
	}
	filename, contentType, data, err := readUpload(r)
	if err != nil {
		return err
	}
	ext, err := uploadExt(filename, contentType)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return biz("请选择图片")
	}
	if len(data) > uploadMaxBytes {
		return biz("图片过大")
	}
	name, err := randomHex(16)
	if err != nil {
		return err
	}
	stored := name + ext
	dir := a.uploadDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, stored), data, 0o644); err != nil {
		return err
	}
	writeOK(w, map[string]any{"url": publicAssetURL(r, stored)})
	return nil
}

func (a *App) serveUpload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validUploadName(name) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(a.uploadDir(), name))
}

func (a *App) uploadDir() string {
	if a.UploadDir != "" {
		return a.UploadDir
	}
	return filepath.Join(os.TempDir(), "tata-uploads")
}

func readUpload(r *http.Request) (filename, contentType string, data []byte, err error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(uploadMaxBytes); err != nil {
			return "", "", nil, biz("文件过大或格式不正确")
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			return "", "", nil, biz("请选择图片")
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, uploadMaxBytes+1))
		if err != nil {
			return "", "", nil, biz("读取图片失败")
		}
		return hdr.Filename, hdr.Header.Get("Content-Type"), raw, nil
	}
	var req struct {
		Filename      string `json:"filename"`
		ContentType   string `json:"content_type"`
		ContentBase64 string `json:"content_base64"`
	}
	if err := decode(r, &req); err != nil {
		return "", "", nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.ContentBase64))
	if err != nil {
		return "", "", nil, biz("图片内容不正确")
	}
	return req.Filename, req.ContentType, raw, nil
}

func uploadExt(filename, contentType string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return ext, nil
	}
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg":
		return ".jpg", nil
	case "image/png":
		return ".png", nil
	case "image/webp":
		return ".webp", nil
	case "image/gif":
		return ".gif", nil
	}
	return "", biz("只支持 jpg、png、webp、gif")
}

func validUploadName(name string) bool {
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return false
	}
	dot := strings.LastIndex(name, ".")
	if dot != uploadNamePattern {
		return false
	}
	hexPart := name[:dot]
	if len(hexPart) != uploadNamePattern {
		return false
	}
	for _, c := range hexPart {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	switch name[dot:] {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	}
	return false
}

func publicAssetURL(r *http.Request, name string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return scheme + "://" + host + "/uploads/" + name
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (a *App) appWishlist(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能提交心愿")
	}
	var req struct {
		Content  string   `json:"content"`
		ImageURL string   `json:"image_url"`
		Images   []string `json:"images"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return biz("请填写心愿内容")
	}
	if runeLen(content) > shellContentMax {
		return biz("内容不能超过 500 字")
	}
	imageURL, err := firstImageURL(req.ImageURL, req.Images)
	if err != nil {
		return err
	}
	var id int64
	if err := a.Store.QueryRow(r.Context(), `INSERT INTO wishlists(user_id, content, image_url) VALUES ($1,$2,$3) RETURNING id`,
		uid, content, imageURL).Scan(&id); err != nil {
		return err
	}
	writeOK(w, map[string]any{"id": id, "message": "心愿已提交"})
	return nil
}

func (a *App) appFeedback(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能提交反馈")
	}
	var req struct {
		Category string   `json:"category"`
		Content  string   `json:"content"`
		ImageURL string   `json:"image_url"`
		Images   []string `json:"images"`
		Email    string   `json:"email"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if !feedbackCategories[strings.TrimSpace(req.Category)] {
		return biz("反馈分类不正确")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return biz("请填写反馈内容")
	}
	if runeLen(content) > shellContentMax {
		return biz("内容不能超过 500 字")
	}
	email := strings.TrimSpace(req.Email)
	if email != "" && !validOptionalEmail(email) {
		return biz("邮箱格式不正确")
	}
	imageURL, err := firstImageURL(req.ImageURL, req.Images)
	if err != nil {
		return err
	}
	var id int64
	if err := a.Store.QueryRow(r.Context(), `INSERT INTO feedback(user_id, category, content, image_url, email) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		uid, strings.TrimSpace(req.Category), content, imageURL, email).Scan(&id); err != nil {
		return err
	}
	writeOK(w, map[string]any{"id": id, "message": "反馈已提交"})
	return nil
}

func (a *App) appGetNotificationPrefs(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能查看通知设置")
	}
	view, err := loadNotificationPrefs(r.Context(), a.Store, uid)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appPutNotificationPrefs(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能保存通知设置")
	}
	var req struct {
		DailyDrawEnabled    *bool  `json:"daily_draw_enabled"`
		DailyDrawTime       string `json:"daily_draw_time"`
		TomorrowDrawEnabled *bool  `json:"tomorrow_draw_enabled"`
		TomorrowDrawTime    string `json:"tomorrow_draw_time"`
		MessagesEnabled     *bool  `json:"messages_enabled"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	cur, err := loadNotificationPrefs(r.Context(), a.Store, uid)
	if err != nil {
		return err
	}
	dailyOn := cur["daily_draw_enabled"].(bool)
	tomorrowOn := cur["tomorrow_draw_enabled"].(bool)
	messagesOn := cur["messages_enabled"].(bool)
	dailyTime := cur["daily_draw_time"].(string)
	tomorrowTime := cur["tomorrow_draw_time"].(string)
	if req.DailyDrawEnabled != nil {
		dailyOn = *req.DailyDrawEnabled
	}
	if req.TomorrowDrawEnabled != nil {
		tomorrowOn = *req.TomorrowDrawEnabled
	}
	if req.MessagesEnabled != nil {
		messagesOn = *req.MessagesEnabled
	}
	if req.DailyDrawTime != "" {
		if err := validClock(req.DailyDrawTime); err != nil {
			return err
		}
		dailyTime = req.DailyDrawTime
	}
	if req.TomorrowDrawTime != "" {
		if err := validClock(req.TomorrowDrawTime); err != nil {
			return err
		}
		tomorrowTime = req.TomorrowDrawTime
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO notification_preferences(user_id, daily_draw_enabled, daily_draw_time, tomorrow_draw_enabled, tomorrow_draw_time, messages_enabled, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,now())
		ON CONFLICT (user_id) DO UPDATE SET
			daily_draw_enabled=EXCLUDED.daily_draw_enabled,
			daily_draw_time=EXCLUDED.daily_draw_time,
			tomorrow_draw_enabled=EXCLUDED.tomorrow_draw_enabled,
			tomorrow_draw_time=EXCLUDED.tomorrow_draw_time,
			messages_enabled=EXCLUDED.messages_enabled,
			updated_at=now()`,
		uid, dailyOn, dailyTime, tomorrowOn, tomorrowTime, messagesOn); err != nil {
		return err
	}
	writeOK(w, map[string]any{
		"daily_draw_enabled": dailyOn, "daily_draw_time": dailyTime,
		"tomorrow_draw_enabled": tomorrowOn, "tomorrow_draw_time": tomorrowTime,
		"messages_enabled": messagesOn,
	})
	return nil
}

func loadNotificationPrefs(ctx context.Context, q rowQuery, uid int64) (map[string]any, error) {
	var dailyOn, tomorrowOn, messagesOn bool
	var dailyTime, tomorrowTime string
	err := q.QueryRow(ctx, `SELECT daily_draw_enabled, daily_draw_time, tomorrow_draw_enabled, tomorrow_draw_time, messages_enabled
		FROM notification_preferences WHERE user_id=$1`, uid).Scan(&dailyOn, &dailyTime, &tomorrowOn, &tomorrowTime, &messagesOn)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{
			"daily_draw_enabled": false, "daily_draw_time": notifDefaultTime,
			"tomorrow_draw_enabled": false, "tomorrow_draw_time": notifDefaultTime,
			"messages_enabled": true,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"daily_draw_enabled": dailyOn, "daily_draw_time": dailyTime,
		"tomorrow_draw_enabled": tomorrowOn, "tomorrow_draw_time": tomorrowTime,
		"messages_enabled": messagesOn,
	}, nil
}

func validClock(raw string) error {
	if _, err := time.Parse("15:04", strings.TrimSpace(raw)); err != nil {
		return biz("时间格式应为 HH:MM")
	}
	return nil
}

func firstImageURL(single string, images []string) (string, error) {
	candidates := []string{strings.TrimSpace(single)}
	for _, img := range images {
		candidates = append(candidates, strings.TrimSpace(img))
	}
	picked := ""
	for _, img := range candidates {
		if img == "" {
			continue
		}
		if picked == "" {
			picked = img
		}
		if err := optionalAssetURL(img); err != nil {
			return "", err
		}
	}
	return picked, nil
}

func optionalAssetURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if utf8.RuneCountInString(raw) > 1024 {
		return biz("图片地址过长")
	}
	if strings.HasPrefix(raw, "/uploads/") && validUploadName(strings.TrimPrefix(raw, "/uploads/")) {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return biz("图片地址不正确")
	}
	return nil
}

func validOptionalEmail(raw string) bool {
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(addr.Address, raw)
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func emptyAsNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func accountDisplay(email string, wechat, apple *string) string {
	if strings.TrimSpace(email) != "" {
		return maskEmail(email)
	}
	if wechat != nil && *wechat != "" {
		return "微信已绑定"
	}
	if apple != nil && *apple != "" {
		return "Apple 已绑定"
	}
	return ""
}

func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return email
	}
	local := []rune(email[:at])
	if len(local) == 0 {
		return email
	}
	return string(local[0]) + "***" + email[at:]
}

func tarotistTags(raw, copy string) []string {
	var tags []string
	if strings.TrimSpace(raw) != "" && raw != "[]" {
		_ = json.Unmarshal([]byte(raw), &tags)
	}
	if len(tags) == 0 && strings.TrimSpace(copy) != "" {
		for _, part := range strings.Split(copy, "、") {
			part = strings.TrimSpace(part)
			if part != "" {
				tags = append(tags, part)
			}
		}
	}
	if tags == nil {
		return []string{}
	}
	return tags
}

func publicBlurb(blurb, copy string) string {
	if strings.TrimSpace(blurb) != "" {
		return blurb
	}
	return copy
}

func attachDailyHome(item map[string]any, target, business string) {
	if target == "" {
		target = "today"
	}
	item["target"] = target
	item["business_date"] = business
	name := firstCardName(item["cards"])
	item["card_name"] = name
	item["short_text"] = clipText(interpText(item["interpretation"]), 48)
	item["yi"] = nil
	item["card_thumbnail"] = nil
	item["has_result"] = name != ""
}

func firstCardName(v any) string {
	var raw []byte
	switch t := v.(type) {
	case json.RawMessage:
		raw = t
	case []byte:
		raw = t
	case string:
		raw = []byte(t)
	default:
		return ""
	}
	var cards []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &cards) != nil || len(cards) == 0 {
		return ""
	}
	return cards[0].Name
}

func interpText(v any) string {
	switch t := v.(type) {
	case *string:
		if t != nil {
			return *t
		}
	case string:
		return t
	}
	return ""
}

func clipText(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max])
}
