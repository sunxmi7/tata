package tata

import (
	"net/http"
	"strings"
	"testing"
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
	token, _ := registerUser(t, ts, store, "nick")
	profile := listOne(t, ts, token, "/api/app/v1/profile")
	name, _ := profile["name"].(string)
	if utf8.RuneCountInString(name) < 4 || utf8.RuneCountInString(name) > 6 || !validGeneratedNickname(name) {
		t.Fatalf("nickname: %#v", name)
	}
	if profile["nickname"] != name {
		t.Fatalf("nickname alias: %#v", profile)
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
}
