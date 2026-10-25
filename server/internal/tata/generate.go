package tata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (a *App) generate(ctx context.Context, id int64, adminRetry bool) (map[string]any, error) {
	var status, kind string
	var userID *int64
	var cards, question *string
	var quotaConsumed, first, rewardConsumed bool
	err := a.Store.QueryRow(ctx, `SELECT status, type, user_id, cards, question, quota_consumed, first_experience, reward_consumed FROM readings WHERE id=$1 AND deleted_at IS NULL`, id).
		Scan(&status, &kind, &userID, &cards, &question, &quotaConsumed, &first, &rewardConsumed)
	if err != nil {
		return nil, biz("记录不存在")
	}
	if adminRetry && status != "FAILED_RETRYABLE" {
		return nil, biz("只有失败可重试的记录能重试")
	}
	if !adminRetry && status != "CARDS_FIXED" && status != "FAILED_RETRYABLE" {
		return nil, biz("当前状态不能生成解读")
	}
	if userID == nil {
		return nil, biz("获取答案前必须登录")
	}
	if !quotaConsumed && !first && !rewardConsumed && kind != "daily" {
		return nil, biz("今日免费体验额度已用完，本次记录已保存到历史记录")
	}
	if cards == nil || *cards == "" {
		return nil, biz("牌面尚未固定")
	}
	vendorName, model, _, _, _, err := a.binding(ctx, taskFor(kind))
	if err != nil {
		_, _ = a.Store.Exec(ctx, `UPDATE readings SET status='FAILED_RETRYABLE', fail_reason=$1, updated_at=now() WHERE id=$2 AND status IN ('CARDS_FIXED','FAILED_RETRYABLE')`, "未绑定供应商", id)
		return nil, err
	}
	unlock, err := a.lockGen(id)
	if err != nil {
		return nil, err
	}
	defer unlock()
	tag, err := a.Store.Exec(ctx, `UPDATE readings SET status='INTERPRETING', vendor_name=$1, model_name=$2, fail_reason=NULL, updated_at=now() WHERE id=$3 AND status IN ('CARDS_FIXED','FAILED_RETRYABLE')`,
		vendorName, model, id)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, biz("生成进行中")
	}
	prompt := buildPrompt(kind, deref(question), *cards)
	if kind == "tarotist" {
		var tid *int64
		_ = a.Store.QueryRow(ctx, `SELECT tarotist_id FROM readings WHERE id=$1`, id).Scan(&tid)
		if tid != nil {
			var name, persona, voice, banned, ask string
			_ = a.Store.QueryRow(ctx, `SELECT name, persona, voice, banned, ask_style FROM tarotists WHERE id=$1`, *tid).Scan(&name, &persona, &voice, &banned, &ask)
			prompt = "共用解牌：先按牌面、牌位、正逆位解释，再按人设说话。不得改牌。\n人设：" + name + "。" + persona + "\n语音：" + voice + "。禁止：" + banned + "。追问：" + ask + "\n" + buildPrompt(kind, deref(question), *cards)
		}
	}
	vendorName, model, text, callErr := a.runEino(ctx, taskFor(kind), prompt)
	if callErr != nil {
		_, _ = a.Store.Exec(ctx, `UPDATE readings SET status='FAILED_RETRYABLE', fail_reason=$1, updated_at=now() WHERE id=$2`, callErr.Error(), id)
		return nil, biz("解读失败，可重试，未重新扣费")
	}
	_, err = a.Store.Exec(ctx, `UPDATE readings SET status='COMPLETED', interpretation=$1, fail_reason=NULL, updated_at=now() WHERE id=$2`, text, id)
	if err != nil {
		return nil, err
	}
	return a.loadReading(ctx, id)
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
	return map[string]any{
		"id": id, "user_id": userID, "anonymous_id": anonID, "type": typ, "question": question,
		"spread_id": spreadID, "tarotist_id": tarotistID, "status": status, "cards": cardsJSON,
		"interpretation": interpretation, "vendor_name": vendor, "model_name": model, "fail_reason": fail,
		"quota_consumed": quota, "first_experience": first, "created_at": created, "updated_at": updated, "deleted_at": deleted,
		"progress": label, "continue": next,
	}, nil
}

func (a *App) loadReading(ctx context.Context, id int64) (map[string]any, error) {
	row := a.Store.QueryRow(ctx, `SELECT id, user_id, anonymous_id, type, question, spread_id, tarotist_id, status, cards, interpretation, vendor_name, model_name, fail_reason, quota_consumed, first_experience, created_at, updated_at, deleted_at FROM readings WHERE id=$1`, id)
	item, err := scanReading(row)
	if err != nil {
		return nil, biz("记录不存在")
	}
	return item, nil
}
