package tata

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
)

const (
	adminModuleContent  = "content"
	adminModuleAI       = "ai"
	adminModuleRecords  = "records"
	adminModuleAccounts = "accounts"
	adminModulePay      = "pay"
)

var adminModuleIDs = []string{
	adminModuleContent,
	adminModuleAI,
	adminModuleRecords,
	adminModuleAccounts,
	adminModulePay,
}

type operatorModule struct {
	Module   string `json:"module"`
	CanWrite bool   `json:"can_write"`
}

func validAdminModule(module string) bool {
	switch module {
	case adminModuleContent, adminModuleAI, adminModuleRecords, adminModuleAccounts, adminModulePay:
		return true
	default:
		return false
	}
}

func adminModuleOf(path string) string {
	p := strings.TrimPrefix(path, "/api/admin/v1/")
	p = strings.Trim(p, "/")
	head, _, _ := strings.Cut(p, "/")
	switch head {
	case "spreads", "cards", "card-settings", "tarotists", "copy", "banners", "push", "deck-products", "deck-unlocks":
		return adminModuleContent
	case "vendors", "bindings", "quota-params", "workflow":
		return adminModuleAI
	case "readings", "ledger", "messages", "calendar":
		return adminModuleRecords
	case "users", "operators", "audit", "account-merges":
		return adminModuleAccounts
	case "iap", "grants", "redeem-codes":
		return adminModulePay
	default:
		return ""
	}
}

func isAdminOperatorMutate(path string) bool {
	p := strings.TrimPrefix(path, "/api/admin/v1/")
	p = strings.Trim(p, "/")
	return p == "operators" || strings.HasPrefix(p, "operators/")
}

func (a *App) checkAdminAccess(r *http.Request, id int64, role string, write bool) error {
	if write && isAdminOperatorMutate(r.URL.Path) && role != "admin" {
		return biz("只有管理员能管理操作员")
	}
	module := adminModuleOf(r.URL.Path)
	if module == "" {
		if !write || role == "admin" {
			return nil
		}
		ok, err := a.operatorHasAnyWrite(r.Context(), id)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		return biz("只读账号不能修改")
	}
	if role == "admin" {
		return nil
	}
	canRead, canWrite, err := a.operatorModuleAccess(r.Context(), id, module)
	if err != nil {
		return err
	}
	if !canRead {
		return biz("没有该模块权限")
	}
	if write && !canWrite {
		return biz("只读账号不能修改")
	}
	return nil
}

func (a *App) operatorModuleAccess(ctx context.Context, operatorID int64, module string) (canRead, canWrite bool, err error) {
	err = a.Store.QueryRow(ctx, `SELECT can_write FROM operator_module_perms WHERE operator_id=$1 AND module=$2`, operatorID, module).Scan(&canWrite)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, canWrite, nil
}

func (a *App) operatorHasAnyWrite(ctx context.Context, operatorID int64) (bool, error) {
	var n int
	err := a.Store.QueryRow(ctx, `SELECT count(*) FROM operator_module_perms WHERE operator_id=$1 AND can_write=true`, operatorID).Scan(&n)
	return n > 0, err
}

func (a *App) loadOperatorPerms(ctx context.Context, operatorID int64) (map[string]bool, error) {
	rows, err := a.Store.Query(ctx, `SELECT module, can_write FROM operator_module_perms WHERE operator_id=$1`, operatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var module string
		var canWrite bool
		if err := rows.Scan(&module, &canWrite); err != nil {
			return nil, err
		}
		out[module] = canWrite
	}
	return out, rows.Err()
}

func (a *App) listOperatorPerms(ctx context.Context) (map[int64][]operatorModule, error) {
	rows, err := a.Store.Query(ctx, `SELECT operator_id, module, can_write FROM operator_module_perms ORDER BY operator_id, module`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]operatorModule{}
	for rows.Next() {
		var operatorID int64
		var item operatorModule
		if err := rows.Scan(&operatorID, &item.Module, &item.CanWrite); err != nil {
			return nil, err
		}
		out[operatorID] = append(out[operatorID], item)
	}
	return out, rows.Err()
}

func (a *App) replaceOperatorModules(ctx context.Context, operatorID int64, mods []operatorModule) error {
	seen := map[string]struct{}{}
	for _, item := range mods {
		if !validAdminModule(item.Module) {
			return biz("模块权限不正确")
		}
		if _, ok := seen[item.Module]; ok {
			return biz("模块权限不正确")
		}
		seen[item.Module] = struct{}{}
	}
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM operator_module_perms WHERE operator_id=$1`, operatorID); err != nil {
		return err
	}
	for _, item := range mods {
		if _, err := tx.Exec(ctx, `INSERT INTO operator_module_perms(operator_id, module, can_write) VALUES ($1,$2,$3)`, operatorID, item.Module, item.CanWrite); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func allAdminModulePerms() []operatorModule {
	out := make([]operatorModule, 0, len(adminModuleIDs))
	for _, id := range adminModuleIDs {
		out = append(out, operatorModule{Module: id, CanWrite: true})
	}
	return out
}

func operatorMenuRoles(role string, perms map[string]bool) []string {
	roles := []string{role}
	for _, id := range adminModuleIDs {
		if role == "admin" {
			roles = append(roles, id)
			continue
		}
		if _, ok := perms[id]; ok {
			roles = append(roles, id)
		}
	}
	return roles
}

func operatorAccessCodes(role string, perms map[string]bool) []string {
	if role == "admin" {
		codes := []string{"write"}
		for _, id := range adminModuleIDs {
			codes = append(codes, id, id+":write")
		}
		return codes
	}
	codes := []string{}
	for _, id := range adminModuleIDs {
		canWrite, ok := perms[id]
		if !ok {
			continue
		}
		codes = append(codes, id)
		if canWrite {
			codes = append(codes, id+":write")
		}
	}
	return codes
}

func operatorHomePath(role string, perms map[string]bool) string {
	if role == "admin" {
		return "/tata/spread"
	}
	homes := []struct{ module, path string }{
		{adminModuleContent, "/tata/spread"},
		{adminModuleAI, "/tata/vendor"},
		{adminModuleRecords, "/tata/reading"},
		{adminModuleAccounts, "/tata/users"},
		{adminModulePay, "/tata/iap-skus"},
	}
	for _, item := range homes {
		if _, ok := perms[item.module]; ok {
			return item.path
		}
	}
	return "/tata/spread"
}

func (a *App) operatorCanWriteModule(ctx context.Context, act actor, module string) bool {
	if act.role == "admin" {
		return true
	}
	_, canWrite, err := a.operatorModuleAccess(ctx, act.id, module)
	return err == nil && canWrite
}
