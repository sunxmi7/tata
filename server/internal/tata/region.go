package tata

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	regionStatusPending  = "pending"
	regionStatusResolved = "resolved"
	regionStatusUnknown  = "unknown"
)

// lookupIPRegion resolves a public IP to province/city. Tests may stub this.
var lookupIPRegion = defaultLookupIPRegion

func (a *App) captureRegisterIP(r *http.Request, uid int64, created bool) {
	if !created || uid <= 0 || r == nil {
		return
	}
	ip := clientIP(r)
	_, _ = a.Store.Exec(r.Context(), `UPDATE app_users SET register_ip=$1, region_status=$2
		WHERE id=$3 AND COALESCE(register_ip,'')=''`, ip, regionStatusPending, uid)
	go a.resolveUserRegion(uid)
}

func (a *App) resolveUserRegion(uid int64) {
	if uid <= 0 || a.Store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var ip string
	err := a.Store.QueryRow(ctx, `SELECT COALESCE(register_ip,'') FROM app_users WHERE id=$1`, uid).Scan(&ip)
	if err != nil {
		return
	}
	ip = strings.TrimSpace(ip)
	if ip == "" || isPrivateOrLocalIP(ip) {
		_, _ = a.Store.Exec(ctx, `UPDATE app_users SET region_province='', region_city='', region_status=$1 WHERE id=$2`,
			regionStatusUnknown, uid)
		return
	}
	province, city, ok := lookupIPRegion(ctx, ip)
	if !ok {
		_, _ = a.Store.Exec(ctx, `UPDATE app_users SET region_province='', region_city='', region_status=$1 WHERE id=$2`,
			regionStatusUnknown, uid)
		return
	}
	_, _ = a.Store.Exec(ctx, `UPDATE app_users SET region_province=$1, region_city=$2, region_status=$3 WHERE id=$4`,
		province, city, regionStatusResolved, uid)
}

func (a *App) adminBackfillUserRegions(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id FROM app_users
		WHERE region_status IN ('', $1) AND COALESCE(register_ip,'')<>''
		ORDER BY id DESC LIMIT 100`, regionStatusPending)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		a.resolveUserRegion(id)
	}
	a.Store.audit(r.Context(), act.id, "backfill", "user_region", "", fmtID(int64(len(ids))))
	writeOK(w, map[string]any{"processed": len(ids)})
	return nil
}

func isPrivateOrLocalIP(raw string) bool {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return true
	}
	return false
}

func defaultLookupIPRegion(ctx context.Context, ip string) (province, city string, ok bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://ip-api.com/json/"+ip+"?lang=zh-CN&fields=status,regionName,city", nil)
	if err != nil {
		return "", "", false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", "", false
	}
	defer res.Body.Close()
	var body struct {
		Status     string `json:"status"`
		RegionName string `json:"regionName"`
		City       string `json:"city"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", "", false
	}
	if body.Status != "success" {
		return "", "", false
	}
	province = strings.TrimSpace(body.RegionName)
	city = strings.TrimSpace(body.City)
	if province == "" && city == "" {
		return "", "", false
	}
	return province, city, true
}
