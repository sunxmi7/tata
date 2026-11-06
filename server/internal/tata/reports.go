package tata

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (a *App) appMonthlyReports(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	loc := a.userLocation(r.Context(), uid)
	now := time.Now().In(loc)
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if month == "" {
		month = now.Format("2006-01")
	}
	year, mon, err := parseYearMonth(month)
	if err != nil {
		return err
	}
	if year > now.Year() || (year == now.Year() && mon > now.Month()) {
		return biz("还不能看未来月份")
	}
	view, err := a.buildMonthlyReport(r.Context(), uid, loc, year, mon, now)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appSaveMonthlyReport(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	var req struct {
		Month string `json:"month"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	loc := a.userLocation(r.Context(), uid)
	now := time.Now().In(loc)
	month := strings.TrimSpace(req.Month)
	if month == "" {
		month = now.Format("2006-01")
	}
	year, mon, err := parseYearMonth(month)
	if err != nil {
		return err
	}
	if year > now.Year() || (year == now.Year() && mon > now.Month()) {
		return biz("还不能看未来月份")
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO monthly_report_saves(user_id, month, saved_at)
		VALUES ($1,$2,now())
		ON CONFLICT (user_id, month) DO UPDATE SET saved_at=now()`, uid, month); err != nil {
		return err
	}
	view, err := a.buildMonthlyReport(r.Context(), uid, loc, year, mon, now)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appYearlyReport(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	year, err := pathID(r)
	if err != nil {
		return err
	}
	if year < 2000 || year > 2100 {
		return biz("年份不正确")
	}
	view, err := a.buildYearlyReport(r.Context(), uid, int(year))
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appUnlockYearlyReport(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	year, err := pathID(r)
	if err != nil {
		return err
	}
	if year < 2000 || year > 2100 {
		return biz("年份不正确")
	}
	yearSub, err := a.hasYearSubscription(r.Context(), uid)
	if err != nil {
		return err
	}
	if !yearSub {
		price, _, err := a.yearlyUnlockPrice(r.Context())
		if err != nil {
			return err
		}
		if price == nil {
			return biz("年卡未开通，单独购买价格未配置")
		}
		return biz("年卡未开通，请通过内购单独解锁")
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO yearly_reports(user_id, year, unlocked, unlock_source, unlocked_at)
		VALUES ($1,$2,TRUE,'year_subscription',now())
		ON CONFLICT (user_id, year) DO UPDATE SET unlocked=TRUE, unlock_source='year_subscription', unlocked_at=COALESCE(yearly_reports.unlocked_at, now())`,
		uid, year); err != nil {
		return err
	}
	view, err := a.buildYearlyReport(r.Context(), uid, int(year))
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) requireAppUser(r *http.Request) (int64, error) {
	uid, _, err := a.appActor(r)
	if err != nil {
		return 0, err
	}
	if uid == 0 {
		return 0, biz("登录后才能查看")
	}
	return uid, nil
}

func (a *App) buildMonthlyReport(ctx context.Context, uid int64, loc *time.Location, year int, mon time.Month, now time.Time) (map[string]any, error) {
	month := time.Date(year, mon, 1, 0, 0, 0, 0, loc).Format("2006-01")
	start := time.Date(year, mon, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 1, 0)
	rows, err := a.Store.Query(ctx, `SELECT question, status, interpretation, created_at
		FROM readings WHERE user_id=$1 AND deleted_at IS NULL AND created_at>=$2 AND created_at<$3
		ORDER BY created_at, id`, uid, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type row struct {
		question, status string
		interpretation   sql.NullString
		created          time.Time
	}
	var items []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.question, &item.status, &item.interpretation, &item.created); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	days := map[string]bool{}
	quote := ""
	for _, item := range items {
		days[item.created.In(loc).Format("2006-01-02")] = true
		if quote == "" && item.status == "COMPLETED" {
			quote = clipText(item.interpretation.String, 80)
		}
	}
	questions := make([]string, len(items))
	for i, item := range items {
		questions[i] = item.question
	}
	topics, err := a.topicHitsFromQuestions(ctx, questions)
	if err != nil {
		return nil, err
	}
	saved := false
	var savedAt sql.NullTime
	if err := a.Store.QueryRow(ctx, `SELECT saved_at FROM monthly_report_saves WHERE user_id=$1 AND month=$2`, uid, month).Scan(&savedAt); err == nil {
		saved = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	status := "ready"
	emptyTitle := ""
	if len(items) == 0 {
		status = "empty"
		emptyTitle = "这个月还没有抽过牌"
	}
	quoteStatus := "pending"
	if quote != "" {
		quoteStatus = "ready"
	}
	months, err := a.monthlyMonthList(ctx, uid, loc, now)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"month":             month,
		"status":            status,
		"empty_title":       emptyTitle,
		"exploration_count": len(items),
		"companion_days":    len(days),
		"focus_topics":      topics,
		"quote":             quote,
		"quote_status":      quoteStatus,
		"saved":             saved,
		"saved_at":          nullTime(savedAt),
		"months":            months,
		"fields_confirmed":  true,
		"content_invented":  false,
	}, nil
}

func (a *App) topicHitsFromQuestions(ctx context.Context, questions []string) ([]map[string]any, error) {
	rows, err := a.Store.Query(ctx, `SELECT title FROM explore_topics ORDER BY sort_order, topic_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var titles []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, err
		}
		titles = append(titles, title)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	counts := make([]int, len(titles))
	total := 0
	for _, q := range questions {
		for i, title := range titles {
			if title != "" && strings.Contains(q, title) {
				counts[i]++
				total++
			}
		}
	}
	out := []map[string]any{}
	for i, title := range titles {
		if counts[i] == 0 {
			continue
		}
		ratio := 0.0
		if total > 0 {
			ratio = float64(counts[i]) / float64(total)
		}
		out = append(out, map[string]any{"title": title, "count": counts[i], "ratio": ratio})
	}
	return out, nil
}

func (a *App) monthlyMonthList(ctx context.Context, uid int64, loc *time.Location, now time.Time) ([]map[string]any, error) {
	out := make([]map[string]any, 0, 12)
	for i := 0; i < 12; i++ {
		t := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -i, 0)
		month := t.Format("2006-01")
		start := t
		end := t.AddDate(0, 1, 0)
		var n int
		if err := a.Store.QueryRow(ctx, `SELECT count(*) FROM readings
			WHERE user_id=$1 AND deleted_at IS NULL AND created_at>=$2 AND created_at<$3`, uid, start.UTC(), end.UTC()).Scan(&n); err != nil {
			return nil, err
		}
		var savedAt sql.NullTime
		saved := false
		if err := a.Store.QueryRow(ctx, `SELECT saved_at FROM monthly_report_saves WHERE user_id=$1 AND month=$2`, uid, month).Scan(&savedAt); err == nil {
			saved = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		out = append(out, map[string]any{"month": month, "exploration_count": n, "saved": saved})
	}
	return out, nil
}

func (a *App) buildYearlyReport(ctx context.Context, uid int64, year int) (map[string]any, error) {
	yearSub, err := a.hasYearSubscription(ctx, uid)
	if err != nil {
		return nil, err
	}
	var unlocked bool
	var source string
	err = a.Store.QueryRow(ctx, `SELECT unlocked, unlock_source FROM yearly_reports WHERE user_id=$1 AND year=$2`, uid, year).Scan(&unlocked, &source)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if yearSub {
		unlocked = true
		if source == "" {
			source = "year_subscription"
		}
	}
	priceMinor, priceCurrency, err := a.yearlyUnlockPrice(ctx)
	if err != nil {
		return nil, err
	}
	sections, ready, err := a.loadYearlySections(ctx, year)
	if err != nil {
		return nil, err
	}
	status := "pending"
	if ready {
		status = "ready"
	}
	bodies := map[string]string{}
	for _, raw := range sections {
		key, _ := raw["key"].(string)
		body, _ := raw["body"].(string)
		bodies[key] = body
	}
	return map[string]any{
		"year":                  year,
		"unlocked":              unlocked,
		"unlock_source":         source,
		"year_member_unlocks":   true,
		"unlock_price_minor":    priceMinor,
		"unlock_price_currency": priceCurrency,
		"content_status":        status,
		"theme":                 bodies["theme"],
		"domain":                bodies["domain"],
		"rhythm":                bodies["rhythm"],
		"action":                bodies["action"],
		"sections":              sections,
		"fields_confirmed":      true,
		"content_invented":      false,
	}, nil
}

func (a *App) yearlyUnlockPrice(ctx context.Context) (any, any, error) {
	var price sql.NullInt64
	var currency sql.NullString
	err := a.Store.QueryRow(ctx, `SELECT price_amount_minor, price_currency FROM iap_skus
		WHERE kind=$1 AND enabled=TRUE AND price_amount_minor IS NOT NULL
		ORDER BY id DESC LIMIT 1`, iapKindYearlyUnlock).Scan(&price, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return nullInt(price), nullString(currency), nil
}

func (a *App) hasYearSubscription(ctx context.Context, uid int64) (bool, error) {
	var n int
	err := a.Store.QueryRow(ctx, `SELECT count(*) FROM subscriptions
		WHERE user_id=$1 AND sku_kind='year' AND status='active' AND expires_at > now()
		AND merge_state NOT IN ('superseded','expired')`, uid).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (a *App) userLocation(ctx context.Context, uid int64) *time.Location {
	tz := "Asia/Shanghai"
	if uid > 0 {
		_ = a.Store.QueryRow(ctx, `SELECT timezone FROM app_users WHERE id=$1`, uid).Scan(&tz)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, _ = time.LoadLocation("Asia/Shanghai")
	}
	if loc == nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return loc
}

func parseYearMonth(s string) (int, time.Month, error) {
	t, err := time.Parse("2006-01", strings.TrimSpace(s))
	if err != nil {
		return 0, 0, biz("月份格式应为 YYYY-MM")
	}
	return t.Year(), t.Month(), nil
}

func (a *App) appGetDeckPreference(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	code, available, err := a.resolveUserDeck(r.Context(), uid)
	if err != nil {
		return err
	}
	writeOK(w, map[string]any{"code": code, "available": available})
	return nil
}

func (a *App) appPutDeckPreference(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return err
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	code := strings.TrimSpace(req.Code)
	if _, ok, err := a.enabledDeck(r.Context(), code); err != nil {
		return err
	} else if !ok {
		return biz("该牌面风格尚未开放")
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE app_users SET deck_code=$1 WHERE id=$2`, code, uid); err != nil {
		return err
	}
	_, available, err := a.resolveUserDeck(r.Context(), uid)
	if err != nil {
		return err
	}
	writeOK(w, map[string]any{"code": code, "available": available})
	return nil
}
