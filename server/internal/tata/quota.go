package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Shared quota priority (B1, free tier keeps the older sub-order):
//  1. registration reward (user-facing 次数)
//  2. today's daily free (user-facing 次数)
//  3. admin-granted free counts (user-facing 次数; skipped when the balance is 0)
//  4. diamonds (user-facing 钻石 / 点数)
//  5. paywall — success resumes the interrupted task; cancel keeps it and does not generate
//
// An admin grant only adds balance. It does not resume a paywall; that stays on IAP credit.
//
// Diamond bucket order is fixed: consumable pack first, then subscription.
// One charge is taken from a single bucket (no split). Pack is spent first because
// subscription diamonds are replaced on period refresh (applyDiamonds), while pack
// diamonds accumulate. Do not reorder without updating TestSharedQuotaPriority.
//
// diamond_cost comes from quota_settings.diamond_cost. NULL or missing means the
// diamond step is unconfigured: do not invent an amount and do not debit diamonds.

const (
	chargeKeyPlay = "play"

	entReward    = "registration_reward"
	entDaily     = "daily_free"
	entPack      = "diamond_pack"
	entSub       = "diamond_subscription"
	entTrial     = "ai_trial"
	entAdminFree = "admin_grant_free"

	quotaCharged   = "charged"
	quotaAlready   = "already"
	quotaPaywall   = "paywall"
	quotaUnconfig  = "diamond_cost_unconfigured"
	quotaAnon      = "anonymous"
	quotaDeferred  = "deferred"
	quotaCancelled = "cancelled"

	resumeHold      = "hold"
	resumeShuffle   = "shuffle"
	resumeDraw      = "draw"
	resumeInterpret = "interpret"
	resumeDaily     = "daily_draw"
	resumeFollowup  = "followup"

	paywallPending = "pending"
	paywallResumed = "resumed"
	paywallCancel  = "cancelled"
	paywallUncfg   = "unconfigured"

	quotaClosedNote = "已按优先级处理：注册奖励，今日免费次数，人工补发免费次数，然后钻石"
	wechatQuotaNote = "注册奖励优先于今日免费次数，提问时自动扣减"
)

type chargeOpts struct {
	UserID       int64
	ReadingID    int64
	ChargeKey    string
	Cycle        string
	AllowTrial   bool
	ResumeAction string
}

type quotaDecision struct {
	Status      string
	Entitlement string
	Amount      int
	Bucket      string
	Continue    bool
	Message     string
}

func (d quotaDecision) paywall() bool { return d.Status == quotaPaywall }

func (d quotaDecision) unconfigured() bool { return d.Status == quotaUnconfig }

// settleSharedQuota debits at most once for (reading, charge key).
// Caller owns the transaction and must commit or roll it back.
func settleSharedQuota(ctx context.Context, tx Tx, opts chargeOpts) (quotaDecision, error) {
	if opts.ChargeKey == "" || opts.ReadingID <= 0 {
		return quotaDecision{}, biz("额度参数不正确")
	}
	if opts.ResumeAction == "" {
		opts.ResumeAction = resumeHold
	}
	if opts.UserID == 0 {
		return quotaDecision{Status: quotaAnon, Message: "登录后才会扣次数或钻石"}, nil
	}
	var kind string
	var rewardUsed, dailyUsed, settled bool
	err := tx.QueryRow(ctx, `SELECT type, reward_consumed, quota_consumed, quota_settled FROM readings WHERE id=$1 FOR UPDATE`, opts.ReadingID).
		Scan(&kind, &rewardUsed, &dailyUsed, &settled)
	if err != nil {
		return quotaDecision{}, err
	}
	var existing string
	err = tx.QueryRow(ctx, `SELECT entitlement FROM quota_charges WHERE reading_id=$1 AND charge_key=$2`, opts.ReadingID, opts.ChargeKey).Scan(&existing)
	if err == nil {
		if err := markPaywallResumed(ctx, tx, opts.ReadingID, opts.ChargeKey); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaAlready, Entitlement: existing, Continue: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return quotaDecision{}, err
	}
	// A play that already settled through an older flag must not be charged again.
	if opts.ChargeKey == chargeKeyPlay && (rewardUsed || dailyUsed || settled) {
		ent := ""
		switch {
		case rewardUsed:
			ent = entReward
		case dailyUsed:
			ent = entDaily
		}
		if err := markPaywallResumed(ctx, tx, opts.ReadingID, opts.ChargeKey); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaAlready, Entitlement: ent, Continue: true}, nil
	}
	if opts.ChargeKey != chargeKeyPlay {
		var playEnt string
		err = tx.QueryRow(ctx, `SELECT entitlement FROM quota_charges WHERE reading_id=$1 AND charge_key=$2`, opts.ReadingID, chargeKeyPlay).Scan(&playEnt)
		if err == nil && playEnt == entTrial {
			return quotaDecision{Status: quotaAlready, Entitlement: entTrial, Continue: true, Message: "试用整段含追问，本次不另扣次数或钻石"}, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return quotaDecision{}, err
		}
	}
	if opts.AllowTrial && opts.ChargeKey == chargeKeyPlay && kind == "tarotist" {
		var used bool
		if err := tx.QueryRow(ctx, `SELECT tarotist_trial_used FROM app_users WHERE id=$1 FOR UPDATE`, opts.UserID).Scan(&used); err != nil {
			return quotaDecision{}, err
		}
		if !used {
			if _, err := tx.Exec(ctx, `UPDATE app_users SET tarotist_trial_used=true WHERE id=$1`, opts.UserID); err != nil {
				return quotaDecision{}, err
			}
			if err := recordCharge(ctx, tx, opts, entTrial, 1, "", "消耗 AI 塔罗师试用"); err != nil {
				return quotaDecision{}, err
			}
			return quotaDecision{Status: quotaCharged, Entitlement: entTrial, Amount: 1, Continue: true}, nil
		}
	}
	ok, err := consumeRewardBalance(ctx, tx, opts.UserID)
	if err != nil {
		return quotaDecision{}, err
	}
	if ok {
		if err := recordCharge(ctx, tx, opts, entReward, 1, "", "消耗注册奖励次数"); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaCharged, Entitlement: entReward, Amount: 1, Continue: true}, nil
	}
	dailyOK, err := takeDailyFree(ctx, tx, opts.UserID, opts.Cycle, opts.ReadingID)
	if err != nil {
		return quotaDecision{}, err
	}
	if dailyOK {
		if err := recordCharge(ctx, tx, opts, entDaily, 1, "", "消耗今日免费次数"); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaCharged, Entitlement: entDaily, Amount: 1, Continue: true}, nil
	}
	granted, err := consumeAdminGrant(ctx, tx, opts.UserID)
	if err != nil {
		return quotaDecision{}, err
	}
	if granted {
		if err := recordCharge(ctx, tx, opts, entAdminFree, 1, "", "消耗人工补发免费次数"); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaCharged, Entitlement: entAdminFree, Amount: 1, Continue: true}, nil
	}
	cost, configured, err := loadDiamondCost(ctx, tx)
	if err != nil {
		return quotaDecision{}, err
	}
	if !configured {
		if err := upsertPaywall(ctx, tx, opts, paywallUncfg); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{
			Status:  quotaUnconfig,
			Message: "今日免费次数已用完，钻石消耗未配置，本次未扣钻石，记录已保留",
		}, nil
	}
	bucket, _, spent, err := spendDiamonds(ctx, tx, opts.UserID, cost)
	if err != nil {
		return quotaDecision{}, err
	}
	if !spent {
		if err := upsertPaywall(ctx, tx, opts, paywallPending); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{
			Status:  quotaPaywall,
			Message: "今日免费次数已用完，钻石不足，请付费后自动继续；取消则保留记录且不继续生成",
		}, nil
	}
	ent := entPack
	if bucket == iapBucketSub {
		ent = entSub
	}
	if err := recordCharge(ctx, tx, opts, ent, cost, bucket, "消耗钻石"); err != nil {
		return quotaDecision{}, err
	}
	return quotaDecision{Status: quotaCharged, Entitlement: ent, Amount: cost, Bucket: bucket, Continue: true}, nil
}

func recordCharge(ctx context.Context, tx Tx, opts chargeOpts, ent string, amount int, bucket, reason string) error {
	tag, err := tx.Exec(ctx, `INSERT INTO quota_charges(user_id, reading_id, charge_key, entitlement, amount, bucket)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (reading_id, charge_key) DO NOTHING`,
		opts.UserID, opts.ReadingID, opts.ChargeKey, ent, amount, bucket)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("额度扣减冲突")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,$3,$4,$5,'consumed')`,
		opts.UserID, opts.ReadingID, ent, amount, reason); err != nil {
		return err
	}
	reward := ent == entReward
	daily := ent == entDaily
	if _, err := tx.Exec(ctx, `UPDATE readings SET
		quota_settled=true,
		reward_consumed = reward_consumed OR $2,
		quota_consumed = quota_consumed OR $3,
		quota_cycle = CASE WHEN $3 THEN $4 ELSE quota_cycle END,
		updated_at=now()
		WHERE id=$1`, opts.ReadingID, reward, daily, opts.Cycle); err != nil {
		return err
	}
	return markPaywallResumed(ctx, tx, opts.ReadingID, opts.ChargeKey)
}

func takeDailyFree(ctx context.Context, tx Tx, uid int64, cycle string, readingID int64) (bool, error) {
	handled, consumed, err := consumeMergedDaily(ctx, tx, uid, cycle)
	if err != nil {
		return false, err
	}
	if handled {
		if !consumed {
			return false, nil
		}
		_, _ = tx.Exec(ctx, `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, readingID)
		return true, nil
	}
	tag, err := tx.Exec(ctx, `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, uid, cycle, readingID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func consumeAdminGrant(ctx context.Context, ex execer, uid int64) (bool, error) {
	tag, err := ex.Exec(ctx, `UPDATE rewards SET admin_grant_remaining = admin_grant_remaining - 1 WHERE user_id=$1 AND admin_grant_remaining > 0`, uid)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func loadDiamondCost(ctx context.Context, ex execer) (int, bool, error) {
	var cost sql.NullInt64
	err := ex.QueryRow(ctx, `SELECT diamond_cost FROM quota_settings WHERE id=1`).Scan(&cost)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if !cost.Valid || cost.Int64 <= 0 {
		return 0, false, nil
	}
	return int(cost.Int64), true, nil
}

// spendDiamonds debits pack first, then subscription. It never splits one charge
// across the two buckets and never invents a cost.
func spendDiamonds(ctx context.Context, tx Tx, uid int64, cost int) (bucket string, after int, ok bool, err error) {
	if cost <= 0 {
		return "", 0, false, nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO diamond_balances(user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return "", 0, false, err
	}
	var pack, sub int
	if err = tx.QueryRow(ctx, `SELECT pack_diamonds, subscription_diamonds FROM diamond_balances WHERE user_id=$1 FOR UPDATE`, uid).Scan(&pack, &sub); err != nil {
		return "", 0, false, err
	}
	if pack >= cost {
		after = pack - cost
		if _, err = tx.Exec(ctx, `UPDATE diamond_balances SET pack_diamonds=$1, updated_at=now() WHERE user_id=$2`, after, uid); err != nil {
			return "", 0, false, err
		}
		if err = writeDiamondSpend(ctx, tx, uid, iapBucketPack, -cost, after); err != nil {
			return "", 0, false, err
		}
		return iapBucketPack, after, true, nil
	}
	if sub >= cost {
		after = sub - cost
		if _, err = tx.Exec(ctx, `UPDATE diamond_balances SET subscription_diamonds=$1, updated_at=now() WHERE user_id=$2`, after, uid); err != nil {
			return "", 0, false, err
		}
		if err = writeDiamondSpend(ctx, tx, uid, iapBucketSub, -cost, after); err != nil {
			return "", 0, false, err
		}
		return iapBucketSub, after, true, nil
	}
	return "", 0, false, nil
}

func writeDiamondSpend(ctx context.Context, tx Tx, uid int64, bucket string, delta, after int) error {
	_, err := tx.Exec(ctx, `INSERT INTO diamond_ledger(user_id, bucket, delta, balance_after, store_transaction_id, reason)
		VALUES ($1,$2,$3,$4,'','quota_spend')`, uid, bucket, delta, after)
	return err
}

func upsertPaywall(ctx context.Context, tx Tx, opts chargeOpts, status string) error {
	_, err := tx.Exec(ctx, `INSERT INTO quota_paywall(reading_id, charge_key, user_id, status, resume_action)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (reading_id, charge_key) DO UPDATE SET
			status=EXCLUDED.status,
			resume_action=EXCLUDED.resume_action,
			user_id=EXCLUDED.user_id,
			updated_at=now()`,
		opts.ReadingID, opts.ChargeKey, opts.UserID, status, opts.ResumeAction)
	return err
}

func markPaywallResumed(ctx context.Context, tx Tx, readingID int64, key string) error {
	_, err := tx.Exec(ctx, `UPDATE quota_paywall SET status=$3, updated_at=now()
		WHERE reading_id=$1 AND charge_key=$2 AND status IN ($4,$5)`,
		readingID, key, paywallResumed, paywallPending, paywallUncfg)
	return err
}

func (a *App) chargeOwned(r *http.Request, readingID int64, key, action string) (quotaDecision, error) {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return quotaDecision{}, err
	}
	if uid == 0 {
		return quotaDecision{Status: quotaAnon, Message: "登录后才会扣次数或钻石"}, nil
	}
	if err := a.ownReading(r.Context(), readingID, uid, anon); err != nil {
		return quotaDecision{}, err
	}
	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return quotaDecision{}, err
	}
	defer tx.Rollback(r.Context())
	decision, err := settleSharedQuota(r.Context(), tx, chargeOpts{
		UserID: uid, ReadingID: readingID, ChargeKey: key, Cycle: a.userCycle(r, uid),
		AllowTrial: true, ResumeAction: action,
	})
	if err != nil {
		return quotaDecision{}, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return quotaDecision{}, err
	}
	return decision, nil
}

// gatePlay charges the play key before the ritual continues.
// Anonymous callers are not pre-charged. A blocked decision is already written.
func (a *App) gatePlay(w http.ResponseWriter, r *http.Request, action string) (bool, error) {
	uid, _, err := a.appActor(r)
	if err != nil {
		return false, err
	}
	if uid == 0 {
		return false, nil
	}
	id, err := pathID(r)
	if err != nil {
		return false, err
	}
	decision, err := a.chargeOwned(r, id, chargeKeyPlay, action)
	if err != nil {
		return false, err
	}
	if decision.Continue {
		return false, nil
	}
	writeQuotaBlock(w, decision, id)
	return true, nil
}

func writeQuotaBlock(w http.ResponseWriter, d quotaDecision, readingID int64) {
	writeJSON(w, http.StatusOK, map[string]any{
		"code":    1,
		"message": d.Message,
		"data": map[string]any{
			"reading_id":                readingID,
			"quota_outcome":             d.Status,
			"entitlement":               d.Entitlement,
			"paywall":                   d.paywall(),
			"diamond_cost_unconfigured": d.unconfigured(),
			"kept":                      true,
			"generated":                 false,
		},
	})
}

func playAlready(ctx context.Context, ex execer, readingID int64) (bool, string, error) {
	var ent string
	err := ex.QueryRow(ctx, `SELECT entitlement FROM quota_charges WHERE reading_id=$1 AND charge_key=$2`, readingID, chargeKeyPlay).Scan(&ent)
	if err == nil {
		return true, ent, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, "", err
	}
	var reward, daily, settled bool
	if err := ex.QueryRow(ctx, `SELECT reward_consumed, quota_consumed, quota_settled FROM readings WHERE id=$1`, readingID).Scan(&reward, &daily, &settled); err != nil {
		return false, "", err
	}
	if reward {
		return true, entReward, nil
	}
	if daily {
		return true, entDaily, nil
	}
	if settled {
		return true, "", nil
	}
	return false, "", nil
}

func (a *App) resumeQuotaAfterCredit(ctx context.Context, uid int64) []map[string]any {
	rows, err := a.Store.Query(ctx, `SELECT reading_id, charge_key, resume_action, resume_payload
		FROM quota_paywall WHERE user_id=$1 AND status IN ($2,$3) ORDER BY reading_id, charge_key`, uid, paywallPending, paywallUncfg)
	if err != nil {
		log.Printf("quota resume list: %v", err)
		return nil
	}
	defer rows.Close()
	type hold struct {
		readingID int64
		key       string
		action    string
		payload   string
	}
	var holds []hold
	for rows.Next() {
		var item hold
		if err := rows.Scan(&item.readingID, &item.key, &item.action, &item.payload); err != nil {
			log.Printf("quota resume scan: %v", err)
			return nil
		}
		holds = append(holds, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("quota resume rows: %v", err)
		return nil
	}
	out := make([]map[string]any, 0, len(holds))
	for _, item := range holds {
		view, err := a.resumeOne(ctx, uid, item.readingID, item.key, item.action, item.payload)
		if err != nil {
			log.Printf("quota resume reading %d: %v", item.readingID, err)
			continue
		}
		if view != nil {
			out = append(out, view)
		}
	}
	return out
}

func (a *App) resumeOne(ctx context.Context, uid, readingID int64, key, action, payload string) (map[string]any, error) {
	cycle := cycleOf(time.Now())
	var tz string
	_ = a.Store.QueryRow(ctx, `SELECT timezone FROM app_users WHERE id=$1`, uid).Scan(&tz)
	if tz != "" {
		cycle = cycleFor(time.Now(), tz)
	}
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	decision, err := settleSharedQuota(ctx, tx, chargeOpts{
		UserID: uid, ReadingID: readingID, ChargeKey: key, Cycle: cycle,
		AllowTrial: true, ResumeAction: action,
	})
	if err != nil {
		return nil, err
	}
	if !decision.Continue {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	generated := false
	switch action {
	case resumeShuffle:
		if _, err := tx.Exec(ctx, `UPDATE readings SET status='SHUFFLING', updated_at=now() WHERE id=$1 AND status='SPREAD_SELECTED'`, readingID); err != nil {
			return nil, err
		}
	case resumeDraw, resumeDaily:
		if err := drawIfEmpty(ctx, tx, readingID, action); err != nil {
			return nil, err
		}
	case resumeFollowup:
		turn, err := followupTurn(key)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(payload) != "" {
			if err := ensureUserTurn(ctx, tx, readingID, turn, payload); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	switch action {
	case resumeInterpret:
		if _, err := a.generate(ctx, readingID, false); err == nil {
			generated = true
		}
	case resumeDaily:
		if _, err := a.generate(ctx, readingID, false); err == nil {
			generated = true
		}
	case resumeFollowup:
		turn, err := followupTurn(key)
		if err == nil && strings.TrimSpace(payload) != "" {
			if _, err := a.answerFollowup(ctx, readingID, uid, turn, payload); err == nil {
				generated = true
			}
		}
	}
	var status string
	_ = a.Store.QueryRow(ctx, `SELECT status FROM readings WHERE id=$1`, readingID).Scan(&status)
	return map[string]any{
		"reading_id": readingID, "charge_key": key, "resume_action": action,
		"entitlement": decision.Entitlement, "quota_outcome": decision.Status,
		"status": status, "generated": generated,
	}, nil
}

func followupTurn(key string) (int64, error) {
	raw, ok := strings.CutPrefix(key, "followup:")
	if !ok || raw == "" {
		return 0, biz("追问额度键不正确")
	}
	turn, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || turn <= 0 {
		return 0, biz("追问额度键不正确")
	}
	return turn, nil
}

func drawIfEmpty(ctx context.Context, tx Tx, readingID int64, action string) error {
	var cards *string
	var spreadID *int64
	var status string
	if err := tx.QueryRow(ctx, `SELECT cards, spread_id, status FROM readings WHERE id=$1`, readingID).Scan(&cards, &spreadID, &status); err != nil {
		return err
	}
	if cards != nil && *cards != "" {
		return nil
	}
	var positions []position
	if action == resumeDaily {
		var question string
		_ = tx.QueryRow(ctx, `SELECT question FROM readings WHERE id=$1`, readingID).Scan(&question)
		name, meaning := "今日", "当日一牌"
		if strings.Contains(question, "明日") {
			name, meaning = "明日", "明日运势"
		}
		positions = []position{{Index: 1, Name: name, Meaning: meaning}}
	} else {
		if spreadID == nil {
			return nil
		}
		var posRaw string
		if err := tx.QueryRow(ctx, `SELECT positions FROM spreads WHERE id=$1`, *spreadID).Scan(&posRaw); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(posRaw), &positions); err != nil {
			return err
		}
	}
	drawn, err := drawCards(positions)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE readings SET cards=$1, status='CARDS_FIXED', updated_at=now() WHERE id=$2 AND (cards IS NULL OR cards='')`, mustJSON(drawn), readingID)
	return err
}

func nextBusinessDate(cycle string) (string, error) {
	t, err := time.Parse("2006-01-02", cycle)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02"), nil
}

func (a *App) appCancelPaywall(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能处理付费")
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
		return err
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	switch req.Action {
	case "cancel":
		tag, err := a.Store.Exec(r.Context(), `UPDATE quota_paywall SET status=$3, updated_at=now()
			WHERE reading_id=$1 AND user_id=$2 AND status IN ($4,$5)`, id, uid, paywallCancel, paywallPending, paywallUncfg)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return biz("没有可取消的付费等待")
		}
		writeOK(w, map[string]any{"cancelled": true, "kept": true, "generated": false, "reading_id": id})
		return nil
	case "resume":
		tasks := a.resumeQuotaAfterCredit(r.Context(), uid)
		writeOK(w, map[string]any{"resumed_tasks": tasks, "generated": resumedAny(tasks)})
		return nil
	default:
		return biz("付费动作不正确")
	}
}

func resumedAny(tasks []map[string]any) bool {
	for _, task := range tasks {
		if task["generated"] == true {
			return true
		}
	}
	return false
}
