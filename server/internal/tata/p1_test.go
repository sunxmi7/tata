package tata

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAppP1RedeemRecordsCampaignsHelp(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "p1")
	other, _ := registerUser(t, ts, store, "p1b")

	if msg, _ := call(t, ts, "", http.MethodGet, "/api/app/v1/campaigns", nil); msg != "401" {
		t.Fatalf("campaigns no token: %s", msg)
	}
	anon := create(t, ts, "", "/api/app/v1/auth/anonymous", map[string]any{})
	anonToken, _ := anon["accessToken"].(string)
	if anonToken == "" {
		t.Fatal("anonymous token missing")
	}
	if msg, _ := call(t, ts, anonToken, http.MethodGet, "/api/app/v1/redeem/records", nil); msg != "登录后才能查看兑换记录" {
		t.Fatalf("anon redeem records: %s", msg)
	}

	empty := listOne(t, ts, token, "/api/app/v1/redeem/records")
	items, ok := empty["items"].([]any)
	if !ok || len(items) != 0 || empty["empty_title"] != "暂无兑换记录" || empty["fields_confirmed"] != false {
		t.Fatalf("empty redeem records: %#v", empty)
	}
	if !strings.Contains(fmt.Sprint(empty["placeholder_note"]), "记录字段待产品确认") {
		t.Fatalf("redeem note: %#v", empty["placeholder_note"])
	}

	code := fmt.Sprintf("P1%d", time.Now().UnixNano())
	if _, err := store.Exec(t.Context(), `INSERT INTO redeem_codes(code, kind, amount, benefit_name, max_redemptions, enabled)
		VALUES ($1,'diamonds',60,'新春钻石礼',1,true)`, strings.ToUpper(code)); err != nil {
		t.Fatal(err)
	}
	create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": code})
	freeCode := fmt.Sprintf("P1F%d", time.Now().UnixNano())
	if _, err := store.Exec(t.Context(), `INSERT INTO redeem_codes(code, kind, amount, benefit_name, enabled)
		VALUES ($1,'free_count',1,'免费一次',true)`, strings.ToUpper(freeCode)); err != nil {
		t.Fatal(err)
	}
	create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": freeCode})

	got := listOne(t, ts, token, "/api/app/v1/redeem/records")
	rows, _ := got["items"].([]any)
	if len(rows) != 2 || got["fields_confirmed"] != false {
		t.Fatalf("redeem records: %#v", got)
	}
	byKind := map[string]map[string]any{}
	for _, raw := range rows {
		row := raw.(map[string]any)
		byKind[row["kind"].(string)] = row
		if _, hasPrice := row["price"]; hasPrice {
			t.Fatalf("invented price on redeem row: %#v", row)
		}
	}
	diamonds := byKind["diamonds"]
	if diamonds["benefit_name"] != "新春钻石礼" || diamonds["amount"].(float64) != 60 || diamonds["unit"] != "diamond" || diamonds["display_label"] != displayLabelTataCoin {
		t.Fatalf("diamond record: %#v", diamonds)
	}
	if diamonds["created_at"] == "" || diamonds["code"] == "" {
		t.Fatalf("diamond record identity: %#v", diamonds)
	}
	free := byKind["free_count"]
	if free["unit"] != "count" || free["display_label"] != nil || free["amount"].(float64) != 1 {
		t.Fatalf("free record: %#v", free)
	}
	otherEmpty := listOne(t, ts, other, "/api/app/v1/redeem/records")
	if otherItems, _ := otherEmpty["items"].([]any); len(otherItems) != 0 {
		t.Fatalf("other user records: %#v", otherEmpty)
	}

	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `UPDATE campaigns SET status='ongoing', enabled=true WHERE campaign_id IN ('friend_invite','membership_benefit')`)
	})

	list := listOne(t, ts, anonToken, "/api/app/v1/campaigns")
	cards, _ := list["items"].([]any)
	if len(cards) < 2 || list["empty_title"] != "暂时没有可参与的活动" || list["footnote"] != "活动规则与奖励以活动详情页为准" {
		t.Fatalf("campaign list: %#v", list)
	}
	first := cards[0].(map[string]any)
	second := cards[1].(map[string]any)
	if first["id"] != "friend_invite" || first["title"] != "邀请好友，一起探索" || first["subtitle"] != "分享塔塔，查看邀请进度与奖励" {
		t.Fatalf("invite card: %#v", first)
	}
	if first["status"] != "ongoing" || first["badge"] != "好友邀请" || first["entry_label"] != "查看活动" || first["entry_enabled"] != true {
		t.Fatalf("invite entry: %#v", first)
	}
	if second["id"] != "membership_benefit" || second["title"] != "参与活动，领取会员体验" || second["badge"] != "" || second["entry_label"] != "查看活动" {
		t.Fatalf("membership card: %#v", second)
	}

	if _, err := store.Exec(t.Context(), `UPDATE campaigns SET status='ended' WHERE campaign_id='friend_invite'`); err != nil {
		t.Fatal(err)
	}
	list = listOne(t, ts, token, "/api/app/v1/campaigns")
	cards, _ = list["items"].([]any)
	first = cards[0].(map[string]any)
	second = cards[1].(map[string]any)
	if first["status"] != "ended" || first["badge"] != "已结束" || first["entry_label"] != "活动已结束" || first["entry_enabled"] != false {
		t.Fatalf("ended card: %#v", first)
	}
	if second["status"] != "ongoing" || second["entry_label"] != "查看活动" {
		t.Fatalf("live card after one ended: %#v", second)
	}

	detail := listOne(t, ts, anonToken, "/api/app/v1/campaigns/friend_invite")
	if detail["title"] != "好友邀请" || detail["body_status"] != "pending" || detail["detail_delivered"] != false {
		t.Fatalf("invite detail: %#v", detail)
	}
	body := fmt.Sprint(detail["body"])
	if body != "详情页不在本次交付范围。奖励、条件与有效期以活动配置为准。" {
		t.Fatalf("invite body: %s", body)
	}
	member := listOne(t, ts, token, "/api/app/v1/campaigns/membership_benefit")
	if member["title"] != "会员福利" || member["body"] != "详情页不在本次交付范围。不预写未确认的会员时长或领取门槛。" {
		t.Fatalf("membership detail: %#v", member)
	}
	if _, ok := member["reward_amount"]; ok {
		t.Fatalf("invented reward: %#v", member)
	}
	if msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/campaigns/not_a_campaign", nil); msg != "活动不存在" {
		t.Fatalf("missing campaign: %s", msg)
	}

	if _, err := store.Exec(t.Context(), `UPDATE campaigns SET enabled=false`); err != nil {
		t.Fatal(err)
	}
	list = listOne(t, ts, token, "/api/app/v1/campaigns")
	cards, _ = list["items"].([]any)
	if len(cards) != 0 || list["empty_title"] != "暂时没有可参与的活动" || list["empty_hint"] != "稍后再来看看" {
		t.Fatalf("empty campaigns: %#v", list)
	}
	if msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/campaigns/friend_invite", nil); msg != "活动不存在" {
		t.Fatalf("disabled campaign: %s", msg)
	}

	if msg, _ := call(t, ts, anonToken, http.MethodGet, "/api/app/v1/campaigns/participations", nil); msg != "登录后才能查看参与记录" {
		t.Fatalf("anon participations: %s", msg)
	}
	part := listOne(t, ts, token, "/api/app/v1/campaigns/participations")
	partItems, _ := part["items"].([]any)
	if len(partItems) != 0 || part["fields_confirmed"] != false || part["empty_title"] != "暂无参与记录" {
		t.Fatalf("participations: %#v", part)
	}
	if !strings.Contains(fmt.Sprint(part["placeholder_note"]), "不展示伪造记录") {
		t.Fatalf("participation note: %#v", part)
	}
	rewards := listOne(t, ts, token, "/api/app/v1/campaigns/rewards")
	rewardItems, _ := rewards["items"].([]any)
	if len(rewardItems) != 0 || rewards["empty_title"] != "暂无奖励记录" || rewards["fields_confirmed"] != false {
		t.Fatalf("rewards: %#v", rewards)
	}
	if strings.Contains(fmt.Sprint(rewards["placeholder_note"]), "28.8") {
		t.Fatalf("reward promise: %#v", rewards)
	}

	cats := listOne(t, ts, anonToken, "/api/app/v1/help/categories")
	categories, _ := cats["categories"].([]any)
	if len(categories) != 3 {
		t.Fatalf("categories: %#v", cats)
	}
	wantTitles := []string{"使用与占卜", "会员与支付", "账号与设置"}
	for i, title := range wantTitles {
		row := categories[i].(map[string]any)
		if row["title"] != title {
			t.Fatalf("category %d: %#v", i, row)
		}
	}
	pay := categories[1].(map[string]any)
	if pay["id"] != "membership_pay" || pay["articles_status"] != "pending" || pay["empty_note"] != "这一类的问题还在整理，条目待确认。" {
		t.Fatalf("pay category: %#v", pay)
	}

	reading := listOne(t, ts, token, "/api/app/v1/help/articles?category=reading")
	readingItems, _ := reading["items"].([]any)
	if len(readingItems) != 5 || reading["category_note"] != "" {
		t.Fatalf("reading articles: %#v", reading)
	}
	if readingItems[0].(map[string]any)["title"] != "如何开始一次塔罗占卜？" {
		t.Fatalf("first article: %#v", readingItems[0])
	}
	account := listOne(t, ts, token, "/api/app/v1/help/articles?category=account")
	accountItems, _ := account["items"].([]any)
	if len(accountItems) != 1 || accountItems[0].(map[string]any)["title"] != "如何切换语言和主题？" {
		t.Fatalf("account articles: %#v", account)
	}
	pending := listOne(t, ts, token, "/api/app/v1/help/articles?category=membership_pay")
	pendingItems, _ := pending["items"].([]any)
	if len(pendingItems) != 0 || pending["category_note"] != "这一类的问题还在整理，条目待确认。" || pending["empty_title"] != "" {
		t.Fatalf("pending category: %#v", pending)
	}

	searchPath := "/api/app/v1/help/articles?" + url.Values{"q": {"占卜"}, "category": {"reading"}}.Encode()
	found := listOne(t, ts, token, searchPath)
	foundItems, _ := found["items"].([]any)
	if len(foundItems) != 2 || found["query"] != "占卜" {
		t.Fatalf("search 占卜: %#v", found)
	}
	for _, raw := range foundItems {
		row := raw.(map[string]any)
		if row["category_title"] != "使用与占卜" || !strings.Contains(fmt.Sprint(row["title"]), "占卜") {
			t.Fatalf("search row: %#v", row)
		}
		if _, hasBody := row["body"]; hasBody {
			t.Fatalf("list leaked body: %#v", row)
		}
	}
	nonePath := "/api/app/v1/help/articles?" + url.Values{"q": {"星座"}}.Encode()
	none := listOne(t, ts, token, nonePath)
	noneItems, _ := none["items"].([]any)
	if len(noneItems) != 0 || none["empty_title"] != "暂时没有找到相关问题" || none["empty_hint"] != "换个关键词，或向我们反馈" {
		t.Fatalf("search empty: %#v", none)
	}
	if msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/help/articles?category=nope", nil); msg != "问题分类不正确" {
		t.Fatalf("bad category: %s", msg)
	}

	article := listOne(t, ts, anonToken, "/api/app/v1/help/articles/how_to_start")
	if article["title"] != "如何开始一次塔罗占卜？" || article["body"] != "待确认" || article["body_status"] != "pending" {
		t.Fatalf("article: %#v", article)
	}
	if article["category_title"] != "使用与占卜" {
		t.Fatalf("article category: %#v", article)
	}
	if strings.Contains(fmt.Sprint(article["body"]), "退款") || strings.Contains(fmt.Sprint(article["body"]), "额度") {
		t.Fatalf("article invented rules: %#v", article)
	}
	if msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/help/articles/missing_article", nil); msg != "问题不存在" {
		t.Fatalf("missing article: %s", msg)
	}
}
