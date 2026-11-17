package tata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAdminRedeemCodes(t *testing.T) {
	store, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})
	token, _ := registerUser(t, ts, store, "redeem-admin")

	listed := listOne(t, ts, ro.access, "/api/admin/v1/redeem-codes")
	if listed["can_write"] != false {
		t.Fatalf("readonly list: %#v", listed)
	}
	msg, _ := call(t, ts, ro.access, http.MethodPost, "/api/admin/v1/redeem-codes", map[string]any{
		"kind": "diamonds", "amount": 10, "benefit_name": "只读不能写",
	})
	if !strings.Contains(msg, "只读") {
		t.Fatalf("readonly create: %s", msg)
	}

	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/redeem-codes", map[string]any{
		"kind": "other", "amount": 10, "benefit_name": "错",
	})
	if msg != "兑换码类型只允许 diamonds 或 free_count" {
		t.Fatalf("kind: %s", msg)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/redeem-codes", map[string]any{
		"kind": "diamonds", "amount": 10, "benefit_name": "礼", "count": 2, "code": "CUSTOM1",
	})
	if msg != "自定义兑换码不能批量生成" {
		t.Fatalf("custom batch: %s", msg)
	}

	custom := fmt.Sprintf("SPRING%d", time.Now().UnixNano()%100000000)
	created := create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"kind": "diamonds", "amount": 60, "benefit_name": "新春礼", "code": strings.ToLower(custom),
		"max_redemptions": 1,
	})
	items, _ := created["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("create custom: %#v", created)
	}
	row := items[0].(map[string]any)
	if row["code"] != strings.ToUpper(custom) || row["amount"].(float64) != 60 || row["redeemed_count"].(float64) != 0 {
		t.Fatalf("custom row: %#v", row)
	}
	msg, _ = call(t, ts, admin.access, http.MethodPost, "/api/admin/v1/redeem-codes", map[string]any{
		"kind": "diamonds", "amount": 60, "benefit_name": "新春礼", "code": custom,
	})
	if msg != "兑换码已存在" {
		t.Fatalf("dup: %s", msg)
	}

	out := create(t, ts, token, "/api/app/v1/redeem", map[string]any{"code": custom})
	if out["amount"].(float64) != 60 || out["kind"] != "diamonds" {
		t.Fatalf("app redeem: %#v", out)
	}

	id := int64(row["id"].(float64))
	msg, _ = call(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/redeem-codes/%d", id), map[string]any{
		"enabled": false,
	})
	if msg != "" {
		t.Fatalf("disable: %s", msg)
	}
	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/redeem", map[string]any{"code": custom}); msg != "兑换码无效" {
		t.Fatalf("disabled after use: %s", msg)
	}

	batch := create(t, ts, admin.access, "/api/admin/v1/redeem-codes", map[string]any{
		"kind": "free_count", "amount": 2, "benefit_name": "体验次数", "count": 3, "max_redemptions": 1,
	})
	batchItems, _ := batch["items"].([]any)
	if len(batchItems) != 3 {
		t.Fatalf("batch: %#v", batch)
	}
	seen := map[string]struct{}{}
	for _, raw := range batchItems {
		item := raw.(map[string]any)
		code := item["code"].(string)
		if _, ok := seen[code]; ok || !strings.Contains(code, "-") {
			t.Fatalf("batch code: %#v", item)
		}
		seen[code] = struct{}{}
	}

	first := batchItems[0].(map[string]any)
	batchID := int64(first["id"].(float64))
	_, raw := call(t, ts, admin.access, http.MethodPut, fmt.Sprintf("/api/admin/v1/redeem-codes/%d", batchID), map[string]any{
		"enabled": false, "benefit_name": "已停用体验",
	})
	var env struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Code != 0 {
		t.Fatalf("update: %s", raw)
	}
	updatedItems, _ := env.Data["items"].([]any)
	updated := updatedItems[0].(map[string]any)
	if updated["enabled"] != false || updated["benefit_name"] != "已停用体验" {
		t.Fatalf("updated: %#v", updated)
	}
	if msg, _ := call(t, ts, token, http.MethodPost, "/api/app/v1/redeem", map[string]any{"code": first["code"]}); msg != "兑换码无效" {
		t.Fatalf("disabled redeem: %s", msg)
	}

	all := listOne(t, ts, admin.access, "/api/admin/v1/redeem-codes")
	if all["can_write"] != true {
		t.Fatalf("admin list: %#v", all)
	}
}
