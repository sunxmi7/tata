package tata

import (
	"net/http"
	"testing"
)

func TestGetProfileWithBlankTimezone(t *testing.T) {
	store, ts := newMergeServer(t)
	token, uid := registerUser(t, ts, store, "tz")
	if _, err := store.Exec(t.Context(), `UPDATE app_users SET timezone='' WHERE id=$1`, uid); err != nil {
		t.Fatal(err)
	}
	profile := listOne(t, ts, token, "/api/app/v1/profile")
	if profile["timezone"] != "Asia/Shanghai" {
		t.Fatalf("timezone fallback: %#v", profile["timezone"])
	}
	if msg, _ := call(t, ts, token, http.MethodGet, "/api/app/v1/profile", nil); msg != "" {
		t.Fatalf("profile: %s", msg)
	}
}
