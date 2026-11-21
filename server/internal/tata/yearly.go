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
	"unicode/utf8"
)

// Pixso 1034:13493 / 661：十二宫 12 张人生领域，不是月份。
var yearlyHousePositions = []position{
	{1, "自我", "这一年的自我与身份"},
	{2, "财富", "这一年的财富与资源"},
	{3, "沟通", "这一年的沟通与学习"},
	{4, "家庭", "这一年的家庭与根基"},
	{5, "创造", "这一年的创造与表达"},
	{6, "日常", "这一年的日常与身体"},
	{7, "伴侣", "这一年的伴侣与关系"},
	{8, "深层", "这一年的深层与转化"},
	{9, "远行", "这一年的远行与视野"},
	{10, "事业", "这一年的事业与方向"},
	{11, "社群", "这一年的社群与协作"},
	{12, "潜意识", "这一年的潜意识与内在"},
}

// Pixso 661:207：四季 4 张春夏秋冬，不是六大领域。
var yearlySeasonPositions = []position{
	{1, "春", "这一年春天的节奏与变化"},
	{2, "夏", "这一年夏天的节奏与变化"},
	{3, "秋", "这一年秋天的节奏与变化"},
	{4, "冬", "这一年冬天的节奏与变化"},
}

type yearlyRow struct {
	unlocked     bool
	source       string
	birthDate    string
	birthTime    string
	birthPlace   string
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
		BirthDate  string `json:"birth_date"`
		BirthTime  string `json:"birth_time"`
		BirthPlace string `json:"birth_place"`
		Gender     string `json:"gender"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	birth, err := parseYearlyBirthDate(req.BirthDate)
	if err != nil {
		return err
	}
	birthTime, err := parseYearlyBirthTime(req.BirthTime)
	if err != nil {
		return err
	}
	birthPlace, err := parseYearlyBirthPlace(req.BirthPlace)
	if err != nil {
		return err
	}
	gender, err := normalizeGender(req.Gender)
	if err != nil {
		return err
	}
	if err := a.requireYearlyUnlocked(r.Context(), uid, year); err != nil {
		return err
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE app_users SET
		birth_date=COALESCE(NULLIF($1,''), birth_date),
		birth_time=COALESCE(NULLIF($2,''), birth_time),
		birth_place=COALESCE(NULLIF($3,''), birth_place),
		gender=COALESCE(NULLIF($4,''), gender)
		WHERE id=$5`, birth, birthTime, birthPlace, gender, uid); err != nil {
		return err
	}
	if gender == "" {
		_ = a.Store.QueryRow(r.Context(), `SELECT COALESCE(gender,'') FROM app_users WHERE id=$1`, uid).Scan(&gender)
	}
	if err := a.ensureYearlyRow(r.Context(), uid, year, ""); err != nil {
		return err
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE yearly_reports SET birth_date=$1, birth_time=$2, birth_place=$3, gender=$4, profile_at=now()
		WHERE user_id=$5 AND year=$6`, birth, birthTime, birthPlace, gender, uid, year); err != nil {
		return err
	}
	view, err := a.buildYearlyReport(r.Context(), uid, year, r)
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
			return biz("十二宫牌已固定")
		}
		positions := yearlyHousePositions
		if mode == "pick" {
			cards, err = drawCardsNumerology(positions, req.Numbers)
		} else {
			cards, err = drawCards(positions)
		}
	case "domains":
		if row.rhythmJSON == "" {
			return biz("请先抽完十二宫牌阵")
		}
		if row.domainJSON != "" {
			return biz("四季牌已固定")
		}
		skip := cardIDSet(row.rhythmJSON)
		positions := yearlySeasonPositions
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
	view, err := a.buildYearlyReport(r.Context(), uid, year, r)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appYearlyGenerate(w http.ResponseWriter, r *http.Request) error {
	if wantsSSE(r) {
		return a.appYearlyGenerateStream(w, r)
	}
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
		return biz("请先抽完十二宫牌阵")
	}
	if row.domainJSON == "" {
		return biz("请先抽完四季牌阵")
	}
	if row.reportStatus == "ready" && strings.TrimSpace(row.report) != "" {
		view, err := a.buildYearlyReport(r.Context(), uid, year, r)
		if err != nil {
			return err
		}
		writeOK(w, view)
		return nil
	}
	if err := a.chargeYearlyGenerate(r.Context(), uid, year); err != nil {
		return err
	}
	run, err := a.ensureYearlyJob(r.Context(), uid, year, row)
	if err != nil {
		return err
	}
	if err := run.wait(r.Context()); err != nil {
		return err
	}
	view, err := a.buildYearlyReport(r.Context(), uid, year, r)
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
	member, err := hasActiveMembership(ctx, a.Store, uid)
	if err != nil {
		return err
	}
	if member {
		return nil
	}
	row, err := a.loadYearlyRow(ctx, uid, year)
	if err != nil {
		return err
	}
	if row.unlocked {
		return biz(paywallNeedMember)
	}
	return biz(paywallNeedMember)
}

func (a *App) chargeYearlyGenerate(ctx context.Context, uid int64, year int) error {
	member, err := hasActiveMembership(ctx, a.Store, uid)
	if err != nil {
		return err
	}
	if !member {
		return biz(paywallNeedMember)
	}
	key := fmt.Sprintf("yearly:%d:%d", uid, year)
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var existing string
	err = tx.QueryRow(ctx, `SELECT entitlement FROM quota_charges WHERE user_id=$1 AND charge_key=$2`, uid, key).Scan(&existing)
	if err == nil {
		return tx.Commit(ctx)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	decision, err := chargeMemberQuota(ctx, tx, chargeOpts{
		UserID: uid, ReadingID: 0, ChargeKey: key, Cycle: cycleOf(time.Now()),
		ResumeAction: resumeHold,
	})
	if err != nil {
		return err
	}
	if !decision.Continue {
		_ = tx.Commit(ctx)
		if decision.Message != "" {
			return biz(decision.Message)
		}
		return biz(paywallNeedQuota)
	}
	return tx.Commit(ctx)
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
	err := a.Store.QueryRow(ctx, `SELECT unlocked, unlock_source, COALESCE(birth_date,''), COALESCE(birth_time,''), COALESCE(birth_place,''), COALESCE(gender,''), profile_at,
		COALESCE(rhythm_cards,''), COALESCE(domain_cards,''), COALESCE(report,''), COALESCE(report_status,'pending'), fail_reason, generated_at
		FROM yearly_reports WHERE user_id=$1 AND year=$2`, uid, year).Scan(
		&row.unlocked, &row.source, &row.birthDate, &row.birthTime, &row.birthPlace, &row.gender, &row.profileAt,
		&row.rhythmJSON, &row.domainJSON, &row.report, &row.reportStatus, &row.failReason, &row.generatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return yearlyRow{reportStatus: "pending"}, nil
	}
	return row, err
}

func (a *App) attachYearlyFlow(ctx context.Context, r *http.Request, uid int64, year int, view map[string]any) error {
	row, err := a.loadYearlyRow(ctx, uid, year)
	if err != nil {
		return err
	}
	var userBirth, userTime, userPlace, userGender sql.NullString
	_ = a.Store.QueryRow(ctx, `SELECT birth_date, birth_time, birth_place, gender FROM app_users WHERE id=$1`, uid).Scan(&userBirth, &userTime, &userPlace, &userGender)
	birth := row.birthDate
	if birth == "" {
		birth = strings.TrimSpace(userBirth.String)
	}
	birthTime := row.birthTime
	if birthTime == "" {
		birthTime = strings.TrimSpace(userTime.String)
	}
	birthPlace := row.birthPlace
	if birthPlace == "" {
		birthPlace = strings.TrimSpace(userPlace.String)
	}
	gender := row.gender
	if gender == "" {
		gender = strings.TrimSpace(userGender.String)
	}
	profileDone := row.profileAt.Valid
	houses := a.yearlyCardsView(ctx, r, row.rhythmJSON)
	seasons := a.yearlyCardsView(ctx, r, row.domainJSON)
	unlocked, _ := view["unlocked"].(bool)
	step := yearlyStep(unlocked, profileDone, len(houses), len(seasons), row.reportStatus)
	view["step"] = step
	view["profile"] = map[string]any{
		"birth_date":  nullableConfig(birth),
		"birth_time":  nullableConfig(birthTime),
		"birth_place": nullableConfig(birthPlace),
		"gender":      nullableConfig(gender),
		"completed":   profileDone,
	}
	view["rhythm_cards"] = houses
	view["domain_cards"] = seasons
	view["house_cards"] = houses
	view["season_cards"] = seasons
	view["report"] = nullableConfig(row.report)
	view["report_status"] = row.reportStatus
	view["generated_at"] = nullTime(row.generatedAt)
	view["fail_reason"] = localizeString(ctx, nullString(row.failReason))
	view["steps"] = []map[string]any{
		{"key": "profile", "title": T(ctx, "填写基本信息"), "done": profileDone},
		{"key": "rhythm", "title": T(ctx, "抽取十二宫牌阵"), "done": len(houses) == 12, "required": 12, "drawn": len(houses)},
		{"key": "domains", "title": T(ctx, "抽取四季牌阵"), "done": len(seasons) == 4, "required": 4, "drawn": len(seasons)},
		{"key": "generate", "title": T(ctx, "生成年度报告"), "done": row.reportStatus == "ready"},
	}
	return nil
}

func yearlyStep(unlocked, profile bool, houseN, seasonN int, reportStatus string) string {
	if !unlocked {
		return "locked"
	}
	if !profile {
		return "profile"
	}
	if houseN < 12 {
		return "rhythm"
	}
	if seasonN < 4 {
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

func (a *App) yearlyCardsView(ctx context.Context, r *http.Request, raw string) []map[string]any {
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
		card["card_thumbnail"] = absoluteAssetAny(r, resolveCardThumbnail(cfg, name))
		if name != "" {
			card["name"] = T(ctx, name)
		}
		if pos, ok := card["position_name"].(string); ok && pos != "" {
			card["position_name"] = T(ctx, pos)
		}
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

func parseYearlyBirthTime(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", biz("请填写出生时间")
	}
	if t, err := time.Parse("15:04", raw); err == nil {
		return t.Format("15:04"), nil
	}
	if t, err := time.Parse("15:04:05", raw); err == nil {
		return t.Format("15:04"), nil
	}
	return "", biz("出生时间格式应为 HH:MM")
}

func parseYearlyBirthPlace(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", biz("请填写出生地点")
	}
	n := utf8.RuneCountInString(raw)
	if n > 80 {
		return "", biz("出生地点过长")
	}
	return raw, nil
}

func buildYearlyPrompt(year int, birth, birthTime, birthPlace, gender, houseJSON, seasonJSON string) string {
	var b strings.Builder
	b.WriteString("你只解读，不得改牌、牌位或正逆位。\n")
	b.WriteString("这是一份年度运势报告。先按 12 张十二宫牌解读人生领域（自我、财富、沟通、家庭、创造、日常、伴侣、深层、远行、事业、社群、潜意识），再按 4 张四季牌解读春夏秋冬的节奏变化，最后给一段总评与行动建议。\n")
	fmt.Fprintf(&b, "年份：%d\n出生日期：%s\n出生时间：%s\n出生地点：%s\n性别：%s\n", year, birth, birthTime, birthPlace, gender)
	fmt.Fprintf(&b, "十二宫牌面：%s\n四季牌面：%s\n", houseJSON, seasonJSON)
	return b.String()
}
