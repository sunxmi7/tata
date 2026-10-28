package tata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestAppShellP0ProfileTopicsLedgerFeedback(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "shell")
	phone := fmt.Sprintf("13%09d", time.Now().UnixNano()%1_000_000_000)
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET phone=$1 WHERE id=$2`, phone, uid); err != nil {
		t.Fatal(err)
	}

	profile := listOne(t, ts, token, "/api/app/v1/profile")
	if profile["name"] != "" || profile["bio"] != "" || profile["avatar_url"] != nil {
		t.Fatalf("empty profile: %#v", profile)
	}
	display, _ := profile["account_display"].(string)
	email, _ := profile["email"].(string)
	if display == "" || display == phone || !strings.Contains(display, "***@") || strings.Contains(display, phone) {
		t.Fatalf("account_display should be masked email, got %#v phone=%s email=%s", profile["account_display"], phone, email)
	}
	must(t, ts, token, http.MethodPut, "/api/app/v1/profile", map[string]any{
		"name": "塔塔", "bio": "爱抽牌", "avatar_url": "https://cdn.example/avatar.png",
	})
	profile = listOne(t, ts, token, "/api/app/v1/profile")
	if profile["name"] != "塔塔" || profile["bio"] != "爱抽牌" || profile["avatar_url"] != "https://cdn.example/avatar.png" {
		t.Fatalf("profile write: %#v", profile)
	}
	if msg, _ := call(t, ts, token, http.MethodPut, "/api/app/v1/profile", map[string]any{"name": strings.Repeat("名", 33)}); !strings.Contains(msg, "姓名") {
		t.Fatalf("long name: %s", msg)
	}

	topics := listOne(t, ts, token, "/api/app/v1/explore/topics")
	groups, _ := topics["groups"].([]any)
	if len(groups) < 2 {
		t.Fatalf("topics groups: %#v", topics)
	}
	titles := map[string]int{}
	topicCount := 0
	for _, raw := range groups {
		g := raw.(map[string]any)
		title, _ := g["title"].(string)
		items, _ := g["topics"].([]any)
		titles[title] = len(items)
		topicCount += len(items)
		if len(items) == 0 {
			t.Fatalf("empty group %s: %#v", title, g)
		}
		first := items[0].(map[string]any)
		if first["topic_id"] == "" || first["title"] == "" {
			t.Fatalf("topic fields: %#v", first)
		}
	}
	if titles["此刻抉择"] == 0 || titles["主题占卜"] == 0 || topicCount < 3 {
		t.Fatalf("expected explore catalog, got %#v", titles)
	}

	ledger := listOne(t, ts, token, "/api/app/v1/wallet/ledger")
	items, ok := ledger["items"].([]any)
	if !ok {
		t.Fatalf("ledger items missing: %#v", ledger)
	}
	if len(items) != 0 {
		t.Fatalf("empty ledger must not invent rows: %#v", ledger)
	}
	if ledger["display_label"] != displayLabelTataCoin || ledger["balance"].(float64) != 0 {
		t.Fatalf("empty ledger meta: %#v", ledger)
	}

	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/redeem", map[string]any{"code": "NO-SUCH-CODE"}); msg != "兑换码无效" {
		t.Fatalf("invalid redeem: %s", msg)
	}

	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/wishlist", map[string]any{"content": ""}); !strings.Contains(msg, "心愿") {
		t.Fatalf("wishlist empty: %s", msg)
	}
	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/wishlist", map[string]any{"content": strings.Repeat("愿", 501)}); !strings.Contains(msg, "500") {
		t.Fatalf("wishlist long: %s", msg)
	}
	wish := create(t, ts, token, "/api/app/v1/wishlist", map[string]any{"content": "希望能换牌背", "image_url": "https://cdn.example/wish.png"})
	if wish["message"] != "心愿已提交" || wish["id"] == nil {
		t.Fatalf("wishlist: %#v", wish)
	}

	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/feedback", map[string]any{"category": "吐槽", "content": "x"}); !strings.Contains(msg, "分类") {
		t.Fatalf("feedback category: %s", msg)
	}
	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/feedback", map[string]any{"category": "使用问题", "content": "卡了", "email": "not-an-email"}); !strings.Contains(msg, "邮箱") {
		t.Fatalf("feedback email: %s", msg)
	}
	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/feedback", map[string]any{"category": "内容建议", "content": strings.Repeat("反", 501)}); !strings.Contains(msg, "500") {
		t.Fatalf("feedback long: %s", msg)
	}
	fb := create(t, ts, token, "/api/app/v1/feedback", map[string]any{
		"category": "其他", "content": "界面很好", "email": "ok@tata.local",
	})
	if fb["message"] != "反馈已提交" {
		t.Fatalf("feedback: %#v", fb)
	}
}

func TestAppShellP0RedeemLedgerUploadsNotificationsQuota(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "redeem")
	code := fmt.Sprintf("TATA%d", time.Now().UnixNano())
	if _, err := store.Exec(t.Context(), `INSERT INTO redeem_codes(code, kind, amount, benefit_name, max_redemptions, enabled)
		VALUES ($1,'diamonds',60,'新春钻石礼',1,true)`, strings.ToUpper(code)); err != nil {
		t.Fatal(err)
	}

	out := create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": strings.ToLower(code)})
	if out["benefit_name"] != "新春钻石礼" || out["amount"].(float64) != 60 || out["kind"] != "diamonds" {
		t.Fatalf("redeem: %#v", out)
	}
	if out["display_label"] != displayLabelTataCoin || out["pack_diamonds"].(float64) != 60 || out["balance"].(float64) != 60 {
		t.Fatalf("redeem balance: %#v", out)
	}
	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/redeem", map[string]any{"code": code}); msg != "兑换码已使用" {
		t.Fatalf("second redeem: %s", msg)
	}
	var pack int
	if err := store.QueryRow(t.Context(), `SELECT pack_diamonds FROM diamond_balances WHERE user_id=$1`, uid).Scan(&pack); err != nil {
		t.Fatal(err)
	}
	if pack != 60 {
		t.Fatalf("pack_diamonds=%d", pack)
	}

	ledger := listOne(t, ts, token, "/api/app/v1/wallet/ledger")
	items, _ := ledger["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("ledger after redeem: %#v", ledger)
	}
	row := items[0].(map[string]any)
	if row["type"] != "redeem" || row["amount"].(float64) != 60 || row["note"] != "兑换码" {
		t.Fatalf("ledger row: %#v", row)
	}

	quota := listOne(t, ts, token, "/api/app/v1/quota")
	if quota["display_label"] != displayLabelTataCoin || quota["paid_label"] != "钻石" || quota["balance"].(float64) != 60 {
		t.Fatalf("quota labels: %#v", quota)
	}
	ent := listOne(t, ts, token, "/api/app/v1/iap/entitlements")
	if ent["display_label"] != displayLabelTataCoin || ent["unit"] != "diamond" || ent["balance"].(float64) != 60 {
		t.Fatalf("entitlements: %#v", ent)
	}
	skus := list(t, ts, token, "/api/app/v1/iap/skus")
	if len(skus) == 0 {
		t.Fatal("skus empty")
	}
	for _, sku := range skus {
		if sku["display_label"] != displayLabelTataCoin || sku["unit"] != "diamond" {
			t.Fatalf("sku label: %#v", sku)
		}
		if sku["price_amount_minor"] != nil {
			t.Fatalf("sku invented price: %#v", sku)
		}
	}

	upload := create(t, ts, token, "/api/app/v1/uploads", map[string]any{
		"filename": "dot.png", "content_type": "image/png", "content_base64": tinyPNG,
	})
	url, _ := upload["url"].(string)
	if !strings.Contains(url, "/uploads/") || !strings.HasSuffix(url, ".png") {
		t.Fatalf("upload url: %#v", upload)
	}
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("fetch upload: %d", res.StatusCode)
	}

	prefs := listOne(t, ts, token, "/api/app/v1/notification-preferences")
	if prefs["daily_draw_enabled"] != false || prefs["messages_enabled"] != true || prefs["daily_draw_time"] != "08:00" {
		t.Fatalf("default prefs: %#v", prefs)
	}
	must(t, ts, token, http.MethodPut, "/api/app/v1/notification-preferences", map[string]any{
		"daily_draw_enabled": true, "daily_draw_time": "09:30",
		"tomorrow_draw_enabled": true, "tomorrow_draw_time": "21:00",
		"messages_enabled": false,
	})
	prefs = listOne(t, ts, token, "/api/app/v1/notification-preferences")
	if prefs["daily_draw_enabled"] != true || prefs["daily_draw_time"] != "09:30" {
		t.Fatalf("saved prefs: %#v", prefs)
	}
	if prefs["tomorrow_draw_enabled"] != true || prefs["tomorrow_draw_time"] != "21:00" || prefs["messages_enabled"] != false {
		t.Fatalf("saved prefs flags: %#v", prefs)
	}
	if msg, _ := call(t, ts, token, http.MethodPut, "/api/app/v1/notification-preferences", map[string]any{"daily_draw_time": "9点"}); !strings.Contains(msg, "时间") {
		t.Fatalf("bad time: %s", msg)
	}

	tarotists := list(t, ts, token, "/api/app/v1/tarotists")
	found := false
	for _, row := range tarotists {
		if row["name"] == "知晚" {
			found = true
			if row["blurb"] == "" || row["display_copy"] == "" {
				t.Fatalf("tarotist home fields: %#v", row)
			}
			tags, _ := row["tags"].([]any)
			if len(tags) == 0 {
				t.Fatalf("tarotist tags: %#v", row)
			}
			if _, ok := row["avatar_url"]; !ok {
				t.Fatalf("tarotist missing avatar_url: %#v", row)
			}
		}
	}
	if !found {
		t.Fatalf("seed tarotist missing: %#v", tarotists)
	}
}

func TestAppShellDailyHomeFieldsPresent(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "dailyhome")
	_ = uid
	raw := get(t, ts, token, "/api/app/v1/calendar")
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if env.Data["days"] == nil || env.Data["tomorrow_date"] == "" || env.Data["cycle"] == "" {
		t.Fatalf("calendar empty list contract: %s", raw)
	}
	if days, ok := env.Data["days"].([]any); !ok {
		t.Fatalf("calendar days must be a list: %s", raw)
	} else if days == nil {
		t.Fatalf("calendar days null: %s", raw)
	}

	msg, body := call(t, ts, token, http.MethodPost, "/api/app/v1/daily", map[string]any{"target": "today"})
	if msg != "" && !strings.Contains(msg, "已占用") && !strings.Contains(msg, "已用完") && !strings.Contains(msg, "钻石") && !strings.Contains(msg, "未配置") {
		t.Fatalf("daily: %s %s", msg, body)
	}
	var daily struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &daily); err != nil {
		t.Fatal(err)
	}
	payload := daily.Data
	if reused, _ := payload["reused"].(bool); reused {
		if reading, ok := payload["reading"].(map[string]any); ok {
			payload = reading
		}
	}
	for _, key := range []string{"card_name", "short_text", "yi", "card_thumbnail", "target", "has_result"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("daily missing %s: %#v", key, payload)
		}
	}
	if payload["yi"] != nil {
		t.Fatalf("yi must stay unset rather than invented: %#v", payload["yi"])
	}
}

func TestRuneLenHelper(t *testing.T) {
	if n := utf8.RuneCountInString(strings.Repeat("愿", 500)); n != 500 {
		t.Fatalf("rune setup %d", n)
	}
}
