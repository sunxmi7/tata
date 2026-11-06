package tata

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdminCopyConfigurable(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE iap_skus SET copy='' WHERE code='sub_year'`)
		_, _ = store.Exec(context.Background(), `DELETE FROM app_config WHERE key LIKE 'yearly.2026.%' OR key LIKE 'card.愚者.%' OR key LIKE 'card.魔术师.%' OR key='card.thumbnail_template' OR key='card.thumbnail_default'`)
		_, _ = store.Exec(context.Background(), `UPDATE help_articles SET body='待确认', body_status='pending' WHERE article_id='how_to_start'`)
		_, _ = store.Exec(context.Background(), `UPDATE campaigns SET detail_body='详情页不在本次交付范围。奖励、条件与有效期以活动配置为准。', detail_status='pending', enabled=true WHERE campaign_id='friend_invite'`)
	})

	var yearID int64
	var yearName string
	var yearPrice float64
	for _, row := range list(t, ts, admin.access, "/api/admin/v1/iap/skus?kind=year_subscription") {
		if row["code"] == "sub_year" {
			yearID = asID(row["id"])
			yearName, _ = row["display_name"].(string)
			yearPrice, _ = row["price_amount_minor"].(float64)
		}
	}
	if yearID == 0 {
		t.Fatal("year sku missing")
	}
	msg, _ := call(t, ts, ro.access, http.MethodPut, "/api/admin/v1/copy/yearly", map[string]any{"year": 2026, "sections": []map[string]any{{"key": "theme", "body": "x"}}})
	if !strings.Contains(msg, "只读") {
		t.Fatalf("readonly copy: %s", msg)
	}
	must(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/iap/skus/%d", yearID), map[string]any{
		"display_name": yearName, "copy": "年卡可参与线下沙龙",
		"price_amount_minor": int64(yearPrice), "price_currency": "CNY", "enabled": true,
	})
	token, uid := registerUser(t, ts, store, "copycfg")
	skus := list(t, ts, token, "/api/app/v1/iap/skus?kind=year_subscription")
	if len(skus) == 0 {
		t.Fatal("year sku missing")
	}
	if skus[0]["copy"] != "年卡可参与线下沙龙" {
		t.Fatalf("sku copy: %#v", skus[0])
	}
	foundPerk := false
	for _, raw := range skus[0]["benefits"].([]any) {
		b := raw.(map[string]any)
		if b["key"] == "campaign_perk" {
			foundPerk = true
			if b["copy_status"] != "ready" || b["body"] != "年卡可参与线下沙龙" || b["included"] != true {
				t.Fatalf("perk: %#v", b)
			}
		}
	}
	if !foundPerk {
		t.Fatalf("benefits: %#v", skus[0]["benefits"])
	}

	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/yearly", map[string]any{
		"year": 2026,
		"sections": []map[string]any{
			{"key": "theme", "body": "把选择看清楚"},
			{"key": "domain", "body": "关系先于效率"},
			{"key": "rhythm", "body": "上半年收，下半年放"},
			{"key": "action", "body": "每月留一个空白日"},
		},
	})
	yearView := listOne(t, ts, token, "/api/app/v1/yearly-reports/2026")
	if yearView["content_status"] != "ready" || yearView["theme"] != "把选择看清楚" {
		t.Fatalf("yearly copy: %#v", yearView)
	}

	cards := listOne(t, ts, admin.access, "/api/admin/v1/copy/cards")
	items, _ := cards["items"].([]any)
	if len(items) != 78 {
		t.Fatalf("card copy count %d", len(items))
	}
	if cards["thumbnail_template"] != nil {
		t.Fatalf("template invented: %#v", cards["thumbnail_template"])
	}
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/cards", map[string]any{"card_name": "愚者", "yi": "换一件浅色外套"})
	day := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
	if _, err := store.Exec(t.Context(), `INSERT INTO readings(user_id, type, question, status, cards, interpretation, business_date)
		VALUES ($1,'daily','今日一牌','COMPLETED','[{"name":"愚者"}]','ok',$2)`, uid, day); err != nil {
		t.Fatal(err)
	}
	msg, raw := call(t, ts, token, http.MethodPost, "/api/app/v1/daily", map[string]any{"target": "today"})
	if !strings.Contains(msg, "已占用") && !strings.Contains(msg, "已有结果") {
		t.Fatalf("daily reuse: %s %s", msg, raw)
	}
	reused := decodeData(t, raw)
	reading, _ := reused["reading"].(map[string]any)
	if reading["yi"] != "换一件浅色外套" {
		t.Fatalf("daily yi: %#v", reading)
	}
	if reading["card_thumbnail"] != defaultCardThumbnailPath {
		t.Fatalf("daily default thumbnail: %#v", reading["card_thumbnail"])
	}

	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/cards", map[string]any{
		"card_name": "愚者", "thumbnail": "https://cdn.example.test/fool.png",
	})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/cards", map[string]any{
		"thumbnail_template": "https://cdn.example.test/rw/{id}.png",
		"thumbnail_default":  "https://cdn.example.test/default.png",
	})
	cards = listOne(t, ts, admin.access, "/api/admin/v1/copy/cards")
	if cards["thumbnail_template"] != "https://cdn.example.test/rw/{id}.png" || cards["thumbnail_default"] != "https://cdn.example.test/default.png" {
		t.Fatalf("template/default: %#v", cards)
	}
	fool := copyCardByName(cards["items"], "愚者")
	magician := copyCardByName(cards["items"], "魔术师")
	if fool["yi"] != "换一件浅色外套" || fool["thumbnail"] != "https://cdn.example.test/fool.png" || fool["card_thumbnail"] != "https://cdn.example.test/fool.png" {
		t.Fatalf("fool assets: %#v", fool)
	}
	if magician["thumbnail"] != nil || magician["card_thumbnail"] != "https://cdn.example.test/rw/1.png" {
		t.Fatalf("magician template: %#v", magician)
	}
	msg, raw = call(t, ts, token, http.MethodPost, "/api/app/v1/daily", map[string]any{"target": "today"})
	if !strings.Contains(msg, "已占用") && !strings.Contains(msg, "已有结果") {
		t.Fatalf("daily reuse after thumb: %s %s", msg, raw)
	}
	reading, _ = decodeData(t, raw)["reading"].(map[string]any)
	if reading["card_thumbnail"] != "https://cdn.example.test/fool.png" || reading["yi"] != "换一件浅色外套" {
		t.Fatalf("daily thumbnail: %#v", reading)
	}
	page := listOne(t, ts, token, "/api/app/v1/readings?limit=10")
	hist, _ := page["items"].([]any)
	if len(hist) == 0 {
		t.Fatalf("history empty: %#v", page)
	}
	if hist[0].(map[string]any)["card_thumbnail"] != "https://cdn.example.test/fool.png" {
		t.Fatalf("history thumbnail: %#v", hist[0])
	}

	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/help", map[string]any{
		"article_id": "how_to_start", "body": "从首页开始一次占卜。",
	})
	article := listOne(t, ts, token, "/api/app/v1/help/articles/how_to_start")
	if article["body_status"] != "ready" || article["body"] != "从首页开始一次占卜。" {
		t.Fatalf("help copy: %#v", article)
	}

	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/campaigns", map[string]any{
		"campaign_id": "friend_invite", "detail_body": "邀请一位朋友即可查看进度。",
	})
	detail := listOne(t, ts, token, "/api/app/v1/campaigns/friend_invite")
	if detail["body_status"] != "ready" || detail["detail_delivered"] != true || detail["body"] != "邀请一位朋友即可查看进度。" {
		t.Fatalf("campaign copy: %#v", detail)
	}

	if msg, _ := call(t, ts, admin.access, http.MethodGet, "/api/admin/v1/copy/membership", nil); !strings.Contains(msg, "文案类型") {
		t.Fatalf("membership moved to sku: %s", msg)
	}
}

func TestDeckCatalogConfigurable(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `DELETE FROM decks WHERE code<>'rider-waite'`)
		_, _ = store.Exec(context.Background(), `UPDATE decks SET name='莱德韦特', thumbnail='', description='', tags='', enabled=true, sort_order=0 WHERE code='rider-waite'`)
	})
	if _, err := store.Exec(t.Context(), `DELETE FROM decks WHERE code<>'rider-waite'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(t.Context(), `UPDATE decks SET name='莱德韦特', thumbnail='', description='', tags='', enabled=true, sort_order=0 WHERE code='rider-waite'`); err != nil {
		t.Fatal(err)
	}

	token, _ := registerUser(t, ts, store, "deckcfg")
	pref := listOne(t, ts, token, "/api/app/v1/deck-preference")
	avail, _ := pref["available"].([]any)
	if pref["code"] != "rider-waite" || len(avail) != 1 {
		t.Fatalf("seed deck: %#v", pref)
	}
	item := avail[0].(map[string]any)
	if item["name"] != "莱德韦特" || item["thumbnail"] != defaultCardThumbnailPath || item["available"] != true {
		t.Fatalf("seed preview: %#v", item)
	}
	if item["description"] != "" {
		t.Fatalf("invented description: %#v", item)
	}
	if tags, _ := item["tags"].([]any); len(tags) != 0 {
		t.Fatalf("invented tags: %#v", item)
	}
	msg, _ := call(t, ts, token, http.MethodPut, "/api/app/v1/deck-preference", map[string]any{"code": "marseille"})
	if !strings.Contains(msg, "尚未开放") {
		t.Fatalf("unknown deck: %s", msg)
	}

	catalog := listOne(t, ts, admin.access, "/api/admin/v1/copy/decks")
	if items, _ := catalog["items"].([]any); len(items) != 1 {
		t.Fatalf("admin decks: %#v", catalog)
	}
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/decks", map[string]any{
		"code": "test-deck", "name": "测试牌面", "enabled": true,
	})
	must(t, ts, token, http.MethodPut, "/api/app/v1/deck-preference", map[string]any{"code": "test-deck"})
	pref = listOne(t, ts, token, "/api/app/v1/deck-preference")
	if pref["code"] != "test-deck" || len(pref["available"].([]any)) != 2 {
		t.Fatalf("switched: %#v", pref)
	}

	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/decks", map[string]any{
		"code": "test-deck", "thumbnail": "https://cdn.example.test/deck.png",
		"description": "简约神秘的原创牌面，适合日常占卜与深度提问。",
		"tags":        []string{"简约大气", "神秘平衡", "通用百搭"},
	})
	pref = listOne(t, ts, token, "/api/app/v1/deck-preference")
	found := false
	for _, raw := range pref["available"].([]any) {
		row := raw.(map[string]any)
		if row["code"] == "test-deck" {
			found = true
			if row["thumbnail"] != "https://cdn.example.test/deck.png" {
				t.Fatalf("deck thumb: %#v", row)
			}
			if row["description"] != "简约神秘的原创牌面，适合日常占卜与深度提问。" {
				t.Fatalf("deck description: %#v", row)
			}
			tags, _ := row["tags"].([]any)
			if len(tags) != 3 || tags[0] != "简约大气" || tags[2] != "通用百搭" {
				t.Fatalf("deck tags: %#v", row)
			}
		}
	}
	if !found {
		t.Fatalf("missing test-deck: %#v", pref)
	}
	adminRow := copyDeckByCode(listOne(t, ts, admin.access, "/api/admin/v1/copy/decks")["items"], "test-deck")
	if adminRow["description"] != "简约神秘的原创牌面，适合日常占卜与深度提问。" {
		t.Fatalf("admin description: %#v", adminRow)
	}

	msg, _ = call(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/decks", map[string]any{
		"code": "test-deck", "tags": []string{"一", "二", "三", "四", "五", "六", "七", "八", "九"},
	})
	if !strings.Contains(msg, "标签过多") {
		t.Fatalf("too many tags: %s", msg)
	}

	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/decks", map[string]any{
		"code": "rider-waite", "enabled": false,
	})
	msg, _ = call(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/decks", map[string]any{
		"code": "test-deck", "enabled": false,
	})
	if !strings.Contains(msg, "至少保留") {
		t.Fatalf("last deck: %s", msg)
	}
}

func copyDeckByCode(raw any, code string) map[string]any {
	items, _ := raw.([]any)
	for _, item := range items {
		row, _ := item.(map[string]any)
		if row["code"] == code {
			return row
		}
	}
	return map[string]any{}
}

func copyCardByName(raw any, name string) map[string]any {
	items, _ := raw.([]any)
	for _, item := range items {
		row, _ := item.(map[string]any)
		if row["card_name"] == name {
			return row
		}
	}
	return map[string]any{}
}

func TestResolveCardThumbnail(t *testing.T) {
	if got := resolveCardThumbnail(nil, "愚者"); got != defaultCardThumbnailPath {
		t.Fatalf("built-in default: %#v", got)
	}
	if got := resolveCardThumbnail(map[string]string{thumbnailDefaultKey: "https://cdn.example.test/default.png"}, "愚者"); got != "https://cdn.example.test/default.png" {
		t.Fatalf("configured default: %#v", got)
	}
	cfg := map[string]string{thumbnailTemplateKey: "https://cdn.example.test/rw/{id}.png"}
	if got := resolveCardThumbnail(cfg, "愚者"); got != "https://cdn.example.test/rw/0.png" {
		t.Fatalf("template: %#v", got)
	}
	if got := resolveCardThumbnail(cfg, "魔术师"); got != "https://cdn.example.test/rw/1.png" {
		t.Fatalf("magician: %#v", got)
	}
	cfg[cardThumbnailKey("愚者")] = "https://cdn.example.test/fool.png"
	if got := resolveCardThumbnail(cfg, "愚者"); got != "https://cdn.example.test/fool.png" {
		t.Fatalf("override: %#v", got)
	}
	if got := resolveCardThumbnail(cfg, ""); got != nil {
		t.Fatalf("blank name: %#v", got)
	}
}

func TestDefaultCardThumbnailFile(t *testing.T) {
	_, ts := newMergeServer(t)
	resp, err := http.Get(ts.URL + defaultCardThumbnailPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("default card: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	buf := make([]byte, 8)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatal(err)
	}
	if string(buf[:4]) != "\x89PNG" {
		t.Fatalf("not png: %x", buf)
	}
}
