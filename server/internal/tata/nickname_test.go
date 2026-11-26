package tata

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRandomNicknameRules(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		nick := randomNickname()
		if !validGeneratedNickname(nick) {
			t.Fatalf("invalid nickname %q", nick)
		}
		seen[nick] = true
	}
	if len(seen) < 8 {
		t.Fatalf("too few distinct nicknames: %d", len(seen))
	}
}

func TestNicknameFromEmailLocal(t *testing.T) {
	if got := nicknameFromEmailLocal("Demo.User+tag@tata.local"); got != "demousertag" {
		t.Fatalf("sanitize: %q", got)
	}
	if got := nicknameFromEmailLocal("alice_bob-1@tata.local"); got != "alice_bob-1" {
		t.Fatalf("keep _-: %q", got)
	}
	if got := nicknameFromEmailLocal("!!!@tata.local"); got != "" {
		t.Fatalf("empty local: %q", got)
	}
	if got := nicknameFromEmailLocal("@tata.local"); got != "" {
		t.Fatalf("missing local: %q", got)
	}
	long := strings.Repeat("a", 40) + "@tata.local"
	if got := nicknameFromEmailLocal(long); utf8.RuneCountInString(got) != 32 {
		t.Fatalf("truncate: %q len=%d", got, utf8.RuneCountInString(got))
	}
}

func TestEmailProfileDefaults(t *testing.T) {
	store, ts := newMergeServer(t)
	email := fmt.Sprintf("nick.user+%d@tata.local", time.Now().UnixNano())
	otp := sendEmailCode(t, ts, "", email, "login")
	session := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{"email": email, "code": otp})
	token := session["accessToken"].(string)
	profile := listOne(t, ts, token, "/api/app/v1/profile")
	name, _ := profile["name"].(string)
	want := nicknameFromEmailLocal(email)
	if name != want || profile["nickname"] != name {
		t.Fatalf("email prefix nickname: got %#v want %q", profile, want)
	}
	avatar, _ := profile["avatar_url"].(string)
	if !strings.Contains(avatar, defaultAvatarName) {
		t.Fatalf("default avatar: %#v", avatar)
	}

	res, err := http.Get(ts.URL + defaultAvatarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("serve default avatar: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}

	must(t, ts, token, http.MethodPut, "/api/app/v1/profile", map[string]any{
		"nickname": "自己取名", "avatar_url": "https://cdn.example/me.png",
	})
	updated := listOne(t, ts, token, "/api/app/v1/profile")
	if updated["name"] != "自己取名" || updated["avatar_url"] != "https://cdn.example/me.png" {
		t.Fatalf("replace defaults: %#v", updated)
	}

	// ensureProfileDefaults must not overwrite an existing nickname.
	var uid int64
	if err := store.QueryRow(t.Context(), `SELECT id FROM app_users WHERE lower(email)=lower($1)`, email).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	testApp(store).ensureProfileDefaults(t.Context(), uid)
	kept := listOne(t, ts, token, "/api/app/v1/profile")
	if kept["name"] != "自己取名" {
		t.Fatalf("defaults overwrote nickname: %#v", kept)
	}
}
