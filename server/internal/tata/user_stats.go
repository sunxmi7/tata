package tata

import (
	"math"
	"net/http"
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
	rows, err := a.Store.Query(r.Context(), `SELECT COALESCE(gender,''), COALESCE(birth_date,''), COALESCE(explore_preferences,'[]')
		FROM app_users WHERE cancelled_at IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()

	now := time.Now().In(time.FixedZone("CST", 8*3600))
	if loc, locErr := time.LoadLocation("Asia/Shanghai"); locErr == nil {
		now = time.Now().In(loc)
	}

	genderCounts := map[string]int{}
	ageCounts := map[string]int{}
	exploreCounts := map[string]int{}
	totalUsers := 0
	exploreTotal := 0

	for rows.Next() {
		var gender, birth, prefs string
		if err := rows.Scan(&gender, &birth, &prefs); err != nil {
			return err
		}
		totalUsers++
		genderCounts[genderBucket(gender)]++
		ageCounts[ageBucket(birth, now)]++
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

	exploreOrder := append(append([]string{}, explorePreferenceOptions...), statsUnselected)
	writeOK(w, map[string]any{
		"total_users": totalUsers,
		"gender":      map[string]any{"total": totalUsers, "items": statItems(genderStatLabels, genderCounts, totalUsers)},
		"age":         map[string]any{"total": totalUsers, "items": statItems(ageStatLabels, ageCounts, totalUsers)},
		"explore":     map[string]any{"total": exploreTotal, "items": statItems(exploreOrder, exploreCounts, exploreTotal)},
	})
	return nil
}
