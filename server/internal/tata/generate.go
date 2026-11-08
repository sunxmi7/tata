package tata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (a *App) generate(ctx context.Context, id int64, adminRetry bool) (map[string]any, error) {
	run, err := a.ensureInterpretJob(ctx, id, adminRetry)
	if err != nil {
		return nil, err
	}
	if err := run.wait(ctx); err != nil {
		return nil, err
	}
	item, err := a.loadReading(ctx, id)
	if err != nil {
		return nil, err
	}
	localizeReading(ctx, item)
	return item, nil
}

func (a *App) binding(ctx context.Context, task string) (name, model, base, key string, timeout *int, err error) {
	var vendorID int64
	err = a.Store.QueryRow(ctx, `SELECT vendor_id, model FROM bindings WHERE task=$1`, task).Scan(&vendorID, &model)
	if err != nil {
		return "", "", "", "", nil, biz("未绑定供应商")
	}
	var enc []byte
	var enabled bool
	err = a.Store.QueryRow(ctx, `SELECT name, base_url, api_key_enc, timeout_sec, enabled FROM vendors WHERE id=$1`, vendorID).Scan(&name, &base, &enc, &timeout, &enabled)
	if err != nil || !enabled {
		return "", "", "", "", nil, biz("未绑定供应商")
	}
	key, err = a.Store.decrypt(enc)
	if err != nil {
		return "", "", "", "", nil, biz("供应商密钥无法读取")
	}
	return name, model, base, key, timeout, nil
}

func buildPrompt(kind, question, cards string) string {
	var b strings.Builder
	b.WriteString("你只解读，不得改牌、牌位或正逆位。\n")
	if kind == "tarotist" {
		b.WriteString("按所选塔罗师人设表达，但牌义不能歪曲。\n")
	} else {
		b.WriteString("不要套用塔罗师人设。逐张解释后给总评。\n")
	}
	fmt.Fprintf(&b, "问题：%s\n牌面：%s\n", question, cards)
	if block := formatMajorMeaningsBlock(cards); block != "" {
		b.WriteString(block)
	}
	return b.String()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func scanReadings(rows interface {
	Next() bool
	Scan(...any) error
}) []map[string]any {
	list := []map[string]any{}
	for rows.Next() {
		item, err := scanReading(rows)
		if err != nil {
			return list
		}
		list = append(list, item)
	}
	return list
}

func scanReading(sc interface{ Scan(...any) error }) (map[string]any, error) {
	var id int64
	var userID, spreadID, tarotistID *int64
	var anon *string
	var typ, question, status string
	var cards, interpretation, vendor, model, fail *string
	var quota, first bool
	var created, updated time.Time
	var deleted *time.Time
	err := sc.Scan(&id, &userID, &anon, &typ, &question, &spreadID, &tarotistID, &status, &cards, &interpretation, &vendor, &model, &fail, &quota, &first, &created, &updated, &deleted)
	if err != nil {
		return nil, err
	}
	anonID := ""
	if anon != nil {
		anonID = *anon
	}
	var cardsJSON any
	if cards != nil && *cards != "" {
		cardsJSON = json.RawMessage(*cards)
	}
	label, next := progressOf(status)
	item := map[string]any{
		"id": id, "user_id": userID, "anonymous_id": anonID, "type": typ, "question": question,
		"spread_id": spreadID, "tarotist_id": tarotistID, "status": status, "cards": cardsJSON,
		"interpretation": interpretation, "vendor_name": vendor, "model_name": model, "fail_reason": fail,
		"quota_consumed": quota, "first_experience": first, "created_at": created, "updated_at": updated, "deleted_at": deleted,
		"progress": label, "continue": next,
	}
	attachReadingPreview(item)
	return item, nil
}

func (a *App) loadReading(ctx context.Context, id int64) (map[string]any, error) {
	return a.loadReadingAbs(ctx, nil, id)
}

func (a *App) loadReadingAbs(ctx context.Context, r *http.Request, id int64) (map[string]any, error) {
	row := a.Store.QueryRow(ctx, `SELECT id, user_id, anonymous_id, type, question, spread_id, tarotist_id, status, cards, interpretation, vendor_name, model_name, fail_reason, quota_consumed, first_experience, created_at, updated_at, deleted_at FROM readings WHERE id=$1`, id)
	item, err := scanReading(row)
	if err != nil {
		return nil, biz("记录不存在")
	}
	a.applyCardThumbnails(ctx, r, item)
	localizeReading(ctx, item)
	return item, nil
}
