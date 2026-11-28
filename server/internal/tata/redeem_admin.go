package tata

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	redeemCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	redeemCodeMaxCount = 100
	redeemBenefitMax   = 64
	redeemCodeMinLen   = 4
	redeemCodeMaxLen   = 64
)

type redeemCodeCreate struct {
	SKUCode        string  `json:"sku_code"`
	AddonDeckCode  string  `json:"addon_deck_code"`
	Kind           string  `json:"kind"`
	Amount         int     `json:"amount"`
	BenefitName    string  `json:"benefit_name"`
	Code           string  `json:"code"`
	Count          int     `json:"count"`
	MaxRedemptions *int    `json:"max_redemptions"`
	ExpiresAt      *string `json:"expires_at"`
	Enabled        *bool   `json:"enabled"`
}

type redeemCodeUpdate struct {
	Enabled        *bool   `json:"enabled"`
	BenefitName    string  `json:"benefit_name"`
	MaxRedemptions *int    `json:"max_redemptions"`
	ExpiresAt      *string `json:"expires_at"`
}

func (a *App) adminListRedeemCodes(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, false)
	if err != nil {
		return err
	}
	rows, err := a.Store.Query(r.Context(), `SELECT c.id, c.code, c.kind, c.amount, c.benefit_name, c.sku_code, c.addon_deck_code,
		c.max_redemptions, c.enabled, c.expires_at, c.created_at,
		(SELECT count(*) FROM redeem_redemptions r WHERE r.code_id=c.id) AS redeemed_count
		FROM redeem_codes c
		ORDER BY c.id DESC LIMIT 200`)
	if err != nil {
		return err
	}
	defer rows.Close()
	items, err := scanRedeemCodes(rows)
	if err != nil {
		return err
	}
	writeOK(w, redeemCodeMeta(a.operatorCanWriteModule(r.Context(), act, adminModulePay), items))
	return nil
}

func (a *App) adminCreateRedeemCodes(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req redeemCodeCreate
	if err := decode(r, &req); err != nil {
		return err
	}
	skuCode := strings.TrimSpace(req.SKUCode)
	addon := strings.TrimSpace(req.AddonDeckCode)
	var kind string
	var amount int
	benefit := strings.TrimSpace(req.BenefitName)
	if skuCode != "" {
		sku, err := loadRedeemSKU(r.Context(), a.Store, skuCode)
		if err != nil {
			return err
		}
		if err := validateRedeemSKU(sku, addon); err != nil {
			return err
		}
		kind = sku.Kind
		skuCode = sku.Code
		if kind == iapKindQuotaPack {
			amount = int(sku.Grant.Int64)
		}
		if benefit == "" {
			benefit = sku.Name
		}
		if kind != "deck_addon" {
			addon = ""
		}
	} else {
		kind = strings.TrimSpace(req.Kind)
		if kind == "" {
			kind = redeemKindBare
		}
		if kind != redeemKindBare {
			return biz("请填写 sku_code，或使用 kind=quota 生成裸额度码")
		}
		if req.Amount <= 0 || req.Amount > grantMaxAmount {
			return biz("兑换数量不正确")
		}
		amount = req.Amount
		addon = ""
		if benefit == "" {
			return biz("请填写权益名称")
		}
	}
	if utf8.RuneCountInString(benefit) > redeemBenefitMax {
		return biz("权益名称过长")
	}
	count := req.Count
	if count == 0 {
		count = 1
	}
	if count < 1 || count > redeemCodeMaxCount {
		return biz("生成数量不正确")
	}
	custom := strings.ToUpper(strings.TrimSpace(req.Code))
	if custom != "" && count != 1 {
		return biz("自定义兑换码不能批量生成")
	}
	maxRedemptions, err := parseRedeemMax(req.MaxRedemptions, false)
	if err != nil {
		return err
	}
	expires, err := parseRedeemExpires(req.ExpiresAt, true)
	if err != nil {
		return err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	codes := make([]string, 0, count)
	if custom != "" {
		if err := validateRedeemCode(custom); err != nil {
			return err
		}
		codes = append(codes, custom)
	} else {
		seen := map[string]struct{}{}
		for len(codes) < count {
			if len(seen) > count*8 {
				return biz("兑换码生成失败")
			}
			code, err := randomRedeemCode()
			if err != nil {
				return err
			}
			if _, ok := seen[code]; ok {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
	}

	tx, err := a.Store.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	now := time.Now().UTC()
	items := make([]map[string]any, 0, len(codes))
	for _, code := range codes {
		var id int64
		for attempt := 0; attempt < 8; attempt++ {
			var err error
			id, err = insertRedeemCode(r.Context(), tx, code, kind, benefit, skuCode, addon, amount, maxRedemptions, enabled, expires, now)
			if err == nil {
				break
			}
			var be *bizError
			if custom == "" && attempt < 7 && errors.As(err, &be) && be.Error() == "兑换码已存在" {
				next, genErr := randomRedeemCode()
				if genErr != nil {
					return genErr
				}
				code = next
				continue
			}
			return err
		}
		if id == 0 {
			return biz("兑换码生成失败")
		}
		items = append(items, redeemCodeView(id, code, kind, benefit, skuCode, addon, amount, maxRedemptions, enabled, expires, now, 0))
	}
	if err := tx.Commit(r.Context()); err != nil {
		return err
	}
	for _, item := range items {
		a.Store.audit(r.Context(), act.id, "create", "redeem_code", fmtID(item["id"].(int64)), item["code"].(string))
	}
	writeOK(w, redeemCodeMeta(true, items))
	return nil
}

func (a *App) adminUpdateRedeemCode(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var req redeemCodeUpdate
	if err := decode(r, &req); err != nil {
		return err
	}
	var kind, benefit, code, skuCode, addon string
	var amount int
	var max sql.NullInt64
	var enabled bool
	var expires sql.NullTime
	var created time.Time
	err = a.Store.QueryRow(r.Context(), `SELECT code, kind, amount, benefit_name, sku_code, addon_deck_code, max_redemptions, enabled, expires_at, created_at
		FROM redeem_codes WHERE id=$1`, id).Scan(&code, &kind, &amount, &benefit, &skuCode, &addon, &max, &enabled, &expires, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return biz("兑换码不存在")
	}
	if err != nil {
		return err
	}
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if name := strings.TrimSpace(req.BenefitName); name != "" {
		if utf8.RuneCountInString(name) > redeemBenefitMax {
			return biz("权益名称过长")
		}
		benefit = name
	}
	if req.MaxRedemptions != nil {
		parsed, err := parseRedeemMax(req.MaxRedemptions, true)
		if err != nil {
			return err
		}
		if parsed.Valid {
			var used int
			if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM redeem_redemptions WHERE code_id=$1`, id).Scan(&used); err != nil {
				return err
			}
			if used > int(parsed.Int64) {
				return biz("核销上限不能低于已核销次数")
			}
		}
		max = parsed
	}
	if req.ExpiresAt != nil {
		parsed, err := parseRedeemExpires(req.ExpiresAt, false)
		if err != nil {
			return err
		}
		expires = parsed
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE redeem_codes SET benefit_name=$1, max_redemptions=$2, enabled=$3, expires_at=$4 WHERE id=$5`,
		benefit, nullInt(max), enabled, nullTime(expires), id); err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "update", "redeem_code", fmtID(id), code)
	item, err := loadRedeemCode(r.Context(), a.Store, id)
	if err != nil {
		return err
	}
	writeOK(w, redeemCodeMeta(true, []map[string]any{item}))
	return nil
}

func redeemCodeMeta(canWrite bool, items []map[string]any) map[string]any {
	if items == nil {
		items = []map[string]any{}
	}
	return map[string]any{
		"can_write":       canWrite,
		"permission_note": "付费模块可写才能生成或停用兑换码；无写权限只可查看。",
		"items":           items,
	}
}

func insertRedeemCode(ctx context.Context, tx Tx, code, kind, benefit, skuCode, addon string, amount int, max sql.NullInt64, enabled bool, expires sql.NullTime, now time.Time) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `INSERT INTO redeem_codes(code, kind, amount, benefit_name, sku_code, addon_deck_code, max_redemptions, enabled, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		code, kind, amount, benefit, skuCode, addon, nullInt(max), enabled, nullTime(expires), now).Scan(&id)
	if err != nil {
		if cardUniqueErr(err) {
			return 0, biz("兑换码已存在")
		}
		return 0, err
	}
	return id, nil
}

func loadRedeemCode(ctx context.Context, store *Store, id int64) (map[string]any, error) {
	var code, kind, benefit, skuCode, addon string
	var amount, redeemed int
	var max sql.NullInt64
	var enabled bool
	var expires sql.NullTime
	var created time.Time
	err := store.QueryRow(ctx, `SELECT c.id, c.code, c.kind, c.amount, c.benefit_name, c.sku_code, c.addon_deck_code, c.max_redemptions,
		c.enabled, c.expires_at, c.created_at,
		(SELECT count(*) FROM redeem_redemptions r WHERE r.code_id=c.id)
		FROM redeem_codes c WHERE c.id=$1`, id).Scan(&id, &code, &kind, &amount, &benefit, &skuCode, &addon, &max, &enabled, &expires, &created, &redeemed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, biz("兑换码不存在")
	}
	if err != nil {
		return nil, err
	}
	return redeemCodeView(id, code, kind, benefit, skuCode, addon, amount, max, enabled, expires, created, redeemed), nil
}

func redeemCodeView(id int64, code, kind, benefit, skuCode, addon string, amount int, max sql.NullInt64, enabled bool, expires sql.NullTime, created time.Time, redeemed int) map[string]any {
	return map[string]any{
		"id": id, "code": code, "kind": kind, "amount": amount, "benefit_name": benefit,
		"sku_code": skuCode, "addon_deck_code": addon,
		"max_redemptions": nullInt(max), "enabled": enabled, "expires_at": nullTime(expires),
		"created_at": created, "redeemed_count": redeemed,
	}
}

func scanRedeemCodes(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]map[string]any, error) {
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var code, kind, benefit, skuCode, addon string
		var amount, redeemed int
		var max sql.NullInt64
		var enabled bool
		var expires sql.NullTime
		var created time.Time
		if err := rows.Scan(&id, &code, &kind, &amount, &benefit, &skuCode, &addon, &max, &enabled, &expires, &created, &redeemed); err != nil {
			return nil, err
		}
		items = append(items, redeemCodeView(id, code, kind, benefit, skuCode, addon, amount, max, enabled, expires, created, redeemed))
	}
	return items, rows.Err()
}

func parseRedeemMax(raw *int, allowZero bool) (sql.NullInt64, error) {
	if raw == nil {
		return sql.NullInt64{}, nil
	}
	if *raw == 0 {
		if allowZero {
			return sql.NullInt64{}, nil
		}
		return sql.NullInt64{}, biz("核销上限不正确")
	}
	if *raw < 0 || *raw > grantMaxAmount {
		return sql.NullInt64{}, biz("核销上限不正确")
	}
	return sql.NullInt64{Int64: int64(*raw), Valid: true}, nil
}

func parseRedeemExpires(raw *string, create bool) (sql.NullTime, error) {
	if raw == nil {
		return sql.NullTime{}, nil
	}
	text := strings.TrimSpace(*raw)
	if text == "" {
		return sql.NullTime{}, nil
	}
	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return sql.NullTime{}, biz("时间格式应为 RFC3339")
	}
	t = t.UTC()
	if create && !t.After(time.Now()) {
		return sql.NullTime{}, biz("过期时间必须晚于现在")
	}
	return sql.NullTime{Time: t, Valid: true}, nil
}

func validateRedeemCode(code string) error {
	n := utf8.RuneCountInString(code)
	if n < redeemCodeMinLen || n > redeemCodeMaxLen {
		return biz("兑换码格式不正确")
	}
	hasLetter := false
	for _, r := range code {
		if r == '-' {
			continue
		}
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			hasLetter = true
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return biz("兑换码格式不正确")
		}
		return biz("兑换码格式不正确")
	}
	if !hasLetter {
		return biz("兑换码格式不正确")
	}
	return nil
}

func randomRedeemCode() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, 9)
	for i := 0; i < 8; i++ {
		pos := i
		if i >= 4 {
			pos++
		}
		out[pos] = redeemCodeAlphabet[int(buf[i])%len(redeemCodeAlphabet)]
	}
	out[4] = '-'
	return string(out), nil
}
