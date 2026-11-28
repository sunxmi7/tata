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
	token, _ := registerUser(t, ts, store, "shell")

	profile := listOne(t, ts, token, "/api/app/v1/profile")
	name, _ := profile["name"].(string)
	email, _ := profile["email"].(string)
	if name == "" || !validGeneratedNickname(name) || profile["nickname"] != name {
		t.Fatalf("random chinese nickname: %#v", profile)
	}
	if strings.HasPrefix(name, "Tarot") {
		t.Fatalf("must not use Tarot+email prefix: %q", name)
	}
	avatar, _ := profile["avatar_url"].(string)
	if !strings.Contains(avatar, defaultAvatarName) || profile["bio"] != "" {
		t.Fatalf("default avatar: %#v", profile)
	}
	if _, ok := profile["phone"]; ok {
		t.Fatalf("profile should not expose phone: %#v", profile)
	}
	display, _ := profile["account_display"].(string)
	if display == "" || display == email || !strings.Contains(display, "***") {
		t.Fatalf("account_display should be masked email, got %#v email=%s", profile["account_display"], email)
	}
	publicID, _ := profile["public_id"].(string)
	if len(publicID) != 6 {
		t.Fatalf("public_id: %#v", profile["public_id"])
	}
	must(t, ts, token, http.MethodPut, "/api/app/v1/profile", map[string]any{
		"name": "塔塔", "bio": "爱抽牌", "avatar_url": "https://cdn.example/avatar.png",
	})
	profile = listOne(t, ts, token, "/api/app/v1/profile")
	if profile["name"] != "塔塔" || profile["bio"] != "爱抽牌" || profile["avatar_url"] != "https://cdn.example/avatar.png" {
		t.Fatalf("profile write: %#v", profile)
	}
	if msg, _ := call(t, ts, token, http.MethodPut, "/api/app/v1/profile", map[string]any{"name": strings.Repeat("名", 33)}); !strings.Contains(msg, "昵称") {
		t.Fatalf("long name: %s", msg)
	}

	topics := listOne(t, ts, token, "/api/app/v1/explore/topics")
	themes, _ := topics["themes"].([]any)
	if len(themes) != 11 {
		t.Fatalf("explore themes: %#v", topics)
	}
	titles := map[string]string{}
	for _, raw := range themes {
		theme := raw.(map[string]any)
		title, _ := theme["title"].(string)
		key, _ := theme["theme_key"].(string)
		if key == "" || theme["subtitle"] == "" || title == "" {
			t.Fatalf("theme fields: %#v", theme)
		}
		if _, ok := theme["spreads"]; ok {
			t.Fatalf("theme index must not include spreads: %#v", theme)
		}
		titles[title] = key
	}
	if titles["Yes or No"] != "yes_or_no" || titles["关系走向"] != "relationship" || titles["二选一"] != "either_or" {
		t.Fatalf("explore catalog: %#v", titles)
	}
	yes := listOne(t, ts, token, "/api/app/v1/explore/topics?theme_key=yes_or_no")
	yesSpreads, _ := yes["spreads"].([]any)
	if yes["theme_key"] != "yes_or_no" || yes["title"] != "Yes or No" || len(yesSpreads) != 1 {
		t.Fatalf("yes or no spreads: %#v", yes)
	}
	first := yesSpreads[0].(map[string]any)
	if first["name"] != "单张是非牌阵" || first["id"] == nil || first["short_sentence"] == "" || first["card_count"] == nil {
		t.Fatalf("spread fields: %#v", first)
	}
	questions, _ := first["quick_questions"].([]any)
	positions, _ := first["positions"].([]any)
	if len(questions) == 0 || len(positions) == 0 {
		t.Fatalf("explore positions: %#v", first)
	}
	for _, key := range []string{"either_or", "relationship", "inner", "shopping"} {
		one := listOne(t, ts, token, "/api/app/v1/explore/topics?theme_key="+key)
		items, _ := one["spreads"].([]any)
		if one["theme_key"] != key || len(items) != 2 {
			t.Fatalf("theme %s spreads: %#v", key, one)
		}
	}
	if msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/explore/topics?theme_key=missing", nil); msg != "探索主题不存在" {
		t.Fatalf("missing theme: %s", msg)
	}

	ledger := listOne(t, ts, token, "/api/app/v1/wallet/ledger")
	items, ok := ledger["items"].([]any)
	if !ok {
		t.Fatalf("ledger items missing: %#v", ledger)
	}
	if len(items) != 0 {
		t.Fatalf("empty ledger must not invent rows: %#v", ledger)
	}
	if ledger["display_label"] != displayLabelQuota || ledger["balance"].(float64) != 0 {
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
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	wishes := list(t, ts, admin.access, "/api/admin/v1/wishlists")
	if len(wishes) == 0 || wishes[0]["content"] != "希望能换牌背" || wishes[0]["image_url"] != "https://cdn.example/wish.png" {
		t.Fatalf("admin wishlists: %#v", wishes)
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
	feedbacks := list(t, ts, admin.access, "/api/admin/v1/feedback")
	if len(feedbacks) == 0 || feedbacks[0]["category"] != "其他" || feedbacks[0]["content"] != "界面很好" || feedbacks[0]["email"] != "ok@tata.local" {
		t.Fatalf("admin feedback: %#v", feedbacks)
	}
}

func TestAppShellP0RedeemLedgerUploadsNotificationsQuota(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "redeem")
	code := fmt.Sprintf("TATA%d", time.Now().UnixNano())
	if _, err := store.Exec(t.Context(), `INSERT INTO redeem_codes(code, kind, amount, benefit_name, max_redemptions, enabled)
		VALUES ($1,'quota',60,'新春钻石礼',1,true)`, strings.ToUpper(code)); err != nil {
		t.Fatal(err)
	}

	out := create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": strings.ToLower(code)})
	if out["benefit_name"] != "新春钻石礼" || out["amount"].(float64) != 60 || out["kind"] != "quota" {
		t.Fatalf("redeem: %#v", out)
	}
	if out["display_label"] != displayLabelQuota || quotaNum(out) != 60 {
		t.Fatalf("redeem balance: %#v", out)
	}
	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/redeem", map[string]any{"code": code}); msg != "兑换码已使用" {
		t.Fatalf("second redeem: %s", msg)
	}
	if quotaOf(t, store, uid) != 60 {
		t.Fatalf("quota_balance=%d", quotaOf(t, store, uid))
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
	if quota["display_label"] != displayLabelQuota || quota["paid_label"] != displayLabelQuota || quota["is_member"] != false {
		t.Fatalf("quota labels: %#v", quota)
	}
	if _, ok := quota["quota_remaining"]; ok {
		t.Fatalf("free user should hide quota: %#v", quota)
	}
	ent := listOne(t, ts, token, "/api/app/v1/iap/entitlements")
	if ent["display_label"] != displayLabelQuota || ent["unit"] != "quota" || ent["is_member"] != false {
		t.Fatalf("entitlements: %#v", ent)
	}
	skus := list(t, ts, token, "/api/app/v1/iap/skus")
	if len(skus) == 0 {
		t.Fatal("skus empty")
	}
	for _, sku := range skus {
		if sku["display_label"] != displayLabelQuota || sku["unit"] != "quota" {
			t.Fatalf("sku label: %#v", sku)
		}
		code, _ := sku["code"].(string)
		if code == "sub_month" && sku["price_amount_minor"].(float64) != 3900 {
			t.Fatalf("month price: %#v", sku)
		}
		if code == "pack_quota_20" || code == "pack_60" {
			t.Fatalf("free user saw pack sku: %#v", sku)
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
	seedGift(t, store, uid, 1)
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
	if msg != "" && !strings.Contains(msg, "已占用") && !strings.Contains(msg, "已用完") && !strings.Contains(msg, "额度") && !strings.Contains(msg, "未配置") && !strings.Contains(msg, "开通会员") {
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
	name, _ := payload["card_name"].(string)
	if name == "" {
		if payload["yi"] != nil || payload["card_message"] != nil {
			t.Fatalf("daily copy must stay unset without a card: %#v", payload)
		}
	} else if payload["yi"] == nil || payload["card_message"] == nil {
		t.Fatalf("daily copy must come from the card library: %#v", payload)
	}
}

func TestRuneLenHelper(t *testing.T) {
	if n := utf8.RuneCountInString(strings.Repeat("愿", 500)); n != 500 {
		t.Fatalf("rune setup %d", n)
	}
}
