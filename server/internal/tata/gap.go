package tata

import (
	"encoding/json"
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
	writeOK(w, map[string]any{"accessToken": access, "id": id, "reward_note": "扣减优先级未闭合，注册奖励不自动扣减"})
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
	var skip bool
	var status string
	if err := a.Store.QueryRow(r.Context(), `SELECT skip_detect, status FROM readings WHERE id=$1`, id).Scan(&skip, &status); err != nil {
		return err
	}
	if skip {
		return biz("未进入问题检测，不能修改问题")
	}
	if status != "QUESTION_CREATED" && status != "SPREAD_SELECTED" {
		return biz("抽牌后不能修改问题")
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
	if replay, ok, err := a.beginIdem(r, "followup:"+fmtID(id), r.Header.Get("Idempotency-Key")); err != nil {
		return err
	} else if ok {
		writeJSON(w, http.StatusOK, json.RawMessage(replay))
		return nil
	}
	var kind, status, question string
	var cards *string
	var tarotistID *int64
	err = a.Store.QueryRow(r.Context(), `SELECT type, status, question, cards, tarotist_id FROM readings WHERE id=$1`, id).Scan(&kind, &status, &question, &cards, &tarotistID)
	if err != nil {
		return err
	}
	if kind != "tarotist" {
		return biz("只有 AI 塔罗师可以追问")
	}
	if status != "COMPLETED" && status != "FAILED_RETRYABLE" {
		return biz("解读完成前不能追问")
	}
	if cards == nil || *cards == "" {
		return biz("牌面尚未固定")
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := decode(r, &req); err != nil || strings.TrimSpace(req.Text) == "" {
		return biz("追问不能为空")
	}
	if strings.Contains(req.Text, "别这样说") || strings.Contains(req.Text, "不舒服") {
		_, _ = a.Store.Exec(r.Context(), `UPDATE app_users SET tease_off=true WHERE id=$1`, uid)
	}
	var turn int64
	_ = a.Store.QueryRow(r.Context(), `SELECT COALESCE(max(turn_id),0)+1 FROM reading_messages WHERE reading_id=$1`, id).Scan(&turn)
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO reading_messages(reading_id, turn_id, seq, role, content) VALUES ($1,$2,0,'user',$3)`, id, turn, req.Text); err != nil {
		return err
	}
	prompt := a.personaPrompt(r, tarotistID, uid) + "\n问题：" + question + "\n牌面：" + *cards + "\n追问：" + req.Text + "\n不要改牌。把回答拆成不超过四个短气泡，气泡之间用空行分隔。"
	step := a.runStep(r, "tarotist", prompt)
	if step.Error != "" {
		writeOK(w, map[string]any{"error": step.Error, "cards_unchanged": true})
		return nil
	}
	parts := strings.Split(step.Output, "\n\n")
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
		bubbles = []string{step.Output}
	}
	for i, bubble := range bubbles {
		_, _ = a.Store.Exec(r.Context(), `INSERT INTO reading_messages(reading_id, turn_id, seq, role, content) VALUES ($1,$2,$3,'assistant',$4)`, id, turn, i+1, bubble)
	}
	body := map[string]any{"turn_id": turn, "bubbles": bubbles, "cards_unchanged": true, "charged": false}
	raw, _ := json.Marshal(map[string]any{"code": 0, "data": body, "message": ""})
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		_, _ = a.Store.Exec(r.Context(), `UPDATE idempotency SET status='done', response=$1 WHERE scope=$2 AND key=$3`, string(raw), "followup:"+fmtID(id), key)
	}
	writeOK(w, body)
	return nil
}

func (a *App) personaPrompt(r *http.Request, tarotistID *int64, uid int64) string {
	persona, voice, banned, ask, name := "", "", "", "", ""
	if tarotistID != nil {
		_ = a.Store.QueryRow(r.Context(), `SELECT name, persona, voice, banned, ask_style FROM tarotists WHERE id=$1`, *tarotistID).Scan(&name, &persona, &voice, &banned, &ask)
	}
	tease := false
	if uid > 0 {
		_ = a.Store.QueryRow(r.Context(), `SELECT tease_off FROM app_users WHERE id=$1`, uid).Scan(&tease)
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
		list = append(list, map[string]any{"id": id, "type": typ, "question": question, "status": status, "business_date": day, "progress": label, "continue": next, "updated_at": updated})
	}
	writeOK(w, list)
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
	var consumed bool
	if err := a.Store.QueryRow(r.Context(), `SELECT quota_consumed FROM readings WHERE id=$1`, id).Scan(&consumed); err != nil {
		return err
	}
	if consumed {
		writeOK(w, map[string]any{"quota_consumed": true, "message": "已经扣过，不再扣"})
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
	cycle := a.userCycle(r, uid)
	handled, consumedDaily, err := consumeMergedDaily(r.Context(), a.Store, uid, cycle)
	if err != nil {
		return err
	}
	if handled {
		if !consumedDaily {
			writeOK(w, map[string]any{"quota_consumed": false, "message": "今日免费体验额度已用完，本次记录已保存到历史记录"})
			return nil
		}
		_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET quota_consumed=true, quota_cycle=$1, updated_at=now() WHERE id=$2`, cycle, id)
		_, _ = a.Store.Exec(r.Context(), `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'daily_free',1,'用户确认消耗今日额度','consumed')`, uid, id)
		_, _ = a.Store.Exec(r.Context(), `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, id)
		writeOK(w, map[string]any{"quota_consumed": true, "message": "已消耗今日免费额度"})
		return nil
	}
	tag, err := a.Store.Exec(r.Context(), `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		writeOK(w, map[string]any{"quota_consumed": false, "message": "今日免费体验额度已用完，本次记录已保存到历史记录"})
		return nil
	}
	_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET quota_consumed=true, quota_cycle=$1, updated_at=now() WHERE id=$2`, cycle, id)
	_, _ = a.Store.Exec(r.Context(), `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'daily_free',1,'用户确认消耗今日额度','consumed')`, uid, id)
	writeOK(w, map[string]any{"quota_consumed": true, "message": "已消耗今日免费额度"})
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
	if err := a.ownReading(r.Context(), id, uid, ""); err != nil {
		return err
	}
	ok, err := consumeRewardBalance(r.Context(), a.Store, uid)
	if err != nil {
		return err
	}
	if !ok {
		return biz("没有可用的注册奖励")
	}
	_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET reward_consumed=true, updated_at=now() WHERE id=$1`, id)
	_, _ = a.Store.Exec(r.Context(), `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'registration_reward',1,'用户显式使用注册奖励','consumed')`, uid, id)
	writeOK(w, map[string]any{"reward_consumed": true, "note": "与每日额度的自动优先级仍未闭合，只在用户点选时扣"})
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
