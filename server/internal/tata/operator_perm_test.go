package tata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestAdminModuleOf(t *testing.T) {
	cases := map[string]string{
		"/api/admin/v1/spreads":            adminModuleContent,
		"/api/admin/v1/cards/1":            adminModuleContent,
		"/api/admin/v1/card-settings":      adminModuleContent,
		"/api/admin/v1/copy/help":          adminModuleContent,
		"/api/admin/v1/push/broadcast":     adminModuleContent,
		"/api/admin/v1/vendors":            adminModuleAI,
		"/api/admin/v1/bindings/interpret": adminModuleAI,
		"/api/admin/v1/quota-params":       adminModuleAI,
		"/api/admin/v1/workflow/simulate":  adminModuleAI,
		"/api/admin/v1/readings/3/retry":   adminModuleRecords,
		"/api/admin/v1/ledger":             adminModuleRecords,
		"/api/admin/v1/users":              adminModuleAccounts,
		"/api/admin/v1/users/stats":        adminModuleAccounts,
		"/api/admin/v1/operators/2":        adminModuleAccounts,
		"/api/admin/v1/audit":              adminModuleAccounts,
		"/api/admin/v1/account-merges":     adminModuleAccounts,
		"/api/admin/v1/iap/skus":           adminModulePay,
		"/api/admin/v1/grants":             adminModulePay,
		"/api/admin/v1/redeem-codes":       adminModulePay,
		"/api/admin/v1/redeem-codes/1":     adminModulePay,
		"/api/admin/v1/uploads":            "",
		"/api/admin/v1/auth/codes":         "",
		"/api/admin/v1/user/info":          "",
		"/api/admin/v1/menu/all":           "",
	}
	for path, want := range cases {
		if got := adminModuleOf(path); got != want {
			t.Fatalf("%s: got %q want %q", path, got, want)
		}
	}
}

func TestAdminModulePermissions(t *testing.T) {
	_, ts := newMergeServer(t)
	admin := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "admin", "password": "admin123"})
	ro := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": "readonly", "password": "readonly123"})

	adminCodes := decodeStringSlice(t, get(t, ts, admin.access, "/api/admin/v1/auth/codes"))
	if !containsAll(adminCodes, "write", "content", "content:write", "pay:write") {
		t.Fatalf("admin codes: %#v", adminCodes)
	}
	roCodes := decodeStringSlice(t, get(t, ts, ro.access, "/api/admin/v1/auth/codes"))
	if containsAll(roCodes, "write") || containsAll(roCodes, "content:write") {
		t.Fatalf("readonly write codes: %#v", roCodes)
	}
	if !containsAll(roCodes, "content", "ai", "records", "accounts", "pay") {
		t.Fatalf("readonly module codes: %#v", roCodes)
	}

	info := listOne(t, ts, admin.access, "/api/admin/v1/user/info")
	roles, _ := info["roles"].([]any)
	if !containsAnyString(roles, "admin", "content") {
		t.Fatalf("admin info roles: %#v", info["roles"])
	}

	uname := fmt.Sprintf("payops%d", time.Now().UnixNano())
	created := create(t, ts, admin.access, "/api/admin/v1/operators", map[string]any{
		"username": uname, "password": "payops123", "role": "readonly",
		"modules": []map[string]any{{"module": "pay", "can_write": true}},
	})
	pay := login(t, ts, "/api/admin/v1/auth/login", map[string]string{"username": uname, "password": "payops123"})
	payCodes := decodeStringSlice(t, get(t, ts, pay.access, "/api/admin/v1/auth/codes"))
	if !containsAll(payCodes, "pay", "pay:write") || containsAll(payCodes, "content") {
		t.Fatalf("pay codes: %#v", payCodes)
	}
	payInfo := listOne(t, ts, pay.access, "/api/admin/v1/user/info")
	if payInfo["homePath"] != "/tata/iap-skus" {
		t.Fatalf("pay home: %#v", payInfo["homePath"])
	}

	msg, _ := call(t, ts, pay.access, http.MethodGet, "/api/admin/v1/spreads", nil)
	if msg != "没有该模块权限" {
		t.Fatalf("pay spreads: %s", msg)
	}
	msg, _ = call(t, ts, pay.access, http.MethodPost, "/api/admin/v1/spreads", map[string]any{"name": "x"})
	if msg != "没有该模块权限" {
		t.Fatalf("pay write spreads: %s", msg)
	}
	if msg, raw := call(t, ts, pay.access, http.MethodGet, "/api/admin/v1/grants", nil); msg != "" {
		t.Fatalf("pay grants: %s %s", msg, raw)
	}
	if msg, raw := call(t, ts, pay.access, http.MethodGet, "/api/admin/v1/redeem-codes", nil); msg != "" {
		t.Fatalf("pay redeem-codes: %s %s", msg, raw)
	}
	msg, _ = call(t, ts, ro.access, http.MethodPost, "/api/admin/v1/operators", map[string]any{
		"username": "x", "password": "x", "role": "readonly",
	})
	if msg != "只有管理员能管理操作员" {
		t.Fatalf("readonly create operator: %s", msg)
	}

	id := int64(created["id"].(float64))
	must(t, ts, admin.access, http.MethodPut, "/api/admin/v1/operators/"+strconv.FormatInt(id, 10), map[string]any{
		"disabled": false,
		"modules":  []map[string]any{{"module": "content", "can_write": false}},
	})
	msg, _ = call(t, ts, pay.access, http.MethodGet, "/api/admin/v1/spreads", nil)
	if msg != "" {
		t.Fatalf("content read after assign: %s", msg)
	}
	msg, _ = call(t, ts, pay.access, http.MethodPost, "/api/admin/v1/cards", map[string]any{"name": "无权写"})
	if msg != "只读账号不能修改" {
		t.Fatalf("content read write: %s", msg)
	}
	msg, _ = call(t, ts, pay.access, http.MethodGet, "/api/admin/v1/grants", nil)
	if msg != "没有该模块权限" {
		t.Fatalf("pay lost: %s", msg)
	}

	ops := list(t, ts, admin.access, "/api/admin/v1/operators")
	found := false
	for _, row := range ops {
		if row["username"] == uname {
			found = true
			mods, _ := row["modules"].([]any)
			if len(mods) != 1 {
				t.Fatalf("payops modules: %#v", row["modules"])
			}
		}
	}
	if !found {
		t.Fatal("payops missing from list")
	}
}

func decodeStringSlice(t *testing.T, raw []byte) []string {
	t.Helper()
	var env struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err, string(raw))
	}
	return env.Data
}

func containsAll(have []string, want ...string) bool {
	set := map[string]struct{}{}
	for _, item := range have {
		set[item] = struct{}{}
	}
	for _, item := range want {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

func containsAnyString(have []any, want ...string) bool {
	set := map[string]struct{}{}
	for _, item := range have {
		s, _ := item.(string)
		set[s] = struct{}{}
	}
	for _, item := range want {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}
