package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Admin membership grants create or extend an active subscriptions row.
// They do not write iap_orders. Quota top-ups stay on POST /grants.
//
// Permission matches grants: pay 模块可写；role=admin 隐含全部模块。
// 月卡延长 30 天，年卡延长 365 天。已有未过期会员时从当前到期日往后加，
// 不把年卡降成月卡。没有有效会员时新开一条 platform=admin 的订阅。

const (
	membershipKindMonth = "month"
	membershipKindYear  = "year"
	membershipDaysMonth = 30
	membershipDaysYear  = 365
	membershipPlatform  = "admin"
)

type membershipRequest struct {
	UserID  int64  `json:"user_id"`
	SKUKind string `json:"sku_kind"`
	Note    string `json:"note"`
}

func (a *App) adminListMemberships(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, false)
	if err != nil {
		return err
	}
	uid, err := adminUserFilter(r)
	if err != nil {
		return err
	}
	items, err := listMembershipRows(r.Context(), a.Store.DB, uid)
	if err != nil {
		return err
	}
	writeOK(w, membershipMeta(a.operatorCanWriteModule(r.Context(), act, adminModulePay), uid, items))
	return nil
}

func (a *App) adminGrantMembership(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	if !a.operatorCanWriteModule(r.Context(), act, adminModulePay) {
		return biz("只读账号不能修改")
	}
	var req membershipRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	note := strings.TrimSpace(req.Note)
	if note == "" {
		return biz("请填写开通原因")
	}
	if len([]rune(note)) > grantNoteMax {
		return biz("开通原因过长")
	}
	kind := strings.TrimSpace(req.SKUKind)
	if kind == "" {
		kind = membershipKindMonth
	}
	days, err := membershipDays(kind)
	if err != nil {
		return err
	}
	if req.UserID <= 0 {
		return biz("请指定用户")
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
	row, extended, err := upsertAdminMembership(r.Context(), tx, req.UserID, kind, days, now)
	if err != nil {
		return err
	}
	if err := writeMembershipAudit(r.Context(), tx, act.id, row, note, extended, days); err != nil {
		return err
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	items, err := listMembershipRows(r.Context(), a.Store.DB, req.UserID)
	if err != nil {
		return err
	}
	out := membershipMeta(true, req.UserID, items)
	out["operator_id"] = act.id
	out["extended"] = extended
	out["duration_days"] = days
	out["membership"] = row
	out["note"] = note
	writeOK(w, out)
	return nil
}

func membershipDays(kind string) (int, error) {
	switch kind {
	case membershipKindMonth:
		return membershipDaysMonth, nil
	case membershipKindYear:
		return membershipDaysYear, nil
	default:
		return 0, biz("请选择月卡或年卡")
	}
}

func membershipMeta(canWrite bool, uid int64, items []map[string]any) map[string]any {
	out := map[string]any{
		"items":             items,
		"can_write":         canWrite,
		"writes_iap_orders": false,
		"sku_kind_default":  membershipKindMonth,
		"sku_kinds":         []string{membershipKindMonth, membershipKindYear},
		"month_days":        membershipDaysMonth,
		"year_days":         membershipDaysYear,
		"permission_note":   "付费模块可写才能开通。月卡 30 天，年卡 365 天。已有会员从到期日延长，不写商店订单。",
	}
	if uid > 0 {
		out["user_id"] = uid
	}
	return out
}

func upsertAdminMembership(ctx context.Context, tx Tx, uid int64, kind string, days int, now time.Time) (map[string]any, bool, error) {
	var id int64
	var currentKind string
	var starts, expires time.Time
	err := tx.QueryRow(ctx, `SELECT id, sku_kind, starts_at, expires_at FROM subscriptions
		WHERE user_id=$1 AND status='active' AND expires_at > $2
		  AND merge_state NOT IN ('superseded','expired')
		ORDER BY CASE sku_kind WHEN 'year' THEN 0 ELSE 1 END, expires_at DESC
		LIMIT 1 FOR UPDATE`, uid, now).Scan(&id, &currentKind, &starts, &expires)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		expires = now.AddDate(0, 0, days)
		err = tx.QueryRow(ctx, `INSERT INTO subscriptions(
			user_id, sku_kind, status, starts_at, expires_at, platform, merge_state)
			VALUES ($1,$2,'active',$3,$4,$5,'') RETURNING id`,
			uid, kind, now, expires, membershipPlatform).Scan(&id)
		if err != nil {
			return nil, false, err
		}
		return membershipView(id, uid, kind, "active", membershipPlatform, now, expires, true), false, nil
	}
	nextKind := currentKind
	if kind == membershipKindYear {
		nextKind = membershipKindYear
	}
	base := expires
	if base.Before(now) {
		base = now
	}
	nextExp := base.AddDate(0, 0, days)
	if _, err := tx.Exec(ctx, `UPDATE subscriptions SET sku_kind=$1, expires_at=$2 WHERE id=$3`, nextKind, nextExp, id); err != nil {
		return nil, false, err
	}
	var platform string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(platform,'') FROM subscriptions WHERE id=$1`, id).Scan(&platform); err != nil {
		return nil, false, err
	}
	return membershipView(id, uid, nextKind, "active", platform, starts, nextExp, true), true, nil
}

func listMembershipRows(ctx context.Context, db *sql.DB, uid int64) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, user_id, sku_kind, status, COALESCE(platform,''), starts_at, expires_at, merge_state
		FROM subscriptions
		WHERE ($1 = 0 OR user_id = $1)
		ORDER BY id DESC LIMIT 200`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().UTC()
	list := []map[string]any{}
	for rows.Next() {
		var id, userID int64
		var kind, status, platform, mergeState string
		var starts, expires time.Time
		if err := rows.Scan(&id, &userID, &kind, &status, &platform, &starts, &expires, &mergeState); err != nil {
			return nil, err
		}
		active := status == "active" && expires.After(now) && mergeState != "superseded" && mergeState != "expired"
		list = append(list, membershipView(id, userID, kind, status, platform, starts, expires, active))
	}
	return list, rows.Err()
}

func membershipView(id, uid int64, kind, status, platform string, starts, expires time.Time, active bool) map[string]any {
	return map[string]any{
		"id": id, "user_id": uid, "sku_kind": kind, "status": status,
		"platform": platform, "starts_at": starts, "expires_at": expires, "active": active,
	}
}

func writeMembershipAudit(ctx context.Context, tx Tx, operatorID int64, row map[string]any, note string, extended bool, days int) error {
	detail := map[string]any{
		"user_id": row["user_id"], "operator_id": operatorID,
		"subscription_id": row["id"], "sku_kind": row["sku_kind"],
		"expires_at": row["expires_at"], "extended": extended,
		"duration_days": days, "note": note, "writes_iap_orders": false,
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_log(operator_id, action, object_type, object_id, detail, actor_kind)
		VALUES ($1,'admin_membership','subscription',$2,$3,'operator')`, operatorID, fmtID(row["id"].(int64)), string(raw))
	return err
}
