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

// Admin grants add member quota only. They do not update iap_orders
// and do not call resumeQuotaAfterCredit.
//
// Permission: pay 模块可写才能补发；role=admin 隐含全部模块。
// 无写权限可查补发记录。流水与审计分属 records / accounts 模块。
//
// quota 写入 quota_balance，并记入 quota_lots。quota_bucket 只能是
// subscription（套餐赠送）或 pack（补充包）。订阅有效期不改。
// 免费次数不能从这里补。兑换码仍按码上的配置入账。
// Reason on every grant row is admin_grant.

const (
	grantKindQuota  = "quota"
	grantKindFree   = "free_count"
	grantReason     = "admin_grant"
	grantBucketPack = "pack"
	grantBucketSub  = "subscription"
	grantMaxAmount  = 100000
	grantNoteMax    = 500
)

type grantRequest struct {
	UserID      int64  `json:"user_id"`
	Quota       int    `json:"quota"`
	QuotaBucket string `json:"quota_bucket"`
	FreeCount   int    `json:"free_count"`
	Reason      string `json:"reason"`
	Note        string `json:"note"`
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
		quota_ledger_id, count_ledger_id, created_at
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
	out := grantMeta(a.operatorCanWriteModule(r.Context(), act, adminModulePay))
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
	if req.FreeCount != 0 {
		return biz("人工补发只补会员额度")
	}
	if req.Quota <= 0 || req.Quota > grantMaxAmount {
		return biz("请填写会员额度")
	}
	bucket := strings.TrimSpace(req.QuotaBucket)
	if bucket != grantBucketPack && bucket != grantBucketSub {
		return biz("请选择套餐赠送或补充包")
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
	item, err := applyQuotaGrant(r.Context(), tx, act.id, req.UserID, req.Quota, bucket, note, now)
	if err != nil {
		return err
	}
	grants := []map[string]any{item}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	out := grantMeta(a.operatorCanWriteModule(r.Context(), act, adminModulePay))
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

func grantMeta(canWrite bool) map[string]any {
	return map[string]any{
		"reason":                       grantReason,
		"quota_bucket_default":         grantBucketPack,
		"quota_buckets":                []string{grantBucketSub, grantBucketPack},
		"quota_bucket_column":          "quota_balance",
		"quota_ledger_bucket_for_pack": iapBucketQuota,
		"writes_iap_orders":            false,
		"resumes_paywall":              false,
		"can_write":                    canWrite,
		"write_role":                   "admin",
		"permission_note":              "付费模块可写才能补发。只补会员额度，写入套餐赠送或补充包。",
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

func applyQuotaGrant(ctx context.Context, tx Tx, operatorID, uid int64, amount int, bucket, note string, now time.Time) (map[string]any, error) {
	ledgerBucket := iapBucketQuota
	after, err := creditQuotaSource(ctx, tx, uid, amount, nil, "", grantReason, bucket)
	if err != nil {
		return nil, err
	}
	var grantID int64
	if err := tx.QueryRow(ctx, `INSERT INTO admin_grants(user_id, operator_id, kind, amount, bucket, ledger_bucket, reason, note, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		uid, operatorID, grantKindQuota, amount, bucket, ledgerBucket, grantReason, note, now).Scan(&grantID); err != nil {
		return nil, err
	}
	var ledgerID int64
	err = tx.QueryRow(ctx, `SELECT id FROM quota_ledger WHERE user_id=$1 AND grant_id IS NULL AND reason=$2 ORDER BY id DESC LIMIT 1`, uid, grantReason).Scan(&ledgerID)
	if err != nil {
		// creditQuota does not set grant_id; update the newest quota row
		_ = tx.QueryRow(ctx, `SELECT id FROM quota_ledger WHERE user_id=$1 AND bucket=$2 ORDER BY id DESC LIMIT 1`, uid, iapBucketQuota).Scan(&ledgerID)
	}
	if ledgerID > 0 {
		if _, err := tx.Exec(ctx, `UPDATE quota_ledger SET grant_id=$1 WHERE id=$2`, grantID, ledgerID); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_grants SET quota_ledger_id=$1 WHERE id=$2`, ledgerID, grantID); err != nil {
		return nil, err
	}
	if err := writeGrantAudit(ctx, tx, operatorID, grantID, map[string]any{
		"grant_id": grantID, "user_id": uid, "operator_id": operatorID,
		"kind": grantKindQuota, "amount": amount, "bucket": bucket, "ledger_bucket": ledgerBucket,
		"reason": grantReason, "note": note, "quota_ledger_id": ledgerID, "balance_after": after,
		"created_at": now, "iap_order_rewritten": false,
	}); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": grantID, "user_id": uid, "operator_id": operatorID,
		"kind": grantKindQuota, "amount": amount, "bucket": bucket, "ledger_bucket": ledgerBucket,
		"reason": grantReason, "note": note, "quota_ledger_id": ledgerID, "balance_after": after,
		"created_at": now,
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
	quota, err := loadQuotaBalance(ctx, q, uid)
	if err != nil {
		return nil, err
	}
	var free, invite int
	err = q.QueryRow(ctx, `SELECT admin_grant_remaining, invite_remaining FROM rewards WHERE user_id=$1`, uid).Scan(&free, &invite)
	if errors.Is(err, sql.ErrNoRows) {
		free, invite = 0, 0
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"quota_remaining":      quota,
		"balance":              quota,
		"free_count_remaining": free,
		"invite_remaining":     invite,
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
			"quota_ledger_id": nullInt(diamondID), "count_ledger_id": nullInt(countID),
			"created_at": created,
		})
	}
	return list, rows.Err()
}
