package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func progressOf(status string) (label, next string) {
	switch status {
	case "QUESTION_CREATED":
		return "问题已提交", "detect"
	case "SPREAD_SELECTED":
		return "牌阵已选择", "shuffle"
	case "SHUFFLING":
		return "洗牌中", "cut"
	case "CUTTING":
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
	tz := "Asia/Shanghai"
	if uid > 0 {
		_ = a.Store.QueryRow(r.Context(), `SELECT timezone FROM app_users WHERE id=$1`, uid).Scan(&tz)
	}
	return cycleFor(time.Now(), tz)
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
	var req struct {
		OpenID string `json:"openid"`
	}
	if err := decode(r, &req); err != nil || req.OpenID == "" {
		writeErr(w, http.StatusOK, "微信身份不能为空")
		return
	}
	var id int64
	var cancelled *time.Time
	err := a.Store.QueryRow(r.Context(), `SELECT id, cancelled_at FROM app_users WHERE wechat_openid=$1`, req.OpenID).Scan(&id, &cancelled)
	if err != nil {
		if a.claimed(r, "wechat", req.OpenID) {
			writeErr(w, http.StatusOK, "该微信已注销，不能重复领取注册奖励")
			return
		}
		err = a.Store.QueryRow(r.Context(), `INSERT INTO app_users(wechat_openid) VALUES ($1) RETURNING id`, req.OpenID).Scan(&id)
		if err != nil {
			writeErr(w, http.StatusOK, "微信登录失败")
			return
		}
		if !a.claimed(r, "wechat", req.OpenID) {
			exp := nextBeijingEight(time.Now())
			_, _ = a.Store.Exec(r.Context(), `INSERT INTO rewards(user_id, granted_at, expires_at) VALUES ($1, now(), $2) ON CONFLICT DO NOTHING`, id, exp)
		}
	}
	if cancelled != nil {
		writeErr(w, http.StatusOK, "账号已注销")
		return
	}
	access, _ := a.signAccess("app", id, "", "")
	settled := a.settleQuestionStart(r, id)
	writeOK(w, map[string]any{"accessToken": access, "id": id, "reward_consumed": settled, "reward_note": wechatQuotaNote})
}

func (a *App) appApple(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sub string `json:"sub"`
	}
	if err := decode(r, &req); err != nil || req.Sub == "" {
		writeErr(w, http.StatusOK, "苹果身份不能为空")
		return
	}
	var id int64
	var cancelled *time.Time
	err := a.Store.QueryRow(r.Context(), `SELECT id, cancelled_at FROM app_users WHERE apple_sub=$1`, req.Sub).Scan(&id, &cancelled)
	if err != nil {
		if a.claimed(r, "apple", req.Sub) {
			writeErr(w, http.StatusOK, "该苹果账号已注销，不能重复领取注册奖励")
			return
		}
		err = a.Store.QueryRow(r.Context(), `INSERT INTO app_users(apple_sub) VALUES ($1) RETURNING id`, req.Sub).Scan(&id)
		if err != nil {
			writeErr(w, http.StatusOK, "苹果登录失败")
			return
		}
		exp := nextBeijingEight(time.Now())
		_, _ = a.Store.Exec(r.Context(), `INSERT INTO rewards(user_id, granted_at, expires_at) VALUES ($1, now(), $2) ON CONFLICT DO NOTHING`, id, exp)
	}
	if cancelled != nil {
		writeErr(w, http.StatusOK, "账号已注销")
		return
	}
	access, _ := a.signAccess("app", id, "", "")
	writeOK(w, map[string]any{"accessToken": access, "id": id, "login": "apple"})
}

func (a *App) appBindApple(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Sub string `json:"sub"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Sub == "" {
		return biz("苹果身份不能为空")
	}
	return a.bindWithConfirm(w, r, bindSpec{kind: "apple", value: req.Sub})
}

func (a *App) appBindEmail(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Email == "" || req.Password == "" {
		return biz("邮箱和密码必填")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return a.bindWithConfirm(w, r, bindSpec{kind: "email", value: req.Email, passwordHash: string(hash)})
}

func (a *App) appBindWechat(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		OpenID string `json:"openid"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.OpenID == "" {
		return biz("微信身份不能为空")
	}
	return a.bindWithConfirm(w, r, bindSpec{kind: "wechat", value: req.OpenID})
}

func (a *App) appBindPhone(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Phone string `json:"phone"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Phone == "" {
		return biz("手机号不能为空")
	}
	return a.bindWithConfirm(w, r, bindSpec{kind: "phone", value: req.Phone})
}

func (a *App) appCancel(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
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
	_, err = a.Store.Exec(r.Context(), `UPDATE app_users SET cancelled_at=now(), email=NULL, wechat_openid=NULL, apple_sub=NULL, phone=NULL, password_hash=NULL WHERE id=$1`, uid)
	if err != nil {
		return err
	}
	_, _ = a.Store.Exec(r.Context(), `UPDATE refresh_tokens SET revoked=true WHERE audience='app' AND subject_id=$1`, uid)
	writeOK(w, true)
	return nil
}

func (a *App) appEditQuestion(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
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
	item, err := a.loadReading(r.Context(), id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) appDetect(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
		return err
	}
	item, err := a.loadReading(r.Context(), id)
	if err != nil {
		return err
	}
	question, _ := item["question"].(string)
	step := a.runStep(r, "detect", question)
	if step.Error != "" {
		return biz(step.Error)
	}
	_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET skip_detect=false, updated_at=now() WHERE id=$1`, id)
	writeOK(w, map[string]any{"text": step.Output, "vendor": step.Vendor, "model": step.Model, "suggestions": 3})
	return nil
}

func (a *App) appShuffle(w http.ResponseWriter, r *http.Request) error {
	stop, err := a.gatePlay(w, r, resumeShuffle)
	if err != nil || stop {
		return err
	}
	return a.moveRitual(w, r, "SPREAD_SELECTED", "SHUFFLING", "请先选择牌阵")
}

func (a *App) appCut(w http.ResponseWriter, r *http.Request) error {
	return a.moveRitual(w, r, "SHUFFLING", "CUTTING", "请先洗牌")
}

func (a *App) moveRitual(w http.ResponseWriter, r *http.Request, from, to, wrong string) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
		return err
	}
	tag, err := a.Store.Exec(r.Context(), `UPDATE readings SET status=$1, updated_at=now() WHERE id=$2 AND status=$3`, to, id, from)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz(wrong)
	}
	item, err := a.loadReading(r.Context(), id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) appFollowup(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
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
		view, err := a.createNormal(r.Context(), uid, anon, strings.TrimSpace(req.Text), "normal", nil, false)
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
		return biz("登录后才会扣次数或钻石")
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
		writeQuotaBlock(w, decision, id)
		return nil
	}
	if err := ensureUserTurn(r.Context(), tx, id, turn, strings.TrimSpace(req.Text)); err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	body, err := a.answerFollowup(r.Context(), id, uid, turn, strings.TrimSpace(req.Text))
	if err != nil {
		a.clearPendingIdem(r, scope, idemKey)
		return err
	}
	body["quota_outcome"] = decision.Status
	body["entitlement"] = decision.Entitlement
	body["charged"] = decision.Status == quotaCharged && decision.Entitlement != entTrial
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
	_, _, output, err := a.runEino(ctx, "tarotist", prompt)
	if err != nil {
		return map[string]any{"turn_id": turn, "error": "生成失败，可重试，未重新扣次数或钻石", "cards_unchanged": true, "bubbles": []string{}}, nil
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

func (a *App) appMessages(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, turn_id, seq, role, content, created_at FROM reading_messages WHERE reading_id=$1 ORDER BY turn_id, seq`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var mid, turn int64
		var seq int
		var role, content string
		var created time.Time
		if err := rows.Scan(&mid, &turn, &seq, &role, &content, &created); err != nil {
			return err
		}
		list = append(list, map[string]any{"id": mid, "turn_id": turn, "seq": seq, "role": role, "content": content, "created_at": created})
	}
	writeOK(w, list)
	return nil
}

func (a *App) appCalendar(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能看日历")
	}
	month := r.URL.Query().Get("month")
	q := `SELECT id, type, question, status, business_date, updated_at FROM readings WHERE user_id=$1 AND deleted_at IS NULL AND business_date<>''`
	args := []any{uid}
	if month != "" {
		q += ` AND business_date LIKE $2`
		args = append(args, month+"%")
	}
	q += ` ORDER BY business_date, id`
	rows, err := a.Store.Query(r.Context(), q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var typ, question, status, day string
		var updated time.Time
		if err := rows.Scan(&id, &typ, &question, &status, &day, &updated); err != nil {
			return err
		}
		label, next := progressOf(status)
		list = append(list, map[string]any{"id": id, "type": typ, "question": question, "status": status, "business_date": day, "has_result": status == "COMPLETED" || status == "CARDS_FIXED" || status == "INTERPRETING" || status == "FAILED_RETRYABLE", "progress": label, "continue": next, "updated_at": updated})
	}
	cycle := a.userCycle(r, uid)
	tomorrow, err := nextBusinessDate(cycle)
	if err != nil {
		return err
	}
	writeOK(w, map[string]any{
		"days": list, "cycle": cycle, "tomorrow_date": tomorrow,
	})
	return nil
}

func (a *App) appConfirmQuota(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
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
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
		return err
	}
	already, ent, err := playAlready(r.Context(), a.Store, id)
	if err != nil {
		return err
	}
	if already {
		writeOK(w, map[string]any{"quota_consumed": true, "entitlement": ent, "message": "已经扣过，不再扣"})
		return nil
	}
	var req struct {
		Consume bool `json:"consume"`
	}
	_ = decode(r, &req)
	if !req.Consume {
		writeOK(w, map[string]any{"quota_consumed": false, "message": "未确认，不扣额度，记录已保留"})
		return nil
	}
	decision, err := a.chargeOwned(r, id, chargeKeyPlay, resumeHold)
	if err != nil {
		return err
	}
	if !decision.Continue {
		writeQuotaBlock(w, decision, id)
		return nil
	}
	writeOK(w, map[string]any{
		"quota_consumed": true, "entitlement": decision.Entitlement, "quota_outcome": decision.Status,
		"message": quotaClosedNote,
	})
	return nil
}

func (a *App) appUseReward(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
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
		writeQuotaBlock(w, decision, id)
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
	rows, err := a.Store.Query(r.Context(), `SELECT id, email, wechat_openid, apple_sub, phone, timezone, tarotist_trial_used, tease_off, cancelled_at, created_at FROM app_users ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var email, openid, apple, phone, tz *string
		var trial, tease bool
		var cancelled *time.Time
		var created time.Time
		if err := rows.Scan(&id, &email, &openid, &apple, &phone, &tz, &trial, &tease, &cancelled, &created); err != nil {
			return err
		}
		list = append(list, map[string]any{"id": id, "email": email, "wechat_openid": openid, "apple_sub": apple, "phone": phone, "timezone": tz, "tarotist_trial_used": trial, "tease_off": tease, "cancelled_at": cancelled, "created_at": created})
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
