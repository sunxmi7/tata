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
	writeOK(w, guideView(g))
	return nil
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
	writeOK(w, guideView(g))
	return nil
}
