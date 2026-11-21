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

// Billing split by current membership (checked on every charge):
//
// Free users:
//  1. today's daily free (1, configurable)
//  2. invite reward counts (keep until used; max 15)
//  3. admin-granted free counts (ops)
//  4. paywall → 开通会员
// Scope: daily 今日/明日, and ordinary readings with spread card_count <= 3.
// Explore, yearly, AI tarotist, follow-up, and >3-card spreads are not free.
//
// Members:
//  1. unified quota_balance
//  2. paywall → 额度补充包
// All play types deduct quota. Daily free is skipped.
// Pack quota is frozen when membership expires (balance kept, spend blocked).
//
// Trials (neither daily nor quota):
//  - onboard_trial: one per device, consumed when interpretation is shown after login
//  - ai_trial: one per account, play only, no follow-up
//
// quota_settings.quota_cost is the member quota cost per charge. NULL means
// do not invent an amount.

const (
	chargeKeyPlay = "play"

	entReward    = "registration_reward" // kept for old rows; no longer granted
	entDaily     = "daily_free"
	entInvite    = "invite_reward"
	entPack      = "quota_pack"         // historical
	entSub       = "quota_subscription" // historical
	entQuota     = "member_quota"
	entTrial     = "ai_trial"
	entOnboard   = "onboard_trial"
	entAdminFree = "admin_grant_free"

	quotaCharged   = "charged"
	quotaAlready   = "already"
	quotaPaywall   = "paywall"
	quotaUnconfig  = "quota_cost_unconfigured"
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

	quotaClosedNote = "已按优先级处理：今日免费次数，邀请奖励次数，人工补发免费次数；会员扣统一额度"
	wechatQuotaNote = "提问时按当前身份扣每日免费、邀请次数或会员额度"

	paywallNeedMember  = "请开通会员后继续"
	paywallNeedQuota   = "额度不足，请购买额度补充包后自动继续；取消则保留记录且不继续生成"
	paywallNeedLogin   = "登录后才能查看解读"
	paywallFreeDone    = "今日免费次数已用完，请开通会员后继续"
	paywallUnconfigMsg = "会员额度单价未配置，本次未扣额度，记录已保留"
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
		return quotaDecision{Status: quotaAnon, Message: paywallNeedLogin}, nil
	}
	var kind string
	var rewardUsed, dailyUsed, settled, onboard bool
	err := tx.QueryRow(ctx, `SELECT type, reward_consumed, quota_consumed, quota_settled, onboard_experience FROM readings WHERE id=$1 FOR UPDATE`, opts.ReadingID).
		Scan(&kind, &rewardUsed, &dailyUsed, &settled, &onboard)
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
	if opts.ChargeKey == chargeKeyPlay && onboard {
		ok, err := consumeOnboardExperience(ctx, tx, opts.UserID, opts.ReadingID)
		if err != nil {
			return quotaDecision{}, err
		}
		if ok {
			if err := recordCharge(ctx, tx, opts, entOnboard, 1, "", "消耗新人引导体验"); err != nil {
				return quotaDecision{}, err
			}
			return quotaDecision{Status: quotaCharged, Entitlement: entOnboard, Amount: 1, Continue: true}, nil
		}
	}

	member, err := hasActiveMembership(ctx, tx, opts.UserID)
	if err != nil {
		return quotaDecision{}, err
	}
	if member {
		return chargeMemberQuota(ctx, tx, opts)
	}

	cardCount, err := readingSpreadCardCount(ctx, tx, opts.ReadingID)
	if err != nil {
		return quotaDecision{}, err
	}
	if !freePlayEligible(kind, cardCount, opts.ChargeKey) {
		if err := upsertPaywall(ctx, tx, opts, paywallPending); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaPaywall, Message: paywallNeedMember}, nil
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
	invited, err := consumeInviteRemaining(ctx, tx, opts.UserID)
	if err != nil {
		return quotaDecision{}, err
	}
	if invited {
		if err := recordCharge(ctx, tx, opts, entInvite, 1, "", "消耗邀请奖励次数"); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaCharged, Entitlement: entInvite, Amount: 1, Continue: true}, nil
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
	if err := upsertPaywall(ctx, tx, opts, paywallPending); err != nil {
		return quotaDecision{}, err
	}
	return quotaDecision{Status: quotaPaywall, Message: paywallFreeDone}, nil
}

func chargeMemberQuota(ctx context.Context, tx Tx, opts chargeOpts) (quotaDecision, error) {
	cost, configured, err := loadQuotaCost(ctx, tx)
	if err != nil {
		return quotaDecision{}, err
	}
	if !configured {
		if err := upsertPaywall(ctx, tx, opts, paywallUncfg); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaUnconfig, Message: paywallUnconfigMsg}, nil
	}
	_, spent, err := spendQuota(ctx, tx, opts.UserID, cost)
	if err != nil {
		return quotaDecision{}, err
	}
	if !spent {
		if err := upsertPaywall(ctx, tx, opts, paywallPending); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaPaywall, Message: paywallNeedQuota}, nil
	}
	if err := recordCharge(ctx, tx, opts, entQuota, cost, iapBucketQuota, "消耗会员额度"); err != nil {
		return quotaDecision{}, err
	}
	return quotaDecision{Status: quotaCharged, Entitlement: entQuota, Amount: cost, Bucket: iapBucketQuota, Continue: true}, nil
}

func freePlayEligible(kind string, cardCount int, chargeKey string) bool {
	if chargeKey != chargeKeyPlay {
		return false
	}
	switch kind {
	case "daily":
		return true
	case "normal":
		return cardCount > 0 && cardCount <= 3
	default:
		return false
	}
}

func readingSpreadCardCount(ctx context.Context, ex execer, readingID int64) (int, error) {
	var n sql.NullInt64
	err := ex.QueryRow(ctx, `SELECT s.card_count FROM readings r LEFT JOIN spreads s ON s.id = r.spread_id WHERE r.id=$1`, readingID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return int(n.Int64), nil
}

func hasActiveMembership(ctx context.Context, q rowQuery, uid int64) (bool, error) {
	_, _, _, ok, err := loadActiveMembership(ctx, q, uid)
	return ok, err
}

func loadActiveMembership(ctx context.Context, q rowQuery, uid int64) (kind string, exp time.Time, auto sql.NullBool, ok bool, err error) {
	err = q.QueryRow(ctx, `SELECT sku_kind, expires_at, auto_renew FROM subscriptions
		WHERE user_id=$1 AND status='active' AND expires_at > now() AND merge_state NOT IN ('superseded','expired')
		ORDER BY CASE sku_kind WHEN 'year' THEN 0 ELSE 1 END, expires_at DESC LIMIT 1`, uid).Scan(&kind, &exp, &auto)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, sql.NullBool{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, sql.NullBool{}, false, err
	}
	return kind, exp, auto, true, nil
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
		_, _ = tx.Exec(ctx, `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT (reading_id) DO NOTHING`, uid, cycle, readingID)
		return true, nil
	}
	limit, err := loadDailyFreeCount(ctx, tx)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, quotaCycleLock(uid, cycle)); err != nil {
		return false, err
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM quota_use WHERE user_id=$1 AND cycle=$2`, uid, cycle).Scan(&n); err != nil {
		return false, err
	}
	if n >= limit {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `INSERT INTO quota_use(user_id, cycle, reading_id) VALUES ($1,$2,$3) ON CONFLICT (reading_id) DO NOTHING`, uid, cycle, readingID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func quotaCycleLock(uid int64, cycle string) int64 {
	h := int64(0)
	for _, r := range cycle {
		h = h*33 + int64(r)
	}
	return uid*1_000_003 + h
}

func loadDailyFreeCount(ctx context.Context, ex execer) (int, error) {
	return loadPositiveSetting(ctx, ex, `SELECT daily_free_count FROM quota_settings WHERE id=1`, 1)
}

func loadDailySlotCount(ctx context.Context, ex execer) (int, error) {
	return loadPositiveSetting(ctx, ex, `SELECT daily_slot_count FROM quota_settings WHERE id=1`, 1)
}

func loadPositiveSetting(ctx context.Context, ex execer, query string, fallback int) (int, error) {
	var n sql.NullInt64
	err := ex.QueryRow(ctx, query).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return 0, err
	}
	if !n.Valid || n.Int64 <= 0 {
		return fallback, nil
	}
	return int(n.Int64), nil
}

func consumeAdminGrant(ctx context.Context, ex execer, uid int64) (bool, error) {
	tag, err := ex.Exec(ctx, `UPDATE rewards SET admin_grant_remaining = admin_grant_remaining - 1 WHERE user_id=$1 AND admin_grant_remaining > 0`, uid)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func consumeInviteRemaining(ctx context.Context, ex execer, uid int64) (bool, error) {
	tag, err := ex.Exec(ctx, `UPDATE rewards SET invite_remaining = invite_remaining - 1 WHERE user_id=$1 AND invite_remaining > 0`, uid)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func creditInviteRemaining(ctx context.Context, tx Tx, uid int64, amount int) error {
	if amount <= 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO rewards(user_id, granted_at, expires_at, remaining, admin_grant_remaining, invite_remaining)
		VALUES ($1, now(), now(), 0, 0, 0)
		ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE rewards SET invite_remaining = invite_remaining + $2 WHERE user_id=$1`, uid, amount)
	return err
}

func consumeOnboardExperience(ctx context.Context, tx Tx, uid, readingID int64) (bool, error) {
	var deviceID string
	err := tx.QueryRow(ctx, `SELECT device_id FROM device_experiences WHERE reading_id=$1 FOR UPDATE`, readingID).Scan(&deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE device_experiences SET consumed=TRUE, user_id=$1, consumed_at=now()
		WHERE device_id=$2 AND consumed=FALSE`, uid, deviceID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func spendQuota(ctx context.Context, tx Tx, uid int64, cost int) (after int, ok bool, err error) {
	if cost <= 0 {
		return 0, false, nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO quota_balances(user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return 0, false, err
	}
	var bal int
	if err = tx.QueryRow(ctx, `SELECT quota_balance FROM quota_balances WHERE user_id=$1 FOR UPDATE`, uid).Scan(&bal); err != nil {
		return 0, false, err
	}
	if bal < cost {
		return 0, false, nil
	}
	after = bal - cost
	if _, err = tx.Exec(ctx, `UPDATE quota_balances SET quota_balance=$1, updated_at=now() WHERE user_id=$2`, after, uid); err != nil {
		return 0, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO quota_ledger(user_id, bucket, delta, balance_after, store_transaction_id, reason)
		VALUES ($1,$2,$3,$4,'','quota_spend')`, uid, iapBucketQuota, -cost, after); err != nil {
		return 0, false, err
	}
	return after, true, nil
}

func creditQuota(ctx context.Context, tx Tx, uid int64, amount int, orderID *int64, txn, reason string) (int, error) {
	if amount <= 0 {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO quota_balances(user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return 0, err
	}
	var bal int
	if err := tx.QueryRow(ctx, `SELECT quota_balance FROM quota_balances WHERE user_id=$1 FOR UPDATE`, uid).Scan(&bal); err != nil {
		return 0, err
	}
	after := bal + amount
	if _, err := tx.Exec(ctx, `UPDATE quota_balances SET quota_balance=$1, updated_at=now() WHERE user_id=$2`, after, uid); err != nil {
		return 0, err
	}
	var oid any
	if orderID != nil {
		oid = *orderID
	}
	_, err := tx.Exec(ctx, `INSERT INTO quota_ledger(user_id, bucket, delta, balance_after, order_id, store_transaction_id, reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, uid, iapBucketQuota, amount, after, oid, txn, reason)
	return after, err
}

func loadQuotaBalance(ctx context.Context, q rowQuery, uid int64) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT quota_balance FROM quota_balances WHERE user_id=$1`, uid).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

func loadQuotaCost(ctx context.Context, ex execer) (int, bool, error) {
	var cost sql.NullInt64
	err := ex.QueryRow(ctx, `SELECT quota_cost FROM quota_settings WHERE id=1`).Scan(&cost)
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

type registrationSettings struct {
	DelayMinutes sql.NullInt64
	ValidDays    int
	GrantCount   int
	SpreadIDs    []int64
}

func loadRegistrationSettings(ctx context.Context, ex execer) (registrationSettings, error) {
	var cfg registrationSettings
	var raw string
	err := ex.QueryRow(ctx, `SELECT registration_grant_delay_minutes, registration_valid_days, registration_grant_count, registration_spread_ids FROM quota_settings WHERE id=1`).
		Scan(&cfg.DelayMinutes, &cfg.ValidDays, &cfg.GrantCount, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return registrationSettings{ValidDays: 3, GrantCount: 1, SpreadIDs: []int64{}}, nil
	}
	if err != nil {
		return registrationSettings{}, err
	}
	if cfg.ValidDays <= 0 {
		cfg.ValidDays = 3
	}
	if cfg.GrantCount <= 0 {
		cfg.GrantCount = 1
	}
	cfg.SpreadIDs = []int64{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.SpreadIDs); err != nil {
			return registrationSettings{}, err
		}
	}
	if cfg.SpreadIDs == nil {
		cfg.SpreadIDs = []int64{}
	}
	return cfg, nil
}

func (a *App) finishRegistration(ctx context.Context, uid int64, created bool) error {
	if created {
		return a.Store.noteRegistration(ctx, uid)
	}
	return a.Store.ensureRegistrationGift(ctx, uid)
}

func (s *Store) noteRegistration(ctx context.Context, uid int64) error {
	if uid <= 0 {
		return nil
	}
	if _, err := s.Exec(ctx, `INSERT INTO registration_gifts(user_id, registered_at) VALUES ($1, now()) ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return err
	}
	return s.ensureRegistrationGift(ctx, uid)
}

func (s *Store) ensureRegistrationGift(ctx context.Context, uid int64) error {
	if uid <= 0 {
		return nil
	}
	var registered time.Time
	var granted sql.NullTime
	err := s.QueryRow(ctx, `SELECT registered_at, granted_at FROM registration_gifts WHERE user_id=$1`, uid).Scan(&registered, &granted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if granted.Valid {
		return nil
	}
	cfg, err := loadRegistrationSettings(ctx, s)
	if err != nil {
		return err
	}
	if !cfg.DelayMinutes.Valid {
		return nil
	}
	if time.Now().Before(registered.Add(time.Duration(cfg.DelayMinutes.Int64) * time.Minute)) {
		return nil
	}
	now := time.Now()
	_, err = s.Exec(ctx, `UPDATE registration_gifts
		SET granted_at=$2, usable_at=$2, expires_at=$3, total=$4, remaining=$4
		WHERE user_id=$1 AND granted_at IS NULL`,
		uid, now, now.Add(time.Duration(cfg.ValidDays)*24*time.Hour), cfg.GrantCount)
	return err
}

func loadRegistrationGift(ctx context.Context, ex execer, uid int64) (map[string]any, error) {
	empty := map[string]any{
		"registered_at": nil, "granted_at": nil, "usable_at": nil, "expires_at": nil,
		"total": 0, "remaining": 0, "used": false, "used_at": nil,
	}
	if uid <= 0 {
		return empty, nil
	}
	var registered time.Time
	var granted, usable, expires, usedAt sql.NullTime
	var total, remaining int
	err := ex.QueryRow(ctx, `SELECT registered_at, granted_at, usable_at, expires_at, total, remaining, used_at FROM registration_gifts WHERE user_id=$1`, uid).
		Scan(&registered, &granted, &usable, &expires, &total, &remaining, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"registered_at": registered,
		"granted_at":    nullTime(granted),
		"usable_at":     nullTime(usable),
		"expires_at":    nullTime(expires),
		"total":         total,
		"remaining":     remaining,
		"used":          usedAt.Valid,
		"used_at":       nullTime(usedAt),
	}, nil
}

func validateRegistrationSpreads(ctx context.Context, ex execer, ids []int64) error {
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id <= 0 {
			return biz("注册赠送白名单牌阵不正确")
		}
		if _, ok := seen[id]; ok {
			return biz("注册赠送白名单牌阵不正确")
		}
		seen[id] = struct{}{}
		var n int
		if err := ex.QueryRow(ctx, `SELECT count(*) FROM spreads WHERE id=$1 AND theme_key='' AND card_count BETWEEN 1 AND 3`, id).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return biz("注册赠送白名单只能选 3 张及以内的普通占卜牌阵")
		}
	}
	return nil
}

func loadFollowupLimit(ctx context.Context, ex execer) (int, bool, error) {
	var limit sql.NullInt64
	err := ex.QueryRow(ctx, `SELECT followup_limit FROM quota_settings WHERE id=1`).Scan(&limit)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if !limit.Valid || limit.Int64 <= 0 {
		return 0, false, nil
	}
	return int(limit.Int64), true, nil
}

// enforceFollowupLimit caps new follow-up turns on a reading.
// reading_messages only stores follow-ups; the original question is readings.question.
func enforceFollowupLimit(ctx context.Context, ex execer, readingID, turn int64) error {
	limit, ok, err := loadFollowupLimit(ctx, ex)
	if err != nil || !ok {
		return err
	}
	var have int
	if err := ex.QueryRow(ctx, `SELECT count(*) FROM reading_messages WHERE reading_id=$1 AND turn_id=$2 AND role='user'`, readingID, turn).Scan(&have); err != nil {
		return err
	}
	if have > 0 {
		return nil
	}
	var used int
	if err := ex.QueryRow(ctx, `SELECT count(DISTINCT turn_id) FROM reading_messages WHERE reading_id=$1 AND role='user'`, readingID).Scan(&used); err != nil {
		return err
	}
	if used >= limit {
		return biz("本条解读追问已达上限")
	}
	return nil
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
		return quotaDecision{Status: quotaAnon, Message: paywallNeedLogin}, nil
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
	writeQuotaBlock(w, r, decision, id)
	return true, nil
}

func writeQuotaBlock(w http.ResponseWriter, r *http.Request, d quotaDecision, readingID int64) {
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":    1,
		"message": T(ctx, d.Message),
		"data": map[string]any{
			"reading_id":              readingID,
			"quota_outcome":           d.Status,
			"entitlement":             d.Entitlement,
			"paywall":                 d.paywall(),
			"quota_cost_unconfigured": d.unconfigured(),
			"kept":                    true,
			"generated":               false,
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
	cycle := cycleFor(time.Now(), userTimezone(ctx, a.Store, uid))
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
