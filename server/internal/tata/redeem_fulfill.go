package tata

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	redeemPlatform     = "redeem"
	redeemKindBare     = "quota"
	redeemAddonDeckMax = 64
)

func redeemSKUKinds() map[string]struct{} {
	return map[string]struct{}{
		iapKindMonth: {}, iapKindYear: {}, iapKindQuotaPack: {}, iapKindYearlyUnlock: {},
		"deck_year": {}, "deck_bundle": {}, "deck_single": {}, "deck_addon": {},
	}
}

type redeemSKU struct {
	ID          int64
	Code        string
	Kind        string
	Name        string
	Grant       sql.NullInt64
	PeriodUnit  string
	PeriodCount sql.NullInt64
	Enabled     bool
}

func loadRedeemSKU(ctx context.Context, q rowQuery, code string) (*redeemSKU, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, nil
	}
	var sku redeemSKU
	err := q.QueryRow(ctx, `SELECT id, code, kind, display_name, quota_grant, period_unit, period_count, enabled
		FROM iap_skus WHERE code=$1`, code).Scan(
		&sku.ID, &sku.Code, &sku.Kind, &sku.Name, &sku.Grant, &sku.PeriodUnit, &sku.PeriodCount, &sku.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sku, nil
}

func validateRedeemSKU(sku *redeemSKU, addon string) error {
	if sku == nil {
		return biz("商品不存在")
	}
	if !sku.Enabled {
		return biz("商品未启用")
	}
	if _, ok := redeemSKUKinds()[sku.Kind]; !ok {
		return biz("该商品不能生成兑换码")
	}
	if sku.Kind == "deck_addon" {
		addon = strings.TrimSpace(addon)
		if addon == "" {
			return biz("牌组加点请填写 addon_deck_code")
		}
		if len([]rune(addon)) > redeemAddonDeckMax {
			return biz("牌组编码过长")
		}
	}
	if sku.Kind == iapKindQuotaPack && !sku.Grant.Valid {
		return biz("补充包未配置额度")
	}
	if sku.Grant.Valid && sku.Grant.Int64 < 0 {
		return biz("入账额度无效")
	}
	return nil
}

func redeemMembershipEnd(sku *redeemSKU, from time.Time) (time.Time, error) {
	n := 1
	if sku.PeriodCount.Valid && sku.PeriodCount.Int64 > 0 {
		n = int(sku.PeriodCount.Int64)
	}
	unit := strings.TrimSpace(sku.PeriodUnit)
	if unit == "" {
		unit = iapPeriod(sku.Kind)
	}
	switch unit {
	case "month":
		return from.AddDate(0, n, 0), nil
	case "year":
		return from.AddDate(n, 0, 0), nil
	default:
		return time.Time{}, biz("订阅周期未配置")
	}
}

// fulfillRedeemSKU applies the same entitlements as IAP for the given SKU.
// Membership uses IAP-style pending stack when the user already has an active sub.
// Yearly unlock always targets the current Beijing calendar year.
func fulfillRedeemSKU(ctx context.Context, tx Tx, uid int64, codeID int64, sku *redeemSKU, addon string) (map[string]any, *int64, error) {
	now := time.Now().UTC()
	out := map[string]any{
		"sku_code": sku.Code, "sku_kind": sku.Kind, "kind": sku.Kind,
	}
	var ledgerID *int64

	switch {
	case sku.Kind == iapKindMonth || sku.Kind == iapKindYear:
		end, err := redeemMembershipEnd(sku, now)
		if err != nil {
			return nil, nil, err
		}
		held, err := upsertRedeemMembership(ctx, tx, uid, sku, now, end)
		if err != nil {
			return nil, nil, err
		}
		out["membership_held"] = held
		out["expires_at"] = end
		if !held && sku.Grant.Valid && sku.Grant.Int64 > 0 {
			amount := int(sku.Grant.Int64)
			id, err := creditRedeemQuota(ctx, tx, uid, amount, codeID)
			if err != nil {
				return nil, nil, err
			}
			ledgerID = &id
			out["amount"] = amount
			out["unit"] = "quota"
		}
	case sku.Kind == iapKindQuotaPack:
		member, err := hasActiveMembership(ctx, tx, uid)
		if err != nil {
			return nil, nil, err
		}
		if !member {
			return nil, nil, biz("仅会员可兑换额度补充包")
		}
		amount := int(sku.Grant.Int64)
		id, err := creditRedeemQuota(ctx, tx, uid, amount, codeID)
		if err != nil {
			return nil, nil, err
		}
		ledgerID = &id
		out["amount"] = amount
		out["unit"] = "quota"
	case sku.Kind == iapKindYearlyUnlock:
		year := beijingNow().Year()
		if _, err := tx.Exec(ctx, `INSERT INTO yearly_reports(user_id, year, unlocked, unlock_source, unlocked_at)
			VALUES ($1,$2,TRUE,'purchase',now())
			ON CONFLICT (user_id, year) DO UPDATE SET unlocked=TRUE, unlock_source='purchase', unlocked_at=COALESCE(yearly_reports.unlocked_at, now())`,
			uid, year); err != nil {
			return nil, nil, err
		}
		out["year"] = year
		out["unlocked"] = true
	case isDeckKind(sku.Kind):
		product, err := loadDeckProductByCode(ctx, tx, sku.Code)
		if err != nil {
			return nil, nil, err
		}
		rows, msg, err := applyDeckUnlock(ctx, tx, uid, product, addon)
		if err != nil {
			return nil, nil, err
		}
		if msg != "" {
			return nil, nil, biz(msg)
		}
		out["deck_codes"] = rows
	default:
		return nil, nil, biz("兑换码无效")
	}
	return out, ledgerID, nil
}

func upsertRedeemMembership(ctx context.Context, tx Tx, uid int64, sku *redeemSKU, start, end time.Time) (bool, error) {
	skuKind := iapPeriod(sku.Kind)
	if skuKind == "" {
		return false, biz("订阅类型不正确")
	}
	var id int64
	var curStatus string
	var curExp time.Time
	err := tx.QueryRow(ctx, `SELECT id, status, expires_at FROM subscriptions
		WHERE user_id=$1 AND status='active' AND expires_at > now()
		  AND merge_state NOT IN ('superseded','expired')
		ORDER BY CASE sku_kind WHEN 'year' THEN 0 ELSE 1 END, expires_at DESC
		LIMIT 1 FOR UPDATE`, uid).Scan(&id, &curStatus, &curExp)
	if errors.Is(err, sql.ErrNoRows) {
		txn := "redeem:" + fmtID(uid)
		err = tx.QueryRow(ctx, `INSERT INTO subscriptions(
			user_id, sku_kind, status, starts_at, expires_at, sku_id, platform, store_original_transaction_id)
			VALUES ($1,$2,'active',$3,$4,$5,$6,$7) RETURNING id`,
			uid, skuKind, start, end, sku.ID, redeemPlatform, txn).Scan(&id)
		return false, err
	}
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	if curStatus == "active" && curExp.After(now) && end.After(curExp) {
		var pending any
		if sku.Grant.Valid && sku.Grant.Int64 > 0 {
			pending = int(sku.Grant.Int64)
		}
		_, err = tx.Exec(ctx, `UPDATE subscriptions
			SET pending_expires_at=$1, pending_quota=$2, pending_order_id=NULL, pending_sku_kind=$3
			WHERE id=$4`, end, pending, skuKind, id)
		return true, err
	}
	_, err = tx.Exec(ctx, `UPDATE subscriptions
		SET sku_kind=$1, status='active', expires_at=$2, sku_id=$3,
			pending_expires_at=NULL, pending_quota=NULL, pending_order_id=NULL, pending_sku_kind=''
		WHERE id=$4`, skuKind, end, sku.ID, id)
	return false, err
}
