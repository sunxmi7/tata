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

func TestEmailProfileDefaults(t *testing.T) {
	store, ts := newMergeServer(t)
	email := fmt.Sprintf("nick.user+%d@tata.local", time.Now().UnixNano())
	otp := sendEmailCode(t, ts, "", email, "login")
	session := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{"email": email, "code": otp})
	token := session["accessToken"].(string)
	profile := listOne(t, ts, token, "/api/app/v1/profile")
	name, _ := profile["name"].(string)
	if name == "" || !validGeneratedNickname(name) || profile["nickname"] != name {
		t.Fatalf("email random nickname: %#v", profile)
	}
	if strings.HasPrefix(name, "Tarot") {
		t.Fatalf("must not use Tarot+email prefix: %q", name)
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

func TestEmailNicknamesVaryAcrossAccounts(t *testing.T) {
	store, ts := newMergeServer(t)
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		email := fmt.Sprintf("vary%d+%d@tata.local", i, time.Now().UnixNano())
		otp := sendEmailCode(t, ts, "", email, "login")
		session := create(t, ts, "", "/api/app/v1/auth/login", map[string]any{"email": email, "code": otp})
		profile := listOne(t, ts, session["accessToken"].(string), "/api/app/v1/profile")
		name, _ := profile["name"].(string)
		if !validGeneratedNickname(name) {
			t.Fatalf("invalid: %#v", profile)
		}
		seen[name] = true
		_ = store
	}
	if len(seen) < 3 {
		t.Fatalf("expected variety across accounts, got %d unique: %#v", len(seen), seen)
	}
}

func TestValidGeneratedNicknameRejectsBad(t *testing.T) {
	if validGeneratedNickname("塔") || validGeneratedNickname("这是七个汉字名") {
		t.Fatal("length")
	}
	if validGeneratedNickname("Tarot星") {
		t.Fatal("latin")
	}
	if utf8.RuneCountInString("星河旅人") != 4 {
		t.Fatal("fixture")
	}
}
