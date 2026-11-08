package tata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// IAP credits diamonds (会员点数). Free 次数 stay on rewards / daily_free / ledger.
// Subscription period refresh and consumable packs use separate buckets.
// Price and diamond_grant are nullable; this file never invents amounts.

const (
	iapPlatformAppStore = "app_store"
	iapPlatformPlay     = "play"
	iapBucketSub        = "subscription"
	iapBucketPack       = "consumable"
	iapVerifyVerified   = "verified"
	iapVerifyFailed     = "failed"
	iapCreditNone       = "none"
	iapCreditCredited   = "credited"
	iapCreditUnconfig   = "amount_unconfigured"
	iapCreditExpired    = "skipped_expired"
	iapSourceVerify     = "verify"
	iapSourceRestore    = "restore"

	iapKindMonth        = "month_subscription"
	iapKindYear         = "year_subscription"
	iapKindReadingPack  = "reading_pack"
	iapKindPack         = "consumable_pack"
	iapKindYearlyUnlock = "yearly_unlock"
)

type iapVerifyRequest struct {
	Platform              string  `json:"platform"`
	ProductID             string  `json:"product_id"`
	TransactionID         string  `json:"transaction_id"`
	OriginalTransactionID string  `json:"original_transaction_id"`
	Receipt               string  `json:"receipt"`
	ExpiresAt             *string `json:"expires_at"`
	PurchasedAt           *string `json:"purchased_at"`
}

type iapRestoreRequest struct {
	Platform  string             `json:"platform"`
	Purchases []iapVerifyRequest `json:"purchases"`
}

type iapPurchase struct {
	Platform              string
	ProductID             string
	TransactionID         string
	OriginalTransactionID string
	Receipt               string
	ExpiresAt             *time.Time
	PurchasedAt           *time.Time
	Source                string
}

type iapSKU struct {
	ID           int64
	Code         string
	Kind         string
	Bucket       string
	Grant        sql.NullInt64
	ReadingGrant sql.NullInt64
	Enabled      bool
}

type rowQuery interface {
	QueryRow(ctx context.Context, query string, args ...any) *sql.Row
}

func (a *App) appIAPVerify(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能验单")
	}
	var req iapVerifyRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	purchase, err := req.purchase(iapSourceVerify)
	if err != nil {
		return err
	}
	out, err := a.verifyPurchase(r.Context(), uid, purchase)
	if err != nil {
		return err
	}
	if out["credit_status"] == iapCreditCredited {
		if tasks := a.resumeQuotaAfterCredit(r.Context(), uid); len(tasks) > 0 {
			out["resumed_tasks"] = tasks
			if view, viewErr := diamondView(r.Context(), a.Store, uid); viewErr == nil {
				out["subscription_diamonds"] = view["subscription_diamonds"]
				out["pack_diamonds"] = view["pack_diamonds"]
			}
		}
	}
	writeOK(w, out)
	return nil
}

func (a *App) appIAPRestore(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能恢复购买")
	}
	var req iapRestoreRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	hook, required, err := loadIAPSettings(r.Context(), a.Store)
	if err != nil {
		return err
	}
	if !hook {
		return biz("恢复购买钩子已关闭")
	}
	if len(req.Purchases) == 0 {
		return biz("没有可恢复的交易")
	}
	if len(req.Purchases) > 20 {
		return biz("一次恢复的交易过多")
	}
	results := make([]any, 0, len(req.Purchases))
	for _, item := range req.Purchases {
		if strings.TrimSpace(item.Platform) == "" {
			item.Platform = req.Platform
		}
		purchase, err := item.purchase(iapSourceRestore)
		if err != nil {
			results = append(results, map[string]any{
				"transaction_id": item.TransactionID,
				"verify_status":  iapVerifyFailed,
				"credit_status":  iapCreditNone,
				"message":        err.Error(),
			})
			continue
		}
		out, err := a.verifyPurchase(r.Context(), uid, purchase)
		if err != nil {
			msg := "内部错误"
			var be *bizError
			if errors.As(err, &be) {
				msg = be.msg
			}
			results = append(results, map[string]any{
				"transaction_id": purchase.TransactionID,
				"verify_status":  iapVerifyFailed,
				"credit_status":  iapCreditNone,
				"message":        msg,
			})
			continue
		}
		results = append(results, out)
	}
	body := map[string]any{
		"restore_hook_enabled":        hook,
		"restore_required_first_pack": required,
		"results":                     results,
	}
	credited := false
	for _, item := range results {
		row, ok := item.(map[string]any)
		if ok && row["credit_status"] == iapCreditCredited {
			credited = true
			break
		}
	}
	if credited {
		if tasks := a.resumeQuotaAfterCredit(r.Context(), uid); len(tasks) > 0 {
			body["resumed_tasks"] = tasks
		}
	}
	writeOK(w, body)
	return nil
}

func (a *App) appIAPEntitlements(w http.ResponseWriter, r *http.Request) error {
	uid, _, err := a.appActor(r)
	if err != nil {
		return err
	}
	if uid == 0 {
		return biz("登录后才能查看钻石余额")
	}
	out, err := diamondView(r.Context(), a.Store, uid)
	if err != nil {
		return err
	}
	writeOK(w, out)
	return nil
}

func (a *App) appIAPSKUs(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	kind, err := parseIAPKindQuery(r)
	if err != nil {
		return err
	}
	list, err := a.listIAPSKUs(r.Context(), true, kind)
	if err != nil {
		return err
	}
	writeOK(w, list)
	return nil
}

func (a *App) appIAPConfig(w http.ResponseWriter, r *http.Request) error {
	if _, _, err := a.appActor(r); err != nil {
		return err
	}
	out, err := iapConfigView(r.Context(), a.Store)
	if err != nil {
		return err
	}
	writeOK(w, out)
	return nil
}

func (a *App) adminListIAPOrders(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	userID, verify, err := iapOrderFilter(r)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT o.id, o.user_id, o.sku_code, o.sku_kind, o.ledger_bucket,
		o.platform, o.product_id, o.store_transaction_id, o.original_transaction_id,
		o.verify_status, o.credit_status, o.credited_diamonds, o.credited_readings,
		o.period_start, o.period_end, o.source, o.verify_message, o.created_at, o.verified_at,
		s.status, s.expires_at, s.sku_kind
		FROM iap_orders o
		LEFT JOIN subscriptions s ON s.id = o.subscription_id
		WHERE ($1 = 0 OR o.user_id = $1) AND ($2 = '' OR o.verify_status = $2)
		ORDER BY o.id DESC LIMIT 200`, userID, verify)
	if err != nil {
		return err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, uid int64
		var skuCode, skuKind, bucket, platform, productID, txn, original string
		var verifyStatus, creditStatus, source, message string
		var credited, readings sql.NullInt64
		var periodStart, periodEnd, verifiedAt sql.NullTime
		var created time.Time
		var subStatus, subKind sql.NullString
		var subExpires sql.NullTime
		if err := rows.Scan(&id, &uid, &skuCode, &skuKind, &bucket, &platform, &productID, &txn, &original,
			&verifyStatus, &creditStatus, &credited, &readings, &periodStart, &periodEnd, &source, &message, &created, &verifiedAt,
			&subStatus, &subExpires, &subKind); err != nil {
			return err
		}
		list = append(list, map[string]any{
			"id": id, "user_id": uid, "sku_code": skuCode, "sku_kind": skuKind, "ledger_bucket": bucket,
			"platform": platform, "product_id": productID, "store_transaction_id": txn, "original_transaction_id": original,
			"verify_status": verifyStatus, "credit_status": creditStatus, "credited_diamonds": nullInt(credited),
			"credited_readings": nullInt(readings),
			"period_start":      nullTime(periodStart), "period_end": nullTime(periodEnd),
			"source": source, "verify_message": message, "created_at": created, "verified_at": nullTime(verifiedAt),
			"subscription_status": nullString(subStatus), "subscription_expires_at": nullTime(subExpires),
			"subscription_sku_kind": nullString(subKind),
			"unit":                  "diamond",
		})
	}
	writeOK(w, list)
	return rows.Err()
}

func (a *App) adminListIAPSKUs(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	kind, err := parseIAPKindQuery(r)
	if err != nil {
		return err
	}
	list, err := a.listIAPSKUs(r.Context(), false, kind)
	if err != nil {
		return err
	}
	writeOK(w, list)
	return nil
}

type iapSKUBody struct {
	Code              string `json:"code"`
	Kind              string `json:"kind"`
	DisplayName       string `json:"display_name"`
	AppStoreProductID string `json:"app_store_product_id"`
	PlayProductID     string `json:"play_product_id"`
	PriceAmountMinor  *int64 `json:"price_amount_minor"`
	PriceCurrency     string `json:"price_currency"`
	DiamondGrant      *int   `json:"diamond_grant"`
	ReadingGrant      *int   `json:"reading_grant"`
	PeriodCount       *int   `json:"period_count"`
	Copy              string `json:"copy"`
	Enabled           *bool  `json:"enabled"`
}

func (a *App) adminCreateIAPSKU(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req iapSKUBody
	if err := decode(r, &req); err != nil {
		return err
	}
	row, err := validateIAPSKUBody(req, true)
	if err != nil {
		return err
	}
	var id int64
	err = a.Store.QueryRow(r.Context(), `INSERT INTO iap_skus(
		code, kind, ledger_bucket, display_name, app_store_product_id, play_product_id,
		price_amount_minor, price_currency, diamond_grant, reading_grant, period_unit, period_count, copy, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		row.code, row.kind, row.bucket, row.name, row.appID, row.playID,
		row.price, row.currency, row.grant, row.readingGrant, row.periodUnit, row.periodCount, row.copy, row.enabled).Scan(&id)
	if err != nil {
		return mapIAPSKUWriteErr(err)
	}
	a.Store.audit(r.Context(), act.id, "create", "iap_sku", fmtID(id), row.code)
	item, err := a.loadIAPSKU(r.Context(), id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

func (a *App) adminUpdateIAPSKU(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var req iapSKUBody
	if err := decode(r, &req); err != nil {
		return err
	}
	var existingKind string
	err = a.Store.QueryRow(r.Context(), `SELECT kind FROM iap_skus WHERE id=$1`, id).Scan(&existingKind)
	if errors.Is(err, sql.ErrNoRows) {
		return biz("商品不存在")
	}
	if err != nil {
		return err
	}
	if req.Kind != "" && req.Kind != existingKind {
		return biz("商品类型不能修改")
	}
	req.Kind = existingKind
	row, err := validateIAPSKUBody(req, false)
	if err != nil {
		return err
	}
	tag, err := a.Store.Exec(r.Context(), `UPDATE iap_skus SET
		display_name=$1, app_store_product_id=$2, play_product_id=$3,
		price_amount_minor=$4, price_currency=$5, diamond_grant=$6, reading_grant=$7,
		period_count=$8, copy=$9, enabled=$10, updated_at=now()
		WHERE id=$11`,
		row.name, row.appID, row.playID, row.price, row.currency, row.grant, row.readingGrant, row.periodCount, row.copy, row.enabled, id)
	if err != nil {
		return mapIAPSKUWriteErr(err)
	}
	if tag.RowsAffected() == 0 {
		return biz("商品不存在")
	}
	a.Store.audit(r.Context(), act.id, "update", "iap_sku", fmtID(id), row.name)
	item, err := a.loadIAPSKU(r.Context(), id)
	if err != nil {
		return err
	}
	writeOK(w, item)
	return nil
}

type iapSKUWrite struct {
	code, kind, bucket, name, periodUnit, copy                       string
	appID, playID, currency, price, grant, readingGrant, periodCount any
	enabled                                                          bool
}

func validateIAPSKUBody(req iapSKUBody, creating bool) (iapSKUWrite, error) {
	kind := strings.TrimSpace(req.Kind)
	if creating && !validIAPKind(kind) {
		return iapSKUWrite{}, biz("商品类型不正确")
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" || runeLen(name) > 64 {
		return iapSKUWrite{}, biz("商品名称不正确")
	}
	copyText := strings.TrimSpace(req.Copy)
	if runeLen(copyText) > 2000 {
		return iapSKUWrite{}, biz("商品文案过长")
	}
	out := iapSKUWrite{kind: kind, name: name, copy: copyText, enabled: req.Enabled == nil || *req.Enabled}
	if creating {
		code := strings.TrimSpace(req.Code)
		if !validIAPCode(code) {
			return iapSKUWrite{}, biz("商品编码不正确")
		}
		out.code = code
		out.bucket, out.periodUnit = iapKindMeta(kind)
	}
	if (kind == iapKindYearlyUnlock || kind == iapKindReadingPack) && req.DiamondGrant != nil {
		if kind == iapKindReadingPack {
			return iapSKUWrite{}, biz("次卡不能发放钻石")
		}
		return iapSKUWrite{}, biz("年运解锁不能发放钻石")
	}
	if kind != iapKindReadingPack && req.ReadingGrant != nil {
		return iapSKUWrite{}, biz("只有次卡可以发放解读次数")
	}
	out.appID = emptyAsNil(strings.TrimSpace(req.AppStoreProductID))
	out.playID = emptyAsNil(strings.TrimSpace(req.PlayProductID))
	if req.PriceAmountMinor != nil {
		if *req.PriceAmountMinor <= 0 {
			return iapSKUWrite{}, biz("价格须为正整数（分）")
		}
		currency := strings.TrimSpace(req.PriceCurrency)
		if currency == "" {
			currency = "CNY"
		}
		if currency != "CNY" && currency != "USD" {
			return iapSKUWrite{}, biz("货币只支持 CNY 或 USD")
		}
		out.price = *req.PriceAmountMinor
		out.currency = currency
	}
	if req.DiamondGrant != nil {
		if *req.DiamondGrant <= 0 {
			return iapSKUWrite{}, biz("钻石发放须为正整数")
		}
		out.grant = *req.DiamondGrant
	}
	if req.ReadingGrant != nil {
		if *req.ReadingGrant <= 0 {
			return iapSKUWrite{}, biz("解读次数须为正整数")
		}
		out.readingGrant = *req.ReadingGrant
	}
	if req.PeriodCount != nil {
		if *req.PeriodCount <= 0 {
			return iapSKUWrite{}, biz("周期数量须为正整数")
		}
		out.periodCount = *req.PeriodCount
	}
	return out, nil
}

func validIAPKind(kind string) bool {
	switch kind {
	case iapKindMonth, iapKindYear, iapKindReadingPack, iapKindPack, iapKindYearlyUnlock:
		return true
	}
	return false
}

func validIAPCode(code string) bool {
	if len(code) < 2 || len(code) > 64 {
		return false
	}
	for _, r := range code {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func iapKindMeta(kind string) (bucket, period string) {
	switch kind {
	case iapKindMonth:
		return iapBucketSub, "month"
	case iapKindYear:
		return iapBucketSub, "year"
	default:
		return iapBucketPack, ""
	}
}

func parseIAPKindQuery(r *http.Request) (string, error) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	if kind == "" {
		return "", nil
	}
	if !validIAPKind(kind) {
		return "", biz("商品类型不正确")
	}
	return kind, nil
}

func mapIAPSKUWriteErr(err error) error {
	msg := err.Error()
	if strings.Contains(msg, "iap_skus_code") || strings.Contains(msg, "iap_skus_pkey") {
		return biz("商品编码已存在")
	}
	if strings.Contains(msg, "iap_skus_app_store_product_id") {
		return biz("App Store 商品号已占用")
	}
	if strings.Contains(msg, "iap_skus_play_product_id") {
		return biz("Play 商品号已占用")
	}
	return err
}

func (a *App) loadIAPSKU(ctx context.Context, id int64) (map[string]any, error) {
	list, err := a.listIAPSKUs(ctx, false, "")
	if err != nil {
		return nil, err
	}
	for _, item := range list {
		if item["id"] == id {
			return item, nil
		}
	}
	return nil, biz("商品不存在")
}

func (a *App) adminIAPDiamonds(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	raw := strings.TrimSpace(r.URL.Query().Get("user_id"))
	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || userID <= 0 {
		return biz("用户 id 不正确")
	}
	view, err := diamondView(r.Context(), a.Store, userID)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, bucket, delta, balance_after, order_id, store_transaction_id, reason, created_at
		FROM diamond_ledger WHERE user_id=$1 ORDER BY id DESC LIMIT 50`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	ledger := []map[string]any{}
	for rows.Next() {
		var rowID int64
		var orderID sql.NullInt64
		var bucket, txn, reason string
		var delta, after int
		var created time.Time
		if err := rows.Scan(&rowID, &bucket, &delta, &after, &orderID, &txn, &reason, &created); err != nil {
			return err
		}
		ledger = append(ledger, map[string]any{
			"id": rowID, "bucket": bucket, "delta": delta, "balance_after": after,
			"order_id": nullInt(orderID), "store_transaction_id": txn, "reason": reason, "created_at": created,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	view["user_id"] = userID
	view["ledger"] = ledger
	writeOK(w, view)
	return nil
}

func (a *App) adminIAPConfig(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	out, err := iapConfigView(r.Context(), a.Store)
	if err != nil {
		return err
	}
	writeOK(w, out)
	return nil
}

func (req iapVerifyRequest) purchase(source string) (iapPurchase, error) {
	expires, err := parseRFC3339Ptr(req.ExpiresAt)
	if err != nil {
		return iapPurchase{}, err
	}
	purchased, err := parseRFC3339Ptr(req.PurchasedAt)
	if err != nil {
		return iapPurchase{}, err
	}
	return iapPurchase{
		Platform:              req.Platform,
		ProductID:             req.ProductID,
		TransactionID:         req.TransactionID,
		OriginalTransactionID: req.OriginalTransactionID,
		Receipt:               req.Receipt,
		ExpiresAt:             expires,
		PurchasedAt:           purchased,
		Source:                source,
	}, nil
}

func (a *App) verifyPurchase(ctx context.Context, uid int64, in iapPurchase) (map[string]any, error) {
	in.Platform = strings.TrimSpace(in.Platform)
	in.ProductID = strings.TrimSpace(in.ProductID)
	in.TransactionID = strings.TrimSpace(in.TransactionID)
	in.OriginalTransactionID = strings.TrimSpace(in.OriginalTransactionID)
	in.Receipt = strings.TrimSpace(in.Receipt)
	if in.Source != iapSourceVerify && in.Source != iapSourceRestore {
		in.Source = iapSourceVerify
	}
	if in.Platform != iapPlatformAppStore && in.Platform != iapPlatformPlay {
		return nil, biz("平台不正确")
	}
	if in.TransactionID == "" || len(in.TransactionID) > 128 {
		return nil, biz("交易号不正确")
	}
	if in.ProductID == "" || len(in.ProductID) > 128 {
		return nil, biz("商品不正确")
	}
	if in.OriginalTransactionID == "" {
		in.OriginalTransactionID = in.TransactionID
	}
	if len(in.OriginalTransactionID) > 128 {
		return nil, biz("原始交易号不正确")
	}
	if in.Receipt == "" {
		return nil, biz("票据为空")
	}
	if len(in.Receipt) > 8192 {
		return nil, biz("票据过长")
	}
	var cancelled *time.Time
	var merged *int64
	err := a.Store.QueryRow(ctx, `SELECT cancelled_at, merged_into FROM app_users WHERE id=$1`, uid).Scan(&cancelled, &merged)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, biz("用户不存在")
	}
	if err != nil {
		return nil, err
	}
	if cancelled != nil {
		return nil, biz("账号已注销")
	}
	if merged != nil {
		return nil, biz("账号已合并到其他账号")
	}

	unlock, err := a.lockIAP(in.Platform, in.TransactionID)
	if err != nil {
		return nil, err
	}
	defer unlock()

	sum := sha256.Sum256([]byte(in.Receipt))
	receiptHash := hex.EncodeToString(sum[:])
	tx, err := a.Store.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var orderID int64
	err = tx.QueryRow(ctx, `INSERT INTO iap_orders(
		user_id, platform, product_id, store_transaction_id, original_transaction_id,
		verify_status, credit_status, source, receipt_sha256)
		VALUES ($1,$2,$3,$4,$5,'pending','none',$6,$7)
		ON CONFLICT (platform, store_transaction_id) DO NOTHING
		RETURNING id`,
		uid, in.Platform, in.ProductID, in.TransactionID, in.OriginalTransactionID, in.Source, receiptHash).Scan(&orderID)
	if errors.Is(err, sql.ErrNoRows) {
		var owner int64
		var status string
		err = tx.QueryRow(ctx, `SELECT id, user_id, verify_status FROM iap_orders
			WHERE platform=$1 AND store_transaction_id=$2 FOR UPDATE`, in.Platform, in.TransactionID).Scan(&orderID, &owner, &status)
		if err != nil {
			return nil, err
		}
		if owner != uid {
			return nil, biz("交易号已使用")
		}
		if status == iapVerifyVerified {
			snap, err := iapOrderSnapshot(ctx, tx, orderID, true)
			if err != nil {
				return nil, err
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return snap, nil
		}
	} else if err != nil {
		return nil, err
	}

	sku, err := findSKU(ctx, tx, in.Platform, in.ProductID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if sku == nil || !sku.Enabled {
		msg := "商品未映射"
		var attached *iapSKU
		if sku != nil && !sku.Enabled {
			msg = "商品未启用"
			attached = sku
		}
		if err := finishIAPOrder(ctx, tx, orderID, iapOrderWrite{VerifyStatus: iapVerifyFailed, CreditStatus: iapCreditNone, Message: msg, SKU: attached}); err != nil {
			return nil, err
		}
		return commitIAPSnapshot(ctx, tx, uid, orderID, in, false)
	}
	if sku.Grant.Valid && sku.Grant.Int64 < 0 {
		if err := finishIAPOrder(ctx, tx, orderID, iapOrderWrite{
			VerifyStatus: iapVerifyFailed, CreditStatus: iapCreditNone, Message: "钻石发放数量无效", SKU: sku,
		}); err != nil {
			return nil, err
		}
		return commitIAPSnapshot(ctx, tx, uid, orderID, in, false)
	}
	if sku.ReadingGrant.Valid && sku.ReadingGrant.Int64 < 0 {
		if err := finishIAPOrder(ctx, tx, orderID, iapOrderWrite{
			VerifyStatus: iapVerifyFailed, CreditStatus: iapCreditNone, Message: "解读次数无效", SKU: sku,
		}); err != nil {
			return nil, err
		}
		return commitIAPSnapshot(ctx, tx, uid, orderID, in, false)
	}

	var periodEnd *time.Time
	if sku.Bucket == iapBucketSub {
		if in.ExpiresAt == nil {
			if err := finishIAPOrder(ctx, tx, orderID, iapOrderWrite{
				VerifyStatus: iapVerifyFailed, CreditStatus: iapCreditNone, Message: "订阅有效期缺失", SKU: sku,
			}); err != nil {
				return nil, err
			}
			return commitIAPSnapshot(ctx, tx, uid, orderID, in, false)
		}
		end := in.ExpiresAt.UTC()
		periodEnd = &end
	}
	periodStart := now
	if in.PurchasedAt != nil {
		periodStart = in.PurchasedAt.UTC()
	}

	write := iapOrderWrite{
		VerifyStatus: iapVerifyVerified,
		CreditStatus: iapCreditCredited,
		SKU:          sku,
		PeriodStart:  &periodStart,
		PeriodEnd:    periodEnd,
	}
	if sku.Bucket == iapBucketSub {
		status := "active"
		if !periodEnd.After(now) {
			status = "expired"
			write.CreditStatus = iapCreditExpired
		}
		subID, err := upsertIAPSubscription(ctx, tx, uid, sku, in, periodStart, *periodEnd, status, orderID)
		if err != nil {
			return nil, err
		}
		write.SubscriptionID = &subID
	}
	if sku.Kind == iapKindYearlyUnlock {
		if write.CreditStatus == iapCreditCredited {
			year := beijingNow().Year()
			if _, err := tx.Exec(ctx, `INSERT INTO yearly_reports(user_id, year, unlocked, unlock_source, unlocked_at)
				VALUES ($1,$2,TRUE,'purchase',now())
				ON CONFLICT (user_id, year) DO UPDATE SET unlocked=TRUE, unlock_source='purchase', unlocked_at=COALESCE(yearly_reports.unlocked_at, now())`,
				uid, year); err != nil {
				return nil, err
			}
		}
	} else if sku.Kind == iapKindReadingPack {
		if write.CreditStatus == iapCreditCredited {
			if !sku.ReadingGrant.Valid {
				write.CreditStatus = iapCreditUnconfig
			} else {
				grant := int(sku.ReadingGrant.Int64)
				write.ReadingCredited = &grant
				if err := creditRedeemFree(ctx, tx, uid, grant); err != nil {
					return nil, err
				}
			}
		}
	} else if write.CreditStatus == iapCreditCredited && !sku.Grant.Valid {
		write.CreditStatus = iapCreditUnconfig
	} else if write.CreditStatus == iapCreditCredited {
		grant := int(sku.Grant.Int64)
		write.Credited = &grant
		reason := diamondReason(in.Source, sku.Bucket)
		if err := applyDiamonds(ctx, tx, uid, sku.Bucket, grant, periodEnd, in.Platform+":"+in.TransactionID, in.TransactionID, orderID, reason); err != nil {
			return nil, err
		}
	}
	if err := finishIAPOrder(ctx, tx, orderID, write); err != nil {
		return nil, err
	}
	return commitIAPSnapshot(ctx, tx, uid, orderID, in, false)
}

func commitIAPSnapshot(ctx context.Context, tx Tx, uid, orderID int64, in iapPurchase, idempotent bool) (map[string]any, error) {
	snap, err := iapOrderSnapshot(ctx, tx, orderID, idempotent)
	if err != nil {
		return nil, err
	}
	detail, err := json.Marshal(map[string]any{
		"order_id": orderID, "verify_status": snap["verify_status"], "credit_status": snap["credit_status"],
		"ledger_bucket": snap["ledger_bucket"], "transaction_id": in.TransactionID, "source": in.Source,
	})
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_log(operator_id, action, object_type, object_id, detail, actor_kind)
		VALUES ($1,$2,'iap_order',$3,$4,'app_user')`, uid, "iap_"+in.Source, fmtID(orderID), string(detail)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return snap, nil
}

type iapOrderWrite struct {
	VerifyStatus    string
	CreditStatus    string
	Message         string
	SKU             *iapSKU
	Credited        *int
	ReadingCredited *int
	PeriodStart     *time.Time
	PeriodEnd       *time.Time
	SubscriptionID  *int64
}

func finishIAPOrder(ctx context.Context, tx Tx, orderID int64, w iapOrderWrite) error {
	var skuID any
	code, kind, bucket := "", "", ""
	if w.SKU != nil {
		skuID = w.SKU.ID
		code, kind, bucket = w.SKU.Code, w.SKU.Kind, w.SKU.Bucket
	}
	var credited any
	if w.Credited != nil {
		credited = *w.Credited
	}
	var readings any
	if w.ReadingCredited != nil {
		readings = *w.ReadingCredited
	}
	var verifiedAt any
	if w.VerifyStatus == iapVerifyVerified {
		verifiedAt = time.Now().UTC()
	}
	var subID any
	if w.SubscriptionID != nil {
		subID = *w.SubscriptionID
	}
	_, err := tx.Exec(ctx, `UPDATE iap_orders SET
		sku_id=$1, sku_code=$2, sku_kind=$3, ledger_bucket=$4,
		verify_status=$5, credit_status=$6, credited_diamonds=$7, credited_readings=$8,
		period_start=$9, period_end=$10, subscription_id=$11,
		verify_message=$12, verified_at=$13, updated_at=now()
		WHERE id=$14`,
		skuID, code, kind, bucket, w.VerifyStatus, w.CreditStatus, credited, readings,
		w.PeriodStart, w.PeriodEnd, subID, w.Message, verifiedAt, orderID)
	return err
}

func upsertIAPSubscription(ctx context.Context, tx Tx, uid int64, sku *iapSKU, in iapPurchase, start, end time.Time, status string, orderID int64) (int64, error) {
	skuKind := "month"
	if sku.Kind == "year_subscription" {
		skuKind = "year"
	}
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM subscriptions
		WHERE user_id=$1 AND platform=$2 AND store_original_transaction_id=$3
		ORDER BY id LIMIT 1 FOR UPDATE`, uid, in.Platform, in.OriginalTransactionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO subscriptions(
			user_id, sku_kind, status, starts_at, expires_at, sku_id, platform, store_original_transaction_id, last_order_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
			uid, skuKind, status, start, end, sku.ID, in.Platform, in.OriginalTransactionID, orderID).Scan(&id)
		return id, err
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `UPDATE subscriptions
		SET sku_kind=$1, status=$2, expires_at=$3, sku_id=$4, last_order_id=$5
		WHERE id=$6`, skuKind, status, end, sku.ID, orderID, id)
	return id, err
}

func applyDiamonds(ctx context.Context, tx Tx, uid int64, bucket string, grant int, periodEnd *time.Time, periodKey, txn string, orderID int64, reason string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO diamond_balances(user_id) VALUES ($1) ON CONFLICT (user_id) DO NOTHING`, uid); err != nil {
		return err
	}
	var subBal, packBal int
	if err := tx.QueryRow(ctx, `SELECT subscription_diamonds, pack_diamonds
		FROM diamond_balances WHERE user_id=$1 FOR UPDATE`, uid).Scan(&subBal, &packBal); err != nil {
		return err
	}
	var delta, after int
	switch bucket {
	case iapBucketSub:
		delta = grant - subBal
		after = grant
		var end any
		if periodEnd != nil {
			end = *periodEnd
		}
		if _, err := tx.Exec(ctx, `UPDATE diamond_balances
			SET subscription_diamonds=$1, subscription_period_end=$2, subscription_period_key=$3, updated_at=now()
			WHERE user_id=$4`, after, end, periodKey, uid); err != nil {
			return err
		}
	case iapBucketPack:
		delta = grant
		after = packBal + grant
		if _, err := tx.Exec(ctx, `UPDATE diamond_balances SET pack_diamonds=$1, updated_at=now() WHERE user_id=$2`, after, uid); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown diamond bucket %s", bucket)
	}
	_, err := tx.Exec(ctx, `INSERT INTO diamond_ledger(user_id, bucket, delta, balance_after, order_id, store_transaction_id, reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, uid, bucket, delta, after, orderID, txn, reason)
	return err
}

func diamondReason(source, bucket string) string {
	if bucket == iapBucketPack {
		if source == iapSourceRestore {
			return "iap_restore_pack_grant"
		}
		return "iap_pack_grant"
	}
	if source == iapSourceRestore {
		return "iap_restore_period_refresh"
	}
	return "iap_period_refresh"
}

func findSKU(ctx context.Context, q rowQuery, platform, productID string) (*iapSKU, error) {
	query := `SELECT id, code, kind, ledger_bucket, diamond_grant, reading_grant, enabled FROM iap_skus WHERE app_store_product_id=$1`
	if platform == iapPlatformPlay {
		query = `SELECT id, code, kind, ledger_bucket, diamond_grant, reading_grant, enabled FROM iap_skus WHERE play_product_id=$1`
	}
	var sku iapSKU
	err := q.QueryRow(ctx, query, productID).Scan(&sku.ID, &sku.Code, &sku.Kind, &sku.Bucket, &sku.Grant, &sku.ReadingGrant, &sku.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sku, nil
}

func iapOrderSnapshot(ctx context.Context, q rowQuery, orderID int64, idempotent bool) (map[string]any, error) {
	var uid int64
	var skuCode, skuKind, bucket, platform, productID, txn, original string
	var verifyStatus, creditStatus, source, message string
	var credited, readings sql.NullInt64
	var periodStart, periodEnd, verifiedAt sql.NullTime
	var created time.Time
	var subStatus, subKind sql.NullString
	var subExpires sql.NullTime
	var auto sql.NullBool
	var subDiamonds, packDiamonds int
	err := q.QueryRow(ctx, `SELECT o.user_id, o.sku_code, o.sku_kind, o.ledger_bucket,
		o.platform, o.product_id, o.store_transaction_id, o.original_transaction_id,
		o.verify_status, o.credit_status, o.credited_diamonds, o.credited_readings,
		o.period_start, o.period_end, o.source, o.verify_message, o.created_at, o.verified_at,
		s.status, s.expires_at, s.sku_kind, s.auto_renew,
		COALESCE(d.subscription_diamonds, 0), COALESCE(d.pack_diamonds, 0)
		FROM iap_orders o
		LEFT JOIN subscriptions s ON s.id = o.subscription_id
		LEFT JOIN diamond_balances d ON d.user_id = o.user_id
		WHERE o.id=$1`, orderID).Scan(&uid, &skuCode, &skuKind, &bucket, &platform, &productID, &txn, &original,
		&verifyStatus, &creditStatus, &credited, &readings, &periodStart, &periodEnd, &source, &message, &created, &verifiedAt,
		&subStatus, &subExpires, &subKind, &auto, &subDiamonds, &packDiamonds)
	if err != nil {
		return nil, err
	}
	var subscription any
	if subStatus.Valid {
		subscription = map[string]any{
			"status":     subStatus.String,
			"sku_kind":   nullString(subKind),
			"expires_at": nullTime(subExpires),
			"auto_renew": nullBool(auto),
		}
	}
	return map[string]any{
		"order_id": orderID, "user_id": uid, "idempotent": idempotent, "unit": "diamond",
		"sku_code": skuCode, "sku_kind": skuKind, "ledger_bucket": bucket,
		"platform": platform, "product_id": productID,
		"store_transaction_id": txn, "original_transaction_id": original,
		"verify_status": verifyStatus, "credit_status": creditStatus,
		"credited_diamonds": nullInt(credited),
		"credited_readings": nullInt(readings),
		"period_start":      nullTime(periodStart), "period_end": nullTime(periodEnd),
		"source": source, "verify_message": message,
		"subscription_diamonds": subDiamonds, "pack_diamonds": packDiamonds,
		"subscription": subscription,
	}, nil
}

func diamondView(ctx context.Context, q rowQuery, uid int64) (map[string]any, error) {
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
	var kind, status string
	var exp time.Time
	var auto sql.NullBool
	err = q.QueryRow(ctx, `SELECT sku_kind, status, expires_at, auto_renew FROM subscriptions
		WHERE user_id=$1 AND status='active' AND expires_at > now() AND merge_state NOT IN ('superseded','expired')
		ORDER BY expires_at DESC LIMIT 1`, uid).Scan(&kind, &status, &exp, &auto)
	var subscription any
	if err == nil {
		subscription = map[string]any{
			"sku_kind": kind, "status": status, "expires_at": exp, "auto_renew": nullBool(auto),
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return map[string]any{
		"unit":                    "diamond",
		"display_label":           T(ctx, displayLabelTataCoin),
		"balance":                 sub + pack,
		"subscription_diamonds":   sub,
		"pack_diamonds":           pack,
		"subscription_period_end": nullTime(periodEnd),
		"subscription":            subscription,
	}, nil
}

func (a *App) listIAPSKUs(ctx context.Context, enabledOnly bool, kind string) ([]map[string]any, error) {
	query := `SELECT id, code, kind, ledger_bucket, display_name, app_store_product_id, play_product_id,
		price_amount_minor, price_currency, diamond_grant, reading_grant, period_unit, period_count, copy, enabled
		FROM iap_skus`
	where := []string{}
	args := []any{}
	if enabledOnly {
		where = append(where, "enabled = TRUE")
	}
	if kind != "" {
		args = append(args, kind)
		where = append(where, fmt.Sprintf("kind=$%d", len(args)))
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += ` ORDER BY CASE kind
		WHEN 'month_subscription' THEN 1
		WHEN 'year_subscription' THEN 2
		WHEN 'reading_pack' THEN 3
		WHEN 'yearly_unlock' THEN 4
		ELSE 5 END, COALESCE(reading_grant, diamond_grant, 0), id`
	rows, err := a.Store.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tarotistCount := 0
	_ = a.Store.QueryRow(ctx, `SELECT count(*) FROM tarotists WHERE enabled=TRUE`).Scan(&tarotistCount)
	followupLimit, followupReady, _ := loadFollowupLimit(ctx, a.Store)
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var code, kind, bucket, name, periodUnit, copyText string
		var appID, playID, currency sql.NullString
		var price, grant, readingGrant, periodCount sql.NullInt64
		var enabled bool
		if err := rows.Scan(&id, &code, &kind, &bucket, &name, &appID, &playID, &price, &currency, &grant, &readingGrant, &periodUnit, &periodCount, &copyText, &enabled); err != nil {
			return nil, err
		}
		list = append(list, map[string]any{
			"id": id, "code": code, "kind": kind, "ledger_bucket": bucket, "display_name": name,
			"app_store_product_id": nullString(appID), "play_product_id": nullString(playID),
			"price_amount_minor": nullInt(price), "price_currency": nullString(currency),
			"diamond_grant": nullInt(grant), "reading_grant": nullInt(readingGrant),
			"period_unit": periodUnit, "period_count": nullInt(periodCount),
			"copy": copyText, "enabled": enabled, "unit": "diamond", "display_label": T(ctx, displayLabelTataCoin),
			"benefits": skuBenefits(ctx, kind, tarotistCount, followupLimit, followupReady, copyText),
		})
	}
	return list, rows.Err()
}

func skuBenefits(ctx context.Context, kind string, tarotistCount, followupLimit int, followupReady bool, skuCopy string) []map[string]any {
	if kind == iapKindPack || kind == iapKindYearlyUnlock || kind == iapKindReadingPack {
		return []map[string]any{}
	}
	year := kind == "year_subscription"
	followup := map[string]any{"key": "followup", "title": T(ctx, "深入追问"), "count": nil, "included": true, "copy_status": "pending"}
	if followupReady {
		followup["count"] = followupLimit
		followup["copy_status"] = "ready"
	}
	perk := map[string]any{"key": "campaign_perk", "title": T(ctx, "专属活动权益"), "count": nil, "included": year, "body": nil, "copy_status": "pending"}
	if year {
		if body := strings.TrimSpace(skuCopy); body != "" {
			perk["body"] = body
			perk["copy_status"] = "ready"
		}
	}
	return []map[string]any{
		{"key": "daily_reading", "title": T(ctx, "每日塔罗解读"), "count": 1, "included": true},
		followup,
		{"key": "tarotists", "title": T(ctx, "塔罗师"), "count": tarotistCount, "included": true},
		{"key": "yearly_report", "title": T(ctx, "年度运势解锁"), "count": nil, "included": year},
		perk,
	}
}

func iapConfigView(ctx context.Context, q rowQuery) (map[string]any, error) {
	hook, required, err := loadIAPSettings(ctx, q)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"restore_hook_enabled":        hook,
		"restore_required_first_pack": required,
	}, nil
}

func loadIAPSettings(ctx context.Context, q rowQuery) (bool, bool, error) {
	var hook, required bool
	err := q.QueryRow(ctx, `SELECT restore_hook_enabled, restore_required_first_pack FROM iap_settings WHERE id=1`).Scan(&hook, &required)
	if errors.Is(err, sql.ErrNoRows) {
		return true, false, nil
	}
	return hook, required, err
}

func iapOrderFilter(r *http.Request) (int64, string, error) {
	userID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("user_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return 0, "", biz("用户 id 不正确")
		}
		userID = id
	}
	verify := strings.TrimSpace(r.URL.Query().Get("verify_status"))
	switch verify {
	case "", "pending", iapVerifyVerified, iapVerifyFailed:
	default:
		return 0, "", biz("验单状态不正确")
	}
	return userID, verify, nil
}

func (a *App) lockIAP(platform, txn string) (func(), error) {
	if a.Redis == nil {
		return func() {}, nil
	}
	key := "iap:" + platform + ":" + txn
	ok, err := a.Redis.Lock(key, 30*time.Second)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, biz("验单进行中")
	}
	return func() { _ = a.Redis.Unlock(key) }, nil
}

func parseRFC3339Ptr(raw *string) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw))
	if err != nil {
		return nil, biz("时间格式应为 RFC3339")
	}
	return &t, nil
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func nullTime(v sql.NullTime) any {
	if !v.Valid {
		return nil
	}
	return v.Time
}

func nullString(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

func nullBool(v sql.NullBool) any {
	if !v.Valid {
		return nil
	}
	return v.Bool
}
