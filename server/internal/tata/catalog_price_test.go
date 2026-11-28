package tata

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPriceLedgersStaySeparate(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	token, uid := registerUser(t, ts, store, "ledger")

	spreads := appSpreadItems(t, ts, token)
	found := map[string]map[string]any{}
	for _, s := range spreads {
		found[s["name"].(string)] = s
	}
	one := found["单张牌占卜"]
	if one["depth_group"] != "quick" || one["depth_label"] != "快速指引" || one["quota_cost"] != float64(5) || one["price_ledger"] != "quota_count" || one["charged"] != false {
		t.Fatalf("single spread ledger: %#v", one)
	}
	if _, ok := one["price_amount_minor"]; ok {
		t.Fatalf("spread carries money: %#v", one)
	}
	tree := found["生命之树牌阵"]
	if tree["card_count"] != float64(10) || tree["quota_cost"] != nil || tree["price_ledger"] != nil || tree["depth_group"] != "panorama" {
		t.Fatalf("tree of life: %#v", tree)
	}
	celtic := found["凯尔特十字牌阵"]
	if celtic["name_en"] != "Celtic Cross" || celtic["description"] == "" || tree["name_en"] != "Tree of Life Spread" || tree["description"] == "" {
		t.Fatalf("panorama copy: %#v %#v", celtic, tree)
	}
	if found["单张牌占卜"]["name_en"] != "One Card Reading" {
		t.Fatalf("english name: %#v", found["单张牌占卜"])
	}
	if found["凯尔特十字牌阵"]["quota_cost"] != float64(10) || found["六张十字牌阵"]["quota_cost"] != float64(8) {
		t.Fatalf("celtic/six: %#v %#v", found["凯尔特十字牌阵"], found["六张十字牌阵"])
	}
	groups := map[string]int{}
	for _, s := range spreads {
		if g, _ := s["depth_group"].(string); g != "" {
			groups[g]++
		}
	}
	if groups["quick"] != 5 || groups["advanced"] != 12 || groups["panorama"] != 2 {
		t.Fatalf("groups: %#v", groups)
	}
	if _, ok := found["单张是非牌阵"]; ok {
		t.Fatalf("explore spread leaked into ordinary list")
	}

	deckSKU := create(t, ts, admin.access, "/api/admin/v1/iap/skus", map[string]any{
		"code": fmt.Sprintf("deck_extra_%d", time.Now().UnixNano()), "kind": "deck_single", "display_name": "单套补一条",
		"price_amount_minor": int64(660),
	})
	if deckSKU["kind"] != "deck_single" {
		t.Fatalf("deck kind: %#v", deckSKU)
	}

	shop := listOne(t, ts, "", "/api/app/v1/deck-shop")
	if shop["ledger"] != "deck_price" {
		t.Fatalf("shop ledger: %#v", shop["ledger"])
	}
	prices := map[string]float64{}
	for _, raw := range shop["products"].([]any) {
		item := raw.(map[string]any)
		prices[item["code"].(string)] = item["price_amount_minor"].(float64)
		if item["ledger"] != "deck_price" || item["deduct_owned"] != false {
			t.Fatalf("product: %#v", item)
		}
	}
	if prices["deck_year"] != 9800 || prices["deck_bundle_4"] != 1200 || prices["deck_single_star"] != 660 || prices["deck_addon"] != 300 {
		t.Fatalf("deck prices: %#v", prices)
	}
	if len(shop["decks"].([]any)) != 4 {
		t.Fatalf("shop decks: %#v", shop["decks"])
	}
	msg, _ := call(t, ts, token, http.MethodPut, "/api/app/v1/deck-preference", map[string]any{"code": "star-realm"})
	if !strings.Contains(msg, "还未解锁") {
		t.Fatalf("locked deck: %s", msg)
	}

	granted := create(t, ts, admin.access, "/api/admin/v1/deck-unlocks", map[string]any{
		"user_id": uid, "product_code": "deck_year",
	})
	if granted["ledger"] != "deck_price" {
		t.Fatalf("grant ledger: %#v", granted)
	}
	var ledgerN int
	if err := store.QueryRow(t.Context(), `SELECT count(*) FROM quota_ledger WHERE user_id=$1`, uid).Scan(&ledgerN); err != nil {
		t.Fatal(err)
	}
	if ledgerN != 0 {
		t.Fatalf("deck grant wrote quota ledger: %d", ledgerN)
	}
	must(t, ts, token, http.MethodPut, "/api/app/v1/deck-preference", map[string]any{"code": "star-realm"})

	deviceID := fmt.Sprintf("device-ledger-%d", time.Now().UnixNano())
	guide := listOne(t, ts, "", "/api/app/v1/onboarding?device_id="+deviceID)
	if guide["show_guide"] != true || guide["reading_trial_available"] != true {
		t.Fatalf("new device: %#v", guide)
	}
	must(t, ts, "", http.MethodPut, "/api/app/v1/onboarding", map[string]any{
		"device_id": deviceID, "gender": "女", "birth_date": "1991-04-02",
		"explore_preferences": []string{"感情与关系", "自我成长"}, "guide_completed": true,
	})
	guide = listOne(t, ts, "", "/api/app/v1/onboarding?device_id="+deviceID)
	if guide["show_guide"] != false || guide["guide_completed"] != true || guide["reading_trial_available"] != true || guide["gender"] != "女性" {
		t.Fatalf("guide split: %#v", guide)
	}
	if _, ok := guide["price_amount_minor"]; ok {
		t.Fatalf("onboarding has a price: %#v", guide)
	}
}
