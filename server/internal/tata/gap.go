package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func progressOf(status string) (label, next string) {
	return progressOfReading(status, false, "")
}

// progressOfReading returns progress label and continue action.
// skipDetect=true (preset/explore) skips question detect → continue=spread.
// draw_mode=numerology skips shuffle/cut → continue=numerology until cards are fixed.
func progressOfReading(status string, skipDetect bool, drawMode string) (label, next string) {
	numerology := strings.TrimSpace(drawMode) == "numerology"
	switch status {
	case "QUESTION_CREATED":
		if skipDetect {
			return "问题已提交", "spread"
		}
		return "问题已提交", "detect"
	case "SPREAD_SELECTED":
		if numerology {
			return "牌阵已选择", "numerology"
		}
		return "牌阵已选择", "shuffle"
	case "SHUFFLING":
		if numerology {
			return "待灵数抽牌", "numerology"
		}
		return "洗牌中", "cut"
	case "CUTTING":
		if numerology {
			return "待灵数抽牌", "numerology"
		}
		return "切牌中", "draw"
	case "CARDS_FIXED":
		return "牌面已固定", "interpret"
	case "INTERPRETING":
		return "解读生成中", "interpret"
	case "COMPLETED":
		return "已完成", ""
	case "FAILED_RETRYABLE":
		return "失败可重试", "interpret"
	default:
		return status, ""
	}
}

func (a *App) userCycle(r *http.Request, uid int64) string {
	return cycleFor(time.Now(), userTimezone(r.Context(), a.Store, uid))
}

func canonicalDevice(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if !validDeviceID(raw) {
		return ""
	}
	return raw
}

func (a *App) rememberLoginDevice(ctx context.Context, uid int64, deviceID string) error {
	deviceID = canonicalDevice(deviceID)
	if uid <= 0 || deviceID == "" {
		return nil
	}
	_, err := a.Store.Exec(ctx, `INSERT INTO user_devices(user_id, device_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, uid, deviceID)
	return err
}

func (a *App) claimDevices(ctx context.Context, uid int64) {
	rows, err := a.Store.Query(ctx, `
		SELECT device_id FROM user_devices WHERE user_id=$1
		UNION
		SELECT lower(device_id) FROM device_experiences WHERE user_id=$1 AND device_id<>''`, uid)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var device string
		if err := rows.Scan(&device); err != nil {
			return
		}
		device = canonicalDevice(device)
		if device == "" {
			continue
		}
		_, _ = a.Store.Exec(ctx, `INSERT INTO identity_claims(kind, value) VALUES ('device',$1) ON CONFLICT DO NOTHING`, device)
	}
}

func (a *App) claimed(r *http.Request, kind, value string) bool {
	if value == "" {
		return false
	}
	var n int
	_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM identity_claims WHERE kind=$1 AND value=$2`, kind, strings.ToLower(value)).Scan(&n)
	return n > 0
}

func (a *App) appWechat(w http.ResponseWriter, r *http.Request) {
	var req wechatAuthBody
	if err := decode(r, &req); err != nil {
		writeErr(w, r, http.StatusOK, "请求字段不正确")
		return
	}
	prof, err := a.wechatIdentity(r.Context(), req.Code)
	if err != nil {
		writeErr(w, r, http.StatusOK, err.Error())
		return
	}
	mergeWeChatHints(&prof, req)
	var id int64
	var cancelled *time.Time
	var merged *int64
	created := false
	err = a.Store.QueryRow(r.Context(), `SELECT id, cancelled_at, merged_into FROM app_users WHERE wechat_openid=$1`, prof.ID).Scan(&id, &cancelled, &merged)
	if err != nil {
		if a.claimed(r, "wechat", prof.ID) {
			writeErr(w, r, http.StatusOK, "该微信已注销，不能重复注册")
			return
		}
		err = a.Store.QueryRow(r.Context(), `INSERT INTO app_users(wechat_openid) VALUES ($1) RETURNING id`, prof.ID).Scan(&id)
		if err != nil {
			writeErr(w, r, http.StatusOK, "微信登录失败")
			return
		}
		created = true
	}
	if cancelled != nil {
		writeErr(w, r, http.StatusOK, "账号已注销")
		return
	}
	if merged != nil {
		writeErr(w, r, http.StatusOK, "账号已合并")
		return
	}
	if err := a.rememberLoginDevice(r.Context(), id, req.DeviceID); err != nil {
		writeErr(w, r, http.StatusOK, "注册赠送记录失败")
		return
	}
	if err := a.finishRegistration(r.Context(), id, created); err != nil {
		writeErr(w, r, http.StatusOK, "注册赠送记录失败")
		return
	}
	a.applyWeChatProfile(r, id, prof)
	a.ensureProfileDefaults(r.Context(), id)
	a.attachDeviceReadings(r.Context(), id, req.DeviceID)
	a.attachDeviceProfileDraft(r.Context(), id, req.DeviceID)
	a.applyDeviceForfeit(r.Context(), id, req.DeviceID)
	a.bindInvite(r.Context(), id, req.DeviceID, req.InviteCode, req.InviteSource)
	access, _ := a.signAccess("app", id, "")
	settled := a.settleQuestionStart(r, id)
	writeOK(w, map[string]any{"accessToken": access, "id": id, "reward_consumed": settled, "reward_note": T(r.Context(), wechatQuotaNote)})
}

func (a *App) appApple(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IdentityToken string `json:"identity_token"`
		InviteCode    string `json:"invite_code"`
		InviteSource  string `json:"invite_source"`
		DeviceID      string `json:"device_id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, r, http.StatusOK, "请求字段不正确")
		return
	}
	sub, err := a.appleIdentity(r.Context(), req.IdentityToken)
	if err != nil {
		writeErr(w, r, http.StatusOK, err.Error())
		return
	}
	var id int64
	var cancelled *time.Time
	var merged *int64
	created := false
	err = a.Store.QueryRow(r.Context(), `SELECT id, cancelled_at, merged_into FROM app_users WHERE apple_sub=$1`, sub).Scan(&id, &cancelled, &merged)
	if err != nil {
		if a.claimed(r, "apple", sub) {
			writeErr(w, r, http.StatusOK, "该苹果账号已注销，不能重复注册")
			return
		}
		err = a.Store.QueryRow(r.Context(), `INSERT INTO app_users(apple_sub) VALUES ($1) RETURNING id`, sub).Scan(&id)
		if err != nil {
			writeErr(w, r, http.StatusOK, "苹果登录失败")
			return
		}
		created = true
	}
	if cancelled != nil {
		writeErr(w, r, http.StatusOK, "账号已注销")
		return
	}
	if merged != nil {
		writeErr(w, r, http.StatusOK, "账号已合并")
		return
	}
	if err := a.rememberLoginDevice(r.Context(), id, req.DeviceID); err != nil {
		writeErr(w, r, http.StatusOK, "注册赠送记录失败")
		return
	}
	if err := a.finishRegistration(r.Context(), id, created); err != nil {
		writeErr(w, r, http.StatusOK, "注册赠送记录失败")
		return
	}
	a.ensureProfileDefaults(r.Context(), id)
	a.attachDeviceReadings(r.Context(), id, req.DeviceID)
	a.attachDeviceProfileDraft(r.Context(), id, req.DeviceID)
	a.applyDeviceForfeit(r.Context(), id, req.DeviceID)
	a.bindInvite(r.Context(), id, req.DeviceID, req.InviteCode, req.InviteSource)
	access, _ := a.signAccess("app", id, "")
	writeOK(w, map[string]any{"accessToken": access, "id": id, "login": "apple"})
}

func (a *App) appBindApple(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		IdentityToken string `json:"identity_token"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	sub, err := a.appleIdentity(r.Context(), req.IdentityToken)
	if err != nil {
		return err
	}
	return a.bindWithConfirm(w, r, bindSpec{kind: "apple", value: sub})
}

func (a *App) appBindEmail(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Email == "" || req.Code == "" {
		return biz("邮箱和验证码必填")
	}
	email := normalizeEmail(req.Email)
	if !validLoginEmail(email) {
		return biz("邮箱不正确")
	}
	if err := a.matchEmailCode(r.Context(), "bind", email, req.Code); err != nil {
		return err
	}
	a.consumeEmailCode(r.Context(), "bind", email)
	return a.bindWithConfirm(w, r, bindSpec{kind: "email", value: email})
}

func (a *App) appBindWechat(w http.ResponseWriter, r *http.Request) error {
	var req wechatAuthBody
	if err := decode(r, &req); err != nil {
		return err
	}
	prof, err := a.wechatIdentity(r.Context(), req.Code)
	if err != nil {
		return err
	}
	mergeWeChatHints(&prof, req)
	return a.bindWithConfirm(w, r, bindSpec{kind: "wechat", value: prof.ID, wechat: prof})
}

func (a *App) appCancel(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能注销")
	}
	var email, openid, apple, phone *string
	if err := a.Store.QueryRow(r.Context(), `SELECT email, wechat_openid, apple_sub, phone FROM app_users WHERE id=$1 AND cancelled_at IS NULL`, uid).Scan(&email, &openid, &apple, &phone); err != nil {
		return biz("账号已注销")
	}
	claim := func(kind string, v *string) {
		if v == nil || *v == "" {
			return
		}
		_, _ = a.Store.Exec(r.Context(), `INSERT INTO identity_claims(kind, value) VALUES ($1,$2) ON CONFLICT DO NOTHING`, kind, strings.ToLower(*v))
	}
	claim("email", email)
	claim("wechat", openid)
	claim("apple", apple)
	claim("phone", phone)
	a.claimDevices(r.Context(), uid)
	_, err = a.Store.Exec(r.Context(), `UPDATE app_users SET cancelled_at=now(), email=NULL, wechat_openid=NULL, apple_sub=NULL, phone=NULL, password_hash=NULL WHERE id=$1`, uid)
	if err != nil {
		return err
	}
	_, _ = a.Store.Exec(r.Context(), `UPDATE refresh_tokens SET revoked=true WHERE audience='app' AND subject_id=$1`, uid)
	writeOK(w, true)
	return nil
}

func (a *App) appEditQuestion(w http.ResponseWriter, r *http.Request) error {
	_, id, _, err := a.beginReading(r)
	if err != nil {
		return err
	}
	var kind string
	var skip bool
	var status string
	if err := a.Store.QueryRow(r.Context(), `SELECT type, skip_detect, status FROM readings WHERE id=$1`, id).Scan(&kind, &skip, &status); err != nil {
		return err
	}
	if kind == "explore" {
		if status == "COMPLETED" || status == "INTERPRETING" {
			return biz("抽牌后不能修改问题")
		}
	} else {
		if skip {
			return biz("未进入问题检测，不能修改问题")
		}
		if status != "QUESTION_CREATED" && status != "SPREAD_SELECTED" {
			return biz("抽牌后不能修改问题")
		}
	}
	var req struct {
		Question string `json:"question"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Question) == "" {
		return biz("问题不能为空")
	}
	if len([]rune(req.Question)) > 200 {
		return biz("问题超过 200 个码点，正式计数口径尚未确认")
	}
	_, err = a.Store.Exec(r.Context(), `UPDATE readings SET question=$1, updated_at=now() WHERE id=$2`, req.Question, id)
	if err != nil {
		return err
	}
	item, err := a.loadReadingAbs(r.Context(), r, id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) appRecommend(w http.ResponseWriter, r *http.Request) error {
	uid, id, _, err := a.beginReading(r)
	if err != nil {
		return err
	}
	var question string
	if err := a.Store.QueryRow(r.Context(), `SELECT question FROM readings WHERE id=$1`, id).Scan(&question); err != nil {
		return err
	}
	payload, err := a.localDetect(r, uid, question)
	if err != nil {
		return err
	}
	a.rememberRecommendation(r.Context(), id, payload)
	writeOK(w, payload)
	return nil
}

func (a *App) rememberRecommendation(ctx context.Context, readingID int64, payload map[string]any) {
	list, _ := payload["recommended_spreads"].([]map[string]any)
	if len(list) == 0 {
		return
	}
	id, _ := list[0]["id"].(int64)
	if id <= 0 {
		return
	}
	_, _ = a.Store.Exec(ctx, `UPDATE readings SET recommended_spread_id=$1 WHERE id=$2 AND spread_id IS NULL`, id, readingID)
}

func (a *App) localDetect(r *http.Request, uid int64, question string) (map[string]any, error) {
	q := strings.TrimSpace(question)
	out := map[string]any{
		"question":          q,
		"source":            "local",
		"decided":           false,
		"model_called":      false,
		"can_keep_question": true,
		"invalid":           q == "",
		"can_select_spread": q != "",
	}
	if q == "" {
		out["reason"] = "empty"
		out["match_status"] = matchStatusInputNotMatchable
		out["mapping_status"] = embeddedSpreadMatchConfig.MappingStatus
		out["detection_status"] = detectionStatusInvalid
		return out, nil
	}
	if err := a.attachRecommendation(r, out); err != nil {
		return nil, err
	}
	member := false
	var err error
	if uid > 0 {
		member, err = hasActiveMembership(r.Context(), a.Store, uid)
		if err != nil {
			return nil, err
		}
	}
	status, _ := out["match_status"].(string)
	switch status {
	case matchStatusMatched:
		out["reason"] = "local_match"
	case matchStatusFallback:
		out["reason"] = "fallback"
	case matchStatusInputNotMatchable:
		out["reason"] = "input_not_matchable"
	case matchStatusNoMatchMainIntent:
		out["reason"] = "no_match_main_intent"
	default:
		out["reason"] = "no_match"
	}
	if !member {
		out["entitlement_note"] = "no_entitlement"
		if status != matchStatusMatched && status != matchStatusFallback {
			out["reason"] = "no_entitlement"
		}
	}
	return out, nil
}

func rejectFreeFollowup(ctx context.Context, q *Store, uid, readingID int64, kind string) error {
	var onboard bool
	if err := q.QueryRow(ctx, `SELECT onboard_experience FROM readings WHERE id=$1`, readingID).Scan(&onboard); err != nil {
		return err
	}
	if onboard {
		return biz("新人体验不能追问")
	}
	switch kind {
	case "tarotist":
		_, ent, err := playAlready(ctx, q, readingID)
		if err != nil {
			return err
		}
		if ent == entTrial {
			return biz("塔罗师不能追问")
		}
		member, err := hasActiveMembership(ctx, q, uid)
		if err != nil {
			return err
		}
		if !member {
			return biz("免费用户不能追问")
		}
		return nil
	case "daily":
		return biz("今日和明日不能追问")
	case "normal", "explore":
		member, err := hasActiveMembership(ctx, q, uid)
		if err != nil {
			return err
		}
		if !member {
			return biz("免费用户不能追问")
		}
		return nil
	default:
		return nil
	}
}

func (a *App) appFollowup(w http.ResponseWriter, r *http.Request) error {
	if wantsSSE(r) {
		return a.appFollowupStream(w, r)
	}
	uid, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, ""); err != nil {
		return err
	}
	scope := "followup:" + fmtID(id)
	idemKey := r.Header.Get("Idempotency-Key")
	if replay, ok, err := a.beginIdem(r, scope, idemKey); err != nil {
		return err
	} else if ok {
		writeJSON(w, http.StatusOK, json.RawMessage(replay))
		return nil
	}
	var req struct {
		Text         string `json:"text"`
		Continuation *bool  `json:"continuation"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Text) == "" {
		a.clearPendingIdem(r, scope, idemKey)
		return biz("追问不能为空")
	}
	if req.Continuation == nil {
		a.clearPendingIdem(r, scope, idemKey)
		return biz("请标明是否延续原问题")
	}
	if !*req.Continuation {
		if len([]rune(req.Text)) > 200 {
			a.clearPendingIdem(r, scope, idemKey)
			return biz("问题超过 200 个码点，正式计数口径尚未确认")
		}
		view, err := a.createNormal(r.Context(), uid, strings.TrimSpace(req.Text), "normal", nil, false, "", "", "")
		if err != nil {
			a.clearPendingIdem(r, scope, idemKey)
			return err
		}
		body := map[string]any{"new_reading": true, "reading": view, "charged_here": false}
		a.finishIdem(r, scope, idemKey, body)
		writeOK(w, body)
		return nil
	}
	var kind, status string
	var cards *string
	err = a.Store.QueryRow(r.Context(), `SELECT type, status, cards FROM readings WHERE id=$1`, id).Scan(&kind, &status, &cards)
	if err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	if kind != "normal" && kind != "explore" && kind != "daily" && kind != "tarotist" {
		a.clearPendingIdem(r, scope, idemKey)
		return biz("当前记录不能追问")
	}
	if status != "COMPLETED" && status != "FAILED_RETRYABLE" {
		a.clearPendingIdem(r, scope, idemKey)
		return biz("解读完成前不能追问")
	}
	if cards == nil || *cards == "" {
		a.clearPendingIdem(r, scope, idemKey)
		return biz("牌面尚未固定")
	}
	if uid == 0 {
		a.clearPendingIdem(r, scope, idemKey)
		return biz(paywallNeedLogin)
	}
	if err := rejectFreeFollowup(r.Context(), a.Store, uid, id, kind); err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	if strings.Contains(req.Text, "别这样说") || strings.Contains(req.Text, "不舒服") {
		_, _ = a.Store.Exec(r.Context(), `UPDATE app_users SET tease_off=true WHERE id=$1`, uid)
	}
	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	defer tx.Rollback(r.Context())
	var locked int64
	if err := tx.QueryRow(r.Context(), `SELECT id FROM readings WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	turn, err := nextFollowupTurn(r.Context(), tx, id)
	if err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	if err := enforceFollowupLimit(r.Context(), tx, id, turn); err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	key := "followup:" + fmtID(turn)
	decision, err := settleSharedQuota(r.Context(), tx, chargeOpts{
		UserID: uid, ReadingID: id, ChargeKey: key, Cycle: a.userCycle(r, uid),
		AllowTrial: true, ResumeAction: resumeFollowup,
	})
	if err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	if !decision.Continue {
		_, _ = tx.Exec(r.Context(), `UPDATE quota_paywall SET resume_payload=$1, updated_at=now() WHERE reading_id=$2 AND charge_key=$3`,
			strings.TrimSpace(req.Text), id, key)
		if err := tx.Commit(r.Context()); err != nil {
			return err
		}
		a.clearPendingIdem(r, scope, idemKey)
		writeQuotaBlock(w, r, decision, id)
		return nil
	}
	if err := ensureUserTurn(r.Context(), tx, id, turn, strings.TrimSpace(req.Text)); err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	run, err := a.ensureFollowupJob(r.Context(), id, uid, turn, strings.TrimSpace(req.Text))
	if err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	if err := run.wait(r.Context()); err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	body, _ := run.result().(map[string]any)
	if body == nil {
		body, err = a.answerFollowup(r.Context(), id, uid, turn, strings.TrimSpace(req.Text))
		if err != nil {
			a.clearPendingIdem(r, scope, idemKey)
			return err
		}
	}
	body["quota_outcome"] = decision.Status
	body["entitlement"] = decision.Entitlement
	body["charged"] = decision.Status == quotaCharged && decision.Entitlement != entTrial
	if err := a.attachRedrawOffer(r.Context(), id, turn, body); err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	if err := a.attachConversation(r, id, body); err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	a.finishIdem(r, scope, idemKey, body)
	writeOK(w, body)
	return nil
}

func (a *App) clearPendingIdem(r *http.Request, scope, key string) {
	if key == "" {
		return
	}
	_, _ = a.Store.Exec(r.Context(), `DELETE FROM idempotency WHERE scope=$1 AND key=$2 AND status='pending'`, scope, key)
}

func (a *App) finishIdem(r *http.Request, scope, key string, data any) {
	if key == "" {
		return
	}
	raw, _ := json.Marshal(map[string]any{"code": 0, "data": data, "message": ""})
	_, _ = a.Store.Exec(r.Context(), `UPDATE idempotency SET status='done', response=$1 WHERE scope=$2 AND key=$3`, string(raw), scope, key)
}

func nextFollowupTurn(ctx context.Context, ex execer, readingID int64) (int64, error) {
	var open sql.NullInt64
	err := ex.QueryRow(ctx, `SELECT turn_id FROM reading_messages
		WHERE reading_id=$1 AND role='user'
		AND NOT EXISTS (
			SELECT 1 FROM reading_messages a
			WHERE a.reading_id=$1 AND a.turn_id=reading_messages.turn_id AND a.role='assistant'
		)
		ORDER BY turn_id DESC LIMIT 1`, readingID).Scan(&open)
	if err == nil && open.Valid {
		return open.Int64, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var turn int64
	if err := ex.QueryRow(ctx, `SELECT COALESCE(max(turn_id),0)+1 FROM reading_messages WHERE reading_id=$1`, readingID).Scan(&turn); err != nil {
		return 0, err
	}
	if turn <= 0 {
		turn = 1
	}
	return turn, nil
}

func ensureUserTurn(ctx context.Context, ex execer, readingID, turn int64, text string) error {
	var n int
	if err := ex.QueryRow(ctx, `SELECT count(*) FROM reading_messages WHERE reading_id=$1 AND turn_id=$2 AND role='user'`, readingID, turn).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := ex.Exec(ctx, `INSERT INTO reading_messages(reading_id, turn_id, seq, role, content) VALUES ($1,$2,0,'user',$3)`, readingID, turn, text)
	return err
}

func (a *App) answerFollowup(ctx context.Context, readingID, uid, turn int64, text string) (map[string]any, error) {
	var question string
	var cards *string
	var tarotistID *int64
	if err := a.Store.QueryRow(ctx, `SELECT question, cards, tarotist_id FROM readings WHERE id=$1`, readingID).Scan(&question, &cards, &tarotistID); err != nil {
		return nil, err
	}
	if cards == nil || *cards == "" {
		return nil, biz("牌面尚未固定")
	}
	prompt := a.personaPrompt(ctx, tarotistID, uid) + "\n问题：" + question + "\n牌面：" + *cards + "\n追问：" + text + "\n不要改牌。把回答拆成不超过四个短气泡，气泡之间用空行分隔。"
	ctx = mergeAIBill(ctx, aiBill{UserID: uid, ReadingID: readingID, Kind: "followup"})
	_, _, output, err := a.runEino(ctx, "tarotist", prompt)
	if err != nil {
		return map[string]any{"turn_id": turn, "error": "生成失败，可重试，未重新扣额度", "cards_unchanged": true, "bubbles": []string{}}, nil
	}
	parts := strings.Split(output, "\n\n")
	bubbles := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		bubbles = append(bubbles, part)
		if len(bubbles) == 4 {
			break
		}
	}
	if len(bubbles) == 0 {
		bubbles = []string{output}
	}
	var existing int
	_ = a.Store.QueryRow(ctx, `SELECT count(*) FROM reading_messages WHERE reading_id=$1 AND turn_id=$2 AND role='assistant'`, readingID, turn).Scan(&existing)
	if existing == 0 {
		for i, bubble := range bubbles {
			if _, err := a.Store.Exec(ctx, `INSERT INTO reading_messages(reading_id, turn_id, seq, role, content) VALUES ($1,$2,$3,'assistant',$4)`, readingID, turn, i+1, bubble); err != nil {
				return nil, err
			}
		}
	}
	_, _ = a.Store.Exec(ctx, `UPDATE readings SET updated_at=now() WHERE id=$1`, readingID)
	return map[string]any{"turn_id": turn, "bubbles": bubbles, "cards_unchanged": true}, nil
}

// answerFollowupStreaming 与 answerFollowup 相同的生成与落库，但流式回调 onChunk 下发原始增量。
// 全部收完后按空行切成不超过四个气泡落库，返回与非流式一致的 body。
func (a *App) answerFollowupStreaming(ctx context.Context, readingID, uid, turn int64, text string, onChunk func(string) error) (map[string]any, error) {
	var question string
	var cards *string
	var tarotistID *int64
	if err := a.Store.QueryRow(ctx, `SELECT question, cards, tarotist_id FROM readings WHERE id=$1`, readingID).Scan(&question, &cards, &tarotistID); err != nil {
		return nil, err
	}
	if cards == nil || *cards == "" {
		return nil, biz("牌面尚未固定")
	}
	prompt := a.personaPrompt(ctx, tarotistID, uid) + "\n问题：" + question + "\n牌面：" + *cards + "\n追问：" + text + "\n不要改牌。把回答拆成不超过四个短气泡，气泡之间用空行分隔。"
	ctx = mergeAIBill(ctx, aiBill{UserID: uid, ReadingID: readingID, Kind: "followup"})
	_, _, output, err := a.runEinoStream(ctx, "tarotist", prompt, onChunk)
	if err != nil {
		return map[string]any{"turn_id": turn, "error": "生成失败，可重试，未重新扣额度", "cards_unchanged": true, "bubbles": []string{}}, nil
	}
	parts := strings.Split(output, "\n\n")
	bubbles := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		bubbles = append(bubbles, part)
		if len(bubbles) == 4 {
			break
		}
	}
	if len(bubbles) == 0 {
		bubbles = []string{output}
	}
	var existing int
	_ = a.Store.QueryRow(ctx, `SELECT count(*) FROM reading_messages WHERE reading_id=$1 AND turn_id=$2 AND role='assistant'`, readingID, turn).Scan(&existing)
	if existing == 0 {
		for i, bubble := range bubbles {
			if _, err := a.Store.Exec(ctx, `INSERT INTO reading_messages(reading_id, turn_id, seq, role, content) VALUES ($1,$2,$3,'assistant',$4)`, readingID, turn, i+1, bubble); err != nil {
				return nil, err
			}
		}
	}
	_, _ = a.Store.Exec(ctx, `UPDATE readings SET updated_at=now() WHERE id=$1`, readingID)
	return map[string]any{"turn_id": turn, "bubbles": bubbles, "cards_unchanged": true}, nil
}

func (a *App) personaPrompt(ctx context.Context, tarotistID *int64, uid int64) string {
	persona, voice, banned, ask, name := "", "", "", "", ""
	if tarotistID != nil {
		_ = a.Store.QueryRow(ctx, `SELECT name, persona, voice, banned, ask_style FROM tarotists WHERE id=$1`, *tarotistID).Scan(&name, &persona, &voice, &banned, &ask)
	}
	tease := false
	if uid > 0 {
		_ = a.Store.QueryRow(ctx, `SELECT tease_off FROM app_users WHERE id=$1`, uid).Scan(&tease)
	}
	b := strings.Builder{}
	b.WriteString("共用解牌：先按牌面、牌位、正逆位解释，再按人设说话。不得改牌。\n")
	b.WriteString("人设：" + name + "。" + persona + "\n语音：" + voice + "。禁止：" + banned + "。追问：" + ask + "\n")
	if name == "照野" {
		b.WriteString("照野只轻度调侃，一次最多两句。")
		if tease {
			b.WriteString("用户已要求停止调侃，本次只认真回答。")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (a *App) appQuestionSuggestions(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Question string `json:"question"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Question) == "" {
		return biz("问题不能为空")
	}
	question := strings.TrimSpace(req.Question)
	det := detectQuestion(question)
	messages := []map[string]any{{
		"kind": "interpretation", "from": "server", "style": "detect",
		"title": "塔塔 · 问题检测", "status": "已完成", "reason_title": "为什么建议修改？",
		"text": det.DetectText,
	}}
	readingID, _ := a.ensureDetectReading(r, question)
	wrap := func(extra map[string]any) map[string]any {
		out := map[string]any{"messages": messages, "has_more": false, "continue": "detect", "skip_detect": false}
		for k, v := range extra {
			out[k] = v
		}
		if readingID > 0 {
			out["reading_id"] = readingID
		}
		return out
	}
	if det.Kind != questionBroad {
		writeOK(w, wrap(nil))
		return nil
	}
	rows, err := a.Store.Query(r.Context(), `SELECT body, category, scene FROM preset_questions WHERE enabled=true
		ORDER BY CASE pool WHEN 'featured' THEN 0 ELSE 1 END, sort_order, question_id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	presets := []presetRow{}
	for rows.Next() {
		var item presetRow
		if err := rows.Scan(&item.body, &item.category, &item.scene); err != nil {
			return err
		}
		item.body = strings.TrimSpace(item.body)
		item.scene = strings.TrimSpace(item.scene)
		if item.scene == "" {
			item.scene = sceneFromCategory(item.category)
		}
		if item.body != "" {
			presets = append(presets, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	picked := refineBroadSuggestions(det, presets)
	if len(picked) == 0 {
		writeOK(w, wrap(nil))
		return nil
	}
	messages = append(messages, map[string]any{
		"kind": "suggestion", "from": "server", "style": "chip", "text": picked[0].body,
	})
	options := make([]map[string]any, 0, len(picked))
	followupOptions := make([]map[string]any, 0, len(picked))
	for i, item := range picked {
		option := map[string]any{"text": item.body, "hint": item.category, "selected": false}
		options = append(options, option)
		followupOptions = append(followupOptions, map[string]any{"text": item.body})
		if i == 1 || (i == 0 && len(picked) == 1) {
			messages = append(messages, map[string]any{
				"kind": "suggestion", "from": "server", "style": "option",
				"text": item.body, "hint": item.category, "selected": false,
			})
		}
	}
	messages = append(messages,
		map[string]any{
			"kind": "suggestion", "from": "server", "style": "followup",
			"title": "继续解读", "options": followupOptions,
		},
		map[string]any{
			"kind": "suggestion", "from": "server", "style": "recommend",
			"title":   fmt.Sprintf("为你推荐 %d 个更清晰的问题", len(options)),
			"options": options,
		},
	)
	writeOK(w, wrap(nil))
	return nil
}

// ensureDetectReading creates or refreshes a hand-input detect reading so history lists it.
// Only for logged-in users; guest detect stays message-only.
func (a *App) ensureDetectReading(r *http.Request, question string) (int64, error) {
	uid, err := a.appActor(r)
	if err != nil || uid <= 0 {
		return 0, err
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return 0, nil
	}
	cycle := a.userCycle(r, uid)
	var id int64
	err = a.Store.QueryRow(r.Context(), `SELECT id FROM readings
		WHERE user_id=$1 AND type='normal' AND skip_detect=false AND status='QUESTION_CREATED' AND deleted_at IS NULL
		ORDER BY id DESC LIMIT 1`, uid).Scan(&id)
	if err == nil && id > 0 {
		_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET question=$1, business_date=$2, updated_at=now() WHERE id=$3`, question, cycle, id)
		return id, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = a.Store.QueryRow(r.Context(), `INSERT INTO readings(user_id, type, question, status, business_date, skip_detect)
		VALUES ($1,'normal',$2,'QUESTION_CREATED',$3,false) RETURNING id`, uid, question, cycle).Scan(&id)
	return id, err
}

func (a *App) appSpreadChoices(w http.ResponseWriter, r *http.Request) error {
	_, id, _, err := a.beginReading(r)
	if err != nil {
		return err
	}
	var selected sql.NullInt64
	var question string
	if err := a.Store.QueryRow(r.Context(), `SELECT spread_id, COALESCE(question,'') FROM readings WHERE id=$1`, id).Scan(&selected, &question); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, name, description, short_sentence, card_count, recommended, thumbnail, positions FROM spreads
		WHERE enabled=true AND theme_key='' ORDER BY recommended DESC, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	spreads := []map[string]any{}
	for rows.Next() {
		var sid int64
		var name, desc, short, thumb, posRaw string
		var count int
		var recommended bool
		if err := rows.Scan(&sid, &name, &desc, &short, &count, &recommended, &thumb, &posRaw); err != nil {
			return err
		}
		var positions any
		if strings.TrimSpace(posRaw) != "" {
			positions = json.RawMessage(posRaw)
		} else {
			positions = []any{}
		}
		spreads = append(spreads, map[string]any{
			"id": sid, "name": T(r.Context(), name), "description": T(r.Context(), desc),
			"short_sentence": short, "card_count": count, "positions": positions,
			"recommended": recommended, "selected": selected.Valid && selected.Int64 == sid,
			"thumbnail": absoluteAssetURL(r, deckPreviewURL(thumb)),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(spreads) == 0 {
		return biz("牌阵不可用")
	}
	// Overlay PRD §6.4 local match onto response recommended flags so App that
	// only reads spread-choices still sees the question-matched spread first.
	_ = a.applyLocalRecommendToSpreadChoices(r, id, question, spreads)
	msg := spreadChoiceMessage(spreads)
	out := map[string]any{
		"reading_id": id,
		"has_more":   false,
		"messages":   []map[string]any{msg},
	}
	if uid, aerr := a.appActor(r); aerr == nil && uid > 0 {
		if deck := a.userDeckBackView(r, uid); deck != nil {
			out["deck"] = deck
			out["deck_back_url"] = deck["back_url"]
		}
	}
	writeOK(w, out)
	return nil
}

// userDeckBackView returns the active deck code + back_url for layout (official or custom).
func (a *App) userDeckBackView(r *http.Request, uid int64) map[string]any {
	code, decks, custom, err := a.resolveUserDeck(r.Context(), uid, r)
	if err != nil {
		return nil
	}
	if custom != nil {
		if id, ok := asInt64OK(custom["id"]); ok && id > 0 {
			d, loadErr := a.loadCustomDeck(r.Context(), uid, id)
			if loadErr == nil {
				back := emptyAsNil(absoluteAssetURL(r, strings.TrimSpace(d.BackURL)))
				return map[string]any{"code": code, "name": d.Name, "back_url": back, "custom": true}
			}
		}
	}
	for _, item := range decks {
		if fmt.Sprint(item["code"]) == code {
			return map[string]any{
				"code": code, "name": item["name"], "back_url": item["back_url"], "custom": false,
			}
		}
	}
	return nil
}

func asInt64OK(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

// applyLocalRecommendToSpreadChoices marks at most one spread as recommended
// (local match or fallback). Does not blank the list when catalog is empty.
func (a *App) applyLocalRecommendToSpreadChoices(r *http.Request, readingID int64, question string, spreads []map[string]any) int64 {
	payload := map[string]any{"question": strings.TrimSpace(question)}
	if err := a.attachRecommendation(r, payload); err != nil {
		return 0
	}
	list, _ := payload["recommended_spreads"].([]map[string]any)
	if len(list) == 0 {
		// Keep any DB recommended flags already loaded; do not wipe to all-false.
		return 0
	}
	matchedID, _ := list[0]["id"].(int64)
	if matchedID <= 0 {
		return 0
	}
	matchedIdx := -1
	for i, item := range spreads {
		id, _ := item["id"].(int64)
		isMatch := id == matchedID
		item["recommended"] = isMatch
		if isMatch {
			matchedIdx = i
		}
	}
	if matchedIdx > 0 {
		matched := spreads[matchedIdx]
		copy(spreads[1:matchedIdx+1], spreads[0:matchedIdx])
		spreads[0] = matched
	}
	a.rememberRecommendation(r.Context(), readingID, payload)
	return matchedID
}

func spreadChoiceTitle() string {
	return "先确认牌阵和张数，再选择抽牌方式。"
}

func conversationDrawModes() []map[string]any {
	return []map[string]any{
		{"id": "normal", "label": "常规抽牌", "hint": "通过感应抽牌，开启你的塔罗探索。"},
		{"id": "numerology", "label": "灵数抽牌", "hint": "输入专属数字，揭晓属于你的塔罗牌。"},
	}
}

func spreadChoiceMessage(spreads []map[string]any) map[string]any {
	return map[string]any{
		"kind": "cards", "from": "server", "style": "choose",
		"title":           spreadChoiceTitle(),
		"draw_mode_label": "抽牌方式", "draw_modes": conversationDrawModes(),
		"spreads": spreads,
	}
}

func numerologyGuide(cardCount int) map[string]any {
	return map[string]any{
		"kind": "interpretation", "from": "server", "style": "guide",
		"title": "输入专属数字", "placeholder": "例如：12、58、62", "card_count": cardCount,
		"input_rows": numerologyInputRows(cardCount),
		"rules":      []string{"按牌位的顺序输入，数字之间用顿号隔开。", "输入范围：1-78的整数，数字不可重复。"},
	}
}

func numerologyInputRows(cardCount int) int {
	if cardCount <= 6 {
		return 1
	}
	return (cardCount + 5) / 6
}

func (a *App) appNumerologyMessages(w http.ResponseWriter, r *http.Request) error {
	_, id, _, err := a.beginReading(r)
	if err != nil {
		return err
	}
	var req struct {
		Numbers []int `json:"numbers"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	var posRaw, drawMode string
	var cardCount int
	err = a.Store.QueryRow(r.Context(), `SELECT s.card_count, s.positions, r.draw_mode FROM readings r JOIN spreads s ON s.id=r.spread_id WHERE r.id=$1`, id).Scan(&cardCount, &posRaw, &drawMode)
	if err == nil && drawMode != "numerology" {
		if drawMode == "" {
			return biz("请先选择抽牌方式")
		}
		return biz("抽牌方式不正确")
	}
	if err != nil {
		return biz("请先选择牌阵")
	}
	var positions []position
	if err := json.Unmarshal([]byte(posRaw), &positions); err != nil {
		return err
	}
	messages := []map[string]any{}
	if len(req.Numbers) == 0 {
		messages = append(messages, numerologyGuide(cardCount))
		writeOK(w, map[string]any{"reading_id": id, "messages": messages, "has_more": false})
		return nil
	}
	slots := make([]map[string]any, cardCount)
	counts := map[int]int{}
	for _, n := range req.Numbers {
		if n >= 1 && n <= 78 {
			counts[n]++
		}
	}
	seen := map[int]int{}
	validCount := 0
	for i := 0; i < cardCount; i++ {
		name := ""
		if i < len(positions) {
			name = positions[i].Name
		}
		number := ""
		mark := "待选择"
		invalid := false
		if i < len(req.Numbers) && req.Numbers[i] != 0 {
			n := req.Numbers[i]
			number = strconv.Itoa(n)
			switch {
			case n < 1 || n > 78:
				mark = "超出范围"
				invalid = true
			case seen[n] > 0:
				mark = "重复数字"
				invalid = true
			default:
				mark = "已选择"
				seen[n] = i + 1
				validCount++
			}
		}
		slots[i] = map[string]any{
			"position": i + 1, "position_name": name,
			"number": number, "mark": mark, "invalid": invalid,
		}
	}
	messages = append(messages, map[string]any{"kind": "cards", "from": "server", "style": "pick", "card_count": cardCount, "slots": slots})
	numbers := make([]string, 0, len(req.Numbers))
	for _, n := range req.Numbers {
		if n != 0 {
			numbers = append(numbers, strconv.Itoa(n))
		}
	}
	if validCount < cardCount {
		messages = append(messages, map[string]any{
			"kind": "interpretation", "from": "server", "style": "error",
			"text":    fmt.Sprintf("请再输入 %d 个数字", cardCount-validCount),
			"numbers": numbers,
		})
	}
	duplicate := false
	for _, count := range counts {
		if count > 1 {
			duplicate = true
			break
		}
	}
	for n, count := range counts {
		if count > 1 {
			messages = append(messages, map[string]any{
				"kind": "interpretation", "from": "server", "style": "error",
				"text":    fmt.Sprintf("数字 %d 已被使用，请更换一个数字。", n),
				"numbers": numbers,
			})
			break
		}
	}
	out := map[string]any{"reading_id": id, "messages": messages, "has_more": false}
	if validCount == cardCount && !duplicate {
		if err := a.fixNumerologyCards(r.Context(), id, req.Numbers, positions); err != nil {
			return err
		}
		if item, loadErr := a.loadReadingAbs(r.Context(), r, id); loadErr == nil {
			out["reading"] = item
			out["status"] = item["status"]
			out["continue"] = item["continue"]
			out["stage"] = item["stage"]
			out["draw_mode"] = item["draw_mode"]
		}
	}
	writeOK(w, out)
	return nil
}

func (a *App) conversationReply(r *http.Request, id int64) (map[string]any, error) {
	messages, err := a.loadConversation(r, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"reading_id": id, "messages": messages, "has_more": false}, nil
}

func (a *App) attachConversation(r *http.Request, id int64, body map[string]any) error {
	view, err := a.conversationReply(r, id)
	if err != nil {
		return err
	}
	for k, v := range view {
		body[k] = v
	}
	return nil
}

func pageMessages(all []map[string]any, limit, offset int) ([]map[string]any, bool) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(all) {
		offset = len(all)
	}
	rest := all[offset:]
	if limit <= 0 || limit >= len(rest) {
		return rest, false
	}
	return rest[:limit], true
}

func (a *App) loadConversation(r *http.Request, id int64) ([]map[string]any, error) {
	var question, cardsRaw, interpretation, readingType string
	var spreadCount int
	if err := a.Store.QueryRow(r.Context(), `SELECT type, question, COALESCE(cards,''), COALESCE(interpretation,''),
		COALESCE((SELECT card_count FROM spreads WHERE id=readings.spread_id), 0)
		FROM readings WHERE id=$1`, id).Scan(&readingType, &question, &cardsRaw, &interpretation, &spreadCount); err != nil {
		return nil, err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT turn_id, seq, role, kind, style, content FROM reading_messages WHERE reading_id=$1 ORDER BY turn_id, seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type storedMessage struct {
		turn           int64
		seq            int
		role, kind     string
		style, content string
	}
	stored := []storedMessage{}
	hasClarifier := false
	for rows.Next() {
		var item storedMessage
		if err := rows.Scan(&item.turn, &item.seq, &item.role, &item.kind, &item.style, &item.content); err != nil {
			return nil, err
		}
		if item.style == "single" {
			hasClarifier = true
		}
		stored = append(stored, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	messages := []map[string]any{}
	// Daily stores system slot names as question; do not surface them as「你的问题」above interpretation.
	if text := strings.TrimSpace(question); text != "" && readingType != "daily" {
		messages = append(messages, labeledQuestion(text))
	}
	cards := a.conversationCards(r, cardsRaw)
	if hasClarifier && spreadCount > 0 && len(cards) > spreadCount {
		cards = cards[:spreadCount]
	}
	if len(cards) > 0 {
		messages = append(messages, spreadCards(cards))
	}
	if text := strings.TrimSpace(interpretation); text != "" {
		messages = append(messages, interpretationBody(text))
	}
	for _, item := range stored {
		if msg := a.storedConversationMessage(r, item.turn, item.seq, item.role, item.kind, item.style, item.content); msg != nil {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

func labeledQuestion(text string) map[string]any {
	return map[string]any{"kind": "question", "from": "user", "style": "labeled", "label": "你的问题", "text": text}
}

func userBubble(turn int64, text string) map[string]any {
	return map[string]any{"kind": "question", "from": "user", "style": "bubble", "turn_id": turn, "text": text}
}

func interpretationBody(text string) map[string]any {
	return map[string]any{"kind": "interpretation", "from": "server", "style": "body", "text": text}
}

func followupBody(turn int64, seq int, text string) map[string]any {
	item := interpretationBody(text)
	item["turn_id"] = turn
	item["seq"] = seq
	return item
}

func spreadCards(cards []map[string]any) map[string]any {
	return map[string]any{"kind": "cards", "from": "server", "style": "spread", "title": "牌面预览", "cards": cards}
}

func singleCard(turn int64, cards []map[string]any) map[string]any {
	return map[string]any{"kind": "cards", "from": "server", "style": "single", "title": "牌面", "turn_id": turn, "cards": cards}
}

func redrawActions() []map[string]any {
	return []map[string]any{
		{"action": "draw", "label": "去抽牌"},
		{"action": "skip", "label": "跳过"},
	}
}

func (a *App) storedConversationMessage(r *http.Request, turn int64, seq int, role, kind, style, content string) map[string]any {
	switch style {
	case "redraw":
		return redrawOfferView(content, turn)
	case "single":
		var card drawnCard
		if err := json.Unmarshal([]byte(content), &card); err != nil {
			return nil
		}
		return singleCard(turn, a.conversationCardItems(r, []drawnCard{card}))
	case "chip", "option", "followup", "recommend", "detect", "guide", "error", "pick", "choose":
		return structuredConversationMessage(turn, style, content)
	case "bubble":
		text := strings.TrimSpace(content)
		if text == "" || role != "user" {
			return nil
		}
		return userBubble(turn, text)
	case "body":
		text := strings.TrimSpace(content)
		if text == "" {
			return nil
		}
		return followupBody(turn, seq, text)
	}
	text := strings.TrimSpace(content)
	if text == "" || strings.HasPrefix(text, "{") {
		return nil
	}
	switch role {
	case "user":
		return userBubble(turn, text)
	case "assistant":
		return followupBody(turn, seq, text)
	default:
		return nil
	}
}

func structuredConversationMessage(turn int64, style, content string) map[string]any {
	switch style {
	case "chip":
		var body struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(content), &body) != nil || strings.TrimSpace(body.Text) == "" {
			return nil
		}
		return map[string]any{"kind": "suggestion", "from": "server", "style": "chip", "turn_id": turn, "text": strings.TrimSpace(body.Text)}
	case "option":
		var body struct {
			Text     string `json:"text"`
			Hint     string `json:"hint"`
			Selected bool   `json:"selected"`
		}
		if json.Unmarshal([]byte(content), &body) != nil || strings.TrimSpace(body.Text) == "" {
			return nil
		}
		return map[string]any{
			"kind": "suggestion", "from": "server", "style": "option", "turn_id": turn,
			"text": strings.TrimSpace(body.Text), "hint": body.Hint, "selected": body.Selected,
		}
	case "followup":
		var body struct {
			Text    string `json:"text"`
			Options []struct {
				Text string `json:"text"`
			} `json:"options"`
		}
		if json.Unmarshal([]byte(content), &body) != nil {
			return nil
		}
		options := make([]map[string]any, 0, len(body.Options))
		for _, option := range body.Options {
			text := strings.TrimSpace(option.Text)
			if text != "" {
				options = append(options, map[string]any{"text": text})
			}
		}
		text := strings.TrimSpace(body.Text)
		if text != "" && len(options) == 0 {
			options = append(options, map[string]any{"text": text})
		}
		if len(options) == 0 {
			return nil
		}
		if text == "" {
			text = options[0]["text"].(string)
		}
		return map[string]any{
			"kind": "suggestion", "from": "server", "style": "followup", "turn_id": turn,
			"title": "继续解读", "text": text, "options": options,
		}
	case "recommend":
		var body struct {
			Title   string `json:"title"`
			Options []struct {
				Text     string `json:"text"`
				Hint     string `json:"hint"`
				Selected bool   `json:"selected"`
			} `json:"options"`
		}
		if json.Unmarshal([]byte(content), &body) != nil || len(body.Options) == 0 {
			return nil
		}
		options := make([]map[string]any, 0, len(body.Options))
		for _, option := range body.Options {
			if strings.TrimSpace(option.Text) == "" {
				continue
			}
			options = append(options, map[string]any{"text": strings.TrimSpace(option.Text), "hint": option.Hint, "selected": option.Selected})
		}
		if len(options) == 0 {
			return nil
		}
		title := strings.TrimSpace(body.Title)
		if title == "" {
			title = fmt.Sprintf("为你推荐 %d 个更清晰的问题", len(options))
		}
		return map[string]any{"kind": "suggestion", "from": "server", "style": "recommend", "turn_id": turn, "title": title, "options": options}
	case "detect":
		var body struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(content), &body) != nil || strings.TrimSpace(body.Text) == "" {
			return nil
		}
		return map[string]any{
			"kind": "interpretation", "from": "server", "style": "detect", "turn_id": turn,
			"title": "塔塔 · 问题检测", "status": "已完成", "reason_title": "为什么建议修改？", "text": strings.TrimSpace(body.Text),
		}
	case "guide":
		var body struct {
			Placeholder string   `json:"placeholder"`
			Rules       []string `json:"rules"`
		}
		if json.Unmarshal([]byte(content), &body) != nil {
			return nil
		}
		placeholder := strings.TrimSpace(body.Placeholder)
		if placeholder == "" {
			placeholder = "例如：12、58、62"
		}
		rules := body.Rules
		if rules == nil {
			rules = []string{}
		}
		return map[string]any{
			"kind": "interpretation", "from": "server", "style": "guide", "turn_id": turn,
			"title": "输入专属数字", "placeholder": placeholder, "rules": rules,
		}
	case "error":
		var body struct {
			Text    string   `json:"text"`
			Numbers []string `json:"numbers"`
		}
		if json.Unmarshal([]byte(content), &body) != nil || strings.TrimSpace(body.Text) == "" {
			return nil
		}
		numbers := body.Numbers
		if numbers == nil {
			numbers = []string{}
		}
		return map[string]any{
			"kind": "interpretation", "from": "server", "style": "error", "turn_id": turn,
			"text": strings.TrimSpace(body.Text), "numbers": numbers,
		}
	case "pick":
		var body struct {
			Slots []struct {
				Position     int    `json:"position"`
				PositionName string `json:"position_name"`
				Number       string `json:"number"`
				Invalid      bool   `json:"invalid"`
			} `json:"slots"`
		}
		if json.Unmarshal([]byte(content), &body) != nil || len(body.Slots) == 0 {
			return nil
		}
		slots := make([]map[string]any, 0, len(body.Slots))
		for _, slot := range body.Slots {
			slots = append(slots, map[string]any{
				"position": slot.Position, "position_name": slot.PositionName,
				"number": slot.Number, "invalid": slot.Invalid,
			})
		}
		return map[string]any{"kind": "cards", "from": "server", "style": "pick", "turn_id": turn, "slots": slots}
	case "choose":
		var body struct {
			Spreads []struct {
				ID          int64  `json:"id"`
				Name        string `json:"name"`
				CardCount   int    `json:"card_count"`
				Recommended bool   `json:"recommended"`
				Selected    bool   `json:"selected"`
			} `json:"spreads"`
		}
		if json.Unmarshal([]byte(content), &body) != nil || len(body.Spreads) == 0 {
			return nil
		}
		spreads := make([]map[string]any, 0, len(body.Spreads))
		for _, spread := range body.Spreads {
			spreads = append(spreads, map[string]any{
				"id": spread.ID, "name": spread.Name, "card_count": spread.CardCount,
				"recommended": spread.Recommended, "selected": spread.Selected,
			})
		}
		item := spreadChoiceMessage(spreads)
		item["turn_id"] = turn
		return item
	default:
		return nil
	}
}

func (a *App) conversationCards(r *http.Request, raw string) []map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var cards []drawnCard
	if err := json.Unmarshal([]byte(raw), &cards); err != nil || len(cards) == 0 {
		return nil
	}
	return a.conversationCardItems(r, cards)
}

func (a *App) conversationCardItems(r *http.Request, cards []drawnCard) []map[string]any {
	cfg := a.cardConfig(r.Context())
	if r != nil {
		if uid, err := a.appActor(r); err == nil && uid > 0 {
			cfg = a.cardConfigForUser(r.Context(), uid)
		}
	}
	out := make([]map[string]any, 0, len(cards))
	for _, card := range cards {
		image := ""
		if resolved, ok := absoluteAssetAny(r, resolveCardThumbnail(cfg, card.Name)).(string); ok {
			image = resolved
		}
		item := map[string]any{
			"position":      card.Position,
			"position_name": card.PositionName,
			"card_id":       card.CardID,
			"name":          card.Name,
			"orientation":   card.Orientation,
			"image":         image,
		}
		if card.Number > 0 {
			item["number"] = card.Number
		}
		out = append(out, item)
	}
	return out
}

type redrawOffer struct {
	Title   string   `json:"title"`
	Actions []string `json:"actions"`
	Choice  string   `json:"choice"`
}

func (a *App) attachRedrawOffer(ctx context.Context, readingID, turn int64, body map[string]any) error {
	offer, err := a.ensureRedrawOffer(ctx, readingID, turn)
	if err != nil || offer == nil {
		return err
	}
	body["redraw"] = offer
	return nil
}

func (a *App) ensureRedrawOffer(ctx context.Context, readingID, turn int64) (map[string]any, error) {
	var kind, drawMode string
	if err := a.Store.QueryRow(ctx, `SELECT type, draw_mode FROM readings WHERE id=$1`, readingID).Scan(&kind, &drawMode); err != nil {
		return nil, err
	}
	if kind != "normal" || drawMode == "numerology" {
		return nil, nil
	}
	var raw string
	err := a.Store.QueryRow(ctx, `SELECT content FROM reading_messages WHERE reading_id=$1 AND turn_id=$2 AND style='redraw'`, readingID, turn).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		var seq int
		if err := a.Store.QueryRow(ctx, `SELECT COALESCE(max(seq),0)+1 FROM reading_messages WHERE reading_id=$1 AND turn_id=$2`, readingID, turn).Scan(&seq); err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(redrawOffer{Title: "补抽牌面", Actions: []string{"draw", "skip"}})
		if _, err := a.Store.Exec(ctx, `INSERT INTO reading_messages(reading_id, turn_id, seq, role, kind, style, content)
			VALUES ($1,$2,$3,'assistant','suggestion','redraw',$4)`, readingID, turn, seq, string(payload)); err != nil {
			return nil, err
		}
		raw = string(payload)
	}
	return redrawOfferView(raw, turn), nil
}

func redrawOfferView(raw string, turn int64) map[string]any {
	var prompt redrawOffer
	if err := json.Unmarshal([]byte(raw), &prompt); err != nil {
		return nil
	}
	title := strings.TrimSpace(prompt.Title)
	if title == "" {
		title = "补抽牌面"
	}
	item := map[string]any{
		"kind": "suggestion", "from": "server", "style": "redraw", "turn_id": turn,
		"title": title, "actions": redrawActions(), "pending": prompt.Choice == "",
	}
	if prompt.Choice != "" {
		item["choice"] = prompt.Choice
	}
	return item
}

func (a *App) attachPendingRedraw(ctx context.Context, readingID int64, item map[string]any) error {
	offer, ok, err := a.pendingRedraw(ctx, readingID)
	if err != nil {
		return err
	}
	item["redraw_pending"] = ok
	if ok {
		item["redraw"] = offer
	}
	return nil
}

func (a *App) pendingRedraw(ctx context.Context, readingID int64) (map[string]any, bool, error) {
	var turn int64
	var raw string
	err := a.Store.QueryRow(ctx, `SELECT turn_id, content FROM reading_messages
		WHERE reading_id=$1 AND style='redraw' AND COALESCE(content::json->>'choice','')=''
		ORDER BY turn_id DESC LIMIT 1`, readingID).Scan(&turn, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	offer := redrawOfferView(raw, turn)
	return offer, offer != nil, nil
}

func (a *App) markPendingRedraws(ctx context.Context, items []map[string]any) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		id, ok := item["id"].(int64)
		if ok && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	holders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		holders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	rows, err := a.Store.Query(ctx, `SELECT DISTINCT reading_id FROM reading_messages
		WHERE style='redraw' AND COALESCE(content::json->>'choice','')=''
		AND reading_id IN (`+strings.Join(holders, ",")+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	pending := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		pending[id] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		id, _ := item["id"].(int64)
		item["redraw_pending"] = pending[id]
	}
	return nil
}

func (a *App) appFollowupRedraw(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, ""); err != nil {
		return err
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := decode(r, &req); err != nil || (req.Action != "draw" && req.Action != "skip") {
		return biz("当前状态不能补抽")
	}
	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var turn, msgID int64
	var raw, cardsRaw string
	err = tx.QueryRow(r.Context(), `SELECT m.id, m.turn_id, m.content, COALESCE(r.cards,'')
		FROM reading_messages m
		JOIN readings r ON r.id=m.reading_id
		WHERE m.reading_id=$1 AND m.style='redraw' AND r.type='normal' AND r.draw_mode<>'numerology'
		ORDER BY m.turn_id DESC LIMIT 1 FOR UPDATE OF m`, id).Scan(&msgID, &turn, &raw, &cardsRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return biz("当前状态不能补抽")
	}
	if err != nil {
		return err
	}
	var prompt redrawOffer
	if err := json.Unmarshal([]byte(raw), &prompt); err != nil || prompt.Choice != "" {
		return biz("当前状态不能补抽")
	}
	prompt.Choice = req.Action
	updated, _ := json.Marshal(prompt)
	if _, err := tx.Exec(r.Context(), `UPDATE reading_messages SET content=$1 WHERE id=$2`, string(updated), msgID); err != nil {
		return err
	}
	body := map[string]any{"redraw": redrawOfferView(string(updated), turn)}
	if req.Action == "draw" {
		card, cards, err := appendClarifier(cardsRaw)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE readings SET cards=$1, updated_at=now() WHERE id=$2`, mustJSON(cards), id); err != nil {
			return err
		}
		var seq int
		if err := tx.QueryRow(r.Context(), `SELECT COALESCE(max(seq),0)+1 FROM reading_messages WHERE reading_id=$1 AND turn_id=$2`, id, turn).Scan(&seq); err != nil {
			return err
		}
		payload, _ := json.Marshal(card)
		if _, err := tx.Exec(r.Context(), `INSERT INTO reading_messages(reading_id, turn_id, seq, role, kind, style, content)
			VALUES ($1,$2,$3,'assistant','cards','single',$4)`, id, turn, seq, string(payload)); err != nil {
			return err
		}
		body["card"] = singleCard(turn, a.conversationCardItems(r, []drawnCard{card}))
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	if err := a.attachConversation(r, id, body); err != nil {
		return err
	}
	writeOK(w, body)
	return nil
}

func appendClarifier(raw string) (drawnCard, []drawnCard, error) {
	var cards []drawnCard
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cards); err != nil {
			return drawnCard{}, nil, err
		}
	}
	if len(cards) >= 78 {
		return drawnCard{}, nil, biz("78 张牌已全部使用")
	}
	skip := map[int]bool{}
	for _, card := range cards {
		if card.CardID > 0 {
			skip[card.CardID] = true
			continue
		}
		if def, ok := cardByName(card.Name); ok {
			skip[def.ID] = true
		}
	}
	added, err := drawCardsSkipping([]position{{Index: len(cards) + 1}}, skip)
	if err != nil {
		return drawnCard{}, nil, err
	}
	cards = append(cards, added[0])
	return added[0], cards, nil
}

func (a *App) appCalendar(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能看日历")
	}
	month := r.URL.Query().Get("month")
	typ := r.URL.Query().Get("type")
	if typ != "" && typ != "normal" && typ != "tarotist" && typ != "explore" && typ != "daily" {
		return biz("type 不正确")
	}
	q := `SELECT r.id, r.type, r.question, r.status, r.business_date, COALESCE(r.target_date,''), r.updated_at, r.created_at, r.cards, r.interpretation, r.tarotist_id, COALESCE(r.theme_key,''), COALESCE(r.draw_mode,''), r.skip_detect, COALESCE(t.name,''), COALESCE(t.avatar_url,'')
FROM readings r
LEFT JOIN tarotists t ON t.id = r.tarotist_id
WHERE r.user_id=$1 AND r.deleted_at IS NULL AND r.business_date<>''`
	args := []any{uid}
	if month != "" {
		q += ` AND r.business_date LIKE $` + strconv.Itoa(len(args)+1)
		args = append(args, month+"%")
	}
	if typ != "" {
		q += ` AND r.type=$` + strconv.Itoa(len(args)+1)
		args = append(args, typ)
	}
	q += ` ORDER BY r.business_date, r.id`
	rows, err := a.Store.Query(r.Context(), q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	cfg := a.cardConfigForUser(r.Context(), uid)
	cycle := a.userCycle(r, uid)
	for rows.Next() {
		var id int64
		var typ, question, status, day, targetDate, themeKey, drawMode string
		var skipDetect bool
		var updated, created time.Time
		var cards, interp *string
		var tarotistID *int64
		var tarotistName, tarotistAvatar string
		if err := rows.Scan(&id, &typ, &question, &status, &day, &targetDate, &updated, &created, &cards, &interp, &tarotistID, &themeKey, &drawMode, &skipDetect, &tarotistName, &tarotistAvatar); err != nil {
			return err
		}
		label, next := progressOfReading(status, skipDetect, drawMode)
		item := map[string]any{
			"id": id, "type": typ, "question": question, "status": status, "business_date": day,
			"target_date": targetDate,
			"theme_key":   themeKey,
			"draw_mode":   drawMode,
			"skip_detect": skipDetect,
			"has_result":  status == "COMPLETED" || status == "CARDS_FIXED" || status == "INTERPRETING" || status == "FAILED_RETRYABLE",
			"progress":    label, "continue": next, "updated_at": updated, "created_at": created,
			"cards": cards, "interpretation": interp, "tarotist_id": tarotistID,
		}
		attachConversationMeta(item, cycle)
		attachCalendarPreview(r, item, cfg, tarotistName, tarotistAvatar)
		list = append(list, item)
	}
	tomorrow, err := nextBusinessDate(cycle)
	if err != nil {
		return err
	}
	out := map[string]any{
		"days": list, "cycle": cycle, "tomorrow_date": tomorrow,
	}
	// Fortune-day lookup for home/calendar day detail: reuse advance-drawn rows by target_date
	// without moving the draw-day bucket in days[].
	if cycleDaily := a.calendarDailyByFortuneDay(r, uid, cycle, cfg); cycleDaily != nil {
		out["cycle_daily"] = cycleDaily
	}
	if tomorrowDaily := a.calendarDailyByFortuneDay(r, uid, tomorrow, cfg); tomorrowDaily != nil {
		out["tomorrow_daily"] = tomorrowDaily
	}
	writeOK(w, out)
	return nil
}

// calendarDailyByFortuneDay returns one daily reading whose fortune day is fortuneDay (target_date reuse).
func (a *App) calendarDailyByFortuneDay(r *http.Request, uid int64, fortuneDay string, cfg map[string]string) map[string]any {
	prevDay, err := prevBusinessDate(fortuneDay)
	if err != nil {
		return nil
	}
	var id int64
	var typ, question, status, day, targetDate, themeKey, drawMode string
	var skipDetect bool
	var updated, created time.Time
	var cards, interp *string
	var tarotistID *int64
	var tarotistName, tarotistAvatar string
	err = a.Store.QueryRow(r.Context(), `SELECT r.id, r.type, r.question, r.status, r.business_date, COALESCE(r.target_date,''), r.updated_at, r.created_at, r.cards, r.interpretation, r.tarotist_id, COALESCE(r.theme_key,''), COALESCE(r.draw_mode,''), r.skip_detect, COALESCE(t.name,''), COALESCE(t.avatar_url,'')
		FROM readings r
		LEFT JOIN tarotists t ON t.id = r.tarotist_id
		WHERE r.user_id=$1 AND r.type='daily' AND r.deleted_at IS NULL AND r.cards IS NOT NULL AND r.cards<>''
			AND `+dailyFortuneMatchSQL+`
		ORDER BY CASE WHEN r.target_date=$2 THEN 0 ELSE 1 END, r.id DESC LIMIT 1`, uid, fortuneDay, prevDay).
		Scan(&id, &typ, &question, &status, &day, &targetDate, &updated, &created, &cards, &interp, &tarotistID, &themeKey, &drawMode, &skipDetect, &tarotistName, &tarotistAvatar)
	if err != nil {
		return nil
	}
	label, next := progressOfReading(status, skipDetect, drawMode)
	item := map[string]any{
		"id": id, "type": typ, "question": question, "status": status, "business_date": day,
		"target_date": targetDate, "theme_key": themeKey, "draw_mode": drawMode, "skip_detect": skipDetect,
		"has_result": true, "progress": label, "continue": next, "updated_at": updated, "created_at": created,
		"cards": cards, "interpretation": interp, "tarotist_id": tarotistID,
	}
	cycle := a.userCycle(r, uid)
	attachConversationMeta(item, cycle)
	attachCalendarPreview(r, item, cfg, tarotistName, tarotistAvatar)
	// Home / day-detail pointers need the full解读正文; days[] list still strips it.
	if interp != nil {
		item["interpretation"] = *interp
	} else {
		item["interpretation"] = nil
	}
	return item
}

func (a *App) appConfirmQuota(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能确认额度")
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, ""); err != nil {
		return err
	}
	already, ent, err := playAlready(r.Context(), a.Store, id)
	if err != nil {
		return err
	}
	if already {
		writeOK(w, map[string]any{"quota_consumed": true, "entitlement": ent, "message": T(r.Context(), "已经扣过，不再扣")})
		return nil
	}
	var req struct {
		Consume bool `json:"consume"`
	}
	_ = decode(r, &req)
	if !req.Consume {
		writeOK(w, map[string]any{"quota_consumed": false, "message": T(r.Context(), "未确认，不扣额度，记录已保留")})
		return nil
	}
	decision, err := a.chargeOwned(r, id, chargeKeyPlay, resumeHold)
	if err != nil {
		return err
	}
	if !decision.Continue {
		writeQuotaBlock(w, r, decision, id)
		return nil
	}
	writeOK(w, map[string]any{
		"quota_consumed": true, "entitlement": decision.Entitlement, "quota_outcome": decision.Status,
		"message": quotaClosedNote,
	})
	return nil
}

func (a *App) appUseReward(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能使用注册奖励")
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	decision, err := a.chargeOwned(r, id, chargeKeyPlay, resumeHold)
	if err != nil {
		return err
	}
	if !decision.Continue {
		writeQuotaBlock(w, r, decision, id)
		return nil
	}
	writeOK(w, map[string]any{
		"reward_consumed": decision.Entitlement == entReward,
		"already":         decision.Status == quotaAlready,
		"entitlement":     decision.Entitlement,
		"quota_outcome":   decision.Status,
		"note":            quotaClosedNote,
	})
	return nil
}

func (a *App) adminListUsers(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, email, wechat_openid, apple_sub, phone,
		COALESCE(NULLIF(timezone,''),'Asia/Shanghai'), tarotist_trial_used, tease_off, cancelled_at, created_at,
		COALESCE(name,''), COALESCE(avatar_url,''), gender, birth_date, COALESCE(explore_preferences,'[]')
		FROM app_users ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var email, openid, apple, phone, gender, birth *string
		var tz, name, avatar, prefs string
		var trial, tease bool
		var cancelled *time.Time
		var created time.Time
		if err := rows.Scan(&id, &email, &openid, &apple, &phone, &tz, &trial, &tease, &cancelled, &created, &name, &avatar, &gender, &birth, &prefs); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "email": email, "wechat_openid": openid, "apple_sub": apple, "phone": phone,
			"timezone": tz, "tarotist_trial_used": trial, "tease_off": tease,
			"cancelled_at": cancelled, "created_at": created,
			"name": name, "nickname": name, "avatar_url": profileAvatarURL(r, avatar),
			"gender": gender, "birth_date": birth, "explore_preferences": parseExplorePreferences(prefs),
		})
	}
	writeOK(w, list)
	return nil
}

func (a *App) adminListAudit(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, operator_id, action, object_type, object_id, detail, created_at, actor_kind FROM audit_log ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, op int64
		var action, objectType, objectID, detail, actorKind string
		var created time.Time
		if err := rows.Scan(&id, &op, &action, &objectType, &objectID, &detail, &created, &actorKind); err != nil {
			return err
		}
		list = append(list, map[string]any{"id": id, "operator_id": op, "action": action, "object_type": objectType, "object_id": objectID, "detail": detail, "actor_kind": actorKind, "created_at": created})
	}
	writeOK(w, list)
	return nil
}

func (a *App) adminListMessages(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, reading_id, turn_id, seq, role, content, created_at FROM reading_messages ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, readingID, turn int64
		var seq int
		var role, content string
		var created time.Time
		if err := rows.Scan(&id, &readingID, &turn, &seq, &role, &content, &created); err != nil {
			return err
		}
		list = append(list, map[string]any{"id": id, "reading_id": readingID, "turn_id": turn, "seq": seq, "role": role, "content": content, "created_at": created})
	}
	writeOK(w, list)
	return nil
}

func (a *App) adminListWishlists(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, user_id, content, image_url, created_at FROM wishlists ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, userID int64
		var content, imageURL string
		var created time.Time
		if err := rows.Scan(&id, &userID, &content, &imageURL, &created); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "user_id": userID, "content": content, "image_url": imageURL, "created_at": created,
		})
	}
	writeOK(w, list)
	return nil
}

func (a *App) adminListFeedback(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, user_id, category, content, image_url, email, created_at FROM feedback ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, userID int64
		var category, content, imageURL, email string
		var created time.Time
		if err := rows.Scan(&id, &userID, &category, &content, &imageURL, &email, &created); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "user_id": userID, "category": category, "content": content,
			"image_url": imageURL, "email": email, "created_at": created,
		})
	}
	writeOK(w, list)
	return nil
}

func (a *App) adminListCalendar(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, user_id, type, question, status, business_date FROM readings WHERE deleted_at IS NULL AND business_date<>'' ORDER BY business_date DESC, id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var userID *int64
		var typ, question, status, day string
		if err := rows.Scan(&id, &userID, &typ, &question, &status, &day); err != nil {
			return err
		}
		label, _ := progressOf(status)
		list = append(list, map[string]any{"id": id, "user_id": userID, "type": typ, "question": question, "status": status, "progress": label, "business_date": day})
	}
	writeOK(w, list)
	return nil
}
