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
	items := scanReadings(rows)
	a.applyCardThumbnails(r.Context(), r, items...)
	for _, item := range items {
		a.localizeReading(r.Context(), item)
	}
	writeOK(w, items)
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
	item, err := a.loadReadingAbs(r.Context(), r, id)
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
	a.applyCardThumbnails(r.Context(), r, item)
	a.Store.audit(r.Context(), act.id, "retry", "reading", fmtID(id), "")
	writeOK(w, item)
	return nil
}

func (a *App) adminLedger(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, user_id, reading_id, entitlement, amount, reason, result, created_at, grant_id FROM ledger ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, userID int64
		var readingID, grantID sql.NullInt64
		var entitlement, reason, result string
		var amount int
		var created time.Time
		if err := rows.Scan(&id, &userID, &readingID, &entitlement, &amount, &reason, &result, &created, &grantID); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "user_id": userID, "reading_id": nullInt(readingID), "entitlement": entitlement,
			"amount": amount, "reason": reason, "result": result, "created_at": created, "grant_id": nullInt(grantID),
		})
	}
	writeOK(w, list)
	return nil
}

func (a *App) appAnonymous(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceID string `json:"device_id"`
	}
	_ = decode(r, &req)
	anon, err := randomID()
	if err != nil {
		writeErr(w, r, http.StatusOK, "内部错误")
		return
	}
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		deviceID = "anon:" + anon
	}
	if len([]rune(deviceID)) > 128 {
		writeErr(w, r, http.StatusOK, "设备标识过长")
		return
	}
	if _, err := a.Store.Exec(r.Context(), `INSERT INTO anon_devices(anonymous_id, device_id) VALUES ($1,$2)
		ON CONFLICT (anonymous_id) DO UPDATE SET device_id=EXCLUDED.device_id`, anon, deviceID); err != nil {
		writeErr(w, r, http.StatusOK, "内部错误")
		return
	}
	access, err := a.signAccess("app", 0, anon, "")
	if err != nil {
		writeErr(w, r, http.StatusOK, "内部错误")
		return
	}
	writeOK(w, map[string]any{"accessToken": access, "anonymous_id": anon, "device_id": deviceID})
}

func (a *App) appLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		AnonymousID string `json:"anonymous_id"`
		InviteCode  string `json:"invite_code"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, r, http.StatusOK, "请求字段不正确")
		return
	}
	if req.Email == "" || req.Code == "" {
		writeErr(w, r, http.StatusOK, "邮箱和验证码必填")
		return
	}
	email := normalizeEmail(req.Email)
	if !validLoginEmail(email) {
		writeErr(w, r, http.StatusOK, "邮箱不正确")
		return
	}
	if err := a.matchEmailCode(r.Context(), "login", email, req.Code); err != nil {
		writeErr(w, r, http.StatusOK, err.Error())
		return
	}
	var id int64
	var cancelled *time.Time
	created := false
	err := a.Store.QueryRow(r.Context(), `SELECT id, cancelled_at FROM app_users WHERE lower(email)=lower($1) AND merged_into IS NULL`, email).Scan(&id, &cancelled)
	if errors.Is(err, sql.ErrNoRows) {
		if a.claimed(r, "email", email) {
			writeErr(w, r, http.StatusOK, "该邮箱已注销，不能重复注册")
			return
		}
		err = a.Store.QueryRow(r.Context(), `INSERT INTO app_users(email) VALUES ($1) RETURNING id`, email).Scan(&id)
		if err != nil {
			err = a.Store.QueryRow(r.Context(), `SELECT id, cancelled_at FROM app_users WHERE lower(email)=lower($1) AND merged_into IS NULL`, email).Scan(&id, &cancelled)
			if err != nil {
				writeErr(w, r, http.StatusOK, "邮箱登录失败")
				return
			}
		} else {
			created = true
		}
	} else if err != nil {
		writeErr(w, r, http.StatusOK, "邮箱登录失败")
		return
	}
	if cancelled != nil {
		writeErr(w, r, http.StatusOK, "账号已注销")
		return
	}
	if err := a.finishRegistration(r.Context(), id, created); err != nil {
		writeErr(w, r, http.StatusOK, "注册赠送记录失败")
		return
	}
	a.ensureProfileDefaults(r.Context(), id)
	a.consumeEmailCode(r.Context(), "login", email)
	if req.AnonymousID != "" {
		_, _ = a.Store.Exec(r.Context(), `UPDATE readings SET user_id=$1 WHERE anonymous_id=$2 AND user_id IS NULL`, id, req.AnonymousID)
	}
	a.tryApplyInvite(r.Context(), id, req.InviteCode)
	access, _ := a.signAccess("app", id, "", "")
	settled := a.settleQuestionStart(r, id)
	writeOK(w, map[string]any{"accessToken": access, "id": id, "login": "email", "reward_consumed": settled, "needs_confirm": false})
}

func (a *App) settleQuestionStart(r *http.Request, uid int64) bool {
	var readingID int64
	var kind string
	var onboard bool
	var spreadID sql.NullInt64
	var cards sql.NullString
	err := a.Store.QueryRow(r.Context(), `SELECT id, type, onboard_experience, spread_id, cards FROM readings
		WHERE user_id=$1 AND deleted_at IS NULL AND quota_settled=false AND reward_consumed=false AND quota_consumed=false
		ORDER BY id DESC LIMIT 1`, uid).Scan(&readingID, &kind, &onboard, &spreadID, &cards)
	if err != nil {
		return false
	}
	hasCards := cards.Valid && strings.TrimSpace(cards.String) != ""
	if onboard && !hasCards {
		return false
	}
	if kind == "explore" || (kind == "normal" && !spreadID.Valid) {
		member, merr := hasActiveMembership(r.Context(), a.Store, uid)
		if merr != nil || !member {
			return false
		}
		if kind == "normal" && !spreadID.Valid {
			return false
		}
	}
	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return false
	}
	defer tx.Rollback(r.Context())
	decision, err := settleSharedQuota(r.Context(), tx, chargeOpts{
		UserID: uid, ReadingID: readingID, ChargeKey: chargeKeyPlay, Cycle: a.userCycle(r, uid),
		AllowTrial: true, ResumeAction: resumeInterpret,
	})
	if err != nil || !decision.Continue {
		if err == nil {
			_ = tx.Commit(r.Context())
		}
		return false
	}
	if err := tx.Commit(r.Context()); err != nil {
		return false
	}
	if onboard && hasCards {
		_, _ = a.generate(r.Context(), readingID, false)
	}
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
	var email, tz, name, bio, avatar, publicID, prefs string
	var birth, gender, openid, apple *string
	if err := a.Store.QueryRow(r.Context(), `SELECT COALESCE(email,''), birth_date, gender, wechat_openid, apple_sub, COALESCE(NULLIF(timezone,''),'Asia/Shanghai'), COALESCE(name,''), COALESCE(bio,''), COALESCE(avatar_url,''), COALESCE(public_id,''), COALESCE(explore_preferences,'[]') FROM app_users WHERE id=$1`, uid).Scan(&email, &birth, &gender, &openid, &apple, &tz, &name, &bio, &avatar, &publicID, &prefs); err != nil {
		return err
	}
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	publicID, err = a.ensurePublicID(r.Context(), uid, publicID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		name = a.assignRandomNickname(r.Context(), uid)
	}
	writeOK(w, map[string]any{
		"id": uid, "email": emptyAsNil(email), "birth_date": birth, "gender": gender,
		"wechat_openid": openid, "apple_sub": apple, "timezone": tz,
		"name": name, "nickname": name, "bio": bio, "avatar_url": profileAvatarURL(r, avatar),
		"public_id":                  publicID,
		"account_display":            accountDisplay(email, openid, apple),
		"explore_preferences":        parseExplorePreferences(prefs),
		"explore_preference_options": explorePreferenceOptions,
	})
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
		BirthDate          string    `json:"birth_date"`
		Gender             string    `json:"gender"`
		Timezone           string    `json:"timezone"`
		Name               string    `json:"name"`
		Nickname           string    `json:"nickname"`
		Bio                string    `json:"bio"`
		AvatarURL          string    `json:"avatar_url"`
		ExplorePreferences *[]string `json:"explore_preferences"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	gender, err := normalizeGender(req.Gender)
	if err != nil {
		return err
	}
	if req.Timezone != "" {
		if _, err := time.LoadLocation(req.Timezone); err != nil {
			return biz("时区不正确")
		}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(req.Nickname)
	}
	if n := runeLen(name); n > 32 {
		return biz("昵称过长")
	}
	if n := runeLen(strings.TrimSpace(req.Bio)); n > 200 {
		return biz("简介过长")
	}
	if err := optionalAssetURL(strings.TrimSpace(req.AvatarURL)); err != nil {
		return err
	}
	_, err = a.Store.Exec(r.Context(), `UPDATE app_users SET
		birth_date=COALESCE(NULLIF($1,''), birth_date),
		gender=COALESCE(NULLIF($2,''), gender),
		timezone=COALESCE(NULLIF($3,''), timezone),
		name=COALESCE(NULLIF($4,''), name),
		bio=COALESCE(NULLIF($5,''), bio),
		avatar_url=COALESCE(NULLIF($6,''), avatar_url)
		WHERE id=$7`, req.BirthDate, gender, req.Timezone, name, strings.TrimSpace(req.Bio), strings.TrimSpace(req.AvatarURL), uid)
	if err != nil {
		return err
	}
	if req.ExplorePreferences != nil {
		prefs, nerr := normalizeExplorePreferences(*req.ExplorePreferences)
		if nerr != nil {
			return nerr
		}
		if _, err := a.Store.Exec(r.Context(), `UPDATE app_users SET explore_preferences=$1 WHERE id=$2`, encodeExplorePreferences(prefs), uid); err != nil {
			return err
		}
	}
	writeOK(w, true)
	return nil
}

func (a *App) appSpreads(w http.ResponseWriter, r *http.Request) error {
	rows, err := a.Store.Query(r.Context(), `SELECT id, name, name_en, description, card_count, positions, charged, recommended, thumbnail,
		theme_key, theme_title, theme_subtitle, short_sentence, depth_group, quota_cost
		FROM spreads WHERE enabled=true AND theme_key='' ORDER BY recommended DESC, sort_order, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, nameEN, desc, pos, thumb, themeKey, themeTitle, themeSub, short, depth string
		var count int
		var charged, recommended bool
		var quota sql.NullInt64
		if err := rows.Scan(&id, &name, &nameEN, &desc, &count, &pos, &charged, &recommended, &thumb, &themeKey, &themeTitle, &themeSub, &short, &depth, &quota); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "name": T(r.Context(), name), "name_en": nameEN, "description": T(r.Context(), desc), "card_count": count,
			"positions": localizeJSONFields(r.Context(), json.RawMessage(pos), "name", "meaning"),
			"charged":   charged, "recommended": recommended, "thumbnail": absoluteAssetURL(r, deckPreviewURL(thumb)),
			"theme_key": themeKey, "theme_title": themeTitle, "theme_subtitle": themeSub,
			"short_sentence": short,
			"depth_group":    depth, "depth_label": depthLabel(depth),
			"quota_cost": quotaCostValue(quota), "price_ledger": quotaLedger(quota),
		})
	}
	writeOK(w, list)
	return nil
}

func (a *App) appBanners(w http.ResponseWriter, r *http.Request) error {
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
	rows, err := a.Store.Query(r.Context(), `SELECT id, name, display_copy, avatar_url, tags, blurb FROM tarotists WHERE enabled=true ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var name, copy, avatar, tagsJSON, blurb string
		if err := rows.Scan(&id, &name, &copy, &avatar, &tagsJSON, &blurb); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "name": name, "display_copy": copy,
			"title": copy, "tags": tarotistTags(tagsJSON, copy),
			"blurb": publicBlurb(blurb, copy), "avatar_url": emptyAsNil(absoluteAssetURL(r, avatar)),
		})
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
	inviteRemaining := 0
	adminGrantRemaining := 0
	subPrompt := false
	dailyFree, _ := loadDailyFreeCount(r.Context(), a.Store)
	dailySlot, _ := loadDailySlotCount(r.Context(), a.Store)
	member := false
	var memberKind string
	var memberExp time.Time
	var memberAuto sql.NullBool
	if uid > 0 {
		if err := a.Store.ensureRegistrationGift(r.Context(), uid); err != nil {
			return err
		}
		memberKind, memberExp, memberAuto, member, err = loadActiveMembership(r.Context(), a.Store, uid)
		if err != nil {
			return err
		}
		_ = a.Store.QueryRow(r.Context(), `SELECT subscription_prompt_required FROM app_users WHERE id=$1`, uid).Scan(&subPrompt)
		_ = a.Store.QueryRow(r.Context(), `SELECT invite_remaining, admin_grant_remaining FROM rewards WHERE user_id=$1`, uid).Scan(&inviteRemaining, &adminGrantRemaining)
		if !member {
			qerr := a.Store.QueryRow(r.Context(), `SELECT used_flags, remaining FROM daily_free_balance WHERE user_id=$1 AND cycle=$2`, uid, cycle).Scan(&usedFlags, &dailyRemaining)
			if qerr == nil {
				used = dailyRemaining <= 0
			} else {
				var n int
				_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM quota_use WHERE user_id=$1 AND cycle=$2`, uid, cycle).Scan(&n)
				usedFlags = n
				dailyRemaining = dailyFree - n
				if dailyRemaining < 0 {
					dailyRemaining = 0
				}
				used = dailyRemaining == 0
			}
		}
	}
	cost, configured, _ := loadQuotaCost(r.Context(), a.Store)
	var quotaCost any
	if configured {
		quotaCost = cost
	}
	followupLimit, followupReady, _ := loadFollowupLimit(r.Context(), a.Store)
	var followup any
	if followupReady {
		followup = followupLimit
	}
	out := map[string]any{
		"cycle": cycle, "reset_at": T(r.Context(), "北京时间 08:00"),
		"is_member":                    member,
		"can_buy_pack":                 member,
		"subscription_prompt_required": subPrompt,
		"free_label":                   T(r.Context(), "次数"),
		"paid_label":                   T(r.Context(), displayLabelQuota),
		"display_label":                T(r.Context(), displayLabelQuota),
		"followup_limit":               followup,
		"quota_cost":                   quotaCost,
		"quota_cost_configured":        configured,
	}
	if uid > 0 {
		gift, gerr := loadRegistrationGift(r.Context(), a.Store, uid)
		if gerr != nil {
			return gerr
		}
		out["registration_gift"] = gift
	}
	if member {
		label := "月度会员"
		if memberKind == "year" {
			label = "年度会员"
		}
		quota, qerr := loadQuotaBalance(r.Context(), a.Store, uid)
		if qerr != nil {
			return qerr
		}
		out["membership"] = map[string]any{
			"sku_kind": memberKind, "status": "active", "expires_at": memberExp,
			"auto_renew": nullBool(memberAuto), "type_label": label,
		}
		out["quota_remaining"] = quota
		out["balance"] = quota
		out["priority"] = T(r.Context(), "会员统一额度")
		out["daily_slot_count"] = dailySlot
	} else {
		var invited int
		if uid > 0 {
			_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM invite_rewards WHERE inviter_id=$1`, uid).Scan(&invited)
		}
		out["daily_free_used"] = used
		out["daily_free_count"] = dailyFree
		out["daily_slot_count"] = dailySlot
		out["daily_free_remaining"] = dailyRemaining
		out["daily_used_flags"] = usedFlags
		out["invite_remaining"] = inviteRemaining
		out["invite_count"] = invited
		out["max_invites"] = inviteMaxPeople
		out["admin_grant_free_remaining"] = adminGrantRemaining
		out["priority"] = T(r.Context(), "今日免费次数 → 邀请奖励次数")
	}
	writeOK(w, out)
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
		DeviceID   string `json:"device_id"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Type == "" {
		req.Type = "normal"
	}
	if req.Source == "explore" && (req.Type == "normal" || req.Type == "explore") {
		req.Type = "explore"
	}
	if req.Type != "normal" && req.Type != "tarotist" && req.Type != "explore" {
		return biz("记录类型不正确")
	}
	if req.Type == "explore" {
		req.SkipDetect = true
	}
	if len([]rune(req.Question)) > 200 {
		return biz("问题超过 200 个码点，正式计数口径尚未确认")
	}
	if req.Type == "tarotist" {
		if uid == 0 {
			return biz("登录后才能体验塔罗师")
		}
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
	view, err := a.createNormal(r.Context(), uid, anon, req.Question, req.Type, req.TarotistID, req.SkipDetect || req.Source == "explore" || req.Source == "preset", strings.TrimSpace(req.DeviceID))
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

func (a *App) createNormal(ctx context.Context, uid int64, anon, question, kind string, tarotistID *int64, skip bool, deviceID string) (map[string]any, error) {
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
	onboard, err := a.prepareOnboard(ctx, tx, uid, anon, kind, deviceID)
	if err != nil {
		return nil, err
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO readings(user_id, anonymous_id, type, question, tarotist_id, status, first_experience, onboard_experience) VALUES ($1,$2,$3,$4,$5,'QUESTION_CREATED',$6,$7) RETURNING id`,
		nullID(uid), nullStr(anon), kind, question, tarotistID, first, onboard).Scan(&id)
	if err != nil {
		return nil, err
	}
	if onboard {
		if _, err := tx.Exec(ctx, `UPDATE device_experiences SET reading_id=$1 WHERE device_id=$2 AND consumed=FALSE`, id, resolveDeviceID(ctx, tx, anon, deviceID)); err != nil {
			return nil, err
		}
	}
	cycle := cycleOf(time.Now())
	if uid > 0 {
		cycle = cycleFor(time.Now(), userTimezone(ctx, tx, uid))
	}
	if kind == "explore" {
		skip = true
	}
	if _, err := tx.Exec(ctx, `UPDATE readings SET business_date=$1, skip_detect=$2 WHERE id=$3`, cycle, skip, id); err != nil {
		return nil, err
	}
	consumed := false
	exhausted := false
	rewardUsed := false
	var decision quotaDecision
	member := false
	if uid > 0 {
		member, err = hasActiveMembership(ctx, tx, uid)
		if err != nil {
			return nil, err
		}
	}
	switch {
	case uid == 0:
		decision.Status = quotaAnon
	case kind == "tarotist":
		decision, err = settleSharedQuota(ctx, tx, chargeOpts{
			UserID: uid, ReadingID: id, ChargeKey: chargeKeyPlay, Cycle: cycle,
			AllowTrial: true, ResumeAction: resumeHold,
		})
	case member && kind != "explore":
		decision, err = settleSharedQuota(ctx, tx, chargeOpts{
			UserID: uid, ReadingID: id, ChargeKey: chargeKeyPlay, Cycle: cycle,
			AllowTrial: true, ResumeAction: resumeHold,
		})
	default:
		decision.Status = quotaDeferred
	}
	if err != nil {
		return nil, err
	}
	if decision.Status == quotaCharged {
		rewardUsed = decision.Entitlement == entReward || decision.Entitlement == entInvite
		consumed = decision.Entitlement == entDaily
		exhausted = false
	} else {
		exhausted = !decision.Continue && decision.Status != quotaDeferred && decision.Status != quotaAnon
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
	item["quota_outcome"] = decision.Status
	item["entitlement"] = decision.Entitlement
	item["paywall"] = decision.paywall()
	item["quota_cost_unconfigured"] = decision.unconfigured()
	item["kept"] = true
	item["generated"] = false
	if exhausted && decision.Message != "" {
		item["quota_prompt"] = T(ctx, decision.Message)
	}
	return item, nil
}

func resolveDeviceID(ctx context.Context, ex execer, anon, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested
	}
	if anon == "" {
		return ""
	}
	var stored string
	_ = ex.QueryRow(ctx, `SELECT device_id FROM anon_devices WHERE anonymous_id=$1`, anon).Scan(&stored)
	if strings.TrimSpace(stored) != "" {
		return stored
	}
	return "anon:" + anon
}

func (a *App) prepareOnboard(ctx context.Context, tx Tx, uid int64, anon, kind, deviceID string) (bool, error) {
	if kind != "normal" || uid != 0 {
		return false, nil
	}
	deviceID = resolveDeviceID(ctx, tx, anon, deviceID)
	if deviceID == "" {
		return false, nil
	}
	var consumed bool
	err := tx.QueryRow(ctx, `SELECT consumed FROM device_experiences WHERE device_id=$1 FOR UPDATE`, deviceID).Scan(&consumed)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(ctx, `INSERT INTO device_experiences(device_id, anonymous_id) VALUES ($1,$2)`, deviceID, anon)
		return err == nil, err
	}
	if err != nil {
		return false, err
	}
	return !consumed, nil
}

func userTimezone(ctx context.Context, ex execer, uid int64) string {
	tz := "Asia/Shanghai"
	if uid <= 0 || ex == nil {
		return tz
	}
	_ = ex.QueryRow(ctx, `SELECT COALESCE(NULLIF(timezone,''),'Asia/Shanghai') FROM app_users WHERE id=$1`, uid).Scan(&tz)
	if tz == "" {
		return "Asia/Shanghai"
	}
	return tz
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
	var cardCount int
	err = a.Store.QueryRow(r.Context(), `SELECT enabled, card_count FROM spreads WHERE id=$1`, req.SpreadID).Scan(&enabled, &cardCount)
	if err != nil || !enabled {
		return biz("牌阵不可用")
	}
	if uid > 0 {
		member, merr := hasActiveMembership(r.Context(), a.Store, uid)
		if merr != nil {
			return merr
		}
		if !member && cardCount > 3 {
			return biz("开通会员后可使用超过三张牌的牌阵")
		}
	} else if cardCount > 3 {
		return biz("开通会员后可使用超过三张牌的牌阵")
	}
	tag, err := a.Store.Exec(r.Context(), `UPDATE readings SET spread_id=$1, spread_name=(SELECT name FROM spreads WHERE id=$1), status='SPREAD_SELECTED', updated_at=now() WHERE id=$2 AND status='QUESTION_CREATED' AND deleted_at IS NULL`, req.SpreadID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("当前状态不能选择牌阵")
	}
	item, err := a.loadReadingAbs(r.Context(), r, id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) appDraw(w http.ResponseWriter, r *http.Request) error {
	stop, err := a.gatePlay(w, r, resumeDraw)
	if err != nil || stop {
		return err
	}
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
		Mode    string `json:"mode"`
		Numbers []int  `json:"numbers"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.Mode != "normal" && req.Mode != "numerology" {
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
	var cards []drawnCard
	if req.Mode == "numerology" {
		cards, err = drawCardsNumerology(positions, req.Numbers)
	} else {
		cards, err = drawCards(positions)
	}
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
	item, err := a.loadReadingAbs(r.Context(), r, id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) appInterpret(w http.ResponseWriter, r *http.Request) error {
	if wantsSSE(r) {
		return a.appInterpretStream(w, r)
	}
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
	var kind, question string
	if err := a.Store.QueryRow(r.Context(), `SELECT type, question FROM readings WHERE id=$1`, id).Scan(&kind, &question); err != nil {
		return err
	}
	if kind == "explore" && strings.TrimSpace(question) == "" {
		return biz("请先填写问题")
	}
	if uid == 0 {
		return biz(paywallNeedLogin)
	}
	stop, err := a.gatePlay(w, r, resumeInterpret)
	if err != nil || stop {
		return err
	}
	item, err := a.generate(r.Context(), id, false)
	if err != nil {
		return err
	}
	a.applyCardThumbnails(r.Context(), r, item)
	writeOK(w, item)
	return nil
}

func (a *App) appListReadings(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	limit := parseLimit(q.Get("limit"), 20, 100)
	typ := q.Get("type")
	if typ != "" && typ != "normal" && typ != "tarotist" && typ != "explore" && typ != "daily" {
		return biz("type 不正确")
	}
	var beforeID int64
	if s := q.Get("before_id"); s != "" {
		v, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil || v <= 0 {
			return biz("before_id 不正确")
		}
		beforeID = v
	}
	// 按 id 降序做游标分页（id 随创建单调，比 updated_at 翻页更稳）。多取一条判断是否还有下一页。
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if uid > 0 {
		add("user_id=$%d", uid)
	} else {
		add("anonymous_id=$%d", anon)
	}
	if typ != "" {
		add("type=$%d", typ)
	}
	if beforeID > 0 {
		add("id<$%d", beforeID)
	}
	args = append(args, limit+1)
	cols := `id, user_id, anonymous_id, type, question, spread_id, tarotist_id, status, cards, interpretation, vendor_name, model_name, fail_reason, quota_consumed, first_experience, created_at, updated_at, deleted_at`
	query := "SELECT " + cols + " FROM readings WHERE " + strings.Join(where, " AND ") + " ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args))
	rows, err := a.Store.Query(r.Context(), query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := scanReadings(rows)
	a.applyCardThumbnails(r.Context(), r, items...)
	for _, item := range items {
		a.localizeReading(r.Context(), item)
	}
	hasMore := false
	var nextCursor int64
	if len(items) > limit {
		items = items[:limit]
		hasMore = true
		if last, ok := items[len(items)-1]["id"].(int64); ok {
			nextCursor = last
		}
	}
	writeOK(w, map[string]any{"items": items, "has_more": hasMore, "next_cursor": nextCursor, "limit": limit})
	return nil
}

// parseLimit 解析分页 limit：空或非法用 def，超过 max 则截到 max。
func parseLimit(s string, def, max int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
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
	item, err := a.loadReadingAbs(r.Context(), r, id)
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
	if wantsSSE(r) {
		return a.appDailyStream(w, r)
	}
	id, target, business, existing, written, err := a.openDaily(w, r, false)
	if err != nil || written {
		return err
	}
	if existing != nil {
		return nil
	}
	item, err := a.generate(r.Context(), id, false)
	if err != nil {
		failed, loadErr := a.loadReadingAbs(r.Context(), r, id)
		if loadErr != nil {
			return err
		}
		a.attachDailyHome(r.Context(), r, failed, target, business)
		writeOK(w, failed)
		return nil
	}
	a.attachDailyHome(r.Context(), r, item, target, business)
	writeOK(w, item)
	return nil
}

func (a *App) openDaily(w http.ResponseWriter, r *http.Request, stream bool) (id int64, target, business string, existing map[string]any, written bool, err error) {
	uid, _, err := a.appActor(r)
	if err != nil {
		return 0, "", "", nil, false, err
	}
	if uid == 0 {
		return 0, "", "", nil, false, biz("登录后才能抽今日一牌")
	}
	var req struct {
		Target string `json:"target"`
	}
	if err := decode(r, &req); err != nil {
		return 0, "", "", nil, false, err
	}
	target = req.Target
	cycle := a.userCycle(r, uid)
	business = cycle
	question := "今日一牌"
	slotName := "今日"
	meaning := "当日一牌"
	switch req.Target {
	case "", "today":
	case "tomorrow":
		business, err = nextBusinessDate(cycle)
		if err != nil {
			return 0, "", "", nil, false, err
		}
		question = "明日运势"
		slotName = "明日"
		meaning = "明日运势"
	default:
		return 0, "", "", nil, false, biz("目标日期不正确")
	}
	slotLimit, err := loadDailySlotCount(r.Context(), a.Store)
	if err != nil {
		return 0, target, business, nil, false, err
	}
	var usedSlots int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings
		WHERE user_id=$1 AND type='daily' AND business_date=$2 AND deleted_at IS NULL
		AND cards IS NOT NULL AND cards <> ''`, uid, business).Scan(&usedSlots); err != nil {
		return 0, target, business, nil, false, err
	}
	if usedSlots >= slotLimit {
		var existingID int64
		err = a.Store.QueryRow(r.Context(), `SELECT id FROM readings
			WHERE user_id=$1 AND type='daily' AND business_date=$2 AND deleted_at IS NULL
			AND cards IS NOT NULL AND cards <> ''
			ORDER BY id DESC LIMIT 1`, uid, business).Scan(&existingID)
		if err != nil {
			return 0, target, business, nil, false, err
		}
		item, loadErr := a.loadReadingAbs(r.Context(), r, existingID)
		if loadErr != nil {
			return 0, target, business, nil, false, loadErr
		}
		if stream {
			return existingID, target, business, item, false, nil
		}
		msg := T(r.Context(), "当日今日一牌名额已占用")
		if business != cycle {
			msg = T(r.Context(), "该日已有结果，不重复扣次数或钻石")
		}
		a.attachDailyHome(r.Context(), r, item, req.Target, business)
		writeJSON(w, http.StatusOK, map[string]any{
			"code": 1, "message": msg,
			"data": map[string]any{"reused": true, "charged": false, "reading": item},
		})
		return existingID, target, business, item, true, nil
	}
	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return 0, target, business, nil, false, err
	}
	defer tx.Rollback(r.Context())
	err = tx.QueryRow(r.Context(), `SELECT id FROM readings
		WHERE user_id=$1 AND type='daily' AND business_date=$2 AND deleted_at IS NULL
		AND (cards IS NULL OR cards='')
		ORDER BY id DESC LIMIT 1 FOR UPDATE`, uid, business).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRow(r.Context(), `INSERT INTO readings(user_id, type, question, status, business_date, quota_cycle)
			VALUES ($1,'daily',$2,'QUESTION_CREATED',$3,$4) RETURNING id`, uid, question, business, cycle).Scan(&id)
	}
	if err != nil {
		return 0, target, business, nil, false, err
	}
	decision, err := settleSharedQuota(r.Context(), tx, chargeOpts{
		UserID: uid, ReadingID: id, ChargeKey: chargeKeyPlay, Cycle: cycle,
		AllowTrial: false, ResumeAction: resumeDaily,
	})
	if err != nil {
		return 0, target, business, nil, false, err
	}
	if !decision.Continue {
		if err := tx.Commit(r.Context()); err != nil {
			return 0, target, business, nil, false, err
		}
		writeQuotaBlock(w, r, decision, id)
		return id, target, business, nil, true, nil
	}
	cards, err := drawCards([]position{{Index: 1, Name: slotName, Meaning: meaning}})
	if err != nil {
		return 0, target, business, nil, false, err
	}
	if _, err := tx.Exec(r.Context(), `UPDATE readings SET cards=$1, status='CARDS_FIXED', question=$2, business_date=$3, updated_at=now() WHERE id=$4`,
		mustJSON(cards), question, business, id); err != nil {
		return 0, target, business, nil, false, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return 0, target, business, nil, false, err
	}
	return id, target, business, nil, false, nil
}
