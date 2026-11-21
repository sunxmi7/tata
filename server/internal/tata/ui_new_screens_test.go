package tata

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestUINewScreensAPIs(t *testing.T) {
	store, ts := newMergeServer(t)
	_, _ = store.Exec(t.Context(), `DELETE FROM app_config WHERE key LIKE 'legal.%'`)
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `DELETE FROM app_config WHERE key LIKE 'legal.%'`)
	})

	legal := listOne(t, ts, "", "/api/app/v1/legal")
	agree, _ := legal["user_agreement"].(map[string]any)
	privacy, _ := legal["privacy_policy"].(map[string]any)
	if agree["title"] != "用户协议" || agree["body_status"] != "pending" || privacy["title"] != "隐私政策" {
		t.Fatalf("legal pending: %#v", legal)
	}

	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/copy/legal", map[string]any{
		"user_agreement": "协议正文。", "privacy_policy": "隐私正文。",
	})
	legal = listOne(t, ts, "", "/api/app/v1/legal")
	agree, _ = legal["user_agreement"].(map[string]any)
	if agree["body"] != "协议正文。" || agree["body_status"] != "ready" {
		t.Fatalf("legal ready: %#v", legal)
	}

	inviter, inviterID := registerUser(t, ts, store, "inviter")
	must(t, ts, inviter, http.MethodPut, "/api/app/v1/profile", map[string]any{
		"name": "小星星", "gender": "女", "birth_date": "2000-05-20",
		"explore_preferences": []string{"感情与关系", "自我成长", "情绪与疗愈"},
	})
	profile := listOne(t, ts, inviter, "/api/app/v1/profile")
	if profile["gender"] != "女性" || profile["birth_date"] != "2000-05-20" {
		t.Fatalf("profile gender alias: %#v", profile)
	}
	must(t, ts, inviter, http.MethodPut, "/api/app/v1/profile", map[string]any{"gender": "其他"})
	profile = listOne(t, ts, inviter, "/api/app/v1/profile")
	if profile["gender"] != "其他" {
		t.Fatalf("profile gender other: %#v", profile)
	}
	must(t, ts, inviter, http.MethodPut, "/api/app/v1/profile", map[string]any{"nickname": "塔塔昵称"})
	profile = listOne(t, ts, inviter, "/api/app/v1/profile")
	if profile["name"] != "塔塔昵称" || profile["nickname"] != "塔塔昵称" {
		t.Fatalf("profile nickname alias: %#v", profile)
	}
	prefs, _ := profile["explore_preferences"].([]any)
	if len(prefs) != 3 || prefs[0] != "感情与关系" {
		t.Fatalf("explore prefs: %#v", profile["explore_preferences"])
	}

	invite := listOne(t, ts, inviter, "/api/app/v1/invites")
	code, _ := invite["code"].(string)
	if code == "" || invite["reward_amount"] != float64(3) || invite["invited_count"] != float64(0) {
		t.Fatalf("invite: %#v", invite)
	}

	inviteeEmail := uniqueEmail("invitee")
	otp := sendEmailCode(t, ts, "", inviteeEmail, "login")
	inviteeSession := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{
		"email": inviteeEmail, "code": otp, "invite_code": code,
	})
	inviteeToken, _ := inviteeSession["accessToken"].(string)
	if inviteeToken == "" {
		t.Fatalf("invitee login: %#v", inviteeSession)
	}
	inviteeNick, _ := listOne(t, ts, inviteeToken, "/api/app/v1/profile")["name"].(string)
	must(t, ts, inviteeToken, http.MethodPut, "/api/app/v1/profile", map[string]any{"name": "新朋友甲"})

	rewards := listOne(t, ts, inviter, "/api/app/v1/campaigns/rewards")
	items, _ := rewards["items"].([]any)
	if len(items) != 1 || rewards["fields_confirmed"] != true {
		t.Fatalf("reward items: %#v", rewards)
	}
	row := items[0].(map[string]any)
	if row["kind"] != "friend_invite" || row["amount"] != float64(3) || row["amount_label"] != "+3 次" {
		t.Fatalf("reward row: %#v", row)
	}
	if row["title"] != fmt.Sprintf("好友 %s 通过你的邀请加入了塔塔", inviteeNick) {
		t.Fatalf("reward title: %#v want %s", row["title"], inviteeNick)
	}
	invite = listOne(t, ts, inviter, "/api/app/v1/invites")
	if invite["invited_count"] != float64(1) {
		t.Fatalf("invited count: %#v", invite)
	}
	var granted int
	if err := store.QueryRow(t.Context(), `SELECT invite_remaining FROM rewards WHERE user_id=$1`, inviterID).Scan(&granted); err != nil {
		t.Fatal(err)
	}
	if granted != 3 {
		t.Fatalf("inviter invite remaining %d", granted)
	}

	msg, _ := call(t, ts, inviteeToken, http.MethodPost, "/api/app/v1/invites/accept", map[string]any{"code": code})
	if msg != "已经绑定过邀请人" {
		t.Fatalf("double accept: %s", msg)
	}

	cards := listOne(t, ts, inviter, "/api/app/v1/cards")
	cardItems, _ := cards["items"].([]any)
	groups, _ := cards["groups"].([]any)
	if len(cardItems) != 78 || len(groups) != 5 {
		t.Fatalf("cards catalog: items=%d groups=%d", len(cardItems), len(groups))
	}
	deck, _ := cards["deck"].(map[string]any)
	if deck["code"] != "rider-waite" {
		t.Fatalf("default deck: %#v", deck)
	}

	created := create(t, ts, inviter, "/api/app/v1/custom-decks", map[string]any{
		"name": "我的月影牌", "face_source": "rider-waite", "status": "saved",
	})
	if created["name"] != "我的月影牌" || created["status"] != "saved" || created["uploaded_count"] != float64(78) {
		t.Fatalf("create custom: %#v", created)
	}
	customID := fmt.Sprintf("%.0f", created["id"])
	must(t, ts, inviter, http.MethodPost, "/api/app/v1/custom-decks/"+customID+"/use", map[string]any{})
	pref := listOne(t, ts, inviter, "/api/app/v1/deck-preference")
	if pref["code"] != "custom:"+customID {
		t.Fatalf("use custom: %#v", pref)
	}

	putOne(t, ts, inviter, "/api/app/v1/custom-decks/"+customID, map[string]any{
		"face_source": "upload", "status": "saved",
		"faces": []map[string]any{{"card_id": 0, "image_url": "https://cdn.example.test/fool.png"}},
	})
	detail := listOne(t, ts, inviter, "/api/app/v1/custom-decks/"+customID)
	if detail["uploaded_count"] != float64(1) {
		t.Fatalf("uploaded custom: %#v", detail)
	}
	customCards := listOne(t, ts, inviter, "/api/app/v1/cards?deck=custom:"+customID)
	first := customCards["items"].([]any)[0].(map[string]any)
	if first["uploaded"] != true {
		t.Fatalf("custom card overlay: %#v", first)
	}

	share := listOne(t, ts, inviter, "/api/app/v1/yearly-reports/2027/share")
	if share["year"] != float64(2027) || share["brand"] != "塔塔 Tarot" || share["share_hint"] == nil {
		t.Fatalf("yearly share: %#v", share)
	}

	spreads := list(t, ts, inviter, "/api/app/v1/spreads")
	if len(spreads) == 0 {
		t.Fatal("spreads empty")
	}
	if _, ok := spreads[0]["theme_key"]; !ok {
		t.Fatalf("spread missing theme: %#v", spreads[0])
	}
	if _, ok := spreads[0]["short_sentence"]; !ok {
		t.Fatalf("spread missing short_sentence: %#v", spreads[0])
	}
}
