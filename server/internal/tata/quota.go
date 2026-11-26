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
// Members:
//  1. quota lots, when the live pool covers the whole charge
//  2. pool above 0 but short: paywall for a pack, no other source
//  3. pool at 0: registration gift, then invite, on the shared whitelist
//  4. paywall → 额度补充包
// Lots may combine. A lot past expires_at is discarded.
//
// Free users have no pool:
//  1. registration gift, then invite, on the shared whitelist
//  2. redeem-code free counts on the old daily / ≤3-card scope
//  3. paywall → 开通会员
// Daily free is not spent. Explore, yearly, and tarotist are outside the gift and invite.
// Pending registration gifts are granted inside playCover/settleSharedQuota;
// App GET /quota is not required before charging.
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

	quotaClosedNote = "已按优先级处理：会员先扣额度池；池子为 0 时用注册赠送、邀请，免费用户还可使用兑换免费次数"
	wechatQuotaNote = "提问时按当前身份扣会员额度、注册赠送或邀请次数"

	paywallNeedMember  = "请开通会员后继续"
	paywallNeedQuota   = "额度不足，请购买额度补充包后自动继续；取消则保留记录且不继续生成"
	paywallNeedLogin   = "登录后才能查看解读"
	paywallUnconfigMsg = "会员额度单价未配置，本次未扣额度，记录已保留"
)

type chargeOpts struct {
	UserID       int64
	ReadingID    int64
	ChargeKey    string
	Cycle        string
	AllowTrial   bool
	ResumeAction string
	PoolOnly     bool
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
	// Interpret / spread settle must grant pending registration gifts itself.
	// App GET /quota is display-only and must not be required before charging.
	if err := ensureRegistrationGift(ctx, tx, opts.UserID); err != nil {
		return quotaDecision{}, err
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
	if strings.HasPrefix(opts.ChargeKey, "followup:") {
		return quotaDecision{Continue: true}, nil
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
			if err := recordCharge(ctx, tx, opts, entTrial, 1, "", "消耗 AI 塔罗师试用", nil); err != nil {
				return quotaDecision{}, err
			}
			return quotaDecision{Status: quotaCharged, Entitlement: entTrial, Amount: 1, Continue: true}, nil
		}
	}
	member, err := hasActiveMembership(ctx, tx, opts.UserID)
	if err != nil {
		return quotaDecision{}, err
	}
	if member {
		return chargeMemberQuota(ctx, tx, opts)
	}
	return chargeComplimentary(ctx, tx, opts, paywallNeedMember)
}

func chargeComplimentary(ctx context.Context, tx Tx, opts chargeOpts, blocked string) (quotaDecision, error) {
	scope, err := giftInviteScope(ctx, tx, opts.ReadingID, opts.ChargeKey)
	if err != nil {
		return quotaDecision{}, err
	}
	if scope {
		gifted, draw, err := consumeRegistrationGift(ctx, tx, opts.UserID)
		if err != nil {
			return quotaDecision{}, err
		}
		if gifted {
			if err := recordCharge(ctx, tx, opts, entReward, 1, "", "消耗注册赠送", []lotDraw{draw}); err != nil {
				return quotaDecision{}, err
			}
			return quotaDecision{Status: quotaCharged, Entitlement: entReward, Amount: 1, Continue: true}, nil
		}
		invited, draw, err := consumeInviteRemaining(ctx, tx, opts.UserID)
		if err != nil {
			return quotaDecision{}, err
		}
		if invited {
			if err := recordCharge(ctx, tx, opts, entInvite, 1, "", "消耗邀请奖励次数", []lotDraw{draw}); err != nil {
				return quotaDecision{}, err
			}
			return quotaDecision{Status: quotaCharged, Entitlement: entInvite, Amount: 1, Continue: true}, nil
		}
	}
	cardCount, err := readingSpreadCardCount(ctx, tx, opts.ReadingID)
	if err != nil {
		return quotaDecision{}, err
	}
	var kind string
	if err := tx.QueryRow(ctx, `SELECT type FROM readings WHERE id=$1`, opts.ReadingID).Scan(&kind); err != nil {
		return quotaDecision{}, err
	}
	if freePlayEligible(kind, cardCount, opts.ChargeKey) {
		granted, draw, err := consumeAdminGrant(ctx, tx, opts.UserID)
		if err != nil {
			return quotaDecision{}, err
		}
		if granted {
			if err := recordCharge(ctx, tx, opts, entAdminFree, 1, "", "消耗兑换免费次数", []lotDraw{draw}); err != nil {
				return quotaDecision{}, err
			}
			return quotaDecision{Status: quotaCharged, Entitlement: entAdminFree, Amount: 1, Continue: true}, nil
		}
	}
	if err := upsertPaywall(ctx, tx, opts, paywallPending); err != nil {
		return quotaDecision{}, err
	}
	return quotaDecision{Status: quotaPaywall, Message: blocked}, nil
}

func chargeMemberQuota(ctx context.Context, tx Tx, opts chargeOpts) (quotaDecision, error) {
	cost, configured, err := loadPlayCost(ctx, tx, opts.ReadingID)
	if err != nil {
		return quotaDecision{}, err
	}
	if !configured {
		if err := upsertPaywall(ctx, tx, opts, paywallUncfg); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaUnconfig, Message: paywallUnconfigMsg}, nil
	}
	if err := expireQuotaLots(ctx, tx, opts.UserID); err != nil {
		return quotaDecision{}, err
	}
	bal, err := loadQuotaBalance(ctx, tx, opts.UserID)
	if err != nil {
		return quotaDecision{}, err
	}
	if bal > 0 && bal < cost {
		if err := upsertPaywall(ctx, tx, opts, paywallPending); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaPaywall, Message: paywallNeedQuota}, nil
	}
	if bal == 0 {
		if opts.PoolOnly {
			if err := upsertPaywall(ctx, tx, opts, paywallPending); err != nil {
				return quotaDecision{}, err
			}
			return quotaDecision{Status: quotaPaywall, Message: paywallNeedQuota}, nil
		}
		return chargeComplimentary(ctx, tx, opts, paywallNeedQuota)
	}
	_, spent, draws, err := spendQuota(ctx, tx, opts.UserID, cost)
	if err != nil {
		return quotaDecision{}, err
	}
	if !spent {
		if err := upsertPaywall(ctx, tx, opts, paywallPending); err != nil {
			return quotaDecision{}, err
		}
		return quotaDecision{Status: quotaPaywall, Message: paywallNeedQuota}, nil
	}
	if err := recordCharge(ctx, tx, opts, entQuota, cost, iapBucketQuota, "消耗会员额度", draws); err != nil {
		return quotaDecision{}, err
	}
	return quotaDecision{Status: quotaCharged, Entitlement: entQuota, Amount: cost, Bucket: iapBucketQuota, Continue: true}, nil
}

func giftInviteScope(ctx context.Context, ex execer, readingID int64, chargeKey string) (bool, error) {
	if chargeKey != chargeKeyPlay || readingID <= 0 {
		return false, nil
	}
	var kind string
	var spreadID sql.NullInt64
	var theme, drawMode string
	if err := ex.QueryRow(ctx, `SELECT r.type, r.spread_id, COALESCE(s.theme_key,''), r.draw_mode
		FROM readings r LEFT JOIN spreads s ON s.id = r.spread_id WHERE r.id=$1`, readingID).Scan(&kind, &spreadID, &theme, &drawMode); err != nil {
		return false, err
	}
	switch kind {
	case "daily":
		return theme == "", nil
	case "normal":
		if theme != "" || !spreadID.Valid {
			return false, nil
		}
		if drawMode == "numerology" {
			return true, nil
		}
		cfg, err := loadRegistrationSettings(ctx, ex)
		if err != nil {
			return false, err
		}
		for _, id := range cfg.SpreadIDs {
			if id == spreadID.Int64 {
				return true, nil
			}
		}
		return false, nil
	default:
		return false, nil
	}
}

func consumeRegistrationGift(ctx context.Context, tx Tx, uid int64) (bool, lotDraw, error) {
	var remaining int
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT remaining, expires_at FROM registration_gifts
		WHERE user_id=$1 AND granted_at IS NOT NULL AND usable_at <= now() AND expires_at > now() AND remaining > 0
		FOR UPDATE`, uid).Scan(&remaining, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return false, lotDraw{}, nil
	}
	if err != nil {
		return false, lotDraw{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE registration_gifts
		SET remaining = remaining - 1, used_at = COALESCE(used_at, now())
		WHERE user_id=$1`, uid); err != nil {
		return false, lotDraw{}, err
	}
	return true, lotDraw{
		LotID: uid, Source: "registration_gift", Amount: 1, Remaining: remaining - 1,
		Expires: sql.NullTime{Time: expires, Valid: true},
	}, nil
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

func recordCharge(ctx context.Context, tx Tx, opts chargeOpts, ent string, amount int, bucket, reason string, draws []lotDraw) error {
	tag, err := tx.Exec(ctx, `INSERT INTO quota_charges(user_id, reading_id, charge_key, entitlement, amount, bucket)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (reading_id, charge_key) DO NOTHING`,
		opts.UserID, opts.ReadingID, opts.ChargeKey, ent, amount, bucket)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return biz("额度扣减冲突")
	}
	var ledgerID int64
	if err := tx.QueryRow(ctx, `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result) VALUES ($1,$2,$3,$4,$5,'consumed') RETURNING id`,
		opts.UserID, opts.ReadingID, ent, amount, reason).Scan(&ledgerID); err != nil {
		return err
	}
	for _, draw := range draws {
		var lot any
		if draw.LotID > 0 {
			lot = draw.LotID
		}
		var exp any
		if draw.Expires.Valid {
			exp = draw.Expires.Time
		}
		if _, err := tx.Exec(ctx, `INSERT INTO ledger_lot_draws(ledger_id, lot_id, source, amount, remaining_after, expires_at)
			VALUES ($1,$2,$3,$4,$5,$6)`, ledgerID, lot, draw.Source, draw.Amount, draw.Remaining, exp); err != nil {
			return err
		}
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

func consumeAdminGrant(ctx context.Context, ex execer, uid int64) (bool, lotDraw, error) {
	var remaining int
	err := ex.QueryRow(ctx, `UPDATE rewards SET admin_grant_remaining = admin_grant_remaining - 1
		WHERE user_id=$1 AND admin_grant_remaining > 0 RETURNING admin_grant_remaining`, uid).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return false, lotDraw{}, nil
	}
	if err != nil {
		return false, lotDraw{}, err
	}
	return true, lotDraw{Source: "redeem", Amount: 1, Remaining: remaining}, nil
}

func consumeInviteRemaining(ctx context.Context, ex execer, uid int64) (bool, lotDraw, error) {
	var grants int
	if err := ex.QueryRow(ctx, `SELECT count(*) FROM invite_grants WHERE user_id=$1`, uid).Scan(&grants); err != nil {
		return false, lotDraw{}, err
	}
	if grants > 0 {
		var id int64
		var remaining int
		var expires time.Time
		err := ex.QueryRow(ctx, `SELECT id, remaining, expires_at FROM invite_grants
			WHERE user_id=$1 AND remaining>0 AND expires_at>now()
			ORDER BY expires_at, id LIMIT 1 FOR UPDATE`, uid).Scan(&id, &remaining, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return false, lotDraw{}, nil
		}
		if err != nil {
			return false, lotDraw{}, err
		}
		tag, err := ex.Exec(ctx, `UPDATE invite_grants SET remaining=remaining-1 WHERE id=$1 AND remaining>0`, id)
		if err != nil {
			return false, lotDraw{}, err
		}
		if tag.RowsAffected() == 0 {
			return false, lotDraw{}, nil
		}
		if _, err := ex.Exec(ctx, `UPDATE rewards SET invite_remaining=GREATEST(invite_remaining-1, 0) WHERE user_id=$1`, uid); err != nil {
			return false, lotDraw{}, err
		}
		return true, lotDraw{
			LotID: id, Source: "invite", Amount: 1, Remaining: remaining - 1,
			Expires: sql.NullTime{Time: expires, Valid: true},
		}, nil
	}
	var remaining int
	err := ex.QueryRow(ctx, `UPDATE rewards SET invite_remaining = invite_remaining - 1
		WHERE user_id=$1 AND invite_remaining > 0 RETURNING invite_remaining`, uid).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return false, lotDraw{}, nil
	}
	if err != nil {
		return false, lotDraw{}, err
	}
	return true, lotDraw{Source: "invite", Amount: 1, Remaining: remaining}, nil
}

func inviteBalance(ctx context.Context, ex execer, uid int64) (int, error) {
	var grants int
	if err := ex.QueryRow(ctx, `SELECT count(*) FROM invite_grants WHERE user_id=$1`, uid).Scan(&grants); err != nil {
		return 0, err
	}
	if grants > 0 {
		var n int
		err := ex.QueryRow(ctx, `SELECT COALESCE(sum(remaining),0) FROM invite_grants WHERE user_id=$1 AND remaining>0 AND expires_at>now()`, uid).Scan(&n)
		return n, err
	}
	var n int
	err := ex.QueryRow(ctx, `SELECT invite_remaining FROM rewards WHERE user_id=$1`, uid).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
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
	var used bool
	if err := tx.QueryRow(ctx, `SELECT reading_trial_used FROM app_users WHERE id=$1 FOR UPDATE`, uid).Scan(&used); err != nil {
		return false, err
	}
	var deviceID string
	err := tx.QueryRow(ctx, `SELECT device_id FROM device_experiences WHERE reading_id=$1 FOR UPDATE`, readingID).Scan(&deviceID)
	if err == nil {
		if _, err := tx.Exec(ctx, `UPDATE device_experiences SET consumed=TRUE, user_id=$1, consumed_at=COALESCE(consumed_at, now())
			WHERE device_id=$2 AND consumed=FALSE`, uid, deviceID); err != nil {
			return false, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if used {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE app_users SET reading_trial_used=TRUE WHERE id=$1 AND reading_trial_used=FALSE`, uid)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

type lotDraw struct {
	LotID     int64
	Source    string
	Amount    int
	Remaining int
	Expires   sql.NullTime
}

func spendQuota(ctx context.Context, tx Tx, uid int64, cost int) (after int, ok bool, draws []lotDraw, err error) {
	if cost <= 0 {
		return 0, false, nil, nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO quota_balances(user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return 0, false, nil, err
	}
	var bal int
	if err = tx.QueryRow(ctx, `SELECT quota_balance FROM quota_balances WHERE user_id=$1 FOR UPDATE`, uid).Scan(&bal); err != nil {
		return 0, false, nil, err
	}
	if err = expireQuotaLots(ctx, tx, uid); err != nil {
		return 0, false, nil, err
	}
	if err = tx.QueryRow(ctx, `SELECT quota_balance FROM quota_balances WHERE user_id=$1`, uid).Scan(&bal); err != nil {
		return 0, false, nil, err
	}
	if bal < cost {
		return 0, false, nil, nil
	}
	var lots int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM quota_lots WHERE user_id=$1 AND remaining>0 AND (expires_at IS NULL OR expires_at > now())`, uid).Scan(&lots); err != nil {
		return 0, false, nil, err
	}
	if lots > 0 {
		draws, err = drawQuotaLots(ctx, tx, uid, cost)
		if err != nil {
			return 0, false, nil, err
		}
	}
	after = bal - cost
	if _, err = tx.Exec(ctx, `UPDATE quota_balances SET quota_balance=$1, updated_at=now() WHERE user_id=$2`, after, uid); err != nil {
		return 0, false, nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO quota_ledger(user_id, bucket, delta, balance_after, store_transaction_id, reason)
		VALUES ($1,$2,$3,$4,'','quota_spend')`, uid, iapBucketQuota, -cost, after); err != nil {
		return 0, false, nil, err
	}
	return after, true, draws, nil
}

// expireQuotaLots drops a pool that has no active membership, and drops lots
// whose own expires_at has passed. Expired remainder is discarded, not frozen.
// Users with no lots keep the legacy balance.
func expireQuotaLots(ctx context.Context, ex execer, uid int64) error {
	switch db := ex.(type) {
	case Tx:
		if err := rollDueSubscription(ctx, db, uid); err != nil {
			return err
		}
	case *Store:
		if err := rollDueSubscriptionOnStore(ctx, db, uid); err != nil {
			return err
		}
	}
	var n int
	if err := ex.QueryRow(ctx, `SELECT count(*) FROM quota_lots WHERE user_id=$1`, uid).Scan(&n); err != nil || n == 0 {
		return err
	}
	member, err := hasActiveMembership(ctx, ex, uid)
	if err != nil {
		return err
	}
	if !member {
		if _, err := ex.Exec(ctx, `UPDATE quota_lots SET remaining=0 WHERE user_id=$1 AND remaining>0`, uid); err != nil {
			return err
		}
		_, err = ex.Exec(ctx, `UPDATE quota_balances SET quota_balance=0, updated_at=now() WHERE user_id=$1`, uid)
		return err
	}
	if _, err := ex.Exec(ctx, `UPDATE quota_lots SET remaining=0
		WHERE user_id=$1 AND expires_at IS NOT NULL AND expires_at <= now() AND remaining>0`, uid); err != nil {
		return err
	}
	_, err = ex.Exec(ctx, `UPDATE quota_balances b
		SET quota_balance = s.live, updated_at=now()
		FROM (
			SELECT COALESCE(SUM(remaining),0) AS live FROM quota_lots
			WHERE user_id=$1 AND remaining>0 AND (expires_at IS NULL OR expires_at > now())
		) s
		WHERE b.user_id=$1`, uid)
	return err
}

func rollDueSubscriptionOnStore(ctx context.Context, store *Store, uid int64) error {
	tx, err := store.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := rollDueSubscription(ctx, tx, uid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func rollDueSubscription(ctx context.Context, tx Tx, uid int64) error {
	var id int64
	var pending time.Time
	var quota sql.NullInt64
	var orderID sql.NullInt64
	err := tx.QueryRow(ctx, `SELECT id, pending_expires_at, pending_quota, pending_order_id
		FROM subscriptions
		WHERE user_id=$1 AND pending_expires_at IS NOT NULL AND expires_at <= now()
		ORDER BY expires_at LIMIT 1 FOR UPDATE`, uid).Scan(&id, &pending, &quota, &orderID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !pending.After(time.Now().UTC()) {
		_, err = tx.Exec(ctx, `UPDATE subscriptions
			SET status='expired', pending_expires_at=NULL, pending_quota=NULL, pending_order_id=NULL, pending_sku_kind=''
			WHERE id=$1`, id)
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE subscriptions
		SET sku_kind=CASE WHEN pending_sku_kind<>'' THEN pending_sku_kind ELSE sku_kind END,
			sku_id=(SELECT sku_id FROM iap_orders WHERE id=subscriptions.pending_order_id),
			status='active', starts_at=expires_at, expires_at=$1,
			pending_expires_at=NULL, pending_quota=NULL, pending_order_id=NULL, pending_sku_kind=''
		WHERE id=$2`, pending, id)
	if err != nil {
		return err
	}
	if quota.Valid && quota.Int64 > 0 {
		var oid *int64
		if orderID.Valid {
			v := orderID.Int64
			oid = &v
		}
		if _, err = creditQuota(ctx, tx, uid, int(quota.Int64), oid, "", "iap_period_refresh"); err != nil {
			return err
		}
	}
	if orderID.Valid && quota.Valid {
		_, err = tx.Exec(ctx, `UPDATE iap_orders SET credit_status='credited', credited_quota=$1, updated_at=now()
			WHERE id=$2 AND credit_status='deferred'`, quota.Int64, orderID.Int64)
	}
	return err
}

func quotaLotSource(reason string) string {
	switch reason {
	case "iap_quota_pack", "iap_pack_grant", "iap_restore_pack_grant":
		return "pack"
	case grantReason, "admin_grant_pack":
		return "admin"
	default:
		return "subscription"
	}
}

func drawQuotaLots(ctx context.Context, tx Tx, uid int64, cost int) ([]lotDraw, error) {
	rows, err := tx.Query(ctx, `SELECT id, source, remaining, expires_at FROM quota_lots
		WHERE user_id=$1 AND remaining>0 AND (expires_at IS NULL OR expires_at > now())
		ORDER BY CASE source WHEN 'subscription' THEN 0 ELSE 1 END, expires_at NULLS LAST, id
		FOR UPDATE`, uid)
	if err != nil {
		return nil, err
	}
	var takes []lotDraw
	left := cost
	for rows.Next() {
		var draw lotDraw
		if err := rows.Scan(&draw.LotID, &draw.Source, &draw.Remaining, &draw.Expires); err != nil {
			rows.Close()
			return nil, err
		}
		if left == 0 || draw.Remaining <= 0 {
			continue
		}
		n := draw.Remaining
		if n > left {
			n = left
		}
		draw.Amount = n
		draw.Remaining -= n
		takes = append(takes, draw)
		left -= n
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if left > 0 {
		return nil, biz("额度批次不足")
	}
	for _, tk := range takes {
		if _, err := tx.Exec(ctx, `UPDATE quota_lots SET remaining = remaining - $2 WHERE id=$1`, tk.LotID, tk.Amount); err != nil {
			return nil, err
		}
	}
	return takes, nil
}

func creditQuota(ctx context.Context, tx Tx, uid int64, amount int, orderID *int64, txn, reason string) (int, error) {
	return creditQuotaSource(ctx, tx, uid, amount, orderID, txn, reason, "")
}

func creditQuotaSource(ctx context.Context, tx Tx, uid int64, amount int, orderID *int64, txn, reason, source string) (int, error) {
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
	if reason == "iap_period_refresh" || reason == "iap_restore_period_refresh" {
		if _, err := tx.Exec(ctx, `UPDATE quota_lots SET remaining=0 WHERE user_id=$1 AND remaining>0`, uid); err != nil {
			return 0, err
		}
		bal = 0
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
	if err != nil {
		return 0, err
	}
	var exp any
	var expAt time.Time
	err = tx.QueryRow(ctx, `SELECT expires_at FROM subscriptions
		WHERE user_id=$1 AND status='active' AND expires_at > now()
		ORDER BY expires_at DESC LIMIT 1`, uid).Scan(&expAt)
	if err == nil {
		exp = expAt
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if source == "" {
		source = quotaLotSource(reason)
	}
	_, err = tx.Exec(ctx, `INSERT INTO quota_lots(user_id, source, total, remaining, expires_at, order_id, reason)
		VALUES ($1,$2,$3,$3,$4,$5,$6)`, uid, source, amount, exp, oid, reason)
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

func loadPlayCost(ctx context.Context, ex execer, readingID int64) (int, bool, error) {
	if readingID > 0 {
		var cost sql.NullInt64
		err := ex.QueryRow(ctx, `SELECT s.quota_cost FROM readings r JOIN spreads s ON s.id=r.spread_id WHERE r.id=$1`, readingID).Scan(&cost)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, false, err
		}
		if err == nil && cost.Valid && cost.Int64 > 0 {
			return int(cost.Int64), true, nil
		}
	}
	return loadQuotaCost(ctx, ex)
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

func loadTokensPerQuota(ctx context.Context, ex execer) (int, bool, error) {
	var n sql.NullInt64
	err := ex.QueryRow(ctx, `SELECT tokens_per_quota FROM quota_settings WHERE id=1`).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if !n.Valid || n.Int64 <= 0 {
		return 0, false, nil
	}
	return int(n.Int64), true, nil
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

func rewardBlocked(ctx context.Context, ex execer, uid int64) (bool, error) {
	if uid <= 0 {
		return false, nil
	}
	var n int
	err := ex.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT 1 FROM user_devices ud
			WHERE ud.user_id=$1 AND (
				EXISTS (SELECT 1 FROM user_devices o WHERE o.device_id=ud.device_id AND o.user_id<>$1)
				OR EXISTS (SELECT 1 FROM identity_claims c WHERE c.kind='device' AND c.value=ud.device_id)
				OR EXISTS (
					SELECT 1 FROM device_experiences d
					WHERE lower(d.device_id)=ud.device_id AND d.user_id IS NOT NULL AND d.user_id<>$1
				)
			)
			UNION ALL
			SELECT 1 FROM app_users u
			JOIN identity_claims c ON (
				(c.kind='email' AND u.email IS NOT NULL AND lower(c.value)=lower(u.email))
				OR (c.kind='wechat' AND u.wechat_openid IS NOT NULL AND lower(c.value)=lower(u.wechat_openid))
				OR (c.kind='apple' AND u.apple_sub IS NOT NULL AND lower(c.value)=lower(u.apple_sub))
				OR (c.kind='phone' AND u.phone IS NOT NULL AND lower(c.value)=lower(u.phone))
			)
			WHERE u.id=$1
		) hits`, uid).Scan(&n)
	return n > 0, err
}

func (s *Store) noteRegistration(ctx context.Context, uid int64) error {
	if uid <= 0 {
		return nil
	}
	if _, err := s.Exec(ctx, `INSERT INTO registration_gifts(user_id, registered_at) VALUES ($1, now()) ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return err
	}
	blocked, err := rewardBlocked(ctx, s, uid)
	if err != nil {
		return err
	}
	if blocked {
		_, err = s.Exec(ctx, `UPDATE registration_gifts SET reward_blocked=TRUE WHERE user_id=$1 AND granted_at IS NULL`, uid)
		return err
	}
	return s.ensureRegistrationGift(ctx, uid)
}

func (s *Store) ensureRegistrationGift(ctx context.Context, uid int64) error {
	return ensureRegistrationGift(ctx, s, uid)
}

func ensureRegistrationGift(ctx context.Context, ex execer, uid int64) error {
	if uid <= 0 {
		return nil
	}
	var registered time.Time
	var granted sql.NullTime
	var blocked bool
	err := ex.QueryRow(ctx, `SELECT registered_at, granted_at, reward_blocked FROM registration_gifts WHERE user_id=$1`, uid).Scan(&registered, &granted, &blocked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if granted.Valid || blocked {
		return nil
	}
	blocked, err = rewardBlocked(ctx, ex, uid)
	if err != nil {
		return err
	}
	if blocked {
		_, err = ex.Exec(ctx, `UPDATE registration_gifts SET reward_blocked=TRUE WHERE user_id=$1 AND granted_at IS NULL`, uid)
		return err
	}
	cfg, err := loadRegistrationSettings(ctx, ex)
	if err != nil {
		return err
	}
	if !cfg.DelayMinutes.Valid {
		return nil
	}
	if time.Now().Before(registered.Add(time.Duration(cfg.DelayMinutes.Int64) * time.Minute)) {
		return nil
	}
	// Use DB now() for usable_at so same-transaction checks like usable_at<=now() pass.
	_, err = ex.Exec(ctx, `UPDATE registration_gifts
		SET granted_at=now(), usable_at=now(),
			expires_at=now() + make_interval(days => $2),
			total=$3, remaining=$3
		WHERE user_id=$1 AND granted_at IS NULL`,
		uid, cfg.ValidDays, cfg.GrantCount)
	return err
}

func loadRegistrationGift(ctx context.Context, ex execer, uid int64) (map[string]any, error) {
	empty := map[string]any{
		"registered_at": nil, "granted_at": nil, "usable_at": nil, "expires_at": nil,
		"total": 0, "remaining": 0, "used": false, "used_at": nil, "reward_blocked": false,
	}
	if uid <= 0 {
		return empty, nil
	}
	var registered time.Time
	var granted, usable, expires, usedAt sql.NullTime
	var total, remaining int
	var blocked bool
	err := ex.QueryRow(ctx, `SELECT registered_at, granted_at, usable_at, expires_at, total, remaining, used_at, reward_blocked FROM registration_gifts WHERE user_id=$1`, uid).
		Scan(&registered, &granted, &usable, &expires, &total, &remaining, &usedAt, &blocked)
	if errors.Is(err, sql.ErrNoRows) {
		return empty, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"registered_at":  registered,
		"granted_at":     nullTime(granted),
		"usable_at":      nullTime(usable),
		"expires_at":     nullTime(expires),
		"total":          total,
		"remaining":      remaining,
		"used":           usedAt.Valid,
		"used_at":        nullTime(usedAt),
		"reward_blocked": blocked,
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
	uid, err := a.appActor(r)
	if err != nil {
		return quotaDecision{}, err
	}
	if uid == 0 {
		return quotaDecision{Status: quotaAnon, Message: paywallNeedLogin}, nil
	}
	if err := a.ownReading(r.Context(), readingID, uid, ""); err != nil {
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

// playCover reports whether the play charge should happen now.
// now: a full entitlement can pay. short: member cost is missing or the pool is
// above 0 but short, so the ritual stops. later: no entitlement yet, shuffle and
// draw stay open and interpretation charges. done: this play was already charged.
func playCover(ctx context.Context, tx Tx, opts chargeOpts) (string, error) {
	already, _, err := playAlready(ctx, tx, opts.ReadingID)
	if err != nil {
		return "", err
	}
	if already {
		return "done", nil
	}
	if err := ensureRegistrationGift(ctx, tx, opts.UserID); err != nil {
		return "", err
	}
	member, err := hasActiveMembership(ctx, tx, opts.UserID)
	if err != nil {
		return "", err
	}
	if member {
		cost, configured, err := loadPlayCost(ctx, tx, opts.ReadingID)
		if err != nil {
			return "", err
		}
		if !configured {
			return "short", nil
		}
		if err := expireQuotaLots(ctx, tx, opts.UserID); err != nil {
			return "", err
		}
		bal, err := loadQuotaBalance(ctx, tx, opts.UserID)
		if err != nil {
			return "", err
		}
		if bal >= cost {
			return "now", nil
		}
		if bal > 0 {
			return "short", nil
		}
	}
	ok, err := hasPlayCredit(ctx, tx, opts)
	if err != nil {
		return "", err
	}
	if ok {
		return "now", nil
	}
	return "later", nil
}

func hasPlayCredit(ctx context.Context, tx Tx, opts chargeOpts) (bool, error) {
	scope, err := giftInviteScope(ctx, tx, opts.ReadingID, opts.ChargeKey)
	if err != nil {
		return false, err
	}
	if scope {
		var n int
		err = tx.QueryRow(ctx, `SELECT remaining FROM registration_gifts
			WHERE user_id=$1 AND granted_at IS NOT NULL AND usable_at<=now() AND expires_at>now() AND remaining>0`, opts.UserID).Scan(&n)
		if err == nil && n > 0 {
			return true, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		bal, err := inviteBalance(ctx, tx, opts.UserID)
		if err != nil {
			return false, err
		}
		if bal > 0 {
			return true, nil
		}
	}
	cardCount, err := readingSpreadCardCount(ctx, tx, opts.ReadingID)
	if err != nil {
		return false, err
	}
	var kind string
	if err := tx.QueryRow(ctx, `SELECT type FROM readings WHERE id=$1`, opts.ReadingID).Scan(&kind); err != nil {
		return false, err
	}
	if !freePlayEligible(kind, cardCount, opts.ChargeKey) {
		return false, nil
	}
	var admin int
	err = tx.QueryRow(ctx, `SELECT admin_grant_remaining FROM rewards WHERE user_id=$1`, opts.UserID).Scan(&admin)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return admin > 0, nil
}

// chargeAtSpread bills a covered play after the spread is stored and before shuffle.
// A short member pool stops here. A user with no entitlement is left for interpretation.
func (a *App) chargeAtSpread(w http.ResponseWriter, r *http.Request, readingID int64) (bool, error) {
	uid, err := a.appActor(r)
	if err != nil {
		return false, err
	}
	if uid == 0 {
		return false, nil
	}
	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return false, err
	}
	defer tx.Rollback(r.Context())
	opts := chargeOpts{
		UserID: uid, ReadingID: readingID, ChargeKey: chargeKeyPlay, Cycle: a.userCycle(r, uid),
		AllowTrial: true, ResumeAction: resumeShuffle,
	}
	cover, err := playCover(r.Context(), tx, opts)
	if err != nil {
		return false, err
	}
	if cover == "later" || cover == "done" {
		// Commit so playCover's pending registration-gift grant is kept.
		// Rolling back here used to drop the gift and force App GET /quota.
		if err := tx.Commit(r.Context()); err != nil {
			return false, err
		}
		return false, nil
	}
	decision, err := settleSharedQuota(r.Context(), tx, opts)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return false, err
	}
	if decision.Continue {
		return false, nil
	}
	writeQuotaBlock(w, r, decision, readingID)
	return true, nil
}

// gatePlay charges before interpretation. Shuffle and draw stay open until a
// play charge exists or a paywall is already waiting.
func (a *App) gatePlay(w http.ResponseWriter, r *http.Request, action string) (bool, error) {
	if strings.TrimSpace(r.Header.Get("Authorization")) == "" {
		return false, nil
	}
	uid, err := a.appActor(r)
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
	open, err := openOnboardReading(r.Context(), a.Store, id, uid)
	if err != nil {
		return false, err
	}
	if open {
		return false, nil
	}
	if action == resumeShuffle || action == resumeDraw {
		already, _, err := playAlready(r.Context(), a.Store, id)
		if err != nil {
			return false, err
		}
		if already {
			return false, nil
		}
		var waiting int
		if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM quota_paywall
			WHERE reading_id=$1 AND charge_key=$2 AND status IN ($3,$4)`, id, chargeKeyPlay, paywallPending, paywallUncfg).Scan(&waiting); err != nil {
			return false, err
		}
		if waiting == 0 {
			return false, nil
		}
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
		var drawMode string
		_ = tx.QueryRow(ctx, `SELECT COALESCE(draw_mode,'') FROM readings WHERE id=$1`, readingID).Scan(&drawMode)
		// Numerology does not use shuffle/cut; keep SPREAD_SELECTED for /numerology.
		if strings.TrimSpace(drawMode) != "numerology" {
			if _, err := tx.Exec(ctx, `UPDATE readings SET status='SHUFFLING', updated_at=now() WHERE id=$1 AND status='SPREAD_SELECTED'`, readingID); err != nil {
				return nil, err
			}
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
	uid, err := a.appActor(r)
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
	if err := a.ownReading(r.Context(), id, uid, ""); err != nil {
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
