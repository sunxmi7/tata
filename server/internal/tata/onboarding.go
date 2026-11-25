package tata

import (
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
	consumed    bool
	forfeited   bool
	gender      string
	birth       string
	preferences string
}

func (a *App) loadDeviceGuide(r *http.Request, deviceID string) (deviceGuide, error) {
	var g deviceGuide
	err := a.Store.QueryRow(r.Context(), `SELECT guide_completed, consumed, trial_forfeited, gender, birth_date, explore_preferences
		FROM device_experiences WHERE device_id=$1`, deviceID).Scan(&g.guideDone, &g.consumed, &g.forfeited, &g.gender, &g.birth, &g.preferences)
	if errors.Is(err, sql.ErrNoRows) {
		return g, nil
	}
	if err != nil {
		return g, err
	}
	g.found = true
	return g, nil
}

func guideView(g deviceGuide) map[string]any {
	show := !g.found || !g.guideDone
	trial := !g.found || (!g.consumed && !g.forfeited)
	return map[string]any{
		"show_guide":              show,
		"guide_completed":         g.found && g.guideDone,
		"reading_trial_available": trial,
		"gender":                  g.gender,
		"birth_date":              g.birth,
		"explore_preferences":     parseExplorePreferences(g.preferences),
	}
}

func (a *App) appGetOnboarding(w http.ResponseWriter, r *http.Request) error {
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if !validDeviceID(deviceID) {
		return biz("缺少设备标识")
	}
	g, err := a.loadDeviceGuide(r, deviceID)
	if err != nil {
		return err
	}
	view, err := a.onboardingView(r, g)
	if err != nil {
		return err
	}
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
	deviceID := strings.TrimSpace(req.DeviceID)
	if !validDeviceID(deviceID) {
		return biz("缺少设备标识")
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
	if g.found {
		if _, err := a.Store.Exec(r.Context(), `UPDATE device_experiences
			SET guide_completed=$1, gender=$2, birth_date=$3, explore_preferences=$4
			WHERE device_id=$5`, done, gender, birth, prefs, deviceID); err != nil {
			return err
		}
	} else if _, err := a.Store.Exec(r.Context(), `INSERT INTO device_experiences(device_id, guide_completed, gender, birth_date, explore_preferences)
		VALUES ($1,$2,$3,$4,$5)`, deviceID, done, gender, birth, prefs); err != nil {
		return err
	}
	g.found = true
	g.guideDone = done
	g.gender = gender
	g.birth = birth
	g.preferences = prefs
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
	deviceID := strings.TrimSpace(req.DeviceID)
	if !validDeviceID(deviceID) {
		return biz("缺少设备标识")
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
	deviceID := strings.TrimSpace(req.DeviceID)
	if !validDeviceID(deviceID) {
		return biz("缺少设备标识")
	}
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
