package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func validDeviceID(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == ':' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func normalizeBirthDate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return "", biz("出生日期格式应为 YYYY-MM-DD")
	}
	return raw, nil
}

type deviceGuide struct {
	found       bool
	guideDone   bool
	offered     bool
	consumed    bool
	forfeited   bool
	gender      string
	birth       string
	preferences string
}

func (a *App) loadDeviceGuide(r *http.Request, deviceID string) (deviceGuide, error) {
	var g deviceGuide
	// Case-insensitive: onboarding writes and login merge must share one key.
	err := a.Store.QueryRow(r.Context(), `SELECT guide_completed, guide_offered, consumed, trial_forfeited, gender, birth_date, explore_preferences
		FROM device_experiences WHERE lower(device_id)=lower($1) ORDER BY CASE WHEN device_id=$1 THEN 0 ELSE 1 END, created_at DESC NULLS LAST LIMIT 1`, deviceID).Scan(&g.guideDone, &g.offered, &g.consumed, &g.forfeited, &g.gender, &g.birth, &g.preferences)
	if errors.Is(err, sql.ErrNoRows) {
		return g, nil
	}
	if err != nil {
		return g, err
	}
	g.found = true
	return g, nil
}

// normalizeOnboardingDevice lowercases device_id so draft save and login merge hit the same row.
func normalizeOnboardingDevice(raw string) (string, error) {
	id := canonicalDevice(raw)
	if id == "" {
		return "", biz("缺少设备标识")
	}
	return id, nil
}

func deviceHasProfile(g deviceGuide) bool {
	if strings.TrimSpace(g.gender) != "" || strings.TrimSpace(g.birth) != "" {
		return true
	}
	return len(parseExplorePreferences(g.preferences)) > 0
}

// attachDeviceProfileDraft copies guide gender/birth/preferences from the device
// draft into the account profile, but only fills empty account fields.
func (a *App) attachDeviceProfileDraft(ctx context.Context, uid int64, deviceID string) {
	if uid <= 0 {
		return
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return
	}
	var gender, birth, prefs string
	err := a.Store.QueryRow(ctx, `SELECT COALESCE(gender,''), COALESCE(birth_date,''), COALESCE(explore_preferences,'[]')
		FROM device_experiences WHERE lower(device_id)=lower($1)`, deviceID).Scan(&gender, &birth, &prefs)
	if err != nil {
		return
	}
	gender = strings.TrimSpace(gender)
	birth = strings.TrimSpace(birth)
	prefs = strings.TrimSpace(prefs)
	if prefs == "" {
		prefs = "[]"
	}
	if gender == "" && birth == "" && prefs == "[]" {
		return
	}
	_, _ = a.Store.Exec(ctx, `UPDATE app_users SET
		gender=COALESCE(NULLIF(btrim(COALESCE(gender,'')),''), NULLIF($2,'')),
		birth_date=COALESCE(NULLIF(btrim(COALESCE(birth_date,'')),''), NULLIF($3,'')),
		explore_preferences=CASE
			WHEN COALESCE(explore_preferences,'') IN ('','[]') AND $4 NOT IN ('','[]') THEN $4
			ELSE explore_preferences
		END
		WHERE id=$1`, uid, gender, birth, prefs)
}

func shouldShowGuide(g deviceGuide) bool {
	// 这台设备只出一次：已经看过、已经标完成、或已经留下档案草稿，都不再出。
	return !g.guideDone && !g.offered && !deviceHasProfile(g)
}

func guideView(g deviceGuide) map[string]any {
	trial := !g.found || (!g.consumed && !g.forfeited)
	return map[string]any{
		"show_guide":              shouldShowGuide(g),
		"guide_completed":         g.found && g.guideDone,
		"reading_trial_available": trial,
		"gender":                  g.gender,
		"birth_date":              g.birth,
		"explore_preferences":     parseExplorePreferences(g.preferences),
	}
}

func (a *App) markGuideSeen(r *http.Request, deviceID string, complete bool) error {
	if complete {
		_, err := a.Store.Exec(r.Context(), `INSERT INTO device_experiences(device_id, guide_offered, guide_completed) VALUES ($1, TRUE, TRUE)
			ON CONFLICT (device_id) DO UPDATE SET guide_offered=TRUE, guide_completed=TRUE`, deviceID)
		return err
	}
	_, err := a.Store.Exec(r.Context(), `INSERT INTO device_experiences(device_id, guide_offered) VALUES ($1, TRUE)
		ON CONFLICT (device_id) DO UPDATE SET guide_offered=TRUE`, deviceID)
	return err
}

func (a *App) appGetOnboarding(w http.ResponseWriter, r *http.Request) error {
	deviceID, err := normalizeOnboardingDevice(r.URL.Query().Get("device_id"))
	if err != nil {
		return err
	}
	g, err := a.loadDeviceGuide(r, deviceID)
	if err != nil {
		return err
	}
	show := shouldShowGuide(g)
	// 第一次查询就记下已展示；档案草稿已在则直接标完成，登录后重启也不会再出。
	if !g.offered || (deviceHasProfile(g) && !g.guideDone) {
		complete := deviceHasProfile(g) || g.guideDone
		if err := a.markGuideSeen(r, deviceID, complete); err != nil {
			return err
		}
		g.offered = true
		if complete {
			g.guideDone = true
		}
		g.found = true
	}
	view, err := a.onboardingView(r, g)
	if err != nil {
		return err
	}
	view["show_guide"] = show
	writeOK(w, view)
	return nil
}

func (a *App) onboardingView(r *http.Request, g deviceGuide) (map[string]any, error) {
	view := guideView(g)
	uid, err := a.appActor(r)
	if err != nil {
		if _, ok := err.(authFail); ok {
			return view, nil
		}
		return nil, err
	}
	if uid == 0 {
		return view, nil
	}
	if g.forfeited {
		if _, err := a.Store.Exec(r.Context(), `UPDATE app_users SET reading_trial_used=TRUE WHERE id=$1 AND reading_trial_used=FALSE`, uid); err != nil {
			return nil, err
		}
	}
	var used bool
	if err := a.Store.QueryRow(r.Context(), `SELECT reading_trial_used FROM app_users WHERE id=$1`, uid).Scan(&used); err != nil {
		return nil, err
	}
	view["reading_trial_available"] = !used
	return view, nil
}

func (a *App) appPutOnboarding(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		DeviceID           string    `json:"device_id"`
		Gender             *string   `json:"gender"`
		BirthDate          *string   `json:"birth_date"`
		ExplorePreferences *[]string `json:"explore_preferences"`
		GuideCompleted     *bool     `json:"guide_completed"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	deviceID, err := normalizeOnboardingDevice(req.DeviceID)
	if err != nil {
		return err
	}
	g, err := a.loadDeviceGuide(r, deviceID)
	if err != nil {
		return err
	}
	gender, birth, prefs := g.gender, g.birth, g.preferences
	if prefs == "" {
		prefs = "[]"
	}
	done := g.guideDone
	if req.Gender != nil {
		gender, err = normalizeGender(*req.Gender)
		if err != nil {
			return err
		}
	}
	if req.BirthDate != nil {
		birth, err = normalizeBirthDate(*req.BirthDate)
		if err != nil {
			return err
		}
	}
	if req.ExplorePreferences != nil {
		items, nerr := normalizeExplorePreferences(*req.ExplorePreferences)
		if nerr != nil {
			return nerr
		}
		prefs = encodeExplorePreferences(items)
	}
	if req.GuideCompleted != nil {
		done = *req.GuideCompleted
	}
	g.gender = gender
	g.birth = birth
	g.preferences = prefs
	// 写入档案草稿即视为这台设备已经走过引导，避免登录后重启又出一遍。
	if deviceHasProfile(g) && req.GuideCompleted == nil {
		done = true
	}
	if g.found {
		if _, err := a.Store.Exec(r.Context(), `UPDATE device_experiences
			SET device_id=$5, guide_completed=$1, guide_offered=TRUE, gender=$2, birth_date=$3, explore_preferences=$4
			WHERE lower(device_id)=lower($5)`, done, gender, birth, prefs, deviceID); err != nil {
			return err
		}
	} else if _, err := a.Store.Exec(r.Context(), `INSERT INTO device_experiences(device_id, guide_completed, guide_offered, gender, birth_date, explore_preferences)
		VALUES ($1,$2,TRUE,$3,$4,$5)`, deviceID, done, gender, birth, prefs); err != nil {
		return err
	}
	g.found = true
	g.guideDone = done
	g.offered = true
	if uid := a.optionalAppUser(r); uid > 0 {
		a.attachDeviceProfileDraft(r.Context(), uid, deviceID)
	}
	view, err := a.onboardingView(r, g)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) appAbandonOnboarding(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.appActor(r)
	if err != nil {
		return err
	}
	var req struct {
		DeviceID string `json:"device_id"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	deviceID, nerr := normalizeOnboardingDevice(req.DeviceID)
	if nerr != nil {
		return nerr
	}
	var asked int
	if uid > 0 {
		err = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings
			WHERE deleted_at IS NULL AND onboard_experience AND btrim(question)<>'' AND (user_id=$1 OR lower(device_id)=lower($2))`, uid, deviceID).Scan(&asked)
	} else {
		err = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings
			WHERE deleted_at IS NULL AND onboard_experience AND btrim(question)<>'' AND lower(device_id)=lower($1)`, deviceID).Scan(&asked)
	}
	if err != nil {
		return err
	}
	if asked > 0 {
		return biz("已填写问题，请同步这次进度")
	}
	if uid > 0 {
		if _, err := a.Store.Exec(r.Context(), `UPDATE readings SET deleted_at=now(), updated_at=now()
			WHERE deleted_at IS NULL AND onboard_experience AND btrim(question)='' AND (user_id=$1 OR lower(device_id)=lower($2))`, uid, deviceID); err != nil {
			return err
		}
		if _, err := a.Store.Exec(r.Context(), `UPDATE app_users SET reading_trial_used=TRUE WHERE id=$1`, uid); err != nil {
			return err
		}
	} else if _, err := a.Store.Exec(r.Context(), `UPDATE readings SET deleted_at=now(), updated_at=now()
		WHERE deleted_at IS NULL AND onboard_experience AND btrim(question)='' AND lower(device_id)=lower($1)`, deviceID); err != nil {
		return err
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO device_experiences(device_id, trial_forfeited) VALUES ($1, TRUE)
		ON CONFLICT (device_id) DO UPDATE SET trial_forfeited=TRUE`, deviceID); err != nil {
		return err
	}
	writeOK(w, map[string]any{"reading_trial_available": false})
	return nil
}

func (a *App) appSyncOnboarding(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.appActor(r)
	if err != nil {
		return err
	}
	var req struct {
		DeviceID string          `json:"device_id"`
		Question string          `json:"question"`
		SpreadID *int64          `json:"spread_id"`
		Cards    json.RawMessage `json:"cards"`
		Status   string          `json:"status"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	deviceID, nerr := normalizeOnboardingDevice(req.DeviceID)
	if nerr != nil {
		return nerr
	}
	a.attachDeviceProfileDraft(r.Context(), uid, deviceID)
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return biz("请先填写问题")
	}
	if len([]rune(question)) > 200 {
		return biz("问题超过 200 个码点，正式计数口径尚未确认")
	}
	status := req.Status
	switch status {
	case "", "QUESTION_CREATED", "SPREAD_SELECTED", "SHUFFLING", "CUTTING", "CARDS_FIXED":
		if status == "" {
			status = "QUESTION_CREATED"
		}
	default:
		return biz("当前状态不能同步")
	}
	if req.SpreadID != nil {
		var enabled bool
		if err := a.Store.QueryRow(r.Context(), `SELECT enabled FROM spreads WHERE id=$1`, *req.SpreadID).Scan(&enabled); err != nil || !enabled {
			return biz("牌阵不可用")
		}
	}
	var used bool
	if err := a.Store.QueryRow(r.Context(), `SELECT reading_trial_used FROM app_users WHERE id=$1`, uid).Scan(&used); err != nil {
		return err
	}
	var existing int64
	err = a.Store.QueryRow(r.Context(), `SELECT id FROM readings
		WHERE deleted_at IS NULL AND type='normal' AND onboard_experience AND status<>'COMPLETED'
		AND (user_id=$1 OR (user_id IS NULL AND lower(device_id)=lower($2)))
		ORDER BY id DESC LIMIT 1`, uid, deviceID).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	cards := ""
	if len(req.Cards) > 0 && string(req.Cards) != "null" {
		cards = string(req.Cards)
	}
	var spread sql.NullInt64
	if req.SpreadID != nil {
		spread = sql.NullInt64{Int64: *req.SpreadID, Valid: true}
	}
	if existing > 0 {
		if _, err := a.Store.Exec(r.Context(), `UPDATE readings SET user_id=$1, question=$2, status=$3, device_id=$4,
			spread_id=COALESCE($5, spread_id),
			spread_name=CASE WHEN $5 IS NULL THEN spread_name ELSE (SELECT name FROM spreads WHERE id=$5) END,
			cards=CASE WHEN $6='' THEN cards ELSE $6 END,
			updated_at=now() WHERE id=$7`, uid, question, status, deviceID, spread, cards, existing); err != nil {
			return err
		}
		item, err := a.loadReadingAbs(r.Context(), r, existing)
		if err != nil {
			return err
		}
		writeOK(w, item)
		return nil
	}
	onboard := !used
	var id int64
	err = a.Store.QueryRow(r.Context(), `INSERT INTO readings(user_id, type, question, spread_id, status, onboard_experience, device_id, cards, spread_name)
		VALUES ($1,'normal',$2,$3,$4,$5,$6,NULLIF($7,''), COALESCE((SELECT name FROM spreads WHERE id=$3), '')) RETURNING id`,
		uid, question, spread, status, onboard, deviceID, cards).Scan(&id)
	if err != nil {
		return err
	}
	item, err := a.loadReadingAbs(r.Context(), r, id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) adminListOnboarding(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT d.device_id, d.guide_completed, d.consumed, d.user_id, d.created_at,
		COALESCE(u.reading_trial_used, FALSE)
		FROM device_experiences d
		LEFT JOIN app_users u ON u.id=d.user_id
		ORDER BY d.created_at DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var device string
		var guideDone, consumed, trialUsed bool
		var userID sql.NullInt64
		var created time.Time
		if err := rows.Scan(&device, &guideDone, &consumed, &userID, &created, &trialUsed); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"device_id": device, "guide_completed": guideDone, "device_trial_consumed": consumed,
			"user_id": nullInt(userID), "reading_trial_used": trialUsed, "created_at": created,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{"items": list})
	return nil
}
