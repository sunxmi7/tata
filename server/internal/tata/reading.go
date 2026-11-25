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
	q := `SELECT id, user_id, type, question, spread_id, tarotist_id, status, cards, interpretation, vendor_name, model_name, fail_reason, quota_consumed, first_experience, created_at, updated_at, deleted_at, draw_mode FROM readings WHERE 1=1`
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
	rows, err := a.Store.Query(r.Context(), `SELECT l.id, l.user_id, l.reading_id, l.entitlement, l.amount, l.reason, l.result, l.created_at, l.grant_id,
		r.status,
		COALESCE((
			SELECT json_agg(json_build_object(
				'lot_id', d.lot_id,
				'source', d.source,
				'amount', d.amount,
				'remaining_after', d.remaining_after,
				'expires_at', d.expires_at
			) ORDER BY d.id)
			FROM ledger_lot_draws d WHERE d.ledger_id = l.id
		), '[]'::json)
		FROM ledger l
		LEFT JOIN readings r ON r.id = l.reading_id
		ORDER BY l.id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, userID int64
		var readingID, grantID sql.NullInt64
		var entitlement, reason, result string
		var readingStatus sql.NullString
		var amount int
		var created time.Time
		var drawsRaw []byte
		if err := rows.Scan(&id, &userID, &readingID, &entitlement, &amount, &reason, &result, &created, &grantID, &readingStatus, &drawsRaw); err != nil {
			return err
		}
		var draws []any
		if len(drawsRaw) > 0 {
			if err := json.Unmarshal(drawsRaw, &draws); err != nil {
				return err
			}
		}
		if draws == nil {
			draws = []any{}
		}
		list = append(list, map[string]any{
			"id": id, "user_id": userID, "reading_id": nullInt(readingID), "entitlement": entitlement,
			"amount": amount, "reason": reason, "result": result, "created_at": created, "grant_id": nullInt(grantID),
			"reading_status": nullString(readingStatus), "draws": draws,
		})
	}
	writeOK(w, list)
	return nil
}

func (a *App) appLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email        string `json:"email"`
		Code         string `json:"code"`
		InviteCode   string `json:"invite_code"`
		InviteSource string `json:"invite_source"`
		DeviceID     string `json:"device_id"`
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
	if err := a.rememberLoginDevice(r.Context(), id, req.DeviceID); err != nil {
		writeErr(w, r, http.StatusOK, "注册赠送记录失败")
		return
	}
	if err := a.finishRegistration(r.Context(), id, created); err != nil {
		writeErr(w, r, http.StatusOK, "注册赠送记录失败")
		return
	}
	a.ensureProfileDefaults(r.Context(), id)
	a.consumeEmailCode(r.Context(), "login", email)
	a.attachDeviceReadings(r.Context(), id, req.DeviceID)
	a.applyDeviceForfeit(r.Context(), id, req.DeviceID)
	a.bindInvite(r.Context(), id, req.DeviceID, req.InviteCode, req.InviteSource)
	access, _ := a.signAccess("app", id, "")
	settled := a.settleQuestionStart(r, id)
	writeOK(w, map[string]any{"accessToken": access, "id": id, "login": "email", "reward_consumed": settled, "needs_confirm": false})
}

func (a *App) attachDeviceReadings(ctx context.Context, uid int64, deviceID string) {
	if uid <= 0 {
		return
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return
	}
	_, _ = a.Store.Exec(ctx, `UPDATE readings SET user_id=$1 WHERE user_id IS NULL AND device_id<>'' AND lower(device_id)=lower($2)`, uid, deviceID)
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
	if onboard {
		var trialUsed bool
		if err := a.Store.QueryRow(r.Context(), `SELECT reading_trial_used FROM app_users WHERE id=$1`, uid).Scan(&trialUsed); err != nil || !trialUsed {
			return false
		}
	}
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
	uid, err := a.appActor(r)
	if err != nil {
		return err
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
	uid, err := a.appActor(r)
	if err != nil {
		return err
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
		FROM spreads WHERE enabled=true AND theme_key='' AND depth_group<>'' ORDER BY sort_order, id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	grouped := map[string][]map[string]any{}
	for rows.Next() {
		var id int64
		var name, nameEN, desc, pos, thumb, themeKey, themeTitle, themeSub, short, depth string
		var count int
		var charged, recommended bool
		var quota sql.NullInt64
		if err := rows.Scan(&id, &name, &nameEN, &desc, &count, &pos, &charged, &recommended, &thumb, &themeKey, &themeTitle, &themeSub, &short, &depth, &quota); err != nil {
			return err
		}
		grouped[depth] = append(grouped[depth], map[string]any{
			"id": id, "name": T(r.Context(), name), "name_en": nameEN, "description": T(r.Context(), desc), "card_count": count,
			"positions": localizeJSONFields(r.Context(), json.RawMessage(pos), "name", "meaning"),
			"charged":   charged, "recommended": recommended, "thumbnail": absoluteAssetURL(r, deckPreviewURL(thumb)),
			"theme_key": themeKey, "theme_title": themeTitle, "theme_subtitle": themeSub,
			"short_sentence": short,
			"depth_group":    depth, "depth_label": depthLabel(depth),
			"quota_cost": quotaCostValue(quota), "price_ledger": quotaLedger(quota),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	groups := make([]map[string]any, 0, 3)
	for _, depth := range []string{"quick", "advanced", "panorama"} {
		spreads := grouped[depth]
		if spreads == nil {
			spreads = []map[string]any{}
		}
		groups = append(groups, map[string]any{
			"depth_group": depth, "depth_label": depthLabel(depth), "title": depthTitle(depth),
			"spreads": spreads,
		})
	}
	writeOK(w, map[string]any{"groups": groups})
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
	uid, err := a.appActor(r)
	if err != nil {
		return err
	}
	cycle := cycleOf(time.Now())
	inviteRemaining := 0
	adminGrantRemaining := 0
	subPrompt := false
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
		inviteRemaining, err = inviteBalance(r.Context(), a.Store, uid)
		if err != nil {
			return err
		}
		_ = a.Store.QueryRow(r.Context(), `SELECT admin_grant_remaining FROM rewards WHERE user_id=$1`, uid).Scan(&adminGrantRemaining)
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
		"cycle":                        cycle,
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
		if qerr := expireQuotaLots(r.Context(), a.Store, uid); qerr != nil {
			return qerr
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
	} else {
		var invited int
		if uid > 0 {
			_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM invite_rewards WHERE inviter_id=$1`, uid).Scan(&invited)
		}
		out["invite_remaining"] = inviteRemaining
		out["invite_count"] = invited
		out["max_invites"] = inviteMaxPeople
		out["admin_grant_free_remaining"] = adminGrantRemaining
		out["priority"] = T(r.Context(), "注册赠送 → 邀请奖励 → 兑换免费次数")
	}
	writeOK(w, out)
	return nil
}

func (a *App) appCreateReading(w http.ResponseWriter, r *http.Request) error {
	uid, device, err := a.divinationActor(r)
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
		DrawMode   string `json:"draw_mode"`
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
	drawMode := strings.TrimSpace(req.DrawMode)
	if drawMode != "" {
		if _, err := parseDrawMode(drawMode); err != nil {
			return err
		}
		if req.Type != "normal" {
			return biz("抽牌方式不正确")
		}
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
	scope := "device:" + strings.ToLower(device)
	if uid > 0 {
		scope = "user:" + strconv.FormatInt(uid, 10)
	}
	if replay, ok, err := a.beginIdem(r, scope, r.Header.Get("Idempotency-Key")); err != nil {
		return err
	} else if ok {
		writeJSON(w, http.StatusOK, json.RawMessage(replay))
		return nil
	}
	boundDevice := strings.TrimSpace(req.DeviceID)
	if uid == 0 {
		boundDevice = device
	}
	view, err := a.createNormal(r.Context(), uid, req.Question, req.Type, req.TarotistID, req.SkipDetect || req.Source == "explore" || req.Source == "preset", boundDevice, drawMode)
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

func parseDrawMode(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "normal", "numerology":
		return strings.TrimSpace(raw), nil
	default:
		return "", biz("抽牌方式不正确")
	}
}

func storeNumerologyMap(ctx context.Context, ex execer, id int64) error {
	var raw string
	if err := ex.QueryRow(ctx, `SELECT numerology_map FROM readings WHERE id=$1`, id).Scan(&raw); err != nil {
		return err
	}
	text := strings.TrimSpace(raw)
	if text != "" && text != "[]" && text != "null" {
		return nil
	}
	slots, err := newNumerologyMap()
	if err != nil {
		return err
	}
	_, err = ex.Exec(ctx, `UPDATE readings SET numerology_map=$1 WHERE id=$2 AND numerology_map IN ('', '[]', 'null')`, mustJSON(slots), id)
	return err
}

func (a *App) createNormal(ctx context.Context, uid int64, question, kind string, tarotistID *int64, skip bool, deviceID, drawMode string) (map[string]any, error) {
	first := false
	boundDevice := strings.TrimSpace(deviceID)
	if uid == 0 {
		if !validDeviceID(boundDevice) {
			return nil, biz("缺少设备标识")
		}
		var n int
		if err := a.Store.QueryRow(ctx, `SELECT count(*) FROM readings WHERE user_id IS NULL AND lower(device_id)=lower($1)`, boundDevice).Scan(&n); err != nil {
			return nil, err
		}
		first = n == 0
	}
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	onboard := false
	if kind == "normal" {
		onboard, err = a.prepareOnboard(ctx, tx, uid, kind, boundDevice)
		if err != nil {
			return nil, err
		}
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO readings(user_id, type, question, tarotist_id, status, first_experience, onboard_experience, device_id, draw_mode) VALUES ($1,$2,$3,$4,'QUESTION_CREATED',$5,$6,$7,$8) RETURNING id`,
		nullID(uid), kind, question, tarotistID, first, onboard, boundDevice, drawMode).Scan(&id)
	if err != nil {
		return nil, err
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
	if drawMode == "numerology" {
		if err := storeNumerologyMap(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	consumed := false
	exhausted := false
	rewardUsed := false
	var decision quotaDecision
	switch {
	case uid == 0:
		decision.Status = quotaAnon
	case kind == "tarotist":
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

func (a *App) prepareOnboard(ctx context.Context, tx Tx, uid int64, kind, deviceID string) (bool, error) {
	if kind != "normal" {
		return false, nil
	}
	if uid > 0 {
		var used bool
		if err := tx.QueryRow(ctx, `SELECT reading_trial_used FROM app_users WHERE id=$1 FOR UPDATE`, uid).Scan(&used); err != nil {
			return false, err
		}
		if used {
			return false, nil
		}
		var open int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM readings WHERE user_id=$1 AND onboard_experience AND NOT quota_settled AND deleted_at IS NULL`, uid).Scan(&open); err != nil {
			return false, err
		}
		return open == 0, nil
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false, nil
	}
	var consumed, forfeited bool
	err := tx.QueryRow(ctx, `SELECT consumed, trial_forfeited FROM device_experiences WHERE device_id=$1 FOR UPDATE`, deviceID).Scan(&consumed, &forfeited)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(ctx, `INSERT INTO device_experiences(device_id) VALUES ($1)`, deviceID)
		return err == nil, err
	}
	if err != nil {
		return false, err
	}
	return !consumed && !forfeited, nil
}

func openOnboardReading(ctx context.Context, ex rowQuery, readingID, uid int64) (bool, error) {
	if readingID <= 0 || uid <= 0 {
		return false, nil
	}
	var onboard, used bool
	err := ex.QueryRow(ctx, `SELECT r.onboard_experience, u.reading_trial_used
		FROM readings r JOIN app_users u ON u.id=$2 WHERE r.id=$1`, readingID, uid).Scan(&onboard, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return onboard && !used, nil
}

func (a *App) consumeOnboardIfReady(ctx context.Context, readingID int64) error {
	var uid sql.NullInt64
	var onboard bool
	var status string
	err := a.Store.QueryRow(ctx, `SELECT user_id, onboard_experience, status FROM readings WHERE id=$1`, readingID).Scan(&uid, &onboard, &status)
	if err != nil || !onboard || !uid.Valid || status != "COMPLETED" {
		return err
	}
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := consumeOnboardExperience(ctx, tx, uid.Int64, readingID); err != nil {
		return err
	}
	if err := recordCharge(ctx, tx, chargeOpts{
		UserID: uid.Int64, ReadingID: readingID, ChargeKey: chargeKeyPlay, ResumeAction: resumeInterpret,
	}, entOnboard, 1, "", "消耗新人引导体验", nil); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE readings SET quota_settled=TRUE WHERE id=$1`, readingID); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
	uid, id, _, err := a.beginReading(r)
	if err != nil {
		return err
	}
	var req struct {
		SpreadID int64   `json:"spread_id"`
		DrawMode *string `json:"draw_mode"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	var kind, storedMode string
	if err := a.Store.QueryRow(r.Context(), `SELECT type, draw_mode FROM readings WHERE id=$1`, id).Scan(&kind, &storedMode); err != nil {
		return err
	}
	resolved := storedMode
	if req.DrawMode != nil {
		mode, err := parseDrawMode(*req.DrawMode)
		if err != nil {
			return err
		}
		if kind != "normal" {
			return biz("抽牌方式不正确")
		}
		resolved = mode
	}
	if resolved != storedMode {
		already, _, err := playAlready(r.Context(), a.Store, id)
		if err != nil {
			return err
		}
		if already {
			return biz("抽牌方式已锁定")
		}
	}
	if kind == "normal" && resolved == "" && uid > 0 {
		open, err := openOnboardReading(r.Context(), a.Store, id, uid)
		if err != nil {
			return err
		}
		if !open {
			return biz("请先选择抽牌方式")
		}
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
	tag, err := a.Store.Exec(r.Context(), `UPDATE readings SET spread_id=$1, spread_name=(SELECT name FROM spreads WHERE id=$1), draw_mode=$2, status='SPREAD_SELECTED',
		spread_selection=CASE WHEN (SELECT recommended FROM spreads WHERE id=$1) THEN 'system_recommendation' ELSE 'manual_selection' END,
		updated_at=now() WHERE id=$3 AND status='QUESTION_CREATED' AND deleted_at IS NULL`, req.SpreadID, resolved, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("当前状态不能选择牌阵")
	}
	if resolved == "numerology" {
		if err := storeNumerologyMap(r.Context(), a.Store, id); err != nil {
			return err
		}
	}
	if uid > 0 {
		open, oerr := openOnboardReading(r.Context(), a.Store, id, uid)
		if oerr != nil {
			return oerr
		}
		if open {
			item, err := a.loadReadingAbs(r.Context(), r, id)
			if err != nil {
				return err
			}
			writeOK(w, item)
			return nil
		}
		stop, err := a.chargeAtSpread(w, r, id)
		if err != nil || stop {
			return err
		}
	}
	item, err := a.loadReadingAbs(r.Context(), r, id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) fixNumerologyCards(ctx context.Context, id int64, numbers []int, positions []position) error {
	slots, err := a.readingNumerologyMap(ctx, id)
	if err != nil {
		return err
	}
	cards, err := cardsFromNumerologyMap(slots, numbers, positions)
	if err != nil {
		return err
	}
	tag, err := a.Store.Exec(ctx, `UPDATE readings SET cards=$1, status='CARDS_FIXED', updated_at=now() WHERE id=$2 AND status IN ('SPREAD_SELECTED','SHUFFLING','CUTTING') AND deleted_at IS NULL`, mustJSON(cards), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("牌面已固定或状态已变化")
	}
	return nil
}

func (a *App) readingNumerologyMap(ctx context.Context, id int64) ([]numerologySlot, error) {
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var raw string
	if err := tx.QueryRow(ctx, `SELECT numerology_map FROM readings WHERE id=$1 FOR UPDATE`, id).Scan(&raw); err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) != "" {
		var slots []numerologySlot
		if err := json.Unmarshal([]byte(raw), &slots); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return slots, nil
	}
	slots, err := newNumerologyMap()
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE readings SET numerology_map=$1 WHERE id=$2`, mustJSON(slots), id); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return slots, nil
}

func (a *App) appRedraw(w http.ResponseWriter, r *http.Request) error {
	_, id, _, err := a.beginReading(r)
	if err != nil {
		return err
	}
	var req struct {
		Number int `json:"number"`
	}
	if err := decode(r, &req); err != nil || req.Number <= 0 {
		return biz("请输入要补抽的灵数")
	}
	var status, cardsRaw, mapRaw, drawMode string
	if err := a.Store.QueryRow(r.Context(), `SELECT status, COALESCE(cards,''), numerology_map, draw_mode FROM readings WHERE id=$1`, id).Scan(&status, &cardsRaw, &mapRaw, &drawMode); err != nil {
		return err
	}
	if drawMode != "numerology" {
		if drawMode == "" {
			return biz("请先选择抽牌方式")
		}
		return biz("请先用灵数抽牌")
	}
	if strings.TrimSpace(mapRaw) == "" {
		return biz("请先用灵数抽牌")
	}
	if status != "CARDS_FIXED" {
		return biz("当前状态不能补抽")
	}
	var cards []drawnCard
	if err := json.Unmarshal([]byte(cardsRaw), &cards); err != nil || len(cards) == 0 {
		return biz("请先用灵数抽牌")
	}
	var slots []numerologySlot
	if err := json.Unmarshal([]byte(mapRaw), &slots); err != nil {
		return err
	}
	if len(cards) >= 78 {
		return biz("78 张牌已全部使用")
	}
	for _, card := range cards {
		if card.Number == req.Number {
			return biz("灵数不能重复")
		}
	}
	added, err := cardsFromNumerologyMap(slots, []int{req.Number}, []position{{Index: len(cards) + 1}})
	if err != nil {
		return err
	}
	for _, card := range cards {
		if card.CardID == added[0].CardID {
			return biz("牌面不能重复")
		}
	}
	cards = append(cards, added[0])
	if _, err := a.Store.Exec(r.Context(), `UPDATE readings SET cards=$1, updated_at=now() WHERE id=$2 AND status='CARDS_FIXED' AND numerology_map=$3`, mustJSON(cards), id, mapRaw); err != nil {
		return err
	}
	item, err := a.loadReadingAbs(r.Context(), r, id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) acceptClientCards(ctx context.Context, id, spreadID int64, cards []drawnCard) error {
	if spreadID <= 0 {
		return biz("请先选择牌阵")
	}
	var drawMode string
	if err := a.Store.QueryRow(ctx, `SELECT draw_mode FROM readings WHERE id=$1`, id).Scan(&drawMode); err != nil {
		return err
	}
	if drawMode == "numerology" {
		return biz("抽牌方式不正确")
	}
	if len(cards) == 0 {
		return biz("请交上抽好的牌")
	}
	var enabled bool
	var cardCount int
	var posRaw string
	err := a.Store.QueryRow(ctx, `SELECT enabled, card_count, positions FROM spreads WHERE id=$1`, spreadID).Scan(&enabled, &cardCount, &posRaw)
	if err != nil || !enabled {
		return biz("牌阵不可用")
	}
	if len(cards) != cardCount {
		return biz("牌数须等于牌阵张数")
	}
	var positions []position
	if err := json.Unmarshal([]byte(posRaw), &positions); err != nil {
		return err
	}
	seen := map[int]bool{}
	for i := range cards {
		card, ok := cardByName(strings.TrimSpace(cards[i].Name))
		if !ok {
			return biz("牌面不正确")
		}
		if seen[card.ID] {
			return biz("牌面不能重复")
		}
		seen[card.ID] = true
		if cards[i].Orientation != "upright" && cards[i].Orientation != "reversed" {
			return biz("正逆位不正确")
		}
		cards[i].CardID = card.ID
		cards[i].Name = card.Name
		cards[i].Position = i + 1
		if cards[i].PositionName == "" && i < len(positions) {
			cards[i].PositionName = positions[i].Name
		}
	}
	tag, err := a.Store.Exec(ctx, `UPDATE readings SET spread_id=$1, spread_name=(SELECT name FROM spreads WHERE id=$1), cards=$2, status='CARDS_FIXED',
		spread_selection=CASE WHEN (SELECT recommended FROM spreads WHERE id=$1) THEN 'system_recommendation' ELSE 'manual_selection' END,
		updated_at=now()
		WHERE id=$3 AND status IN ('QUESTION_CREATED','SPREAD_SELECTED','SHUFFLING','CUTTING') AND deleted_at IS NULL`,
		spreadID, mustJSON(cards), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("牌面已固定或状态已变化")
	}
	return nil
}

func (a *App) appInterpret(w http.ResponseWriter, r *http.Request) error {
	if wantsSSE(r) {
		return a.appInterpretStream(w, r)
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
	if err := a.takeClientCards(r, id); err != nil {
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
	if _, err := a.generate(r.Context(), id, false); err != nil {
		return err
	}
	view, err := a.conversationReply(r, id)
	if err != nil {
		return err
	}
	writeOK(w, view)
	return nil
}

func (a *App) takeClientCards(r *http.Request, id int64) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	var req struct {
		SpreadID int64       `json:"spread_id"`
		Cards    []drawnCard `json:"cards"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.SpreadID == 0 && len(req.Cards) == 0 {
		return nil
	}
	var status string
	var cards sql.NullString
	if err := a.Store.QueryRow(r.Context(), `SELECT status, cards FROM readings WHERE id=$1`, id).Scan(&status, &cards); err != nil {
		return err
	}
	// App often posts cards on every interpret. Once locked, keep the fixed face and continue.
	switch status {
	case "CARDS_FIXED", "FAILED_RETRYABLE", "INTERPRETING", "COMPLETED":
		if cards.Valid && strings.TrimSpace(cards.String) != "" && strings.TrimSpace(cards.String) != "[]" && strings.TrimSpace(cards.String) != "null" {
			return nil
		}
	}
	return a.acceptClientCards(r.Context(), id, req.SpreadID, req.Cards)
}

func (a *App) appListReadings(w http.ResponseWriter, r *http.Request) error {
	uid, device, err := a.divinationActor(r)
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
		add("user_id IS NULL AND lower(device_id)=lower($%d)", device)
	}
	if typ != "" {
		add("type=$%d", typ)
	}
	if beforeID > 0 {
		add("id<$%d", beforeID)
	}
	args = append(args, limit+1)
	cols := `id, user_id, type, question, spread_id, tarotist_id, status, cards, interpretation, vendor_name, model_name, fail_reason, quota_consumed, first_experience, created_at, updated_at, deleted_at, draw_mode`
	query := "SELECT " + cols + " FROM readings WHERE " + strings.Join(where, " AND ") + " ORDER BY id DESC LIMIT $" + strconv.Itoa(len(args))
	rows, err := a.Store.Query(r.Context(), query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := scanReadings(rows)
	if err := a.markPendingRedraws(r.Context(), items); err != nil {
		return err
	}
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
	_, id, _, err := a.beginReading(r)
	if err != nil {
		return err
	}
	item, err := a.loadReadingAbs(r.Context(), r, id)
	if err != nil {
		return err
	}
	if err := a.attachPendingRedraw(r.Context(), id, item); err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) appDeleteReading(w http.ResponseWriter, r *http.Request) error {
	uid, err := a.appActor(r)
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
	if err := a.ownReading(r.Context(), id, uid, ""); err != nil {
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
	uid, err := a.appActor(r)
	if err != nil {
		return 0, "", "", nil, false, err
	}
	if uid == 0 {
		return 0, "", "", nil, false, biz("登录后才能抽今日一牌")
	}
	var req struct {
		Target string      `json:"target"`
		Cards  []drawnCard `json:"cards"`
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
	clientCards, err := normalizeDailyCards(req.Cards, slotName)
	if err != nil {
		return 0, "", "", nil, false, err
	}
	var usedSlots int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings
		WHERE user_id=$1 AND type='daily' AND business_date=$2
		AND cards IS NOT NULL AND cards <> ''`, uid, business).Scan(&usedSlots); err != nil {
		return 0, target, business, nil, false, err
	}
	if usedSlots >= 1 {
		var existingID int64
		err = a.Store.QueryRow(r.Context(), `SELECT id FROM readings
			WHERE user_id=$1 AND type='daily' AND business_date=$2 AND deleted_at IS NULL
			AND cards IS NOT NULL AND cards <> ''
			ORDER BY id DESC LIMIT 1`, uid, business).Scan(&existingID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, target, business, nil, false, biz("该日已使用，删除记录后不能再抽")
		}
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
			msg = T(r.Context(), "该日已有结果，不重复扣额度")
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
	cards := clientCards
	if len(cards) == 0 {
		cards, err = drawCards([]position{{Index: 1, Name: slotName, Meaning: meaning}})
		if err != nil {
			return 0, target, business, nil, false, err
		}
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

// normalizeDailyCards accepts the single card App drew for today/tomorrow.
// Empty means the server draws. More than one card is rejected.
func normalizeDailyCards(in []drawnCard, slotName string) ([]drawnCard, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) != 1 {
		return nil, biz("今日一牌只收一张牌")
	}
	name := strings.TrimSpace(in[0].Name)
	card, ok := cardByName(name)
	if !ok {
		return nil, biz("牌面不正确")
	}
	ori := strings.TrimSpace(in[0].Orientation)
	if ori != "upright" && ori != "reversed" {
		return nil, biz("正逆位不正确")
	}
	posName := strings.TrimSpace(in[0].PositionName)
	if posName == "" {
		posName = slotName
	}
	return []drawnCard{{
		Position:     1,
		PositionName: posName,
		CardID:       card.ID,
		Name:         card.Name,
		Orientation:  ori,
	}}, nil
}
