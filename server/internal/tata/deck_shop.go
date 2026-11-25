package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const deckYearUnlockCode = "*"

type deckProduct struct {
	Code         string
	Kind         string
	Name         string
	Price        int
	Currency     string
	DeckCodes    string
	CoversFuture bool
	DeductOwned  bool
	Note         string
	Enabled      bool
	Sort         int
}

func validDeckProductKind(kind string) bool {
	switch kind {
	case "deck_year", "deck_bundle", "deck_single", "deck_addon":
		return true
	default:
		return false
	}
}

func (p deckProduct) codes() []string {
	var codes []string
	if json.Unmarshal([]byte(p.DeckCodes), &codes) != nil {
		return []string{}
	}
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code != "" {
			out = append(out, code)
		}
	}
	return out
}

func (p deckProduct) public() map[string]any {
	return map[string]any{
		"code": p.Code, "kind": p.Kind, "name": p.Name,
		"price_amount_minor": p.Price, "currency": p.Currency,
		"deck_codes": p.codes(), "covers_future": p.CoversFuture,
		"deduct_owned": p.DeductOwned, "note": p.Note, "enabled": p.Enabled,
		"ledger": "deck_price",
	}
}

func (s *Store) listDeckProducts(ctx context.Context, onlyEnabled bool) ([]deckProduct, error) {
	q := `SELECT code, kind, display_name, COALESCE(price_amount_minor, 0), COALESCE(price_currency, 'CNY'), deck_codes, covers_future, deduct_owned, note, enabled, sort_order
		FROM iap_skus
		WHERE kind IN ('deck_year','deck_bundle','deck_single','deck_addon')`
	if onlyEnabled {
		q += ` AND enabled=true`
	}
	q += ` ORDER BY sort_order, code`
	rows, err := s.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []deckProduct{}
	for rows.Next() {
		var p deckProduct
		if err := rows.Scan(&p.Code, &p.Kind, &p.Name, &p.Price, &p.Currency, &p.DeckCodes, &p.CoversFuture, &p.DeductOwned, &p.Note, &p.Enabled, &p.Sort); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (a *App) loadDeckProduct(ctx context.Context, code string) (deckProduct, error) {
	list, err := a.Store.listDeckProducts(ctx, false)
	if err != nil {
		return deckProduct{}, err
	}
	for _, p := range list {
		if p.Code == code {
			return p, nil
		}
	}
	return deckProduct{}, biz("牌组商品不存在")
}

type deckUnlockState struct {
	codes map[string]bool
	year  bool
}

func (a *App) loadDeckUnlocks(ctx context.Context, uid int64) (deckUnlockState, error) {
	state := deckUnlockState{codes: map[string]bool{}}
	if uid <= 0 {
		return state, nil
	}
	rows, err := a.Store.Query(ctx, `SELECT deck_code, ends_at FROM user_deck_unlocks WHERE user_id=$1`, uid)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	now := time.Now()
	for rows.Next() {
		var code string
		var ends sql.NullTime
		if err := rows.Scan(&code, &ends); err != nil {
			return state, err
		}
		if ends.Valid && !ends.Time.After(now) {
			continue
		}
		if code == deckYearUnlockCode {
			state.year = true
			continue
		}
		state.codes[code] = true
	}
	return state, rows.Err()
}

func (st deckUnlockState) allows(code string, required bool) bool {
	if !required {
		return true
	}
	return st.year || st.codes[code]
}

func (a *App) appDeckShop(w http.ResponseWriter, r *http.Request) error {
	products, err := a.Store.listDeckProducts(r.Context(), true)
	if err != nil {
		return err
	}
	decks, err := a.Store.listDecks(r.Context(), true)
	if err != nil {
		return err
	}
	uid := a.optionalAppUser(r)
	unlocks, err := a.loadDeckUnlocks(r.Context(), uid)
	if err != nil {
		return err
	}
	items := make([]map[string]any, 0, len(products))
	for _, p := range products {
		items = append(items, p.public())
	}
	shopDecks := []map[string]any{}
	var defaultDeck map[string]any
	for _, d := range decks {
		if d.Code == "rider-waite" && !d.UnlockRequired {
			defaultDeck = map[string]any{"code": d.Code, "name": T(r.Context(), d.Name), "priced": false}
		}
		if !d.UnlockRequired {
			continue
		}
		shopDecks = append(shopDecks, map[string]any{
			"code": d.Code, "name": T(r.Context(), d.Name), "tags": parseDeckTags(d.Tags),
			"unlock_required": true, "unlocked": unlocks.allows(d.Code, true),
		})
	}
	writeOK(w, map[string]any{
		"ledger": "deck_price", "currency": "CNY",
		"products": items, "decks": shopDecks, "default_deck": defaultDeck,
	})
	return nil
}

func (a *App) optionalAppUser(r *http.Request) int64 {
	uid, err := a.appActor(r)
	if err != nil {
		return 0
	}
	return uid
}

func (a *App) adminListDeckProducts(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	list, err := a.Store.listDeckProducts(r.Context(), false)
	if err != nil {
		return err
	}
	items := make([]map[string]any, 0, len(list))
	for _, p := range list {
		item := p.public()
		item["sort_order"] = p.Sort
		items = append(items, item)
	}
	writeOK(w, items)
	return nil
}

func (a *App) adminUpdateDeckProduct(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	code := strings.TrimSpace(r.PathValue("code"))
	cur, err := a.loadDeckProduct(r.Context(), code)
	if err != nil {
		return err
	}
	var req struct {
		Name    *string `json:"name"`
		Price   *int    `json:"price_amount_minor"`
		Enabled *bool   `json:"enabled"`
		Note    *string `json:"note"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	name := cur.Name
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		name = strings.TrimSpace(*req.Name)
	}
	price := cur.Price
	if req.Price != nil {
		if *req.Price < 0 {
			return biz("价格不能为负")
		}
		price = *req.Price
	}
	enabled := cur.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	note := cur.Note
	if req.Note != nil {
		note = strings.TrimSpace(*req.Note)
	}
	if _, err := a.Store.Exec(r.Context(), `UPDATE iap_skus SET display_name=$1, price_amount_minor=$2, enabled=$3, note=$4, updated_at=now() WHERE code=$5`,
		name, price, enabled, note, code); err != nil {
		return err
	}
	a.Store.audit(r.Context(), act.id, "update", "deck_product", code, name)
	writeOK(w, true)
	return nil
}

func (a *App) adminGrantDeckUnlock(w http.ResponseWriter, r *http.Request) error {
	act, err := a.adminActor(r, true)
	if err != nil {
		return err
	}
	var req struct {
		UserID      int64  `json:"user_id"`
		ProductCode string `json:"product_code"`
		DeckCode    string `json:"deck_code"`
	}
	if err := decode(r, &req); err != nil {
		return err
	}
	if req.UserID <= 0 {
		return biz("请填写用户")
	}
	var exists int
	if err := a.Store.QueryRow(r.Context(), `SELECT count(*) FROM app_users WHERE id=$1 AND merged_into IS NULL AND cancelled_at IS NULL`, req.UserID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return biz("用户不存在")
	}
	product, err := a.loadDeckProduct(r.Context(), strings.TrimSpace(req.ProductCode))
	if err != nil {
		return err
	}
	if !validDeckProductKind(product.Kind) {
		return biz("牌组商品类型不正确")
	}
	if product.Kind == "deck_addon" {
		code := strings.TrimSpace(req.DeckCode)
		if code == "" {
			return biz("请指定要解锁的牌组")
		}
		if _, ok, err := a.enabledDeck(r.Context(), code); err != nil {
			return err
		} else if !ok {
			return biz("牌组不存在")
		}
	}
	rows, msg, err := applyDeckUnlock(r.Context(), a.Store, req.UserID, product, req.DeckCode)
	if err != nil {
		return err
	}
	if msg != "" {
		return biz(msg)
	}
	a.Store.audit(r.Context(), act.id, "grant", "deck_unlock", product.Code, fmtID(req.UserID))
	writeOK(w, map[string]any{"user_id": req.UserID, "product_code": product.Code, "deck_codes": rows, "ledger": "deck_price"})
	return nil
}

type sqlExec interface {
	Exec(ctx context.Context, query string, args ...any) (tagResult, error)
}

func applyDeckUnlock(ctx context.Context, exec sqlExec, uid int64, product deckProduct, addonCode string) ([]string, string, error) {
	ends := any(nil)
	rows := []string{}
	switch product.Kind {
	case "deck_year":
		rows = []string{deckYearUnlockCode}
		ends = time.Now().AddDate(1, 0, 0)
	case "deck_addon":
		code := strings.TrimSpace(addonCode)
		if code == "" {
			return nil, "请指定要解锁的牌组", nil
		}
		rows = []string{code}
	default:
		rows = product.codes()
		if len(rows) == 0 {
			return nil, "这个商品没有包含牌组", nil
		}
	}
	for _, code := range rows {
		if _, err := exec.Exec(ctx, `INSERT INTO user_deck_unlocks(user_id, deck_code, source, product_code, ends_at)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (user_id, deck_code) DO UPDATE SET source=EXCLUDED.source, product_code=EXCLUDED.product_code, ends_at=EXCLUDED.ends_at`,
			uid, code, product.Kind, product.Code, ends); err != nil {
			return nil, "", err
		}
	}
	return rows, "", nil
}
