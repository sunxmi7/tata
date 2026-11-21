package tata

import (
	"database/sql"
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
	gender      string
	birth       string
	preferences string
}

func (a *App) loadDeviceGuide(r *http.Request, deviceID string) (deviceGuide, error) {
	var g deviceGuide
	err := a.Store.QueryRow(r.Context(), `SELECT guide_completed, consumed, gender, birth_date, explore_preferences
		FROM device_experiences WHERE device_id=$1`, deviceID).Scan(&g.guideDone, &g.consumed, &g.gender, &g.birth, &g.preferences)
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
	trial := !g.found || !g.consumed
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
	uid, _, err := a.appActor(r)
	if err != nil {
		if _, ok := err.(authFail); ok {
			return view, nil
		}
		return nil, err
	}
	if uid == 0 {
		return view, nil
	}
	view["reading_trial_available"] = false
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
