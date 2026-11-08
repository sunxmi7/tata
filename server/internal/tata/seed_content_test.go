package tata

import (
	"testing"
)

func TestPresetQuestionsAndCardThumbs(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "presetq")
	data := listOne(t, ts, token, "/api/app/v1/preset-questions")
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
	if _, ok := first["category"]; ok {
		t.Fatalf("featured should not show category: %#v", first)
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
