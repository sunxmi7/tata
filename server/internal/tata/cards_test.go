package tata

import (
	"context"
	"net/http"
	"testing"
)

func TestAdminCardsCatalog(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})
	t.Cleanup(func() {
		_, _ = store.Exec(context.Background(), `DELETE FROM cards WHERE id >= 78 OR name LIKE '测试牌-%'`)
		_, _ = store.Exec(context.Background(), `UPDATE cards SET yi='' WHERE name='愚者'`)
		_, _ = store.Exec(context.Background(), `DELETE FROM app_config WHERE key='card.thumbnail_template' OR key='card.thumbnail_default'`)
		_ = store.seedCards(context.Background())
	})

	data := listOne(t, ts, admin.access, "/api/admin/v1/cards")
	items, _ := data["items"].([]any)
	if len(items) < 78 {
		t.Fatalf("cards %d", len(items))
	}
	fool := copyCardByName(items, "愚者")
	if fool["id"] != float64(0) || fool["card_name"] != "愚者" || fool["arcana"] != "major" {
		t.Fatalf("fool: %#v", fool)
	}

	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/cards/0", map[string]any{
		"yi": "换一件浅色外套", "name_en": "The Fool",
	})
	fool = copyCardByName(listOne(t, ts, admin.access, "/api/admin/v1/cards")["items"], "愚者")
	if fool["yi"] != "换一件浅色外套" || fool["name_en"] != "The Fool" {
		t.Fatalf("updated fool: %#v", fool)
	}

	msg, _ := call(t, ts, admin.access, http.MethodPut, "/api/admin/v1/cards/0", map[string]any{"name": "别的名字"})
	if msg != "不能修改牌名" {
		t.Fatalf("rename: %s", msg)
	}

	must(t, ts, admin.access, http.MethodPost, "/api/admin/v1/cards", map[string]any{
		"name": "测试牌-catalog", "name_en": "Test Card", "enabled": true,
	})
	created := copyCardByName(listOne(t, ts, admin.access, "/api/admin/v1/cards")["items"], "测试牌-catalog")
	if created["name"] != "测试牌-catalog" || created["name_en"] != "Test Card" {
		t.Fatalf("create: %#v", created)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/cards", map[string]any{"name": "愚者"})
	if msg != "牌名已存在" {
		t.Fatalf("dup: %s", msg)
	}

	msg, _ = call(t, ts, ro.access, http.MethodPut, "/api/admin/v1/cards/0", map[string]any{"yi": "只读"})
	if msg != "只读账号不能修改" {
		t.Fatalf("readonly: %s", msg)
	}

	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/card-settings", map[string]any{
		"thumbnail_template": "https://cdn.example.test/rw/{id}.png",
	})
	settings := listOne(t, ts, admin.access, "/api/admin/v1/cards")
	if settings["thumbnail_template"] != "https://cdn.example.test/rw/{id}.png" {
		t.Fatalf("settings: %#v", settings)
	}

	copyCards := listOne(t, ts, admin.access, "/api/admin/v1/copy/cards")
	copyItems, _ := copyCards["items"].([]any)
	if len(copyItems) != 78 {
		t.Fatalf("copy/cards still 78, got %d", len(copyItems))
	}
}
