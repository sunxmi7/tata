package tata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"tata/server/internal/readingflow"
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
	a.localizeReading(ctx, item)
	return item, nil
}

func (a *App) binding(ctx context.Context, task string) (vendorID int64, name, model, base, key string, timeout *int, err error) {
	err = a.Store.QueryRow(ctx, `SELECT vendor_id, model FROM bindings WHERE task=$1`, task).Scan(&vendorID, &model)
	if err != nil {
		return 0, "", "", "", "", nil, biz("未绑定供应商")
	}
	var enc []byte
	var enabled bool
	err = a.Store.QueryRow(ctx, `SELECT name, base_url, api_key_enc, timeout_sec, enabled FROM vendors WHERE id=$1`, vendorID).Scan(&name, &base, &enc, &timeout, &enabled)
	if err != nil || !enabled {
		return 0, "", "", "", "", nil, biz("未绑定供应商")
	}
	key, err = a.Store.decrypt(enc)
	if err != nil {
		return 0, "", "", "", "", nil, biz("供应商密钥无法读取")
	}
	return vendorID, name, model, base, key, timeout, nil
}

func buildPrompt(kind, question, cards string) string {
	return buildInterpretPrompt(kind, question, "", "", cards, nil)
}

// interpretPromptMode selects which interpret-rules module to load into the system prompt.
func interpretPromptMode(kind, question string) string {
	switch strings.TrimSpace(kind) {
	case "daily":
		if strings.Contains(question, "明日") {
			return readingflow.ModeDailyTomorrow
		}
		return readingflow.ModeDailyToday
	case "explore":
		return readingflow.ModeExplore
	case "tarotist":
		return readingflow.ModeTarotist
	default:
		return readingflow.ModeNormal
	}
}

// buildInterpretPrompt 组装用户侧解读材料（系统规则在 readingflow.InterpretSystem）。
// explore 且用户未提问时，用主题分类与牌阵名作为场景，不伪造用户问题。
// positions 来自当前牌阵配置；普通/探索展开 name+meaning，今日/明日只带槽位名。
func buildInterpretPrompt(kind, question, themeTitle, spreadName, cards string, positions []position) string {
	var b strings.Builder
	b.WriteString("只根据下列材料生成正式解读正文。不得改牌、牌位或正逆位。不要输出思考过程、规则说明或工作备注。\n")
	if kind == "tarotist" {
		b.WriteString("按所选塔罗师人设表达，但牌义不能歪曲。\n")
	}
	q := strings.TrimSpace(question)
	themeTitle = strings.TrimSpace(themeTitle)
	spreadName = strings.TrimSpace(spreadName)
	wroteSpread := false
	if q != "" {
		fmt.Fprintf(&b, "问题：%s\n", q)
	} else if kind == "explore" && (themeTitle != "" || spreadName != "") {
		b.WriteString("用户未填写问题，请按主题与牌阵解读。\n")
		if themeTitle != "" {
			fmt.Fprintf(&b, "主题：%s\n", themeTitle)
		}
		if spreadName != "" {
			fmt.Fprintf(&b, "牌阵：%s\n", spreadName)
			wroteSpread = true
		}
	} else {
		fmt.Fprintf(&b, "问题：%s\n", q)
	}
	if spreadName != "" && !wroteSpread {
		fmt.Fprintf(&b, "牌阵：%s\n", spreadName)
	}
	if block := formatPositionsBlock(kind, cards, positions); block != "" {
		b.WriteString(block)
	}
	fmt.Fprintf(&b, "牌面：%s\n", cards)
	if block := formatMajorMeaningsBlock(cards); block != "" {
		b.WriteString(block)
	}
	return b.String()
}

func parseDrawnCards(cardsJSON string) []drawnCard {
	if cardsJSON == "" || cardsJSON == "null" {
		return nil
	}
	var drawn []drawnCard
	if err := json.Unmarshal([]byte(cardsJSON), &drawn); err != nil {
		return nil
	}
	return drawn
}

func positionAt(positions []position, card drawnCard, order int) (position, bool) {
	if card.Position > 0 {
		for _, p := range positions {
			idx := p.Index
			if idx <= 0 {
				continue
			}
			if idx == card.Position {
				return p, true
			}
		}
	}
	if order >= 0 && order < len(positions) {
		return positions[order], true
	}
	return position{}, false
}

// formatPositionsBlock expands spread positions for the user prompt.
// daily: slot names only; normal/explore/tarotist: name + meaning in draw order.
func formatPositionsBlock(kind, cardsJSON string, positions []position) string {
	drawn := parseDrawnCards(cardsJSON)
	if len(drawn) == 0 && len(positions) == 0 {
		return ""
	}
	if kind == "daily" {
		names := make([]string, 0, max(len(drawn), 1))
		if len(drawn) == 0 {
			if len(positions) > 0 && strings.TrimSpace(positions[0].Name) != "" {
				names = append(names, strings.TrimSpace(positions[0].Name))
			} else {
				names = append(names, "运势")
			}
		} else {
			for i, c := range drawn {
				name := strings.TrimSpace(c.PositionName)
				if p, ok := positionAt(positions, c, i); ok && name == "" {
					name = strings.TrimSpace(p.Name)
				}
				if name == "" {
					name = "运势"
				}
				names = append(names, name)
			}
		}
		return "牌位：" + strings.Join(names, "、") + "\n"
	}

	var b strings.Builder
	b.WriteString("牌位（按抽牌顺序）：\n")
	if len(drawn) == 0 {
		for i, p := range positions {
			idx := p.Index
			if idx <= 0 {
				idx = i + 1
			}
			name := strings.TrimSpace(p.Name)
			if name == "" {
				name = fmt.Sprintf("位置%d", idx)
			}
			line := fmt.Sprintf("%d. %s", idx, name)
			if meaning := strings.TrimSpace(p.Meaning); meaning != "" {
				line += "：" + meaning
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
		return b.String()
	}
	for i, c := range drawn {
		idx := c.Position
		if idx <= 0 {
			idx = i + 1
		}
		name := strings.TrimSpace(c.PositionName)
		meaning := ""
		if p, ok := positionAt(positions, c, i); ok {
			if name == "" {
				name = strings.TrimSpace(p.Name)
			}
			meaning = strings.TrimSpace(p.Meaning)
		}
		if name == "" {
			name = fmt.Sprintf("位置%d", idx)
		}
		line := fmt.Sprintf("%d. %s", idx, name)
		if meaning != "" {
			line += "：" + meaning
		}
		cardName := strings.TrimSpace(c.Name)
		if cardName != "" {
			line += " / " + cardName + " " + orientationLabel(c.Orientation)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func (a *App) interpretThemeContext(ctx context.Context, id int64) (themeTitle, spreadName string, err error) {
	err = a.Store.QueryRow(ctx, `SELECT COALESCE(s.theme_title, ''),
		COALESCE(NULLIF(r.spread_name, ''), s.name, '')
		FROM readings r
		LEFT JOIN spreads s ON s.id = r.spread_id
		WHERE r.id = $1`, id).Scan(&themeTitle, &spreadName)
	return themeTitle, spreadName, err
}

func (a *App) loadReadingPositions(ctx context.Context, readingID int64) []position {
	var raw string
	err := a.Store.QueryRow(ctx, `SELECT COALESCE(s.positions, '[]')
		FROM readings r
		LEFT JOIN spreads s ON s.id = r.spread_id
		WHERE r.id = $1`, readingID).Scan(&raw)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var positions []position
	if err := json.Unmarshal([]byte(raw), &positions); err != nil {
		return nil
	}
	return positions
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
	var typ, question, status string
	var cards, interpretation, vendor, model, fail *string
	var quota, first bool
	var created, updated time.Time
	var deleted *time.Time
	var drawMode, themeKey, businessDate string
	err := sc.Scan(&id, &userID, &typ, &question, &spreadID, &tarotistID, &status, &cards, &interpretation, &vendor, &model, &fail, &quota, &first, &created, &updated, &deleted, &drawMode, &themeKey, &businessDate)
	if err != nil {
		return nil, err
	}
	var cardsJSON any
	if cards != nil && *cards != "" {
		cardsJSON = json.RawMessage(*cards)
	}
	label, next := progressOf(status)
	item := map[string]any{
		"id": id, "user_id": userID, "type": typ, "question": question,
		"spread_id": spreadID, "tarotist_id": tarotistID, "status": status, "cards": cardsJSON,
		"interpretation": interpretation, "vendor_name": vendor, "model_name": model, "fail_reason": fail,
		"quota_consumed": quota, "first_experience": first, "created_at": created, "updated_at": updated, "deleted_at": deleted,
		"draw_mode": drawMode, "theme_key": themeKey, "business_date": businessDate,
		"progress": label, "continue": next,
	}
	attachReadingPreview(item)
	attachConversationMeta(item, "")
	return item, nil
}

func (a *App) loadReading(ctx context.Context, id int64) (map[string]any, error) {
	return a.loadReadingAbs(ctx, nil, id)
}

func (a *App) loadReadingAbs(ctx context.Context, r *http.Request, id int64) (map[string]any, error) {
	row := a.Store.QueryRow(ctx, `SELECT `+readingSelectCols+` FROM readings WHERE id=$1`, id)
	item, err := scanReading(row)
	if err != nil {
		return nil, biz("记录不存在")
	}
	if err := a.fillThemeKeyFromSpread(ctx, item); err != nil {
		return nil, err
	}
	today := ""
	if r != nil {
		if uid, _ := a.appActor(r); uid > 0 {
			today = a.userCycle(r, uid)
		} else {
			today = cycleOf(time.Now())
		}
	}
	attachConversationMeta(item, today)
	a.applyCardThumbnails(ctx, r, item)
	a.localizeReading(ctx, item)
	return item, nil
}

func readingSpreadID(item map[string]any) int64 {
	switch v := item["spread_id"].(type) {
	case *int64:
		if v != nil {
			return *v
		}
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}

// fillThemeKeyFromSpread backfills readings.theme_key from the selected spread when create omitted it.
func (a *App) fillThemeKeyFromSpread(ctx context.Context, items ...map[string]any) error {
	cache := map[int64]string{}
	for _, item := range items {
		if item == nil {
			continue
		}
		if key, _ := item["theme_key"].(string); strings.TrimSpace(key) != "" {
			continue
		}
		sid := readingSpreadID(item)
		if sid <= 0 {
			continue
		}
		key, ok := cache[sid]
		if !ok {
			if err := a.Store.QueryRow(ctx, `SELECT COALESCE(theme_key,'') FROM spreads WHERE id=$1`, sid).Scan(&key); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					cache[sid] = ""
					continue
				}
				return err
			}
			cache[sid] = key
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		item["theme_key"] = key
		if readingID, ok := item["id"].(int64); ok && readingID > 0 {
			_, _ = a.Store.Exec(ctx, `UPDATE readings SET theme_key=$1 WHERE id=$2 AND theme_key=''`, key, readingID)
		}
	}
	return nil
}
