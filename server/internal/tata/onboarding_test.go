package tata

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestOnboardingGuideFollowsDeviceTrialFollowsAccount(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "guideacct")
	deviceID := fmt.Sprintf("device-guide-%d", time.Now().UnixNano())
	must(t, ts, token, http.MethodPut, "/api/app/v1/onboarding", map[string]any{
		"device_id": deviceID, "guide_completed": true,
	})
	guide := listOne(t, ts, token, "/api/app/v1/onboarding?device_id="+deviceID)
	if guide["show_guide"] != false || guide["reading_trial_available"] != true {
		t.Fatalf("logged-in account still has the starter reading: %#v", guide)
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "第一次解读", "type": "normal", "device_id": deviceID})
	rid := asID(reading["id"])
	var storedDevice string
	if err := store.QueryRow(t.Context(), `SELECT device_id FROM readings WHERE id=$1`, rid).Scan(&storedDevice); err != nil {
		t.Fatal(err)
	}
	if storedDevice != deviceID {
		t.Fatalf("device bind: %s", storedDevice)
	}
	spreadID := ensureTimeFlow(t, store)
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", rid), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	var used bool
	if err := store.QueryRow(t.Context(), `SELECT reading_trial_used FROM app_users WHERE id=$1`, uid).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used {
		t.Fatal("spread select consumed a free reading")
	}

	nextDevice := deviceID + "-next"
	again := listOne(t, ts, token, "/api/app/v1/onboarding?device_id="+nextDevice)
	if again["show_guide"] != true || again["reading_trial_available"] != true {
		t.Fatalf("old account new device: %#v", again)
	}

	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	listed := listOne(t, ts, admin.access, "/api/admin/v1/onboarding")
	found := false
	for _, raw := range listed["items"].([]any) {
		row, _ := raw.(map[string]any)
		if row["device_id"] == deviceID && row["guide_completed"] == true {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin onboarding: %#v", listed)
	}
}

func TestAnonymousReadingFollowsDevice(t *testing.T) {
	store, ts := newMergeServer(t)
	deviceID := fmt.Sprintf("device-anon-%d", time.Now().UnixNano())
	anon := create(t, ts, "", "/api/app/v1/auth/anonymous", map[string]any{"device_id": deviceID})
	anonToken, _ := anon["accessToken"].(string)
	reading := create(t, ts, anonToken, "/api/app/v1/readings", map[string]any{
		"question": "登录前的问题", "type": "normal", "device_id": deviceID,
	})
	rid := asID(reading["id"])
	email := uniqueEmail("devbind")
	otp := sendEmailCode(t, ts, "", email, "login")
	session := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{
		"email": email, "code": otp, "device_id": deviceID,
	})
	uid := asID(session["id"])
	var owner int64
	if err := store.QueryRow(t.Context(), `SELECT user_id FROM readings WHERE id=$1`, rid).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != uid {
		t.Fatalf("device reading owner=%d want %d", owner, uid)
	}
}

func TestDeletedDailyKeepsTheDay(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "dailydel")
	seedGift(t, store, uid, 1)
	first := create(t, ts, token, "/api/app/v1/daily", map[string]any{"target": "today"})
	must(t, ts, token, http.MethodDelete, fmt.Sprintf("/api/app/v1/readings/%d", asID(first["id"])), nil)
	msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/daily", map[string]any{"target": "today"})
	if !strings.Contains(msg, "删除记录后不能再抽") {
		t.Fatalf("deleted daily: %s", msg)
	}
}

func TestOnboardChargesOnSuccessAndAbandonExpires(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "onboardok")
	deviceID := fmt.Sprintf("device-onboard-%d", time.Now().UnixNano())
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "第一次解读", "type": "normal", "device_id": deviceID})
	rid := asID(reading["id"])
	spreadID := ensureTimeFlow(t, store)
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", rid), map[string]any{"spread_id": spreadID, "draw_mode": "normal"})
	var used bool
	var charges int
	if err := store.QueryRow(t.Context(), `SELECT reading_trial_used FROM app_users WHERE id=$1`, uid).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_charges WHERE reading_id=$1`, rid).Scan(&charges); err != nil {
		t.Fatal(err)
	}
	if used || charges != 0 {
		t.Fatalf("spread consumed the starter reading used=%v charges=%d", used, charges)
	}
	if _, err := store.Exec(t.Context(), `UPDATE readings SET status='COMPLETED', interpretation='ok', cards='[]' WHERE id=$1`, rid); err != nil {
		t.Fatal(err)
	}
	if err := (&App{Store: store}).consumeOnboardIfReady(t.Context(), rid); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRow(t.Context(), `SELECT reading_trial_used FROM app_users WHERE id=$1`, uid).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Fatal("success did not consume the starter reading")
	}
	msg, _ := call(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/followups", rid), map[string]any{"text": "再问一句", "continuation": true})
	if !strings.Contains(msg, "不能追问") {
		t.Fatalf("starter followup: %s", msg)
	}

	other, otherID := registerUser(t, ts, store, "onboarddrop")
	emptyDevice := deviceID + "-empty"
	empty := create(t, ts, other, "/api/app/v1/readings", map[string]any{"question": "   ", "type": "normal", "device_id": emptyDevice})
	must(t, ts, other, http.MethodPost, "/api/app/v1/onboarding/abandon", map[string]any{"device_id": emptyDevice})
	var deleted *time.Time
	if err := store.QueryRow(t.Context(), `SELECT deleted_at FROM readings WHERE id=$1`, asID(empty["id"])).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if deleted == nil {
		t.Fatal("empty question stayed in history")
	}
	if err := store.QueryRow(t.Context(), `SELECT reading_trial_used FROM app_users WHERE id=$1`, otherID).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if !used {
		t.Fatal("abandon kept the starter entitlement")
	}
	synced := create(t, ts, token, "/api/app/v1/onboarding/sync", map[string]any{
		"device_id": deviceID + "-sync", "question": "本地没传完的问题", "status": "QUESTION_CREATED",
	})
	var onboard bool
	var question string
	if err := store.QueryRow(t.Context(), `SELECT onboard_experience, question FROM readings WHERE id=$1`, asID(synced["id"])).Scan(&onboard, &question); err != nil {
		t.Fatal(err)
	}
	if question != "本地没传完的问题" || onboard {
		t.Fatalf("sync after the account already used the starter reading onboard=%v question=%s", onboard, question)
	}
}

func TestInviteLinkAttributesOnLogin(t *testing.T) {
	store, ts := newMergeServer(t)
	inviterToken, inviterID := registerUser(t, ts, store, "inviter")
	view := listOne(t, ts, inviterToken, "/api/app/v1/invites")
	if view["link"] != "/invite/"+fmt.Sprint(inviterID) {
		t.Fatalf("link: %#v", view["link"])
	}
	deviceID := fmt.Sprintf("device-invite-%d", time.Now().UnixNano())
	must(t, ts, "", http.MethodPost, "/api/app/v1/invites/open", map[string]any{"device_id": deviceID, "code": fmt.Sprint(inviterID)})
	email := uniqueEmail("invited")
	otp := sendEmailCode(t, ts, "", email, "login")
	session := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{"email": email, "code": otp, "device_id": deviceID})
	var source string
	if err := store.QueryRow(t.Context(), `SELECT source FROM invite_rewards WHERE invitee_id=$1`, asID(session["id"])).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "link" {
		t.Fatalf("source: %s", source)
	}
}

func TestRecommendUsesConfiguredSpread(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "recommend")
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "这份工作还要不要继续", "type": "normal"})
	rid := asID(reading["id"])
	payload := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/recommend", rid), map[string]any{})
	spread, _ := payload["recommended_spread"].(map[string]any)
	if payload["match_status"] != "RECOMMENDED" || spread["name"] != "时间流" || payload["mapping_status"] != "unconfigured" {
		t.Fatalf("recommend: %#v", payload)
	}
}
