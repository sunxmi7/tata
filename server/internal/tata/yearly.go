package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var yearlyRhythmPositions = []position{
	{1, "一月", "这一年一月的节奏"},
	{2, "二月", "这一年二月的节奏"},
	{3, "三月", "这一年三月的节奏"},
	{4, "四月", "这一年四月的节奏"},
	{5, "五月", "这一年五月的节奏"},
	{6, "六月", "这一年六月的节奏"},
	{7, "七月", "这一年七月的节奏"},
	{8, "八月", "这一年八月的节奏"},
	{9, "九月", "这一年九月的节奏"},
	{10, "十月", "这一年十月的节奏"},
	{11, "十一月", "这一年十一月的节奏"},
	{12, "十二月", "这一年十二月的节奏"},
}

var yearlyDomainPositions = []position{
	{1, "感情", "这一年的感情与亲密关系"},
	{2, "事业", "这一年的事业与方向"},
	{3, "财运", "这一年的财务与资源"},
	{4, "健康", "这一年的身体与状态"},
	{5, "人际", "这一年的人际与协作"},
	{6, "成长", "这一年的学习与成长"},
}

type yearlyRow struct {
	unlocked     bool
	source       string
	birthDate    string
	gender       string
	profileAt    sql.NullTime
	rhythmJSON   string
	domainJSON   string
	report       string
	reportStatus string
	failReason   sql.NullString
	generatedAt  sql.NullTime
}

func (a *App) appYearlyProfile(w http.ResponseWriter, r *http.Request) error {
	uid, year, err := a.yearlyActor(r)
	if err != nil {
		return err
	}
	var req struct {
		BirthDate string `json:"birth_date"`
		Gender    string `json:"gender"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	birth, err := parseYearlyBirthDate(req.BirthDate)
	if err != nil {
		return err
	}
	gender := strings.TrimSpace(req.Gender)
	if gender != "" && gender != "女性" && gender != "男性" && gender != "不愿透露" {
		return biz("性别选项不正确")
	}
	if err := a.requireYearlyUnlocked(r.Context(), uid, year); err != nil {
		return err
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE app_users SET
		birth_date=COALESCE(NULLIF($1,''), birth_date),
		gender=COALESCE(NULLIF($2,''), gender)
		WHERE id=$3`, birth, gender, uid); err != nil {
		return err
	}
	if gender == "" {
		_ = a.Store.QueryRow(r.Context(), `SELECT COALESCE(gender,'') FROM app_users WHERE id=$1`, uid).Scan(&gender)
	}
	if err := a.ensureYearlyRow(r.Context(), uid, year, ""); err != nil {
		return err
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE yearly_reports SET birth_date=$1, gender=$2, profile_at=now()
		WHERE user_id=$3 AND year=$4`, birth, gender, uid, year); err != nil {
		return err
	}
	view, err := a.buildYearlyReport(r.Context(), uid, year)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appYearlyRhythm(w http.ResponseWriter, r *http.Request) error {
	return a.yearlyDraw(w, r, "rhythm")
}

func (a *App) appYearlyDomains(w http.ResponseWriter, r *http.Request) error {
	return a.yearlyDraw(w, r, "domains")
}

func (a *App) yearlyDraw(w http.ResponseWriter, r *http.Request, kind string) error {
	uid, year, err := a.yearlyActor(r)
	if err != nil {
		return err
	}
	var req struct {
		Mode    string `json:"mode"`
		Numbers []int  `json:"numbers"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := a.requireYearlyUnlocked(r.Context(), uid, year); err != nil {
		return err
	}
	if err := a.ensureYearlyRow(r.Context(), uid, year, ""); err != nil {
		return err
	}
	row, err := a.loadYearlyRow(r.Context(), uid, year)
	if err != nil {
		return err
	}
	if !row.profileAt.Valid {
		return biz("请先填写基本信息")
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		if len(req.Numbers) > 0 {
			mode = "pick"
		} else {
			mode = "normal"
		}
	}
	if mode != "normal" && mode != "pick" {
		return biz("抽牌方式不正确")
	}
	var cards []drawnCard
	switch kind {
	case "rhythm":
		if row.rhythmJSON != "" {
			return biz("全年节奏牌已固定")
		}
		positions := yearlyRhythmPositions
		if mode == "pick" {
			cards, err = drawCardsNumerology(positions, req.Numbers)
		} else {
			cards, err = drawCards(positions)
		}
	case "domains":
		if row.rhythmJSON == "" {
			return biz("请先选完 12 张全年节奏牌")
		}
		if row.domainJSON != "" {
			return biz("六大领域牌已固定")
		}
		skip := cardIDSet(row.rhythmJSON)
		positions := yearlyDomainPositions
		if mode == "pick" {
			cards, err = drawCardsNumerologySkipping(positions, req.Numbers, skip)
		} else {
			cards, err = drawCardsSkipping(positions, skip)
		}
	default:
		return biz("抽牌步骤不正确")
	}
	if err != nil {
		return err
	}
	col := "rhythm_cards"
	if kind == "domains" {
		col = "domain_cards"
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE yearly_reports SET `+col+`=$1 WHERE user_id=$2 AND year=$3`, mustJSON(cards), uid, year); err != nil {
		return err
	}
	view, err := a.buildYearlyReport(r.Context(), uid, year)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appYearlyGenerate(w http.ResponseWriter, r *http.Request) error {
	uid, year, err := a.yearlyActor(r)
	if err != nil {
		return err
	}
	var req struct{}
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := a.requireYearlyUnlocked(r.Context(), uid, year); err != nil {
		return err
	}
	if err := a.ensureYearlyRow(r.Context(), uid, year, ""); err != nil {
		return err
	}
	row, err := a.loadYearlyRow(r.Context(), uid, year)
	if err != nil {
		return err
	}
	if !row.profileAt.Valid {
		return biz("请先填写基本信息")
	}
	if row.rhythmJSON == "" {
		return biz("请先选完 12 张全年节奏牌")
	}
	if row.domainJSON == "" {
		return biz("请先抽完六大领域牌")
	}
	if row.reportStatus == "ready" && strings.TrimSpace(row.report) != "" {
		view, err := a.buildYearlyReport(r.Context(), uid, year)
		if err != nil {
			return err
		}
		writeOK(w, view)
		return nil
	}
	vendorName, model, _, _, _, err := a.binding(r.Context(), "interpret")
	if err != nil {
		_, _ = a.Store.Exec(r.Context(), `UPDATE yearly_reports SET report_status='failed', fail_reason=$1 WHERE user_id=$2 AND year=$3`, "未绑定供应商", uid, year)
		return err
	}
	unlock, err := a.lockNamed("yearly:" + fmtID(int64(uid)) + ":" + fmtID(int64(year)))
	if err != nil {
		return err
	}
	defer unlock()
	tag, err := a.Store.Exec(r.Context(), `UPDATE yearly_reports SET report_status='generating', fail_reason=NULL
		WHERE user_id=$1 AND year=$2 AND report_status IN ('pending','failed','generating')`, uid, year)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("生成进行中")
	}
	prompt := buildYearlyPrompt(year, row.birthDate, row.gender, row.rhythmJSON, row.domainJSON)
	_, _, text, callErr := a.runEino(r.Context(), "interpret", prompt)
	if callErr != nil {
		_, _ = a.Store.Exec(r.Context(), `UPDATE yearly_reports SET report_status='failed', fail_reason=$1 WHERE user_id=$2 AND year=$3`, callErr.Error(), uid, year)
		return biz("解读失败，可重试")
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE yearly_reports SET report_status='ready', report=$1, fail_reason=NULL, generated_at=now()
		WHERE user_id=$2 AND year=$3`, text, uid, year); err != nil {
		return err
	}
	_ = vendorName
	_ = model
	view, err := a.buildYearlyReport(r.Context(), uid, year)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) yearlyActor(r *http.Request) (int64, int, error) {
	uid, err := a.requireAppUser(r)
	if err != nil {
		return 0, 0, err
	}
	year, err := pathID(r)
	if err != nil {
		return 0, 0, err
	}
	if year < 2000 || year > 2100 {
		return 0, 0, biz("年份不正确")
	}
	return uid, int(year), nil
}

func (a *App) requireYearlyUnlocked(ctx context.Context, uid int64, year int) error {
	yearSub, err := a.hasYearSubscription(ctx, uid)
	if err != nil {
		return err
	}
	row, err := a.loadYearlyRow(ctx, uid, year)
	if err != nil {
		return err
	}
	if yearSub || row.unlocked {
		return nil
	}
	return biz("请先解锁年度运势")
}

func (a *App) ensureYearlyRow(ctx context.Context, uid int64, year int, source string) error {
	yearSub, err := a.hasYearSubscription(ctx, uid)
	if err != nil {
		return err
	}
	if source == "" {
		if yearSub {
			source = "year_subscription"
		} else {
			source = "purchase"
		}
	}
	_, err = a.Store.Exec(ctx, `INSERT INTO yearly_reports(user_id, year, unlocked, unlock_source, unlocked_at)
		VALUES ($1,$2,TRUE,$3,now())
		ON CONFLICT (user_id, year) DO UPDATE SET
			unlocked=TRUE,
			unlock_source=CASE WHEN yearly_reports.unlock_source='' THEN EXCLUDED.unlock_source ELSE yearly_reports.unlock_source END,
			unlocked_at=COALESCE(yearly_reports.unlocked_at, now())`, uid, year, source)
	return err
}

func (a *App) loadYearlyRow(ctx context.Context, uid int64, year int) (yearlyRow, error) {
	var row yearlyRow
	err := a.Store.QueryRow(ctx, `SELECT unlocked, unlock_source, COALESCE(birth_date,''), COALESCE(gender,''), profile_at,
		COALESCE(rhythm_cards,''), COALESCE(domain_cards,''), COALESCE(report,''), COALESCE(report_status,'pending'), fail_reason, generated_at
		FROM yearly_reports WHERE user_id=$1 AND year=$2`, uid, year).Scan(
		&row.unlocked, &row.source, &row.birthDate, &row.gender, &row.profileAt,
		&row.rhythmJSON, &row.domainJSON, &row.report, &row.reportStatus, &row.failReason, &row.generatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return yearlyRow{reportStatus: "pending"}, nil
	}
	return row, err
}

func (a *App) attachYearlyFlow(ctx context.Context, uid int64, year int, view map[string]any) error {
	row, err := a.loadYearlyRow(ctx, uid, year)
	if err != nil {
		return err
	}
	var userBirth, userGender sql.NullString
	_ = a.Store.QueryRow(ctx, `SELECT birth_date, gender FROM app_users WHERE id=$1`, uid).Scan(&userBirth, &userGender)
	birth := row.birthDate
	if birth == "" {
		birth = strings.TrimSpace(userBirth.String)
	}
	gender := row.gender
	if gender == "" {
		gender = strings.TrimSpace(userGender.String)
	}
	profileDone := row.profileAt.Valid
	rhythm := a.yearlyCardsView(ctx, row.rhythmJSON)
	domains := a.yearlyCardsView(ctx, row.domainJSON)
	unlocked, _ := view["unlocked"].(bool)
	step := yearlyStep(unlocked, profileDone, len(rhythm), len(domains), row.reportStatus)
	view["step"] = step
	view["profile"] = map[string]any{
		"birth_date": nullableConfig(birth),
		"gender":     nullableConfig(gender),
		"completed":  profileDone,
	}
	view["rhythm_cards"] = rhythm
	view["domain_cards"] = domains
	view["report"] = nullableConfig(row.report)
	view["report_status"] = row.reportStatus
	view["fail_reason"] = nullString(row.failReason)
	view["generated_at"] = nullTime(row.generatedAt)
	view["steps"] = []map[string]any{
		{"key": "profile", "title": "填写基本信息", "done": profileDone},
		{"key": "rhythm", "title": "选12张牌，看全年节奏", "done": len(rhythm) == 12, "required": 12, "drawn": len(rhythm)},
		{"key": "domains", "title": "抽6张牌，看六大领域", "done": len(domains) == 6, "required": 6, "drawn": len(domains)},
		{"key": "generate", "title": "生成年度报告", "done": row.reportStatus == "ready"},
	}
	return nil
}

func yearlyStep(unlocked, profile bool, rhythmN, domainN int, reportStatus string) string {
	if !unlocked {
		return "locked"
	}
	if !profile {
		return "profile"
	}
	if rhythmN < 12 {
		return "rhythm"
	}
	if domainN < 6 {
		return "domains"
	}
	if reportStatus == "ready" {
		return "ready"
	}
	if reportStatus == "generating" {
		return "generating"
	}
	return "generate"
}

func (a *App) yearlyCardsView(ctx context.Context, raw string) []map[string]any {
	if strings.TrimSpace(raw) == "" {
		return []map[string]any{}
	}
	var cards []map[string]any
	if json.Unmarshal([]byte(raw), &cards) != nil {
		return []map[string]any{}
	}
	cfg := a.cardConfig(ctx)
	for _, card := range cards {
		name, _ := card["name"].(string)
		card["card_thumbnail"] = resolveCardThumbnail(cfg, name)
	}
	return cards
}

func cardIDSet(raw string) map[int]bool {
	out := map[int]bool{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var cards []drawnCard
	if json.Unmarshal([]byte(raw), &cards) != nil {
		return out
	}
	for _, card := range cards {
		out[card.CardID] = true
	}
	return out
}

func parseYearlyBirthDate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", biz("请填写出生日期")
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return "", biz("出生日期格式应为 YYYY-MM-DD")
	}
	today := beijingNow()
	if t.After(time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())) {
		return "", biz("出生日期不能是未来")
	}
	if t.Year() < 1900 {
		return "", biz("出生日期不正确")
	}
	return t.Format("2006-01-02"), nil
}

func buildYearlyPrompt(year int, birth, gender, rhythmJSON, domainJSON string) string {
	var b strings.Builder
	b.WriteString("你只解读，不得改牌、牌位或正逆位。\n")
	b.WriteString("这是一份年度运势报告。先按 12 张牌解读全年节奏（一月到十二月），再按 6 张牌解读六大领域（感情、事业、财运、健康、人际、成长），最后给一段总评与行动建议。\n")
	fmt.Fprintf(&b, "年份：%d\n出生日期：%s\n性别：%s\n", year, birth, gender)
	fmt.Fprintf(&b, "全年节奏牌面：%s\n六大领域牌面：%s\n", rhythmJSON, domainJSON)
	return b.String()
}
