package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Admin grants add diamonds and/or free counts. They do not update iap_orders
// and do not call resumeQuotaAfterCredit.
//
// Permission: operators.role = admin may write (adminActor write=true).
// role = readonly may list grants, audit_log, and ledgers. DEC-015's finer
// matrix is still open; this is the existing read/write role switch.
//
// Diamond buckets: omitted diamond_bucket means pack, which adds pack_diamonds
// and writes diamond_ledger.bucket = consumable. subscription adds
// subscription_diamonds and writes bucket = subscription. Both are additive.
// Subscription period fields are not replaced (unlike an IAP period refresh).
//
// Free counts add rewards.admin_grant_remaining. They do not change
// rewards.remaining or its expiry. Spend order is after today's free count
// and before diamonds. Reason on every grant row is admin_grant.

const (
	grantKindDiamonds = "diamonds"
	grantKindFree     = "free_count"
	grantReason       = "admin_grant"
	grantBucketPack   = "pack"
	grantBucketSub    = "subscription"
	grantFreeResult   = "granted"
	grantFreeEnt      = "free_count"
	grantMaxAmount    = 100000
	grantNoteMax      = 500
)

type grantRequest struct {
	UserID        int64  `json:"user_id"`
	Diamonds      int    `json:"diamonds"`
	DiamondBucket string `json:"diamond_bucket"`
	FreeCount     int    `json:"free_count"`
	Reason        string `json:"reason"`
	Note          string `json:"note"`
}

func (a *App) adminListGrants(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, false)
	if err != nil {
		return err
	}
	userID, err := optionalUserID(r)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, user_id, operator_id, kind, amount, bucket, ledger_bucket, reason, note,
		diamond_ledger_id, count_ledger_id, created_at
		FROM admin_grants
		WHERE ($1 = 0 OR user_id = $1)
		ORDER BY id DESC LIMIT 200`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	list, err := scanGrants(rows)
	if err != nil {
		return err
	}
	out := grantMeta(act.role)
	out["grants"] = list
	if userID > 0 {
		bal, err := grantBalances(r.Context(), a.Store, userID)
		if err != nil {
			return err
		}
		for k, v := range bal {
			out[k] = v
		}
		out["user_id"] = userID
	}
	writeOK(w, out)
	return nil
}

func (a *App) adminCreateGrant(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req grantRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	note := strings.TrimSpace(req.Note)
	if note == "" {
		return biz("请填写补发原因")
	}
	if len([]rune(note)) > grantNoteMax {
		return biz("补发原因过长")
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = grantReason
	}
	if reason != grantReason {
		return biz("补发原因类型只允许 admin_grant")
	}
	if req.UserID <= 0 {
		return biz("请指定用户")
	}
	if req.Diamonds < 0 || req.FreeCount < 0 {
		return biz("补发数量不正确")
	}
	if req.Diamonds == 0 && req.FreeCount == 0 {
		return biz("请填写钻石或免费次数")
	}
	if req.Diamonds > grantMaxAmount || req.FreeCount > grantMaxAmount {
		return biz("补发数量超出单次上限")
	}
	bucket := strings.TrimSpace(req.DiamondBucket)
	if bucket == "" {
		bucket = grantBucketPack
	}
	if bucket != grantBucketPack && bucket != grantBucketSub {
		return biz("钻石桶只允许 pack 或 subscription，默认 pack（pack_diamonds）")
	}

	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if err := lockGrantUser(r.Context(), tx, req.UserID); err != nil {
		return err
	}
	now := time.Now().UTC()
	grants := []map[string]any{}
	if req.Diamonds > 0 {
		item, err := applyDiamondGrant(r.Context(), tx, act.id, req.UserID, req.Diamonds, bucket, note, now)
		if err != nil {
			return err
		}
		grants = append(grants, item)
	}
	if req.FreeCount > 0 {
		item, err := applyFreeGrant(r.Context(), tx, act.id, req.UserID, req.FreeCount, note, now)
		if err != nil {
			return err
		}
		grants = append(grants, item)
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	out := grantMeta(act.role)
	out["user_id"] = req.UserID
	out["operator_id"] = act.id
	out["grants"] = grants
	out["resumed_tasks"] = []any{}
	bal, err := grantBalances(r.Context(), a.Store, req.UserID)
	if err != nil {
		return err
	}
	for k, v := range bal {
		out[k] = v
	}
	writeOK(w, out)
	return nil
}

func grantMeta(role string) map[string]any {
	return map[string]any{
		"reason":                         grantReason,
		"diamond_bucket_default":         grantBucketPack,
		"diamond_bucket_column":          "pack_diamonds",
		"diamond_ledger_bucket_for_pack": "consumable",
		"writes_iap_orders":              false,
		"resumes_paywall":                false,
		"can_write":                      role == "admin",
		"write_role":                     "admin",
		"permission_note":                "operators.role=admin 可写补发；readonly 默认可查流水与补发记录。写接口使用 adminActor(write=true)。",
	}
}

func optionalUserID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, biz("用户 id 不正确")
	}
	return id, nil
}

func lockGrantUser(ctx context.Context, tx Tx, uid int64) error {
	var cancelled *time.Time
	var merged *int64
	err := tx.QueryRow(ctx, `SELECT cancelled_at, merged_into FROM app_users WHERE id=$1 FOR UPDATE`, uid).Scan(&cancelled, &merged)
	if errors.Is(err, sql.ErrNoRows) {
		return biz("用户不存在")
	}
	if err != nil {
		return err
	}
	if cancelled != nil {
		return biz("账号已注销")
	}
	if merged != nil {
		return biz("账号已合并，请补发到主账号")
	}
	return nil
}

func applyDiamondGrant(ctx context.Context, tx Tx, operatorID, uid int64, amount int, bucket, note string, now time.Time) (map[string]any, error) {
	ledgerBucket := iapBucketPack
	if bucket == grantBucketSub {
		ledgerBucket = iapBucketSub
	}
	if _, err := tx.Exec(ctx, `INSERT INTO diamond_balances(user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return nil, err
	}
	var subBal, packBal int
	var periodEnd sql.NullTime
	var periodKey string
	if err := tx.QueryRow(ctx, `SELECT subscription_diamonds, pack_diamonds, subscription_period_end, subscription_period_key
		FROM diamond_balances WHERE user_id=$1 FOR UPDATE`, uid).Scan(&subBal, &packBal, &periodEnd, &periodKey); err != nil {
		return nil, err
	}
	after := packBal + amount
	if bucket == grantBucketSub {
		after = subBal + amount
		if _, err := tx.Exec(ctx, `UPDATE diamond_balances SET subscription_diamonds=$1, updated_at=now() WHERE user_id=$2`, after, uid); err != nil {
			return nil, err
		}
	} else if _, err := tx.Exec(ctx, `UPDATE diamond_balances SET pack_diamonds=$1, updated_at=now() WHERE user_id=$2`, after, uid); err != nil {
		return nil, err
	}
	var grantID int64
	if err := tx.QueryRow(ctx, `INSERT INTO admin_grants(user_id, operator_id, kind, amount, bucket, ledger_bucket, reason, note, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		uid, operatorID, grantKindDiamonds, amount, bucket, ledgerBucket, grantReason, note, now).Scan(&grantID); err != nil {
		return nil, err
	}
	var ledgerID int64
	if err := tx.QueryRow(ctx, `INSERT INTO diamond_ledger(user_id, bucket, delta, balance_after, store_transaction_id, reason, grant_id, created_at)
		VALUES ($1,$2,$3,$4,'',$5,$6,$7) RETURNING id`,
		uid, ledgerBucket, amount, after, grantReason, grantID, now).Scan(&ledgerID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_grants SET diamond_ledger_id=$1 WHERE id=$2`, ledgerID, grantID); err != nil {
		return nil, err
	}
	if err := writeGrantAudit(ctx, tx, operatorID, grantID, map[string]any{
		"grant_id": grantID, "user_id": uid, "operator_id": operatorID,
		"kind": grantKindDiamonds, "amount": amount, "bucket": bucket, "ledger_bucket": ledgerBucket,
		"reason": grantReason, "note": note, "diamond_ledger_id": ledgerID, "balance_after": after,
		"created_at": now, "iap_order_rewritten": false,
	}); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": grantID, "user_id": uid, "operator_id": operatorID,
		"kind": grantKindDiamonds, "amount": amount, "bucket": bucket, "ledger_bucket": ledgerBucket,
		"reason": grantReason, "note": note, "diamond_ledger_id": ledgerID, "balance_after": after,
		"created_at": now,
	}, nil
}

func applyFreeGrant(ctx context.Context, tx Tx, operatorID, uid int64, amount int, note string, now time.Time) (map[string]any, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO rewards(user_id, granted_at, expires_at, remaining, admin_grant_remaining)
		VALUES ($1, now(), now(), 0, 0)
		ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return nil, err
	}
	var remaining, rewardRemaining int
	if err := tx.QueryRow(ctx, `UPDATE rewards SET admin_grant_remaining = admin_grant_remaining + $2
		WHERE user_id=$1 RETURNING admin_grant_remaining, remaining`, uid, amount).Scan(&remaining, &rewardRemaining); err != nil {
		return nil, err
	}
	var grantID int64
	if err := tx.QueryRow(ctx, `INSERT INTO admin_grants(user_id, operator_id, kind, amount, bucket, ledger_bucket, reason, note, created_at)
		VALUES ($1,$2,$3,$4,'','',$5,$6,$7) RETURNING id`,
		uid, operatorID, grantKindFree, amount, grantReason, note, now).Scan(&grantID); err != nil {
		return nil, err
	}
	var ledgerID int64
	if err := tx.QueryRow(ctx, `INSERT INTO ledger(user_id, reading_id, entitlement, amount, reason, result, grant_id, created_at)
		VALUES ($1, NULL, $2, $3, $4, $5, $6, $7) RETURNING id`,
		uid, grantFreeEnt, amount, grantReason, grantFreeResult, grantID, now).Scan(&ledgerID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_grants SET count_ledger_id=$1 WHERE id=$2`, ledgerID, grantID); err != nil {
		return nil, err
	}
	if err := writeGrantAudit(ctx, tx, operatorID, grantID, map[string]any{
		"grant_id": grantID, "user_id": uid, "operator_id": operatorID,
		"kind": grantKindFree, "amount": amount, "reason": grantReason, "note": note,
		"count_ledger_id": ledgerID, "free_count_after": remaining,
		"registration_reward_remaining": rewardRemaining,
		"created_at":                    now, "iap_order_rewritten": false,
	}); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": grantID, "user_id": uid, "operator_id": operatorID,
		"kind": grantKindFree, "amount": amount, "bucket": "", "ledger_bucket": "",
		"reason": grantReason, "note": note, "count_ledger_id": ledgerID,
		"free_count_after": remaining, "created_at": now,
	}, nil
}

func writeGrantAudit(ctx context.Context, tx Tx, operatorID, grantID int64, detail map[string]any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_log(operator_id, action, object_type, object_id, detail, actor_kind)
		VALUES ($1,'admin_grant','admin_grant',$2,$3,'operator')`, operatorID, fmtID(grantID), string(raw))
	return err
}

func grantBalances(ctx context.Context, q rowQuery, uid int64) (map[string]any, error) {
	var sub, pack int
	var periodEnd sql.NullTime
	err := q.QueryRow(ctx, `SELECT subscription_diamonds, pack_diamonds, subscription_period_end FROM diamond_balances WHERE user_id=$1`, uid).
		Scan(&sub, &pack, &periodEnd)
	if errors.Is(err, sql.ErrNoRows) {
		sub, pack = 0, 0
		err = nil
	}
	if err != nil {
		return nil, err
	}
	var free, reward int
	err = q.QueryRow(ctx, `SELECT admin_grant_remaining, remaining FROM rewards WHERE user_id=$1`, uid).Scan(&free, &reward)
	if errors.Is(err, sql.ErrNoRows) {
		free, reward = 0, 0
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"pack_diamonds":                 pack,
		"subscription_diamonds":         sub,
		"subscription_period_end":       nullTime(periodEnd),
		"free_count_remaining":          free,
		"registration_reward_remaining": reward,
	}, nil
}

func scanGrants(rows *sql.Rows) ([]map[string]any, error) {
	list := []map[string]any{}
	for rows.Next() {
		var id, userID, operatorID int64
		var diamondID, countID sql.NullInt64
		var kind, bucket, ledgerBucket, reason, note string
		var amount int
		var created time.Time
		if err := rows.Scan(&id, &userID, &operatorID, &kind, &amount, &bucket, &ledgerBucket, &reason, &note, &diamondID, &countID, &created); err != nil {
			return nil, err
		}
		list = append(list, map[string]any{
			"id": id, "user_id": userID, "operator_id": operatorID,
			"kind": kind, "amount": amount, "bucket": bucket, "ledger_bucket": ledgerBucket,
			"reason": reason, "note": note,
			"diamond_ledger_id": nullInt(diamondID), "count_ledger_id": nullInt(countID),
			"created_at": created,
		})
	}
	return list, rows.Err()
}
