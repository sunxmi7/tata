package tata

import (
	"encoding/json"
	"net/http"
	"strings"
)

var tasks = []string{"detect", "rewrite", "recommend", "interpret", "tarotist"}

func knownTask(task string) bool {
	for _, t := range tasks {
		if t == task {
			return true
		}
	}
	return false
}

type vendorBody struct {
	Name       string   `json:"name"`
	BaseURL    string   `json:"base_url"`
	APIKey     string   `json:"api_key"`
	Models     []string `json:"models"`
	TimeoutSec *int     `json:"timeout_sec"`
	Enabled    *bool    `json:"enabled"`
}

func (a *App) adminListVendors(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, name, base_url, api_key_enc, models, timeout_sec, enabled FROM vendors ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		item, err := scanVendor(rows)
		if err != nil {
			return err
		}
		list = append(list, item)
	}
	writeOK(w, list)
	return nil
}

type vendorScanner interface {
	Scan(dest ...any) error
}

func scanVendor(sc vendorScanner) (map[string]any, error) {
	var id int64
	var name, base, models string
	var key []byte
	var timeout *int
	var enabled bool
	if err := sc.Scan(&id, &name, &base, &key, &models, &timeout, &enabled); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "name": name, "base_url": base, "api_key_set": len(key) > 0,
		"models": json.RawMessage(models), "timeout_sec": timeout, "enabled": enabled,
	}, nil
}

func (a *App) adminCreateVendor(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req vendorBody
	if err := decode(r, &req); err != nil {
		return err
	}
	enc, enabled, err := a.prepareVendor(req, nil, false)
	if err != nil {
		return err
	}
	var id int64
	err = a.Store.QueryRow(r.Context(), `INSERT INTO vendors(name, base_url, api_key_enc, models, timeout_sec, enabled) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		req.Name, strings.TrimRight(req.BaseURL, "/"), enc, mustJSON(req.Models), req.TimeoutSec, enabled).Scan(&id)
	if err != nil {
		return err
	}
	detail := "创建供应商"
	if req.APIKey != "" {
		detail = "密钥已设置"
	}
	a.Store.audit(r.Context(), act.id, "create", "vendor", fmtID(id), detail)
	writeOK(w, map[string]any{"id": id})
	return nil
}

func (a *App) prepareVendor(req vendorBody, oldKey []byte, wasEnabled bool) ([]byte, bool, error) {
	if req.Name == "" || len(req.Models) == 0 {
		return nil, false, biz("名称和模型必填")
	}
	if err := validBase(req.BaseURL); err != nil {
		return nil, false, err
	}
	enc := oldKey
	if req.APIKey != "" {
		var err error
		enc, err = a.Store.encrypt(req.APIKey)
		if err != nil {
			return nil, false, err
		}
	}
	enabled := wasEnabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if enabled && len(enc) == 0 {
		return nil, false, biz("启用前必须设置密钥")
	}
	return enc, enabled, nil
}

func (a *App) adminUpdateVendor(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var req vendorBody
	if err := decode(r, &req); err != nil {
		return err
	}
	var old []byte
	var was bool
	err = a.Store.QueryRow(r.Context(), `SELECT api_key_enc, enabled FROM vendors WHERE id=$1`, id).Scan(&old, &was)
	if err != nil {
		return biz("供应商不存在")
	}
	enc, enabled, err := a.prepareVendor(req, old, was)
	if err != nil {
		return err
	}
	if !enabled {
		var n int
		if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM bindings WHERE vendor_id=$1`, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return biz("供应商仍被调用绑定引用，先改绑定再停用")
		}
	}
	_, err = a.Store.Exec(r.Context(), `UPDATE vendors SET name=$1, base_url=$2, api_key_enc=$3, models=$4, timeout_sec=$5, enabled=$6 WHERE id=$7`,
		req.Name, strings.TrimRight(req.BaseURL, "/"), enc, mustJSON(req.Models), req.TimeoutSec, enabled, id)
	if err != nil {
		return err
	}
	detail := "更新供应商"
	if req.APIKey != "" {
		detail = "密钥已更新"
	}
	a.Store.audit(r.Context(), act.id, "update", "vendor", fmtID(id), detail)
	writeOK(w, true)
	return nil
}

func (a *App) adminDeleteVendor(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var n int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM bindings WHERE vendor_id=$1`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return biz("供应商仍被调用绑定引用，只能停用")
	}
	tag, err := a.Store.Exec(r.Context(), `DELETE FROM vendors WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("供应商不存在")
	}
	a.Store.audit(r.Context(), act.id, "delete", "vendor", fmtID(id), "")
	writeOK(w, true)
	return nil
}

func (a *App) adminProbeVendor(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var name, base, models string
	var key []byte
	var timeout *int
	var enabled bool
	err = a.Store.QueryRow(r.Context(), `SELECT name, base_url, api_key_enc, models, timeout_sec, enabled FROM vendors WHERE id=$1`, id).Scan(&name, &base, &key, &models, &timeout, &enabled)
	if err != nil {
		return biz("供应商不存在")
	}
	if !enabled {
		return biz("供应商未启用")
	}
	plain, err := a.Store.decrypt(key)
	if err != nil {
		return biz("密钥无法读取")
	}
	var modelList []string
	_ = json.Unmarshal([]byte(models), &modelList)
	if len(modelList) == 0 {
		return biz("没有模型")
	}
	if _, err := a.einoChat(r.Context(), base, plain, modelList[0], timeout, "连通性探测", "ping"); err != nil {
		return biz("连通性探测失败")
	}
	a.Store.audit(r.Context(), act.id, "probe", "vendor", fmtID(id), name)
	writeOK(w, map[string]any{"ok": true})
	return nil
}

func (a *App) adminListBindings(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT task, vendor_id, model FROM bindings`)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := map[string]map[string]any{}
	for rows.Next() {
		var task, model string
		var vid int64
		if err := rows.Scan(&task, &vid, &model); err != nil {
			return err
		}
		found[task] = map[string]any{"task": task, "vendor_id": vid, "model": model}
	}
	list := []map[string]any{}
	for _, task := range tasks {
		if item, ok := found[task]; ok {
			list = append(list, item)
			continue
		}
		list = append(list, map[string]any{"task": task, "vendor_id": nil, "model": ""})
	}
	writeOK(w, list)
	return nil
}

func (a *App) adminPutBinding(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	task := r.PathValue("task")
	if !knownTask(task) {
		return biz("未知调用")
	}
	var req struct {
		VendorID int64  `json:"vendor_id"`
		Model    string `json:"model"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	var models string
	var enabled bool
	err = a.Store.QueryRow(r.Context(), `SELECT models, enabled FROM vendors WHERE id=$1`, req.VendorID).Scan(&models, &enabled)
	if err != nil || !enabled {
		return biz("供应商不存在或未启用")
	}
	var modelList []string
	_ = json.Unmarshal([]byte(models), &modelList)
	ok := false
	for _, m := range modelList {
		if m == req.Model {
			ok = true
		}
	}
	if !ok {
		return biz("模型不属于该供应商")
	}
	_, err = a.Store.Exec(r.Context(), `INSERT INTO bindings(task, vendor_id, model, updated_at) VALUES ($1,$2,$3,now())
		ON CONFLICT (task) DO UPDATE SET vendor_id=EXCLUDED.vendor_id, model=EXCLUDED.model, updated_at=now()`,
		task, req.VendorID, req.Model)
	if err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "bind", "binding", task, req.Model)
	writeOK(w, true)
	return nil
}
