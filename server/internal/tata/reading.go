package tata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"database/sql"
	"golang.org/x/crypto/bcrypt"
)

func taskFor(kind string) string {
	if kind == "tarotist" {
		return "tarotist"
	}
	return "interpret"
}

func (a *App) adminListReadings(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	q := `SELECT id, user_id, anonymous_id, type, question, spread_id, tarotist_id, status, cards, interpretation, vendor_name, model_name, fail_reason, quota_consumed, first_experience, created_at, updated_at, deleted_at FROM readings WHERE 1=1`
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		q += fmt.Sprintf(" AND %s $%d", cond, len(args))
	}
	if v := r.URL.Query().Get("user_id"); v != "" {
		add("user_id =", v)
	}
	if v := r.URL.Query().Get("type"); v != "" {
		add("type =", v)
	}
	if v := r.URL.Query().Get("status"); v != "" {
		add("status =", v)
	}
	if v := r.URL.Query().Get("from"); v != "" {
		add("created_at >=", v)
	}
	if v := r.URL.Query().Get("to"); v != "" {
		add("created_at <=", v)
	}
	q += " ORDER BY updated_at DESC LIMIT 100"
	rows, err := a.Store.Query(r.Context(), q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	writeOK(w, scanReadings(rows))
	return nil
}

func (a *App) adminGetReading(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	id, err := pathID(r)
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

func (a *App) adminRetry(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	item, err := a.generate(r.Context(), id, true)
	if err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "retry", "reading", fmtID(id), "")
	writeOK(w, item)
	return nil
}

func (a *App) adminLedger(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, user_id, reading_id, entitlement, amount, reason, result, created_at FROM ledger ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, userID, readingID int64
		var entitlement, reason, result string
		var amount int
		var created time.Time
		if err := rows.Scan(&id, &userID, &readingID, &entitlement, &amount, &reason, &result, &created); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "user_id": userID, "reading_id": readingID, "entitlement": entitlement,
			"amount": amount, "reason": reason, "result": result, "created_at": created,
		})
	}
	writeOK(w, list)
	return nil
}

func (a *App) appAnonymous(w http.ResponseWriter, r *http.Request) {
	anon, err := randomID()
	if err != nil {
		writeErr(w, http.StatusOK, "内部错误")
		return
	}
	access, err := a.signAccess("app", 0, anon, "")
	if err != nil {
		writeErr(w, http.StatusOK, "内部错误")
		return
	}
	writeOK(w, map[string]any{"accessToken": access, "anonymous_id": anon})
}

func (a *App) appRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		AnonymousID string `json:"anonymous_id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusOK, "请求字段不正确")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeErr(w, http.StatusOK, "邮箱和密码必填")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusOK, "内部错误")
		return
	}
	var id int64
	err = a.Store.QueryRow(r.Context(), `INSERT INTO app_users(email, password_hash) VALUES ($1,$2) RETURNING id`, req.Email, string(hash)).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusOK, "邮箱已注册")
		return
	}
	exp := nextBeijingEight(time.Now())
	if !a.claimed(r, "email", strings.ToLower(req.Email)) {
		_, _ = a.Store.Exec(r.Context(), `INSERT INTO rewards(user_id, granted_at, expires_at) VALUES ($1, now(), $2)`, id, exp)
	} else {
		exp = time.Time{}
	}
	if req.AnonymousID != "" {
		_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET user_id=$1 WHERE anonymous_id=$2 AND user_id IS NULL`, id, req.AnonymousID)
	}
	settled := a.settleQuestionStart(r, id)
	access, _ := a.signAccess("app", id, "", "")
	writeOK(w, map[string]any{"accessToken": access, "id": id, "registration_reward_expires_at": exp, "reward_consumed": settled, "reward_note": "一开始提问就消耗本次次数；中途退出可从历史继续，不再扣"})
}

func (a *App) appLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		AnonymousID string `json:"anonymous_id"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusOK, "请求字段不正确")
		return
	}
	var id int64
	var hash string
	var cancelled *time.Time
	err := a.Store.QueryRow(r.Context(), `SELECT id, password_hash, cancelled_at FROM app_users WHERE email=$1`, req.Email).Scan(&id, &hash, &cancelled)
	if err != nil || cancelled != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		writeErr(w, http.StatusOK, "邮箱或密码错误")
		return
	}
	if req.AnonymousID != "" {
		_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET user_id=$1 WHERE anonymous_id=$2 AND user_id IS NULL`, id, req.AnonymousID)
	}
	access, _ := a.signAccess("app", id, "", "")
	settled := a.settleQuestionStart(r, id)
	prompt := ""
	if !settled {
		cycle := a.userCycle(r, id)
		var used int
		_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM quota_use WHERE user_id=$1 AND cycle=$2`, id, cycle).Scan(&used)
		if used > 0 {
			prompt = "今日免费体验额度已用完，本次记录已保存到历史记录"
		}
	}
	writeOK(w, map[string]any{"accessToken": access, "id": id, "reward_consumed": settled, "needs_confirm": false, "quota_prompt": prompt})
}

func (a *App) settleQuestionStart(r *http.Request, uid int64) bool {
	var readingID int64
	err := a.Store.QueryRow(r.Context(), `SELECT id FROM readings WHERE user_id=$1 AND deleted_at IS NULL AND reward_consumed=false AND quota_consumed=false ORDER BY id DESC LIMIT 1`, uid).Scan(&readingID)
	if err != nil {
		return false
	}
	ok, err := consumeRewardBalance(r.Context(), a.Store, uid)
	if err == nil && ok {
		_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET reward_consumed=true, updated_at=now() WHERE id=$1`, readingID)
		_, _ = a.Store.Exec(r.Context(), `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'registration_reward',1,'开始提问即消耗注册奖励','consumed')`, uid, readingID)
		return true
	}
	cycle := a.userCycle(r, uid)
	handled, consumedDaily, derr := consumeMergedDaily(r.Context(), a.Store, uid, cycle)
	if derr != nil {
		return false
	}
	if handled {
		if !consumedDaily {
			return false
		}
		_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET quota_consumed=true, quota_cycle=$1, updated_at=now() WHERE id=$2`, cycle, readingID)
		_, _ = a.Store.Exec(r.Context(), `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'daily_free',1,'开始提问即消耗本次次数','consumed')`, uid, readingID)
		_, _ = a.Store.Exec(r.Context(), `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, readingID)
		return true
	}
	tag, err := a.Store.Exec(r.Context(), `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, readingID)
	if err != nil || tag.RowsAffected() == 0 {
		return false
	}
	_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET quota_consumed=true, quota_cycle=$1, updated_at=now() WHERE id=$2`, cycle, readingID)
	_, _ = a.Store.Exec(r.Context(), `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'daily_free',1,'开始提问即消耗本次次数','consumed')`, uid, readingID)
	return true
}

func (a *App) appGetProfile(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("匿名用户没有档案")
	}
	var email, tz string
	var birth, gender, openid, apple, phone *string
	if err := a.Store.QueryRow(r.Context(), `SELECT email, birth_date, gender, wechat_openid, apple_sub, phone, timezone FROM app_users WHERE id=$1`, uid).Scan(&email, &birth, &gender, &openid, &apple, &phone, &tz); err != nil {
		return err
	}
	writeOK(w, map[string]any{"id": uid, "email": email, "birth_date": birth, "gender": gender, "wechat_openid": openid, "apple_sub": apple, "phone": phone, "timezone": tz})
	return nil
}

func (a *App) appPutProfile(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("匿名用户没有档案")
	}
	var req struct {
		BirthDate string `json:"birth_date"`
		Gender    string `json:"gender"`
		Timezone  string `json:"timezone"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Gender != "" && req.Gender != "女性" && req.Gender != "男性" && req.Gender != "不愿透露" {
		return biz("性别选项不正确")
	}
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			return biz("时区不正确")
		}
	}
	_, err = a.Store.Exec(r.Context(), `UPDATE app_users SET birth_date=COALESCE(NULLIF($1,''), birth_date), gender=COALESCE(NULLIF($2,''), gender), timezone=COALESCE(NULLIF($3,''), timezone) WHERE id=$4`, req.BirthDate, req.Gender, req.Timezone, uid)
	if err != nil {
		return err
	}
	writeOK(w, true)
	return nil
}

func (a *App) appSpreads(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, name, description, card_count, positions, charged FROM spreads WHERE enabled=true ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, desc, pos string
		var count int
		var charged bool
		if err := rows.Scan(&id, &name, &desc, &count, &pos, &charged); err != nil {
			return err
		}
		list = append(list, map[string]any{"id": id, "name": name, "description": desc, "card_count": count, "positions": json.RawMessage(pos), "charged": charged})
	}
	writeOK(w, list)
	return nil
}

func (a *App) appBanners(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	now := time.Now()
	rows, err := a.Store.Query(r.Context(), `SELECT id, copy, start_at, end_at, priority, target FROM banners WHERE start_at<=$1 AND end_at>$1 ORDER BY priority DESC, id DESC`, now)
	if err != nil {
		return err
	}
	defer rows.Close()
	writeOK(w, scanBanners(rows))
	return nil
}

func (a *App) appTarotists(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, name, display_copy FROM tarotists WHERE enabled=true ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, copy string
		if err := rows.Scan(&id, &name, &copy); err != nil {
			return err
		}
		list = append(list, map[string]any{"id": id, "name": name, "display_copy": copy})
	}
	writeOK(w, list)
	return nil
}

func (a *App) appQuota(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	cycle := cycleOf(time.Now())
	used := false
	dailyRemaining := 0
	usedFlags := 0
	rewardRemaining := 0
	subRemaining := 0
	packRemaining := 0
	subPrompt := false
	if uid > 0 {
		_ = a.Store.QueryRow(r.Context(), `SELECT remaining FROM rewards WHERE user_id=$1 AND remaining>0 AND expires_at>now()`, uid).Scan(&rewardRemaining)
		_ = a.Store.QueryRow(r.Context(), `SELECT subscription_remaining, pack_remaining FROM paid_pools WHERE user_id=$1`, uid).Scan(&subRemaining, &packRemaining)
		_ = a.Store.QueryRow(r.Context(), `SELECT subscription_prompt_required FROM app_users WHERE id=$1`, uid).Scan(&subPrompt)
		err = a.Store.QueryRow(r.Context(), `SELECT used_flags, remaining FROM daily_free_balance WHERE user_id=$1 AND cycle=$2`, uid, cycle).Scan(&usedFlags, &dailyRemaining)
		if err == nil {
			used = dailyRemaining <= 0
		} else {
			var n int
			_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM quota_use WHERE user_id=$1 AND cycle=$2`, uid, cycle).Scan(&n)
			used = n > 0
			if used {
				usedFlags = 1
			} else {
				dailyRemaining = 1
			}
		}
	}
	writeOK(w, map[string]any{
		"cycle": cycle, "reset_at": "北京时间 08:00",
		"daily_free_used": used, "daily_free_count": 1,
		"daily_free_remaining": dailyRemaining, "daily_used_flags": usedFlags,
		"reward_remaining":            rewardRemaining,
		"paid_subscription_remaining": subRemaining, "paid_pack_remaining": packRemaining,
		"subscription_prompt_required": subPrompt,
	})
	return nil
}

func (a *App) appCreateReading(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	var req struct {
		Question   string `json:"question"`
		Type       string `json:"type"`
		Source     string `json:"source"`
		SkipDetect bool   `json:"skip_detect"`
		TarotistID *int64 `json:"tarotist_id"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Type == "" {
		req.Type = "normal"
	}
	if req.Type != "normal" && req.Type != "tarotist" {
		return biz("记录类型不正确")
	}
	if len([]rune(req.Question)) > 200 {
		return biz("问题超过 200 个码点，正式计数口径尚未确认")
	}
	if req.Type == "tarotist" {
		if req.TarotistID == nil {
			return biz("请选择塔罗师")
		}
		var enabled bool
		err := a.Store.QueryRow(r.Context(), `SELECT enabled FROM tarotists WHERE id=$1`, *req.TarotistID).Scan(&enabled)
		if err != nil || !enabled {
			return biz("塔罗师不可用")
		}
	}
	scope := "anon:" + anon
	if uid > 0 {
		scope = "user:" + strconv.FormatInt(uid, 10)
	}
	if replay, ok, err := a.beginIdem(r, scope, r.Header.Get("Idempotency-Key")); err != nil {
		return err
	} else if ok {
		writeJSON(w, http.StatusOK, json.RawMessage(replay))
		return nil
	}
	view, err := a.createNormal(r.Context(), uid, anon, req.Question, req.Type, req.TarotistID, req.SkipDetect || req.Source == "explore" || req.Source == "preset")
	if err != nil {
		_, _ = a.Store.Exec(r.Context(), `DELETE FROM idempotency WHERE scope=$1 AND key=$2`, scope, r.Header.Get("Idempotency-Key"))
		return err
	}
	body := map[string]any{"code": 0, "data": view, "message": ""}
	raw, _ := json.Marshal(body)
	if key := r.Header.Get("Idempotency-Key"); key != "" {
		_, _ = a.Store.Exec(r.Context(), `UPDATE idempotency SET status='done', response=$1 WHERE scope=$2 AND key=$3`, string(raw), scope, key)
	}
	writeJSON(w, http.StatusOK, body)
	return nil
}

func (a *App) beginIdem(r *http.Request, scope, key string) (string, bool, error) {
	if key == "" {
		return "", false, nil
	}
	tag, err := a.Store.Exec(r.Context(), `INSERT INTO idempotency(scope, key, status) VALUES ($1,$2,'pending') ON CONFLICT DO NOTHING`, scope, key)
	if err != nil {
		return "", false, err
	}
	if tag.RowsAffected() == 1 {
		return "", false, nil
	}
	var status string
	var resp *string
	err = a.Store.QueryRow(r.Context(), `SELECT status, response FROM idempotency WHERE scope=$1 AND key=$2`, scope, key).Scan(&status, &resp)
	if err != nil {
		return "", false, err
	}
	if status == "done" && resp != nil {
		return *resp, true, nil
	}
	return "", false, biz("重复请求处理中")
}

func (a *App) createNormal(ctx context.Context, uid int64, anon, question, kind string, tarotistID *int64, skip bool) (map[string]any, error) {
	first := false
	if uid == 0 {
		var n int
		if err := a.Store.QueryRow(ctx, `SELECT count(*) FROM readings WHERE anonymous_id=$1`, anon).Scan(&n); err != nil {
			return nil, err
		}
		first = n == 0
	}
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO readings(user_id, anonymous_id, type, question, tarotist_id, status, first_experience) VALUES ($1,$2,$3,$4,$5,'QUESTION_CREATED',$6) RETURNING id`,
		nullID(uid), nullStr(anon), kind, question, tarotistID, first).Scan(&id)
	if err != nil {
		return nil, err
	}
	cycle := cycleOf(time.Now())
	if _, err := tx.Exec(ctx, `UPDATE readings SET business_date=$1, skip_detect=$2 WHERE id=$3`, cycle, skip, id); err != nil {
		return nil, err
	}
	if kind == "tarotist" && uid > 0 {
		var used bool
		if err := tx.QueryRow(ctx, `SELECT tarotist_trial_used FROM app_users WHERE id=$1`, uid).Scan(&used); err != nil {
			return nil, err
		}
		if used {
			return nil, biz("AI 塔罗师试用已用完，不扣每日额度")
		}
		if _, err := tx.Exec(ctx, `UPDATE app_users SET tarotist_trial_used=true WHERE id=$1`, uid); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return a.loadReading(ctx, id)
	}
	consumed := false
	exhausted := false
	rewardUsed := false
	if uid > 0 {
		rewardOK, err := consumeRewardBalance(ctx, tx, uid)
		if err != nil {
			return nil, err
		}
		if rewardOK {
			rewardUsed = true
			if _, err := tx.Exec(ctx, `UPDATE readings SET reward_consumed=true, updated_at=now() WHERE id=$1`, id); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'registration_reward',1,'新用户登录后扣注册奖励','consumed')`, uid, id); err != nil {
				return nil, err
			}
		} else {
			handled, consumedDaily, err := consumeMergedDaily(ctx, tx, uid, cycle)
			if err != nil {
				return nil, err
			}
			if handled {
				if consumedDaily {
					consumed = true
					if _, err := tx.Exec(ctx, `UPDATE readings SET quota_consumed=true, quota_cycle=$1, updated_at=now() WHERE id=$2`, cycle, id); err != nil {
						return nil, err
					}
					if _, err := tx.Exec(ctx, `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'daily_free',1,'开始提问即消耗本次次数','consumed')`, uid, id); err != nil {
						return nil, err
					}
					_, _ = tx.Exec(ctx, `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, id)
				} else {
					exhausted = true
				}
			} else {
				tag, err := tx.Exec(ctx, `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, id)
				if err != nil {
					return nil, err
				}
				if tag.RowsAffected() == 1 {
					consumed = true
					if _, err := tx.Exec(ctx, `UPDATE readings SET quota_consumed=true, quota_cycle=$1, updated_at=now() WHERE id=$2`, cycle, id); err != nil {
						return nil, err
					}
					if _, err := tx.Exec(ctx, `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'daily_free',1,'开始提问即消耗本次次数','consumed')`, uid, id); err != nil {
						return nil, err
					}
				} else {
					exhausted = true
				}
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	item, err := a.loadReading(ctx, id)
	if err != nil {
		return nil, err
	}
	item["quota_exhausted"] = exhausted
	item["quota_consumed"] = consumed || item["quota_consumed"] == true
	item["reward_consumed"] = rewardUsed || item["reward_consumed"] == true
	item["needs_confirm"] = false
	if exhausted {
		item["quota_prompt"] = "今日免费体验额度已用完，本次记录已保存到历史记录"
	}
	return item, nil
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (a *App) appSelectSpread(w http.ResponseWriter, r *http.Request) error {
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
	var req struct {
		SpreadID int64 `json:"spread_id"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	var enabled bool
	err = a.Store.QueryRow(r.Context(), `SELECT enabled FROM spreads WHERE id=$1`, req.SpreadID).Scan(&enabled)
	if err != nil || !enabled {
		return biz("牌阵不可用")
	}
	tag, err := a.Store.Exec(r.Context(), `UPDATE readings SET spread_id=$1, spread_name=(SELECT name FROM spreads WHERE id=$1), status='SPREAD_SELECTED', updated_at=now() WHERE id=$2 AND status='QUESTION_CREATED' AND deleted_at IS NULL`, req.SpreadID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("当前状态不能选择牌阵")
	}
	item, err := a.loadReading(r.Context(), id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) appDraw(w http.ResponseWriter, r *http.Request) error {
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
	var req struct {
		Mode string `json:"mode"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Mode == "numerology" {
		return biz("灵数映射未确认，本期不生成牌面")
	}
	if req.Mode != "normal" {
		return biz("抽牌方式不正确")
	}
	var spreadID *int64
	var status string
	err = a.Store.QueryRow(r.Context(), `SELECT spread_id, status FROM readings WHERE id=$1`, id).Scan(&spreadID, &status)
	if err != nil || spreadID == nil {
		return biz("请先选择牌阵")
	}
	if status == "SHUFFLING" {
		return biz("请先完成切牌并点击下一步")
	}
	if status != "SPREAD_SELECTED" && status != "CUTTING" {
		return biz("请先选择牌阵")
	}
	var posRaw string
	if err := a.Store.QueryRow(r.Context(), `SELECT positions FROM spreads WHERE id=$1`, *spreadID).Scan(&posRaw); err != nil {
		return err
	}
	var positions []position
	if err := json.Unmarshal([]byte(posRaw), &positions); err != nil {
		return err
	}
	cards, err := drawCards(positions)
	if err != nil {
		return err
	}
	tag, err := a.Store.Exec(r.Context(), `UPDATE readings SET cards=$1, status='CARDS_FIXED', updated_at=now() WHERE id=$2 AND status IN ('SPREAD_SELECTED','CUTTING')`, mustJSON(cards), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("牌面已固定或状态已变化")
	}
	item, err := a.loadReading(r.Context(), id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) appInterpret(w http.ResponseWriter, r *http.Request) error {
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
	item, err := a.generate(r.Context(), id, false)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) appListReadings(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	var rows *sql.Rows
	if uid > 0 {
		rows, err = a.Store.Query(r.Context(), `SELECT id, user_id, anonymous_id, type, question, spread_id, tarotist_id, status, cards, interpretation, vendor_name, model_name, fail_reason, quota_consumed, first_experience, created_at, updated_at, deleted_at FROM readings WHERE user_id=$1 AND deleted_at IS NULL ORDER BY updated_at DESC`, uid)
	} else {
		rows, err = a.Store.Query(r.Context(), `SELECT id, user_id, anonymous_id, type, question, spread_id, tarotist_id, status, cards, interpretation, vendor_name, model_name, fail_reason, quota_consumed, first_experience, created_at, updated_at, deleted_at FROM readings WHERE anonymous_id=$1 AND deleted_at IS NULL ORDER BY updated_at DESC`, anon)
	}
	if err != nil {
		return err
	}
	defer rows.Close()
	writeOK(w, scanReadings(rows))
	return nil
}

func (a *App) appGetReading(w http.ResponseWriter, r *http.Request) error {
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
	writeOK(w, item)
	return nil
}

func (a *App) appDeleteReading(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能删除")
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
		return err
	}
	_, err = a.Store.Exec(r.Context(), `UPDATE readings SET deleted_at=now(), updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	writeOK(w, true)
	return nil
}

func (a *App) appDaily(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能抽今日一牌")
	}
	cycle := cycleOf(time.Now())
	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	useBalance, remaining, slotRemaining, err := lockDailyBalance(r.Context(), tx, uid, cycle)
	if err != nil {
		return err
	}
	if useBalance {
		if slotRemaining <= 0 {
			return biz("当日今日一牌名额已占用")
		}
		if remaining <= 0 {
			return biz("今日免费资格已用完")
		}
	} else {
		taken, err := dailySlotTaken(r.Context(), tx, uid, cycle)
		if err != nil {
			return err
		}
		if taken {
			return biz("当日今日一牌名额已占用")
		}
	}
	cards, err := drawCards([]position{{Index: 1, Name: "今日", Meaning: "当日一牌"}})
	if err != nil {
		return err
	}
	var id int64
	err = tx.QueryRow(r.Context(), `INSERT INTO readings(user_id, type, question, status, cards, quota_cycle) VALUES ($1,'daily','今日一牌','CARDS_FIXED',$2,$3) RETURNING id`,
		uid, mustJSON(cards), cycle).Scan(&id)
	if err != nil {
		return err
	}
	if useBalance {
		tag, err := tx.Exec(r.Context(), `UPDATE daily_free_balance
			SET remaining=remaining-1, used_flags=used_flags+1, daily_slot_remaining=daily_slot_remaining-1, daily_slot_used=daily_slot_used+1
			WHERE user_id=$1 AND cycle=$2 AND remaining>0 AND daily_slot_remaining>0`, uid, cycle)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return biz("今日免费资格已用完")
		}
		_, _ = tx.Exec(r.Context(), `INSERT INTO daily_slot(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, id)
		_, _ = tx.Exec(r.Context(), `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, id)
	} else {
		tag, err := tx.Exec(r.Context(), `INSERT INTO daily_slot(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return biz("当日今日一牌名额已占用")
		}
		tag, err = tx.Exec(r.Context(), `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return biz("今日免费资格已用完")
		}
	}
	if _, err := tx.Exec(r.Context(), `UPDATE readings SET quota_consumed=true, business_date=$2, updated_at=now() WHERE id=$1`, id, cycle); err != nil {
		return err
	}
	if _, err := tx.Exec(r.Context(), `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,'daily_free',1,'今日一牌提交','consumed')`, uid, id); err != nil {
		return err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	item, err := a.generate(r.Context(), id, false)
	if err != nil {
		failed, loadErr := a.loadReading(r.Context(), id)
		if loadErr != nil {
			return err
		}
		writeOK(w, failed)
		return nil
	}
	writeOK(w, item)
	return nil
}
