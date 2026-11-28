package tata

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func adminUserFilter(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, biz("用户 id 不正确")
	}
	return id, nil
}

func (a *App) adminListRegistrationGifts(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	uid, err := adminUserFilter(r)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT user_id, registered_at, granted_at, usable_at, expires_at, total, remaining, used_at
		FROM registration_gifts WHERE ($1 = 0 OR user_id = $1)
		ORDER BY registered_at DESC, user_id DESC LIMIT 200`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var uid int64
		var registered time.Time
		var granted, usable, expires, used sql.NullTime
		var total, remaining int
		if err := rows.Scan(&uid, &registered, &granted, &usable, &expires, &total, &remaining, &used); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"user_id": uid, "registered_at": registered,
			"granted_at": nullTime(granted), "usable_at": nullTime(usable), "expires_at": nullTime(expires),
			"total": total, "remaining": remaining, "used_at": nullTime(used),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{"items": list})
	return nil
}

func (a *App) adminListQuotaLots(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	uid, err := adminUserFilter(r)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, user_id, source, total, remaining, granted_at, expires_at, order_id, reason
		FROM quota_lots WHERE ($1 = 0 OR user_id = $1) ORDER BY id DESC LIMIT 200`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	now := time.Now()
	list := []map[string]any{}
	for rows.Next() {
		var id, uid int64
		var source, reason string
		var total, remaining int
		var granted time.Time
		var expires sql.NullTime
		var orderID sql.NullInt64
		if err := rows.Scan(&id, &uid, &source, &total, &remaining, &granted, &expires, &orderID, &reason); err != nil {
			return err
		}
		expired := expires.Valid && !expires.Time.After(now)
		list = append(list, map[string]any{
			"id": id, "user_id": uid, "source": source, "total": total, "remaining": remaining,
			"granted_at": granted, "expires_at": nullTime(expires), "order_id": nullInt(orderID),
			"reason": reason, "expired": expired,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{"items": list})
	return nil
}

func (a *App) adminListYearlyQualifications(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	uid, err := adminUserFilter(r)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT user_id, year, unlocked, unlock_source, unlocked_at, report_status, generated_at
		FROM yearly_reports WHERE ($1 = 0 OR user_id = $1) ORDER BY year DESC, user_id DESC LIMIT 200`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var uid int64
		var year int
		var unlocked bool
		var source, status string
		var unlockedAt, generated sql.NullTime
		if err := rows.Scan(&uid, &year, &unlocked, &source, &unlockedAt, &status, &generated); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"user_id": uid, "year": year, "unlocked": unlocked, "unlock_source": source,
			"unlocked_at": nullTime(unlockedAt), "report_status": status, "generated_at": nullTime(generated),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{"items": list})
	return nil
}
