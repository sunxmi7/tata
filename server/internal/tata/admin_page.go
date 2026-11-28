package tata

import (
	"encoding/csv"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type adminPage struct {
	Limit  int
	Offset int
}

func parseAdminPage(r *http.Request) (adminPage, error) {
	p := adminPage{Limit: 50}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return p, biz("分页参数不正确")
		}
		p.Limit = n
	}
	if p.Limit > 200 {
		p.Limit = 200
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return p, biz("分页参数不正确")
		}
		p.Offset = n
	}
	return p, nil
}

func writeAdminPage(w http.ResponseWriter, items any, total, limit, offset int) {
	writeOK(w, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset})
}

func parseTimeRange(r *http.Request, fromKey, toKey string) (from, to *time.Time, err error) {
	if fromKey == "" {
		fromKey = "from"
	}
	if toKey == "" {
		toKey = "to"
	}
	if raw := strings.TrimSpace(r.URL.Query().Get(fromKey)); raw != "" {
		t, perr := parseAdminTime(raw)
		if perr != nil {
			return nil, nil, biz("开始时间不正确")
		}
		from = &t
	}
	if raw := strings.TrimSpace(r.URL.Query().Get(toKey)); raw != "" {
		t, perr := parseAdminTime(raw)
		if perr != nil {
			return nil, nil, biz("结束时间不正确")
		}
		to = &t
	}
	if from != nil && to != nil && to.Before(*from) {
		return nil, nil, biz("时间范围不正确")
	}
	return from, to, nil
}

func parseAdminTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	if t, err := time.ParseInLocation("2006-01-02", raw, loc); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("bad time")
}

func writeCSV(w http.ResponseWriter, filename string, header []string, rows [][]string) error {
	if filename == "" {
		filename = "export.csv"
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	// UTF-8 BOM for Excel.
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func queryBoolFilter(raw string) (set bool, value bool, err error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "":
		return false, false, nil
	case "1", "true", "yes":
		return true, true, nil
	case "0", "false", "no":
		return true, false, nil
	default:
		return false, false, biz("筛选参数不正确")
	}
}

func nullableStringPtr(v *string) any {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil
	}
	return strings.TrimSpace(*v)
}
