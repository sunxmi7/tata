package tata

import (
	"net/http"
	"testing"
)

func TestPresetQuestionsAndCardThumbs(t *testing.T) {
	_, ts := newMergeServer(t)
	data := listOne(t, ts, "", "/api/app/v1/preset-questions")
	featured, _ := data["featured"].([]any)
	backup, _ := data["backup"].([]any)
	if len(featured) != 15 {
		t.Fatalf("featured %d", len(featured))
	}
	if len(backup) != 36 {
		t.Fatalf("backup %d", len(backup))
	}
	if data["more_per_page"] != float64(5) {
		t.Fatalf("more_per_page %#v", data["more_per_page"])
	}
	first, _ := featured[0].(map[string]any)
	if first["body"] != "最近的我，最需要关注什么？" {
		t.Fatalf("first featured: %#v", first)
	}
	if first["scene"] != "growth" || first["category"] != "自我成长" {
		t.Fatalf("featured scene: %#v", first)
	}
	scenes, _ := data["scenes"].([]any)
	if len(scenes) != 6 {
		t.Fatalf("scenes %d %#v", len(scenes), data["scenes"])
	}
	love := listOne(t, ts, "", "/api/app/v1/preset-questions?scene=relationship")
	loveF, _ := love["featured"].([]any)
	loveB, _ := love["backup"].([]any)
	if len(loveF) != 3 || len(loveB) != 6 {
		t.Fatalf("relationship counts featured=%d backup=%d", len(loveF), len(loveB))
	}
	for _, raw := range append(append([]any{}, loveF...), loveB...) {
		row, _ := raw.(map[string]any)
		if row["scene"] != "relationship" {
			t.Fatalf("filtered row: %#v", row)
		}
	}
	career := listOne(t, ts, "", "/api/app/v1/preset-questions?scene=career")
	careerB, _ := career["backup"].([]any)
	if len(careerB) != 6 || careerB[0].(map[string]any)["scene"] != "career" {
		t.Fatalf("career filter: %#v", career)
	}
	msg, _ := call(t, ts, "", http.MethodGet, "/api/app/v1/preset-questions?scene=unknown", nil)
	if msg != "场景不正确" {
		t.Fatalf("bad scene: %s", msg)
	}
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	copyCards := listOne(t, ts, admin.access, "/api/admin/v1/copy/cards")
	items, _ := copyCards["items"].([]any)
	if len(items) != 78 {
		t.Fatalf("cards %d", len(items))
	}
	fool := copyCardByName(items, "愚者")
	if fool["thumbnail"] != "/uploads/cards/rider_waite_tarot_big_the_fool_00.jpg" {
		t.Fatalf("fool thumb: %#v", fool["thumbnail"])
	}
	empress := copyCardByName(items, "女皇")
	if empress["thumbnail"] != "/uploads/cards/rider_waite_tarot_big_the_empress_03.jpg" {
		t.Fatalf("empress mapped from 皇后: %#v", empress["thumbnail"])
	}
}

func TestSeedCardFileAllowed(t *testing.T) {
	if !validUploadPath("cards/rider_waite_tarot_big_the_fool_00.jpg") {
		t.Fatal("seed card path rejected")
	}
	if validUploadPath("cards/../secret.jpg") {
		t.Fatal("traversal allowed")
	}
}
