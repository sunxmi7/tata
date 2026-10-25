package tata

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"

	"tata/server/internal/service"
)

func (a *App) adminLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		writeErr(w, http.StatusOK, "请求字段不正确")
		return
	}
	var id int64
	var hash, role string
	var disabled bool
	err := a.Store.QueryRow(r.Context(), `SELECT id, password_hash, role, disabled FROM operators WHERE username=$1`, req.Username).Scan(&id, &hash, &role, &disabled)
	if err != nil || disabled || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusOK, "用户名或密码错误")
		return
	}
	access, err := a.signAccess("admin", id, "", role)
	if err != nil {
		writeErr(w, http.StatusOK, "内部错误")
		return
	}
	refresh, err := a.issueRefresh(r.Context(), "admin", id)
	if err != nil {
		writeErr(w, http.StatusOK, "内部错误")
		return
	}
	name := "管理员"
	if role == "readonly" {
		name = "只读"
	}
	writeOK(w, map[string]any{
		"accessToken": access, "refreshToken": refresh,
		"id": id, "username": req.Username, "realName": name, "roles": []string{role},
	})
}

func (a *App) adminLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	_ = decode(r, &req)
	if req.RefreshToken != "" {
		_, _ = a.Store.Exec(r.Context(), `UPDATE refresh_tokens SET revoked=true WHERE id=$1`, req.RefreshToken)
	}
	writeOK(w, true)
}

func (a *App) adminRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := decode(r, &req); err != nil || req.RefreshToken == "" {
		writeErr(w, http.StatusUnauthorized, "刷新令牌无效")
		return
	}
	var aud string
	var sub int64
	var exp time.Time
	var revoked bool
	err := a.Store.QueryRow(r.Context(), `SELECT audience, subject_id, expires_at, revoked FROM refresh_tokens WHERE id=$1`, req.RefreshToken).Scan(&aud, &sub, &exp, &revoked)
	if err != nil || aud != "admin" || revoked || time.Now().After(exp) {
		writeErr(w, http.StatusUnauthorized, "刷新令牌无效")
		return
	}
	var role string
	var disabled bool
	if err := a.Store.QueryRow(r.Context(), `SELECT role, disabled FROM operators WHERE id=$1`, sub).Scan(&role, &disabled); err != nil || disabled {
		writeErr(w, http.StatusUnauthorized, "账号已停用")
		return
	}
	access, err := a.signAccess("admin", sub, "", role)
	if err != nil {
		writeErr(w, http.StatusOK, "内部错误")
		return
	}
	writeOK(w, access)
}

func (a *App) adminInfo(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, false)
	if err != nil {
		return err
	}
	var username, role string
	if err := a.Store.QueryRow(r.Context(), `SELECT username, role FROM operators WHERE id=$1`, act.id).Scan(&username, &role); err != nil {
		return err
	}
	writeOK(w, map[string]any{"id": act.id, "username": username, "realName": username, "roles": []string{role}, "homePath": "/tata/spread", "desc": role, "token": ""})
	return nil
}

func (a *App) adminCodes(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, false)
	if err != nil {
		return err
	}
	codes := []string{}
	if act.role == "admin" {
		codes = []string{"write"}
	}
	writeOK(w, codes)
	return nil
}

func (a *App) adminMenu(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	type item struct {
		Name string         `json:"name"`
		Path string         `json:"path"`
		Meta map[string]any `json:"meta"`
	}
	items := []item{
		{Name: "Spread", Path: "/spread", Meta: map[string]any{"title": "牌阵", "order": 1}},
		{Name: "Tarotist", Path: "/tarotist", Meta: map[string]any{"title": "塔罗师", "order": 2}},
		{Name: "Vendor", Path: "/vendor", Meta: map[string]any{"title": "模型供应商", "order": 3}},
		{Name: "Quota", Path: "/quota", Meta: map[string]any{"title": "额度参数", "order": 4}},
		{Name: "Banner", Path: "/banner", Meta: map[string]any{"title": "首页提示窗", "order": 5}},
		{Name: "Reading", Path: "/reading", Meta: map[string]any{"title": "占卜记录", "order": 6}},
		{Name: "Ledger", Path: "/ledger", Meta: map[string]any{"title": "额度流水", "order": 7}},
		{Name: "Operator", Path: "/operator", Meta: map[string]any{"title": "操作员", "order": 8}},
		{Name: "Users", Path: "/users", Meta: map[string]any{"title": "用户账号", "order": 9}},
		{Name: "Calendar", Path: "/calendar", Meta: map[string]any{"title": "日历", "order": 10}},
		{Name: "Messages", Path: "/messages", Meta: map[string]any{"title": "追问消息", "order": 11}},
		{Name: "Audit", Path: "/audit", Meta: map[string]any{"title": "审计日志", "order": 12}},
		{Name: "IAPOrders", Path: "/iap-orders", Meta: map[string]any{"title": "IAP 订单", "order": 13}},
		{Name: "Grant", Path: "/grant", Meta: map[string]any{"title": "人工补发", "order": 14}},
	}
	writeOK(w, items)
	return nil
}

type spreadBody struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CardCount   int        `json:"card_count"`
	Positions   []position `json:"positions"`
	Enabled     *bool      `json:"enabled"`
	Charged     *bool      `json:"charged"`
}

func (b spreadBody) enabled() bool {
	return b.Enabled == nil || *b.Enabled
}
func (b spreadBody) charged() bool { return b.Charged != nil && *b.Charged }

func (a *App) adminListSpreads(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := service.ListSpreads(r.Context())
	if err != nil {
		return err
	}
	list := []map[string]any{}
	for _, row := range rows {
		list = append(list, map[string]any{
			"id": row.Id, "name": row.Name, "description": row.Description,
			"card_count": row.CardCount, "positions": json.RawMessage(row.Positions),
			"enabled": row.Enabled, "charged": row.Charged,
		})
	}
	writeOK(w, list)
	return nil
}

func (a *App) adminCreateSpread(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req spreadBody
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := validateSpread(req); err != nil {
		return err
	}
	var id int64
	err = a.Store.QueryRow(r.Context(), `INSERT INTO spreads(name, description, card_count, positions, enabled, charged) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		req.Name, req.Description, req.CardCount, mustJSON(req.Positions), req.enabled(), req.charged()).Scan(&id)
	if err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "create", "spread", fmtID(id), req.Name)
	writeOK(w, map[string]any{"id": id})
	return nil
}

func validateSpread(req spreadBody) error {
	if req.Name == "" || req.CardCount < 1 || len(req.Positions) != req.CardCount {
		return biz("牌阵名称、牌数和牌位不一致")
	}
	return nil
}

func (a *App) adminUpdateSpread(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var req spreadBody
	if err := decode(r, &req); err != nil {
		return err
	}
	if err := validateSpread(req); err != nil {
		return err
	}
	var oldCount int
	var oldPos string
	err = a.Store.QueryRow(r.Context(), `SELECT card_count, positions FROM spreads WHERE id=$1`, id).Scan(&oldCount, &oldPos)
	if err != nil {
		return biz("牌阵不存在")
	}
	if oldCount != req.CardCount || oldPos != mustJSON(req.Positions) {
		var n int
		if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings WHERE spread_id=$1`, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return biz("牌阵已被占卜使用，不能修改牌数或牌位")
		}
	}
	tag, err := a.Store.Exec(r.Context(), `UPDATE spreads SET name=$1, description=$2, card_count=$3, positions=$4, enabled=$5, charged=$6 WHERE id=$7`,
		req.Name, req.Description, req.CardCount, mustJSON(req.Positions), req.enabled(), req.charged(), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("牌阵不存在")
	}
	a.Store.audit(r.Context(), act.id, "update", "spread", fmtID(id), req.Name)
	writeOK(w, true)
	return nil
}

func (a *App) adminDeleteSpread(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var n int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings WHERE spread_id=$1`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return biz("牌阵已被引用，只能停用")
	}
	tag, err := a.Store.Exec(r.Context(), `DELETE FROM spreads WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("牌阵不存在")
	}
	a.Store.audit(r.Context(), act.id, "delete", "spread", fmtID(id), "")
	writeOK(w, true)
	return nil
}

func (a *App) adminListTarotists(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, name, persona, display_copy, voice, banned, ask_style, enabled FROM tarotists ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, persona, copy, voice, banned, ask string
		var enabled bool
		if err := rows.Scan(&id, &name, &persona, &copy, &voice, &banned, &ask, &enabled); err != nil {
			return err
		}
		list = append(list, map[string]any{"id": id, "name": name, "persona": persona, "display_copy": copy, "voice": voice, "banned": banned, "ask_style": ask, "enabled": enabled})
	}
	writeOK(w, list)
	return nil
}

func (a *App) adminCreateTarotist(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req struct {
		Name        string `json:"name"`
		Persona     string `json:"persona"`
		DisplayCopy string `json:"display_copy"`
		Voice       string `json:"voice"`
		Banned      string `json:"banned"`
		AskStyle    string `json:"ask_style"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Name == "" {
		return biz("名称必填")
	}
	enabled := req.Enabled == nil || *req.Enabled
	var id int64
	err = a.Store.QueryRow(r.Context(), `INSERT INTO tarotists(name, persona, display_copy, voice, banned, ask_style, enabled) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		req.Name, req.Persona, req.DisplayCopy, req.Voice, req.Banned, req.AskStyle, enabled).Scan(&id)
	if err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "create", "tarotist", fmtID(id), req.Name)
	writeOK(w, map[string]any{"id": id})
	return nil
}

func (a *App) adminUpdateTarotist(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var req struct {
		Name        string `json:"name"`
		Persona     string `json:"persona"`
		DisplayCopy string `json:"display_copy"`
		Voice       string `json:"voice"`
		Banned      string `json:"banned"`
		AskStyle    string `json:"ask_style"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Name == "" {
		return biz("名称必填")
	}
	enabled := req.Enabled == nil || *req.Enabled
	tag, err := a.Store.Exec(r.Context(), `UPDATE tarotists SET name=$1, persona=$2, display_copy=$3, voice=COALESCE(NULLIF($4,''), voice), banned=COALESCE(NULLIF($5,''), banned), ask_style=COALESCE(NULLIF($6,''), ask_style), enabled=$7 WHERE id=$8`,
		req.Name, req.Persona, req.DisplayCopy, req.Voice, req.Banned, req.AskStyle, enabled, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("塔罗师不存在")
	}
	a.Store.audit(r.Context(), act.id, "update", "tarotist", fmtID(id), "人设已更新")
	writeOK(w, true)
	return nil
}

func (a *App) adminQuota(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	cost, configured, err := loadDiamondCost(r.Context(), a.Store)
	if err != nil {
		return err
	}
	var diamondCost any
	if configured {
		diamondCost = cost
	}
	writeOK(w, map[string]any{
		"reset_at":                "北京时间 08:00",
		"daily_free_count":        1,
		"shared_scope":            "全玩法共用：注册奖励，今日免费次数，人工补发免费次数，然后钻石",
		"failure_policy":          "已扣不退",
		"editable":                false,
		"diamond_cost":            diamondCost,
		"diamond_cost_configured": configured,
		"diamond_spend_order":     "pack_then_subscription",
		"free_label":              "次数",
		"paid_label":              "钻石",
	})
	return nil
}

func (a *App) rejectQuotaWrite(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, true); err != nil {
		return err
	}
	return biz("本期不可修改重置时刻或每日次数")
}

func (a *App) rejectLedgerWrite(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, true); err != nil {
		return err
	}
	return biz("流水不能新增或修改")
}

func (a *App) adminListBanners(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, copy, start_at, end_at, priority, target FROM banners ORDER BY priority DESC, id DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	writeOK(w, scanBanners(rows))
	return nil
}

func (a *App) adminCreateBanner(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	b, err := readBanner(r)
	if err != nil {
		return err
	}
	var id int64
	err = a.Store.QueryRow(r.Context(), `INSERT INTO banners(copy, start_at, end_at, priority, target) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		b.copy, b.start, b.end, b.priority, b.target).Scan(&id)
	if err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "create", "banner", fmtID(id), "")
	writeOK(w, map[string]any{"id": id})
	return nil
}

func (a *App) adminUpdateBanner(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	b, err := readBanner(r)
	if err != nil {
		return err
	}
	tag, err := a.Store.Exec(r.Context(), `UPDATE banners SET copy=$1, start_at=$2, end_at=$3, priority=$4, target=$5 WHERE id=$6`,
		b.copy, b.start, b.end, b.priority, b.target, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("提示窗不存在")
	}
	a.Store.audit(r.Context(), act.id, "update", "banner", fmtID(id), "")
	writeOK(w, true)
	return nil
}

type bannerIn struct {
	copy     string
	start    time.Time
	end      time.Time
	priority int
	target   string
}

func readBanner(r *http.Request) (bannerIn, error) {
	var req struct {
		Copy     string `json:"copy"`
		StartAt  string `json:"start_at"`
		EndAt    string `json:"end_at"`
		Priority int    `json:"priority"`
		Target   string `json:"target"`
	}
	if err := decode(r, &req); err != nil {
		return bannerIn{}, err
	}
	if req.Copy == "" {
		return bannerIn{}, biz("文案必填")
	}
	start, err1 := time.Parse(time.RFC3339, req.StartAt)
	end, err2 := time.Parse(time.RFC3339, req.EndAt)
	if err1 != nil || err2 != nil || !end.After(start) {
		return bannerIn{}, biz("展示时间不正确")
	}
	return bannerIn{req.Copy, start, end, req.Priority, req.Target}, nil
}

func scanBanners(rows interface {
	Next() bool
	Scan(...any) error
}) []map[string]any {
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var copy, target string
		var start, end time.Time
		var priority int
		if err := rows.Scan(&id, &copy, &start, &end, &priority, &target); err != nil {
			return list
		}
		list = append(list, map[string]any{
			"id": id, "copy": copy, "start_at": start, "end_at": end, "priority": priority, "target": target,
		})
	}
	return list
}

func (a *App) adminListOperators(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, username, role, disabled FROM operators ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var username, role string
		var disabled bool
		if err := rows.Scan(&id, &username, &role, &disabled); err != nil {
			return err
		}
		list = append(list, map[string]any{"id": id, "username": username, "role": role, "disabled": disabled})
	}
	writeOK(w, list)
	return nil
}

func (a *App) adminCreateOperator(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Username == "" || req.Password == "" || (req.Role != "admin" && req.Role != "readonly") {
		return biz("操作员字段不正确")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	var id int64
	err = a.Store.QueryRow(r.Context(), `INSERT INTO operators(username, password_hash, role) VALUES ($1,$2,$3) RETURNING id`, req.Username, string(hash), req.Role).Scan(&id)
	if err != nil {
		return biz("用户名已存在")
	}
	a.Store.audit(r.Context(), act.id, "create", "operator", fmtID(id), req.Role)
	writeOK(w, map[string]any{"id": id})
	return nil
}

func (a *App) adminUpdateOperator(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var req struct {
		Disabled *bool `json:"disabled"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if id == act.id && req.Disabled != nil && *req.Disabled {
		return biz("不能停用自己")
	}
	disabled := req.Disabled != nil && *req.Disabled
	tag, err := a.Store.Exec(r.Context(), `UPDATE operators SET disabled=$1 WHERE id=$2`, disabled, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("操作员不存在")
	}
	if disabled {
		_, _ = a.Store.Exec(r.Context(), `UPDATE refresh_tokens SET revoked=true WHERE audience='admin' AND subject_id=$1`, id)
	}
	a.Store.audit(r.Context(), act.id, "update", "operator", fmtID(id), "disabled="+strconv.FormatBool(disabled))
	writeOK(w, true)
	return nil
}
