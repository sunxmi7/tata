package tata

import (
	"net/http"
	"testing"
	"time"
)

func TestAgeBucket(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	cases := map[string]string{
		"":           "未填写",
		"not-a-date": "未填写",
		"2012-11-19": "15以下",
		"2011-11-18": "15-18",
		"2008-11-18": "15-18",
		"2007-11-18": "19-22",
		"2004-11-18": "19-22",
		"2003-11-18": "23-24",
		"2002-11-18": "23-24",
		"2001-11-18": "25-30",
		"1996-11-18": "25-30",
		"1995-11-18": "31-35",
		"1991-11-18": "31-35",
		"1990-11-18": "36-40",
		"1986-11-18": "36-40",
		"1985-11-18": "41-50",
		"1976-11-18": "41-50",
		"1975-11-18": "51以上",
		"2011-11-19": "15以下",
	}
	for birth, want := range cases {
		if got := ageBucket(birth, now); got != want {
			t.Fatalf("ageBucket(%q)=%q want %q", birth, got, want)
		}
	}
}

func TestAdminUserListAndStats(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	before := listOne(t, ts, admin.access, "/api/admin/v1/users/stats")
	beforeGender := statCount(t, before, "gender", "女性")
	beforeAge := statCount(t, before, "age", "25-30")
	beforeExplore := statCount(t, before, "explore", "感情与关系")
	beforeEmpty := statCount(t, before, "explore", "未选择")

	token, uid := registerUser(t, ts, store, "admin-stats")
	must(t, ts, token, http.MethodPut, "/api/app/v1/profile", map[string]any{
		"name":                "统计昵称",
		"gender":              "女性",
		"birth_date":          "1998-11-18",
		"explore_preferences": []string{"感情与关系", "自我成长"},
		"timezone":            "Asia/Shanghai",
	})

	users := list(t, ts, admin.access, "/api/admin/v1/users")
	var row map[string]any
	for _, item := range users {
		if int64(item["id"].(float64)) == uid {
			row = item
			break
		}
	}
	if row == nil {
		t.Fatal("created user missing from admin list")
	}
	if row["name"] != "统计昵称" || row["nickname"] != "统计昵称" {
		t.Fatalf("admin nickname: %#v", row)
	}
	if row["gender"] != "女性" || row["birth_date"] != "1998-11-18" {
		t.Fatalf("admin profile: %#v", row)
	}
	avatar, _ := row["avatar_url"].(string)
	if avatar == "" {
		t.Fatalf("admin avatar: %#v", row["avatar_url"])
	}
	prefs, _ := row["explore_preferences"].([]any)
	if len(prefs) != 2 {
		t.Fatalf("admin explore: %#v", row["explore_preferences"])
	}

	after := listOne(t, ts, admin.access, "/api/admin/v1/users/stats")
	if statCount(t, after, "gender", "女性") != beforeGender+1 {
		t.Fatalf("gender count: before=%d after=%#v", beforeGender, after["gender"])
	}
	if statCount(t, after, "age", "25-30") != beforeAge+1 {
		t.Fatalf("age count: before=%d after=%#v", beforeAge, after["age"])
	}
	if statCount(t, after, "explore", "感情与关系") != beforeExplore+1 {
		t.Fatalf("explore count: before=%d after=%#v", beforeExplore, after["explore"])
	}
	if statCount(t, after, "explore", "未选择") != beforeEmpty {
		t.Fatalf("unselected should stay: before=%d after=%#v", beforeEmpty, after["explore"])
	}

	must(t, ts, token, http.MethodPost, "/api/app/v1/auth/cancel", map[string]any{})
	cancelled := listOne(t, ts, admin.access, "/api/admin/v1/users/stats")
	if statCount(t, cancelled, "gender", "女性") != beforeGender {
		t.Fatalf("cancelled still counted: %#v", cancelled["gender"])
	}
}

func statCount(t *testing.T, stats map[string]any, group, label string) int {
	t.Helper()
	block, _ := stats[group].(map[string]any)
	items, _ := block["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["label"] == label {
			n, _ := item["count"].(float64)
			return int(n)
		}
	}
	t.Fatalf("%s %s missing: %#v", group, label, stats[group])
	return 0
}
