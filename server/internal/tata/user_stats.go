package tata

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const statsUnspecified = "未填写"
const statsUnselected = "未选择"

var genderStatLabels = []string{"女性", "男性", "其他", "不愿透露", statsUnspecified}

var ageStatLabels = []string{
	"15以下", "15-18", "19-22", "23-24", "25-30", "31-35", "36-40", "41-50", "51以上", statsUnspecified,
}

func ageBucket(birth string, now time.Time) string {
	birth = strings.TrimSpace(birth)
	if birth == "" {
		return statsUnspecified
	}
	t, err := time.ParseInLocation("2006-01-02", birth, now.Location())
	if err != nil {
		return statsUnspecified
	}
	age := now.Year() - t.Year()
	if now.Month() < t.Month() || (now.Month() == t.Month() && now.Day() < t.Day()) {
		age--
	}
	if age < 0 {
		return statsUnspecified
	}
	switch {
	case age < 15:
		return "15以下"
	case age <= 18:
		return "15-18"
	case age <= 22:
		return "19-22"
	case age <= 24:
		return "23-24"
	case age <= 30:
		return "25-30"
	case age <= 35:
		return "31-35"
	case age <= 40:
		return "36-40"
	case age <= 50:
		return "41-50"
	default:
		return "51以上"
	}
}

func genderBucket(raw string) string {
	switch strings.TrimSpace(raw) {
	case "女性", "男性", "其他", "不愿透露":
		return strings.TrimSpace(raw)
	default:
		return statsUnspecified
	}
}

func statItems(order []string, counts map[string]int, total int) []map[string]any {
	out := make([]map[string]any, 0, len(order))
	for _, label := range order {
		n := counts[label]
		out = append(out, map[string]any{
			"label":   label,
			"count":   n,
			"percent": percentOf(n, total),
		})
	}
	return out
}

func percentOf(n, total int) float64 {
	if total <= 0 || n <= 0 {
		return 0
	}
	return math.Round(float64(n)*1000/float64(total)) / 10
}

func (a *App) adminUserStats(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return biz("天数不正确")
		}
		if n > 90 {
			n = 90
		}
		days = n
	}

	rows, err := a.Store.Query(r.Context(), `SELECT COALESCE(gender,''), COALESCE(birth_date,''), COALESCE(explore_preferences,'[]'),
		COALESCE(region_province,'')
		FROM app_users WHERE cancelled_at IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()

	now := beijingNow()
	genderCounts := map[string]int{}
	ageCounts := map[string]int{}
	exploreCounts := map[string]int{}
	regionCounts := map[string]int{}
	totalUsers := 0
	exploreTotal := 0

	for rows.Next() {
		var gender, birth, prefs, province string
		if err := rows.Scan(&gender, &birth, &prefs, &province); err != nil {
			return err
		}
		totalUsers++
		genderCounts[genderBucket(gender)]++
		ageCounts[ageBucket(birth, now)]++
		regionLabel := strings.TrimSpace(province)
		if regionLabel == "" {
			regionLabel = "未知"
		}
		regionCounts[regionLabel]++
		selected := parseExplorePreferences(prefs)
		if len(selected) == 0 {
			exploreCounts[statsUnselected]++
			exploreTotal++
			continue
		}
		for _, item := range selected {
			exploreCounts[item]++
			exploreTotal++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	dailyNew, err := a.userDailyNew(r.Context(), days, now)
	if err != nil {
		return err
	}

	regionOrder := make([]string, 0, len(regionCounts))
	for label := range regionCounts {
		if label != "未知" {
			regionOrder = append(regionOrder, label)
		}
	}
	sort.Strings(regionOrder)
	regionOrder = append(regionOrder, "未知")

	exploreOrder := append(append([]string{}, explorePreferenceOptions...), statsUnselected)
	writeOK(w, map[string]any{
		"total_users": totalUsers,
		"gender":      map[string]any{"total": totalUsers, "items": statItems(genderStatLabels, genderCounts, totalUsers)},
		"age":         map[string]any{"total": totalUsers, "items": statItems(ageStatLabels, ageCounts, totalUsers)},
		"explore":     map[string]any{"total": exploreTotal, "items": statItems(exploreOrder, exploreCounts, exploreTotal)},
		"region":      map[string]any{"total": totalUsers, "items": statItems(regionOrder, regionCounts, totalUsers)},
		"daily_new":   dailyNew,
		"notes": []string{
			"画像统计未注销用户（含历史已合并账号）",
			"日新增按 Asia/Shanghai 的 created_at，且未合并",
			"注册地区按 IP 回填省份，失败归「未知」",
			"日活/留存/渠道见友盟，不在此页",
		},
	})
	return nil
}

func (a *App) userDailyNew(ctx context.Context, days int, now time.Time) ([]map[string]any, error) {
	loc := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	rows, err := a.Store.Query(ctx, `
		SELECT to_char((created_at AT TIME ZONE 'Asia/Shanghai')::date, 'YYYY-MM-DD') AS d, count(*)::int
		FROM app_users
		WHERE merged_into IS NULL AND created_at >= $1
		GROUP BY 1 ORDER BY 1`, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDate := map[string]int{}
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return nil, err
		}
		byDate[d] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		out = append(out, map[string]any{"date": day, "count": byDate[day]})
	}
	return out, nil
}
