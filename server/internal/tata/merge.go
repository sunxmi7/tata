package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Account merge runs only after an explicit confirm call.
// Bind conflict creates a pending request and does not move balances, readings, or messages.
// Unbind / rebind is intentionally not implemented.

const rewardConsumeSQL = `UPDATE rewards SET remaining = remaining - 1, consumed_at = CASE WHEN remaining <= 1 THEN now() ELSE consumed_at END WHERE user_id=$1 AND remaining > 0 AND expires_at > now()`

var errMergeAlready = errors.New("merge already applied")

var identityColumns = map[string]string{
	"wechat": "wechat_openid",
	"apple":  "apple_sub",
	"email":  "email",
	"phone":  "phone",
}

type bindSpec struct {
	kind         string
	value        string
	passwordHash string
}

type execer interface {
	QueryRow(ctx context.Context, query string, args ...any) *sql.Row
	Exec(ctx context.Context, query string, args ...any) (tagResult, error)
}

type mergeParties struct {
	PrimaryUserID   int64 `json:"primary_user_id"`
	SecondaryUserID int64 `json:"secondary_user_id"`
	SurvivingUserID int64 `json:"surviving_user_id,omitempty"`
}

type mergeOutcome struct {
	Merged                          bool         `json:"merged"`
	Before                          mergeParties `json:"before"`
	After                           mergeParties `json:"after"`
	ConfirmAt                       time.Time    `json:"confirm_at"`
	RewardBeforePrimary             int          `json:"reward_before_primary"`
	RewardBeforeSecondary           int          `json:"reward_before_secondary"`
	RewardRemaining                 int          `json:"reward_remaining"`
	DailyFreeRemaining              int          `json:"daily_free_remaining"`
	DailyUsedFlags                  int          `json:"daily_used_flags"`
	DailySlotRemaining              int          `json:"daily_slot_remaining"`
	DailySlotUsed                   int          `json:"daily_slot_used"`
	PaidSubscriptionBeforePrimary   int          `json:"paid_subscription_before_primary"`
	PaidSubscriptionBeforeSecondary int          `json:"paid_subscription_before_secondary"`
	PaidPackBeforePrimary           int          `json:"paid_pack_before_primary"`
	PaidPackBeforeSecondary         int          `json:"paid_pack_before_secondary"`
	PaidSubscriptionRemaining       int          `json:"paid_subscription_remaining"`
	PaidPackRemaining               int          `json:"paid_pack_remaining"`
	ReadingsMoved                   int64        `json:"readings_moved"`
	MessagesKept                    int64        `json:"messages_kept"`
	SubscriptionResolution          string       `json:"subscription_resolution"`
	SubscriptionPromptRequired      bool         `json:"subscription_prompt_required"`
	SubscriptionWinnerID            int64        `json:"subscription_winner_id,omitempty"`
	IdentityPromptRequired          bool         `json:"identity_prompt_required"`
	RetainedIdentities              []string     `json:"retained_identities,omitempty"`
}

type subscriptionRow struct {
	ID         int64
	Status     string
	MergeState string
	ExpiresAt  time.Time
}

type subscriptionDecision struct {
	Resolution string
	Prompt     bool
	WinnerID   int64
	PromptIDs  []int64
	Superseded []int64
	Expired    []int64
}

type userRow struct {
	email, pass, wechat, apple, phone *string
	cancelled                         *time.Time
	merged                            *int64
}

type dailyContrib struct {
	used, remaining, slotUsed, slotRemaining int
}

func consumeRewardBalance(ctx context.Context, ex execer, uid int64) (bool, error) {
	tag, err := ex.Exec(ctx, rewardConsumeSQL, uid)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func consumeMergedDaily(ctx context.Context, ex execer, uid int64, cycle string) (handled, consumed bool, err error) {
	var remaining int
	err = ex.QueryRow(ctx, `SELECT remaining FROM daily_free_balance WHERE user_id=$1 AND cycle=$2`, uid, cycle).Scan(&remaining)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, false, nil
		}
		return false, false, err
	}
	if remaining <= 0 {
		return true, false, nil
	}
	tag, err := ex.Exec(ctx, `UPDATE daily_free_balance SET remaining=remaining-1, used_flags=used_flags+1 WHERE user_id=$1 AND cycle=$2 AND remaining>0`, uid, cycle)
	if err != nil {
		return true, false, err
	}
	return true, tag.RowsAffected() == 1, nil
}

func lockDailyBalance(ctx context.Context, ex execer, uid int64, cycle string) (ok bool, remaining, slotRemaining int, err error) {
	err = ex.QueryRow(ctx, `SELECT remaining, daily_slot_remaining FROM daily_free_balance WHERE user_id=$1 AND cycle=$2 FOR UPDATE`, uid, cycle).Scan(&remaining, &slotRemaining)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, 0, nil
	}
	if err != nil {
		return false, 0, 0, err
	}
	return true, remaining, slotRemaining, nil
}

func dailySlotTaken(ctx context.Context, ex execer, uid int64, cycle string) (bool, error) {
	var id int64
	err := ex.QueryRow(ctx, `SELECT reading_id FROM daily_slot WHERE user_id=$1 AND cycle=$2`, uid, cycle).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *App) bindWithConfirm(w http.ResponseWriter, r *http.Request, spec bindSpec) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能绑定")
	}
	col, ok := identityColumns[spec.kind]
	if !ok || spec.value == "" {
		return biz("请求字段不正确")
	}
	storedValue := spec.value
	if spec.kind == "email" {
		storedValue = strings.ToLower(strings.TrimSpace(spec.value))
	}
	var current *string
	var cancelled *time.Time
	var merged *int64
	q := fmt.Sprintf(`SELECT %s, cancelled_at, merged_into FROM app_users WHERE id=$1`, col)
	if err := a.Store.QueryRow(r.Context(), q, uid).Scan(&current, &cancelled, &merged); err != nil {
		return biz("账号已注销")
	}
	if cancelled != nil {
		return biz("账号已注销")
	}
	if merged != nil {
		return biz("账号已合并")
	}
	if current != nil && *current != "" {
		same := *current == spec.value
		if spec.kind == "email" {
			same = strings.EqualFold(*current, spec.value)
		}
		if same {
			if spec.kind == "email" {
				return biz("邮箱已绑定或账号已注销")
			}
			writeOK(w, true)
			return nil
		}
		return biz("该登录方式已绑定，换绑未开放")
	}

	var other int64
	var findErr error
	if spec.kind == "email" {
		findErr = a.Store.QueryRow(r.Context(), `SELECT id FROM app_users WHERE lower(email)=lower($1) AND id<>$2`, spec.value, uid).Scan(&other)
	} else {
		findQ := fmt.Sprintf(`SELECT id FROM app_users WHERE %s=$1 AND id<>$2`, col)
		findErr = a.Store.QueryRow(r.Context(), findQ, spec.value, uid).Scan(&other)
	}
	if findErr != nil {
		if !errors.Is(findErr, sql.ErrNoRows) {
			return findErr
		}
		return a.bindFreeIdentity(w, r, spec, col, uid)
	}
	var otherCancelled *time.Time
	var otherMerged *int64
	if err := a.Store.QueryRow(r.Context(), `SELECT cancelled_at, merged_into FROM app_users WHERE id=$1`, other).Scan(&otherCancelled, &otherMerged); err != nil {
		return err
	}
	if otherCancelled != nil {
		return biz("对方账号已注销，不能合并")
	}
	if otherMerged != nil {
		return biz("对方账号已合并")
	}
	requestID, created, err := a.ensureMergeRequest(r.Context(), uid, other, spec.kind, storedValue, spec.passwordHash)
	if err != nil {
		return err
	}
	if created {
		detail, _ := json.Marshal(map[string]any{
			"before":            map[string]int64{"primary_user_id": uid, "secondary_user_id": other},
			"after":             map[string]int64{"primary_user_id": uid, "secondary_user_id": other},
			"merged":            false,
			"confirm_required":  true,
			"identity_kind":     spec.kind,
			"confirmer_kind":    "app_user",
			"confirmer_user_id": uid,
		})
		a.Store.auditActor(r.Context(), uid, "app_user", "account_merge_conflict", "app_user", fmtID(uid), string(detail))
	}
	writeBindConflict(w, map[string]any{
		"bound":             false,
		"conflict":          true,
		"merged":            false,
		"confirm_required":  true,
		"merge_request_id":  requestID,
		"primary_user_id":   uid,
		"secondary_user_id": other,
		"identity_kind":     spec.kind,
	})
	return nil
}

func (a *App) bindFreeIdentity(w http.ResponseWriter, r *http.Request, spec bindSpec, col string, uid int64) error {
	var tag tagResult
	var err error
	if spec.kind == "email" {
		tag, err = a.Store.Exec(r.Context(), `UPDATE app_users SET email=$1, password_hash=$2 WHERE id=$3 AND cancelled_at IS NULL AND merged_into IS NULL AND email IS NULL`, spec.value, spec.passwordHash, uid)
	} else {
		upd := fmt.Sprintf(`UPDATE app_users SET %s=$1 WHERE id=$2 AND cancelled_at IS NULL AND merged_into IS NULL AND %s IS NULL`, col, col)
		tag, err = a.Store.Exec(r.Context(), upd, spec.value, uid)
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return biz("该身份已绑定其他账号")
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		if spec.kind == "email" {
			return biz("邮箱已绑定或账号已注销")
		}
		return biz("账号已注销")
	}
	writeOK(w, true)
	return nil
}

func (a *App) ensureMergeRequest(ctx context.Context, primary, secondary int64, kind, value, passHash string) (int64, bool, error) {
	var id int64
	err := a.Store.QueryRow(ctx, `SELECT id FROM account_merge_requests
		WHERE primary_user_id=$1 AND secondary_user_id=$2 AND identity_kind=$3 AND identity_value=$4 AND status='pending_confirm'
		ORDER BY id DESC LIMIT 1`, primary, secondary, kind, value).Scan(&id)
	if err == nil {
		if passHash != "" {
			_, _ = a.Store.Exec(ctx, `UPDATE account_merge_requests SET email_password_hash=$1 WHERE id=$2 AND status='pending_confirm'`, passHash, id)
		}
		return id, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	err = a.Store.QueryRow(ctx, `INSERT INTO account_merge_requests(primary_user_id, secondary_user_id, identity_kind, identity_value, email_password_hash, status)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),'pending_confirm') RETURNING id`, primary, secondary, kind, value, passHash).Scan(&id)
	if err != nil {
		var again int64
		if err2 := a.Store.QueryRow(ctx, `SELECT id FROM account_merge_requests
			WHERE primary_user_id=$1 AND secondary_user_id=$2 AND identity_kind=$3 AND identity_value=$4 AND status='pending_confirm'
			ORDER BY id DESC LIMIT 1`, primary, secondary, kind, value).Scan(&again); err2 == nil {
			return again, false, nil
		}
		return 0, false, err
	}
	return id, true, nil
}

func writeBindConflict(w http.ResponseWriter, payload map[string]any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"code":    1,
		"data":    payload,
		"message": "身份已被其他账号占用，需要确认后才会合并",
	})
}

func (s *Store) auditActor(ctx context.Context, operatorID int64, actorKind, action, objectType, objectID, detail string) {
	if actorKind == "" {
		actorKind = "operator"
	}
	_, _ = s.Exec(ctx, `INSERT INTO audit_log(operator_id, action, object_type, object_id, detail, actor_kind) VALUES ($1,$2,$3,$4,$5,$6)`,
		operatorID, action, objectType, objectID, detail, actorKind)
}

func (a *App) appMergeConfirm(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能确认合并")
	}
	var req struct {
		MergeRequestID int64 `json:"merge_request_id"`
		Confirm        bool  `json:"confirm"`
	}
	if err := decode(r, &req); err != nil || req.MergeRequestID <= 0 {
		return biz("请求字段不正确")
	}
	if !req.Confirm {
		return biz("未确认，不会合并账号")
	}
	var status string
	var primary int64
	var result string
	err = a.Store.QueryRow(r.Context(), `SELECT status, primary_user_id, result FROM account_merge_requests WHERE id=$1`, req.MergeRequestID).Scan(&status, &primary, &result)
	if err != nil {
		return biz("合并请求不存在")
	}
	if primary != uid {
		return biz("只能确认自己发起的合并")
	}
	if status == "merged" {
		return writeStoredMerge(w, result)
	}
	if status != "pending_confirm" {
		return biz("合并请求不可确认")
	}
	out, err := a.applyMerge(r.Context(), req.MergeRequestID, uid)
	if errors.Is(err, errMergeAlready) {
		_ = a.Store.QueryRow(r.Context(), `SELECT result FROM account_merge_requests WHERE id=$1`, req.MergeRequestID).Scan(&result)
		return writeStoredMerge(w, result)
	}
	if err != nil {
		return err
	}
	writeOK(w, out)
	return nil
}

func writeStoredMerge(w http.ResponseWriter, raw string) error {
	if raw == "" {
		return biz("合并结果缺失")
	}
	var out mergeOutcome
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return err
	}
	writeOK(w, out)
	return nil
}

func (a *App) applyMerge(ctx context.Context, requestID, actor int64) (mergeOutcome, error) {
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return mergeOutcome{}, err
	}
	defer tx.Rollback(ctx)

	var status string
	var primary, secondary int64
	var passHash *string
	err = tx.QueryRow(ctx, `SELECT status, primary_user_id, secondary_user_id, email_password_hash
		FROM account_merge_requests WHERE id=$1 FOR UPDATE`, requestID).Scan(&status, &primary, &secondary, &passHash)
	if err != nil {
		return mergeOutcome{}, err
	}
	if primary != actor {
		return mergeOutcome{}, biz("只能确认自己发起的合并")
	}
	if status == "merged" {
		return mergeOutcome{}, errMergeAlready
	}
	if status != "pending_confirm" {
		return mergeOutcome{}, biz("合并请求不可确认")
	}

	first, second := primary, secondary
	if first > second {
		first, second = second, first
	}
	locked := map[int64]userRow{}
	for _, id := range []int64{first, second} {
		row, err := lockUser(ctx, tx, id)
		if err != nil {
			return mergeOutcome{}, err
		}
		locked[id] = row
	}
	primaryUser := locked[primary]
	secondaryUser := locked[secondary]
	if primaryUser.cancelled != nil {
		return mergeOutcome{}, biz("账号已注销")
	}
	if secondaryUser.cancelled != nil {
		return mergeOutcome{}, biz("对方账号已注销，不能合并")
	}
	if primaryUser.merged != nil || secondaryUser.merged != nil {
		return mergeOutcome{}, biz("账号已合并")
	}

	now := time.Now().UTC()
	var messagesBefore int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM reading_messages m JOIN readings r ON r.id=m.reading_id WHERE r.user_id IN ($1,$2)`, primary, secondary).Scan(&messagesBefore); err != nil {
		return mergeOutcome{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE readings SET user_id=$1, updated_at=now() WHERE user_id=$2`, primary, secondary)
	if err != nil {
		return mergeOutcome{}, err
	}
	readingsMoved := tag.RowsAffected()
	if _, err := tx.Exec(ctx, `UPDATE ledger SET user_id=$1 WHERE user_id=$2`, primary, secondary); err != nil {
		return mergeOutcome{}, err
	}
	var messagesAfter int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM reading_messages m JOIN readings r ON r.id=m.reading_id WHERE r.user_id=$1`, primary).Scan(&messagesAfter); err != nil {
		return mergeOutcome{}, err
	}
	if messagesAfter != messagesBefore {
		return mergeOutcome{}, fmt.Errorf("reading messages changed across merge: before %d after %d", messagesBefore, messagesAfter)
	}

	rewardP, rewardS, rewardAfter, err := foldRewards(ctx, tx, primary, secondary, now)
	if err != nil {
		return mergeOutcome{}, err
	}
	daily, err := foldDaily(ctx, tx, primary, secondary, now)
	if err != nil {
		return mergeOutcome{}, err
	}
	subP, packP, subS, packS, subAfter, packAfter, err := foldPaid(ctx, tx, primary, secondary)
	if err != nil {
		return mergeOutcome{}, err
	}
	decision, err := foldSubscriptions(ctx, tx, primary, secondary, now)
	if err != nil {
		return mergeOutcome{}, err
	}
	hash := ""
	if passHash != nil {
		hash = *passHash
	}
	retained, err := moveIdentities(ctx, tx, requestID, primary, secondary, primaryUser, secondaryUser, hash, decision.Prompt, now)
	if err != nil {
		return mergeOutcome{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE refresh_tokens SET revoked=true WHERE audience='app' AND subject_id=$1`, secondary); err != nil {
		return mergeOutcome{}, err
	}

	out := mergeOutcome{
		Merged:              true,
		Before:              mergeParties{PrimaryUserID: primary, SecondaryUserID: secondary},
		After:               mergeParties{PrimaryUserID: primary, SecondaryUserID: secondary, SurvivingUserID: primary},
		ConfirmAt:           now,
		RewardBeforePrimary: rewardP, RewardBeforeSecondary: rewardS, RewardRemaining: rewardAfter,
		DailyFreeRemaining: daily.remaining, DailyUsedFlags: daily.used,
		DailySlotRemaining: daily.slotRemaining, DailySlotUsed: daily.slotUsed,
		PaidSubscriptionBeforePrimary: subP, PaidSubscriptionBeforeSecondary: subS,
		PaidPackBeforePrimary: packP, PaidPackBeforeSecondary: packS,
		PaidSubscriptionRemaining: subAfter, PaidPackRemaining: packAfter,
		ReadingsMoved: readingsMoved, MessagesKept: messagesAfter,
		SubscriptionResolution: decision.Resolution, SubscriptionPromptRequired: decision.Prompt,
		SubscriptionWinnerID:   decision.WinnerID,
		IdentityPromptRequired: len(retained) > 0, RetainedIdentities: retained,
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return mergeOutcome{}, err
	}
	tag, err = tx.Exec(ctx, `UPDATE account_merge_requests SET status='merged', confirm_at=$1, result=$2, subscription_resolution=$3 WHERE id=$4 AND status='pending_confirm'`,
		now, string(raw), decision.Resolution, requestID)
	if err != nil {
		return mergeOutcome{}, err
	}
	if tag.RowsAffected() == 0 {
		return mergeOutcome{}, errMergeAlready
	}
	detail, err := json.Marshal(map[string]any{
		"before":            out.Before,
		"after":             out.After,
		"confirm_at":        out.ConfirmAt,
		"merge_result":      out,
		"confirmer_kind":    "app_user",
		"confirmer_user_id": primary,
	})
	if err != nil {
		return mergeOutcome{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_log(operator_id, action, object_type, object_id, detail, actor_kind) VALUES ($1,'account_merge','app_user',$2,$3,'app_user')`,
		primary, fmtID(primary), string(detail)); err != nil {
		return mergeOutcome{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return mergeOutcome{}, err
	}
	return out, nil
}

func lockUser(ctx context.Context, tx Tx, id int64) (userRow, error) {
	var row userRow
	err := tx.QueryRow(ctx, `SELECT email, password_hash, wechat_openid, apple_sub, phone, cancelled_at, merged_into FROM app_users WHERE id=$1 FOR UPDATE`, id).
		Scan(&row.email, &row.pass, &row.wechat, &row.apple, &row.phone, &row.cancelled, &row.merged)
	return row, err
}

func rewardBalance(ctx context.Context, ex execer, uid int64, now time.Time) (int, time.Time, error) {
	var rem int
	var exp time.Time
	err := ex.QueryRow(ctx, `SELECT remaining, expires_at FROM rewards WHERE user_id=$1`, uid).Scan(&rem, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	if rem < 0 || !exp.After(now) {
		return 0, time.Time{}, nil
	}
	return rem, exp, nil
}

func foldRewards(ctx context.Context, tx Tx, primary, secondary int64, now time.Time) (int, int, int, error) {
	beforeP, expP, err := rewardBalance(ctx, tx, primary, now)
	if err != nil {
		return 0, 0, 0, err
	}
	beforeS, expS, err := rewardBalance(ctx, tx, secondary, now)
	if err != nil {
		return 0, 0, 0, err
	}
	after := beforeP + beforeS
	if after > 0 {
		exp := expP
		if beforeP == 0 || (beforeS > 0 && expS.After(expP)) {
			exp = expS
		}
		if _, err := tx.Exec(ctx, `INSERT INTO rewards(user_id, granted_at, expires_at, remaining, consumed_at)
			VALUES ($1, now(), $2, $3, NULL)
			ON CONFLICT (user_id) DO UPDATE SET remaining=EXCLUDED.remaining, expires_at=EXCLUDED.expires_at, consumed_at=NULL`,
			primary, exp, after); err != nil {
			return 0, 0, 0, err
		}
	} else if _, err := tx.Exec(ctx, `UPDATE rewards SET remaining=0 WHERE user_id=$1`, primary); err != nil {
		return 0, 0, 0, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM rewards WHERE user_id=$1`, secondary); err != nil {
		return 0, 0, 0, err
	}
	return beforeP, beforeS, after, nil
}

func foldPaid(ctx context.Context, tx Tx, primary, secondary int64) (subP, packP, subS, packS, subAfter, packAfter int, err error) {
	subP, packP, err = poolOf(ctx, tx, primary)
	if err != nil {
		return
	}
	subS, packS, err = poolOf(ctx, tx, secondary)
	if err != nil {
		return
	}
	subAfter = subP + subS
	packAfter = packP + packS
	_, err = tx.Exec(ctx, `INSERT INTO paid_pools(user_id, subscription_remaining, pack_remaining, updated_at)
		VALUES ($1,$2,$3,now())
		ON CONFLICT (user_id) DO UPDATE SET subscription_remaining=EXCLUDED.subscription_remaining, pack_remaining=EXCLUDED.pack_remaining, updated_at=now()`,
		primary, subAfter, packAfter)
	if err != nil {
		return
	}
	_, err = tx.Exec(ctx, `DELETE FROM paid_pools WHERE user_id=$1`, secondary)
	return
}

func poolOf(ctx context.Context, ex execer, uid int64) (int, int, error) {
	var sub, pack int
	err := ex.QueryRow(ctx, `SELECT subscription_remaining, pack_remaining FROM paid_pools WHERE user_id=$1`, uid).Scan(&sub, &pack)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	return sub, pack, err
}

func foldSubscriptions(ctx context.Context, tx Tx, primary, secondary int64, now time.Time) (subscriptionDecision, error) {
	if _, err := tx.Exec(ctx, `UPDATE subscriptions SET user_id=$1 WHERE user_id=$2`, primary, secondary); err != nil {
		return subscriptionDecision{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id, status, merge_state, expires_at FROM subscriptions WHERE user_id=$1 ORDER BY id`, primary)
	if err != nil {
		return subscriptionDecision{}, err
	}
	defer rows.Close()
	var list []subscriptionRow
	for rows.Next() {
		var row subscriptionRow
		if err := rows.Scan(&row.ID, &row.Status, &row.MergeState, &row.ExpiresAt); err != nil {
			return subscriptionDecision{}, err
		}
		list = append(list, row)
	}
	if err := rows.Err(); err != nil {
		return subscriptionDecision{}, err
	}
	decision := decideSubscriptions(list, now)
	for _, id := range decision.Expired {
		if _, err := tx.Exec(ctx, `UPDATE subscriptions SET merge_state='expired' WHERE id=$1 AND merge_state NOT IN ('superseded','selected')`, id); err != nil {
			return subscriptionDecision{}, err
		}
	}
	for _, id := range decision.Superseded {
		if _, err := tx.Exec(ctx, `UPDATE subscriptions SET merge_state='superseded' WHERE id=$1`, id); err != nil {
			return subscriptionDecision{}, err
		}
	}
	for _, id := range decision.PromptIDs {
		if _, err := tx.Exec(ctx, `UPDATE subscriptions SET merge_state='prompt_required' WHERE id=$1`, id); err != nil {
			return subscriptionDecision{}, err
		}
	}
	if decision.WinnerID > 0 {
		if _, err := tx.Exec(ctx, `UPDATE subscriptions SET merge_state='selected' WHERE id=$1`, decision.WinnerID); err != nil {
			return subscriptionDecision{}, err
		}
	}
	return decision, nil
}

func decideSubscriptions(rows []subscriptionRow, now time.Time) subscriptionDecision {
	var open []subscriptionRow
	var expired []int64
	for _, row := range rows {
		if subscriptionCandidate(row, now) {
			open = append(open, row)
			continue
		}
		expired = append(expired, row.ID)
	}
	if len(open) == 0 {
		return subscriptionDecision{Resolution: "none", Expired: expired}
	}
	maxAt := open[0].ExpiresAt
	for _, row := range open[1:] {
		if row.ExpiresAt.After(maxAt) {
			maxAt = row.ExpiresAt
		}
	}
	var tied []subscriptionRow
	var shorter []int64
	for _, row := range open {
		if row.ExpiresAt.Equal(maxAt) {
			tied = append(tied, row)
		} else {
			shorter = append(shorter, row.ID)
		}
	}
	if len(tied) > 1 {
		ids := make([]int64, len(tied))
		for i, row := range tied {
			ids[i] = row.ID
		}
		return subscriptionDecision{Resolution: "prompt_required", Prompt: true, PromptIDs: ids, Superseded: shorter, Expired: expired}
	}
	return subscriptionDecision{Resolution: "selected", WinnerID: tied[0].ID, Superseded: shorter, Expired: expired}
}

func subscriptionCandidate(row subscriptionRow, now time.Time) bool {
	if !row.ExpiresAt.After(now) {
		return false
	}
	switch row.MergeState {
	case "superseded", "expired":
		return false
	}
	switch row.Status {
	case "active", "prompt_required", "":
		return true
	default:
		return false
	}
}

func foldDaily(ctx context.Context, tx Tx, primary, secondary int64, now time.Time) (dailyContrib, error) {
	current := cycleOf(now)
	cycles := map[string]struct{}{current: {}}
	for _, q := range []string{
		`SELECT cycle FROM quota_use WHERE user_id=$1`,
		`SELECT cycle FROM daily_slot WHERE user_id=$1`,
		`SELECT cycle FROM daily_free_balance WHERE user_id=$1`,
	} {
		for _, id := range []int64{primary, secondary} {
			rows, err := tx.Query(ctx, q, id)
			if err != nil {
				return dailyContrib{}, err
			}
			for rows.Next() {
				var cycle string
				if err := rows.Scan(&cycle); err != nil {
					rows.Close()
					return dailyContrib{}, err
				}
				cycles[cycle] = struct{}{}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return dailyContrib{}, err
			}
			rows.Close()
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO daily_free_uses(user_id, cycle, reading_id, source_user_id, kind)
		SELECT $1, cycle, reading_id, user_id, 'quota_use' FROM quota_use WHERE user_id IN ($2,$3)
		ON CONFLICT DO NOTHING`, primary, primary, secondary); err != nil {
		return dailyContrib{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO daily_free_uses(user_id, cycle, reading_id, source_user_id, kind)
		SELECT $1, cycle, reading_id, user_id, 'daily_slot' FROM daily_slot WHERE user_id IN ($2,$3)
		ON CONFLICT DO NOTHING`, primary, primary, secondary); err != nil {
		return dailyContrib{}, err
	}
	var currentOut dailyContrib
	for cycle := range cycles {
		left, err := loadDailyContrib(ctx, tx, primary, cycle, current)
		if err != nil {
			return dailyContrib{}, err
		}
		right, err := loadDailyContrib(ctx, tx, secondary, cycle, current)
		if err != nil {
			return dailyContrib{}, err
		}
		sum := dailyContrib{
			used: left.used + right.used, remaining: left.remaining + right.remaining,
			slotUsed: left.slotUsed + right.slotUsed, slotRemaining: left.slotRemaining + right.slotRemaining,
		}
		if _, err := tx.Exec(ctx, `INSERT INTO daily_free_balance(user_id, cycle, used_flags, remaining, daily_slot_used, daily_slot_remaining)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (user_id, cycle) DO UPDATE SET used_flags=EXCLUDED.used_flags, remaining=EXCLUDED.remaining, daily_slot_used=EXCLUDED.daily_slot_used, daily_slot_remaining=EXCLUDED.daily_slot_remaining`,
			primary, cycle, sum.used, sum.remaining, sum.slotUsed, sum.slotRemaining); err != nil {
			return dailyContrib{}, err
		}
		if cycle == current {
			currentOut = sum
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM daily_free_balance WHERE user_id=$1`, secondary); err != nil {
		return dailyContrib{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM quota_use WHERE user_id IN ($1,$2)`, primary, secondary); err != nil {
		return dailyContrib{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM daily_slot WHERE user_id IN ($1,$2)`, primary, secondary); err != nil {
		return dailyContrib{}, err
	}
	return currentOut, nil
}

func loadDailyContrib(ctx context.Context, tx Tx, uid int64, cycle, current string) (dailyContrib, error) {
	var c dailyContrib
	err := tx.QueryRow(ctx, `SELECT used_flags, remaining, daily_slot_used, daily_slot_remaining FROM daily_free_balance WHERE user_id=$1 AND cycle=$2`, uid, cycle).
		Scan(&c.used, &c.remaining, &c.slotUsed, &c.slotRemaining)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return c, err
	}
	var quotaN, slotN int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM quota_use WHERE user_id=$1 AND cycle=$2`, uid, cycle).Scan(&quotaN); err != nil {
		return c, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM daily_slot WHERE user_id=$1 AND cycle=$2`, uid, cycle).Scan(&slotN); err != nil {
		return c, err
	}
	if quotaN > 0 || slotN > 0 {
		c.used = 1
	}
	if slotN > 0 {
		c.slotUsed = 1
	}
	if cycle == current {
		if c.used == 0 {
			c.remaining = 1
		}
		if c.slotUsed == 0 {
			c.slotRemaining = 1
		}
	}
	return c, nil
}

func moveIdentities(ctx context.Context, tx Tx, requestID, primary, secondary int64, prim, sec userRow, passwordHash string, subPrompt bool, now time.Time) ([]string, error) {
	var setEmail, setPass, setWechat, setApple, setPhone *string
	var retained []string
	assign := func(kind string, dst **string, cur, incoming *string) error {
		if incoming == nil || *incoming == "" {
			return nil
		}
		if cur == nil || *cur == "" {
			v := *incoming
			*dst = &v
			return nil
		}
		same := *cur == *incoming
		if kind == "email" {
			same = strings.EqualFold(*cur, *incoming)
		}
		if same {
			return nil
		}
		retained = append(retained, kind+":"+*incoming)
		_, err := tx.Exec(ctx, `INSERT INTO retained_identities(merge_request_id, user_id, source_user_id, kind, value, reason)
			VALUES ($1,$2,$3,$4,$5,'primary_slot_occupied')`, requestID, primary, secondary, kind, *incoming)
		return err
	}
	if err := assign("email", &setEmail, prim.email, sec.email); err != nil {
		return nil, err
	}
	if err := assign("wechat", &setWechat, prim.wechat, sec.wechat); err != nil {
		return nil, err
	}
	if err := assign("apple", &setApple, prim.apple, sec.apple); err != nil {
		return nil, err
	}
	if err := assign("phone", &setPhone, prim.phone, sec.phone); err != nil {
		return nil, err
	}
	if setEmail != nil {
		if passwordHash != "" {
			setPass = &passwordHash
		} else if sec.pass != nil && *sec.pass != "" {
			v := *sec.pass
			setPass = &v
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE app_users SET email=NULL, password_hash=NULL, wechat_openid=NULL, apple_sub=NULL, phone=NULL, merged_into=$1, merged_at=$2 WHERE id=$3`,
		primary, now, secondary); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE app_users SET
		email=COALESCE($1, email),
		password_hash=COALESCE($2, password_hash),
		wechat_openid=COALESCE($3, wechat_openid),
		apple_sub=COALESCE($4, apple_sub),
		phone=COALESCE($5, phone),
		subscription_prompt_required=$6,
		identity_prompt_required=$7
		WHERE id=$8`,
		setEmail, setPass, setWechat, setApple, setPhone, subPrompt, len(retained) > 0, primary); err != nil {
		return nil, err
	}
	return retained, nil
}

func (a *App) adminListAccountMerges(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, primary_user_id, secondary_user_id, identity_kind, status, subscription_resolution, confirm_at, result, created_at
		FROM account_merge_requests ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, primary, secondary int64
		var kind, status, resolution, result string
		var confirmAt *time.Time
		var created time.Time
		if err := rows.Scan(&id, &primary, &secondary, &kind, &status, &resolution, &confirmAt, &result, &created); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "primary_user_id": primary, "secondary_user_id": secondary,
			"identity_kind": kind, "status": status, "subscription_resolution": resolution,
			"confirm_at": confirmAt, "result": result, "created_at": created,
		})
	}
	writeOK(w, list)
	return nil
}
