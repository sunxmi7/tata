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
	if guide["show_guide"] != false || guide["reading_trial_available"] != false {
		t.Fatalf("logged-in guide has no free reading: %#v", guide)
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
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", rid), map[string]any{"spread_id": spreadID})
	var used bool
	if err := store.QueryRow(t.Context(), `SELECT reading_trial_used FROM app_users WHERE id=$1`, uid).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used {
		t.Fatal("spread select consumed a free reading")
	}

	nextDevice := deviceID + "-next"
	again := listOne(t, ts, token, "/api/app/v1/onboarding?device_id="+nextDevice)
	if again["show_guide"] != true || again["reading_trial_available"] != false {
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
