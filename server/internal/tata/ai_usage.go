package tata

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"tata/server/internal/readingflow"
)

// aiBill is who a model call belongs to. Zero ids are stored as NULL.
// Kind is the business operation (interpret, followup, yearly, probe, simulate).
// Empty kind falls back to the binding task.
type aiBill struct {
	UserID    int64
	ReadingID int64
	JobID     int64
	Kind      string
	Vendor    string
}

type aiBillKey struct{}

func mergeAIBill(ctx context.Context, extra aiBill) context.Context {
	prev, _ := ctx.Value(aiBillKey{}).(aiBill)
	if extra.UserID != 0 {
		prev.UserID = extra.UserID
	}
	if extra.ReadingID != 0 {
		prev.ReadingID = extra.ReadingID
	}
	if extra.JobID != 0 {
		prev.JobID = extra.JobID
	}
	if extra.Kind != "" {
		prev.Kind = extra.Kind
	}
	if extra.Vendor != "" {
		prev.Vendor = extra.Vendor
	}
	return context.WithValue(ctx, aiBillKey{}, prev)
}

func (a *App) watchUsage(ctx context.Context) context.Context {
	bill, _ := ctx.Value(aiBillKey{}).(aiBill)
	return readingflow.WithUsageHook(ctx, func(ev readingflow.UsageEvent) {
		a.insertAIUsage(bill, ev)
	})
}

func (a *App) insertAIUsage(bill aiBill, ev readingflow.UsageEvent) {
	if !ev.Usage.Billable() {
		return
	}
	kind := bill.Kind
	if kind == "" {
		kind = ev.Task
	}
	vendor := ev.Vendor
	if vendor == "" {
		vendor = bill.Vendor
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := a.Store.Exec(ctx, `INSERT INTO ai_usages(
		user_id, reading_id, job_id, task, kind, vendor_name, model_name,
		prompt_tokens, completion_tokens, total_tokens, cached_tokens, reasoning_tokens, ok)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		nullIfZero(bill.UserID), nullIfZero(bill.ReadingID), nullIfZero(bill.JobID),
		ev.Task, kind, vendor, ev.Model,
		ev.Usage.PromptTokens, ev.Usage.CompletionTokens, ev.Usage.TotalTokens,
		ev.Usage.CachedTokens, ev.Usage.ReasoningTokens, ev.Err == nil)
	if err != nil {
		log.Printf("ai usage: %v", err)
	}
}

// attachReadingTokenTotals 把同一条占卜的供应商 token 加总写回列表。没有调用时为 0。
func (a *App) attachReadingTokenTotals(ctx context.Context, items ...map[string]any) error {
	ids := make([]int64, 0, len(items))
	seen := map[int64]struct{}{}
	for _, item := range items {
		item["prompt_tokens"] = 0
		item["completion_tokens"] = 0
		item["total_tokens"] = 0
		id := readingMapID(item)
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, len(ids))
	holders := make([]string, len(ids))
	for i, id := range ids {
		args[i] = id
		holders[i] = fmt.Sprintf("$%d", i+1)
	}
	rows, err := a.Store.Query(ctx, `SELECT reading_id,
		COALESCE(SUM(prompt_tokens), 0),
		COALESCE(SUM(completion_tokens), 0),
		COALESCE(SUM(total_tokens), 0)
		FROM ai_usages WHERE reading_id IN (`+strings.Join(holders, ",")+`)
		GROUP BY reading_id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	totals := map[int64][3]int{}
	for rows.Next() {
		var id int64
		var prompt, completion, total int
		if err := rows.Scan(&id, &prompt, &completion, &total); err != nil {
			return err
		}
		totals[id] = [3]int{prompt, completion, total}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		sum, ok := totals[readingMapID(item)]
		if !ok {
			continue
		}
		item["prompt_tokens"] = sum[0]
		item["completion_tokens"] = sum[1]
		item["total_tokens"] = sum[2]
	}
	return nil
}

func usageKindLabel(kind, task string) string {
	switch kind {
	case "interpret":
		if task == "tarotist" {
			return "塔罗师解读"
		}
		return "解读"
	case "detect":
		return "问题检测"
	case "rewrite":
		return "问题改写"
	case "recommend":
		return "牌阵推荐"
	case "followup":
		return "追问"
	case "yearly":
		return "年运"
	case "probe":
		return "连通性探测"
	case "simulate":
		return "模拟"
	default:
		if kind != "" {
			return kind
		}
		return task
	}
}

func (a *App) adminReadingUsages(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, false); err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var question string
	err = a.Store.QueryRow(r.Context(), `SELECT question FROM readings WHERE id=$1`, id).Scan(&question)
	if err != nil {
		return biz("记录不存在")
	}
	rows, err := a.Store.Query(r.Context(), `SELECT id, task, kind, vendor_name, model_name,
		prompt_tokens, completion_tokens, total_tokens, cached_tokens, reasoning_tokens, ok, created_at
		FROM ai_usages WHERE reading_id=$1 ORDER BY id`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	steps := []map[string]any{}
	var prompt, completion, total int
	for rows.Next() {
		var stepID int64
		var task, kind, vendor, model string
		var p, c, tot, cached, reasoning int
		var ok bool
		var created time.Time
		if err := rows.Scan(&stepID, &task, &kind, &vendor, &model, &p, &c, &tot, &cached, &reasoning, &ok, &created); err != nil {
			return err
		}
		prompt += p
		completion += c
		total += tot
		steps = append(steps, map[string]any{
			"id": stepID, "task": task, "kind": kind, "kind_label": usageKindLabel(kind, task),
			"vendor_name": vendor, "model_name": model,
			"prompt_tokens": p, "completion_tokens": c, "total_tokens": tot,
			"cached_tokens": cached, "reasoning_tokens": reasoning,
			"ok": ok, "created_at": created,
			"price_minor": nil, "price_currency": nil,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeOK(w, map[string]any{
		"reading_id": id, "question": question,
		"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": total,
		"price_minor": nil, "price_currency": nil,
		"steps": steps,
	})
	return nil
}

func readingMapID(item map[string]any) int64 {
	switch n := item["id"].(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
