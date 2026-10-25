package tata

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"tata/server/internal/readingflow"
)

type flowStep struct {
	Task      string `json:"task"`
	Vendor    string `json:"vendor"`
	Model     string `json:"model"`
	Input     string `json:"input"`
	Output    string `json:"output"`
	Error     string `json:"error"`
	Persisted bool   `json:"persisted"`
}

func (a *App) runStep(r *http.Request, task, input string) flowStep {
	vendor, model, output, err := a.runEino(r.Context(), task, input)
	step := flowStep{Task: task, Vendor: vendor, Model: model, Input: input, Output: output}
	if err != nil {
		step.Error = err.Error()
	}
	return step
}

func (a *App) adminSimulate(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminActor(r, true); err != nil {
		return err
	}
	var before int
	_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings`).Scan(&before)
	var req struct {
		Question   string          `json:"question"`
		SpreadID   int64           `json:"spread_id"`
		Cards      json.RawMessage `json:"cards"`
		TarotistID int64           `json:"tarotist_id"`
		Step       string          `json:"step"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return biz("请求字段不正确")
	}
	if req.Question == "" {
		req.Question = "接下来三个月工作怎么选"
	}
	cards := string(req.Cards)
	if cards == "" || cards == "null" {
		positions := []position{{Index: 1, Name: "过去", Meaning: ""}, {Index: 2, Name: "现在", Meaning: ""}, {Index: 3, Name: "未来", Meaning: ""}}
		if req.SpreadID > 0 {
			var raw string
			if err := a.Store.QueryRow(r.Context(), `SELECT positions FROM spreads WHERE id=$1`, req.SpreadID).Scan(&raw); err == nil {
				_ = json.Unmarshal([]byte(raw), &positions)
			}
		}
		drawn, err := drawCards(positions)
		if err != nil {
			return err
		}
		cards = mustJSON(drawn)
	}
	persona := ""
	if req.TarotistID > 0 {
		_ = a.Store.QueryRow(r.Context(), `SELECT persona FROM tarotists WHERE id=$1`, req.TarotistID).Scan(&persona)
	}
	if req.Step != "" {
		return a.simulateOne(w, r, req.Step, req.Question, cards, persona)
	}
	out, err := readingflow.RunWorkflow(r.Context(), a.workflowSpecs(r.Context()), readingflow.Input{
		Question: req.Question,
		Cards:    cards,
		Persona:  persona,
	})
	if err != nil {
		return err
	}
	steps := []flowStep{
		toStep("detect", out.DetectText, out.DetectVendor, out.DetectModel, out.DetectError),
		toStep("rewrite", out.RewriteText, out.RewriteVendor, out.RewriteModel, out.RewriteError),
		toStep("recommend", out.RecommendText, out.RecommendVendor, out.RecommendModel, out.RecommendError),
		toStep("interpret", out.InterpretText, out.InterpretVendor, out.InterpretModel, out.InterpretError),
	}
	if persona != "" {
		steps = append(steps, toStep("tarotist", out.TarotistText, out.TarotistVendor, out.TarotistModel, out.TarotistError))
	}
	var after int
	_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings`).Scan(&after)
	if after != before {
		return biz("模拟流程不应写入占卜记录")
	}
	writeOK(w, map[string]any{
		"engine":    "eino",
		"persisted": false,
		"cards":     json.RawMessage(cards),
		"app_call": map[string]any{
			"method": "POST",
			"path":   "/api/app/v1/readings/{id}/workflow",
			"body": map[string]any{
				"steps": []string{"detect", "rewrite", "recommend", "interpret"},
			},
			"note": "App 在牌面固定前调用检测、改写、推荐；牌面固定后调用 interpret。interpret 才会写入解读。模拟流程不写库、不扣费。",
		},
		"steps": steps,
	})
	return nil
}
func (a *App) simulateOne(w http.ResponseWriter, r *http.Request, step, question, cards, persona string) error {
	title := map[string]string{
		"detect": "问题检测", "rewrite": "问题改写", "recommend": "牌阵推荐",
		"draw": "抽牌", "interpret": "解读", "tarotist": "塔罗师解读",
	}[step]
	if title == "" {
		return biz("未知步骤")
	}
	var before int
	_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings`).Scan(&before)
	text, vendor, model, errText := "", "", "", ""
	switch step {
	case "draw":
		var drawn []drawnCard
		if err := json.Unmarshal([]byte(cards), &drawn); err != nil || len(drawn) == 0 {
			return biz("抽牌失败")
		}
		lines := make([]string, 0, len(drawn))
		for _, c := range drawn {
			side := "正位"
			if c.Orientation == "reversed" {
				side = "逆位"
			}
			lines = append(lines, c.PositionName+"："+c.Name+"，"+side)
		}
		text = strings.Join(lines, "\n")
	case "detect", "rewrite", "recommend":
		got := a.runStep(r, step, question)
		text, vendor, model, errText = got.Output, got.Vendor, got.Model, got.Error
	case "interpret", "tarotist":
		if cards == "" || cards == "null" {
			return biz("先抽牌，再解读")
		}
		kind := "normal"
		if step == "tarotist" {
			kind = "tarotist"
		}
		got := a.runStep(r, step, buildPrompt(kind, question, cards))
		text, vendor, model, errText = got.Output, got.Vendor, got.Model, got.Error
	}
	var after int
	_ = a.Store.QueryRow(r.Context(), `SELECT count(*) FROM readings`).Scan(&after)
	if after != before {
		return biz("模拟流程不应写入占卜记录")
	}
	body := map[string]any{
		"step": step, "title": title, "text": text,
		"vendor": vendor, "model": model, "error": errText,
	}
	if step == "draw" {
		body["cards"] = json.RawMessage(cards)
	}
	writeOK(w, body)
	return nil
}

func (a *App) appWorkflow(w http.ResponseWriter, r *http.Request) error {
	uid, anon, err := a.appActor(r)
	if err != nil {
		return err
	}
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := a.ownReading(r.Context(), id, uid, anon); err != nil {
		return err
	}
	var req struct {
		Steps []string `json:"steps"`
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	item, err := a.loadReading(r.Context(), id)
	if err != nil {
		return err
	}
	question, _ := item["question"].(string)
	cards := ""
	if raw, ok := item["cards"]; ok && raw != nil {
		b, _ := json.Marshal(raw)
		cards = string(b)
	}
	if len(req.Steps) == 0 {
		if cards == "" || cards == "null" {
			req.Steps = []string{"detect", "rewrite", "recommend"}
		} else {
			req.Steps = []string{"interpret"}
		}
	}
	steps := make([]flowStep, 0, len(req.Steps))
	for _, task := range req.Steps {
		switch task {
		case "detect", "rewrite", "recommend":
			steps = append(steps, a.runStep(r, task, question))
		case "interpret", "tarotist":
			if cards == "" || cards == "null" {
				steps = append(steps, flowStep{Task: task, Error: "牌面尚未固定", Input: question})
				continue
			}
			kind := "normal"
			if task == "tarotist" || item["type"] == "tarotist" {
				kind = "tarotist"
			}
			saved, genErr := a.generate(r.Context(), id, false)
			step := flowStep{Task: task, Input: buildPrompt(kind, question, cards), Persisted: genErr == nil}
			if genErr != nil {
				step.Error = genErr.Error()
			} else {
				step.Output, _ = saved["interpretation"].(string)
				if v, ok := saved["vendor_name"].(*string); ok && v != nil {
					step.Vendor = *v
				}
				if m, ok := saved["model_name"].(*string); ok && m != nil {
					step.Model = *m
				}
				item = saved
			}
			steps = append(steps, step)
		default:
			steps = append(steps, flowStep{Task: task, Error: "未知步骤"})
		}
	}
	writeOK(w, map[string]any{
		"engine":   "eino",
		"reading":  item,
		"steps":    steps,
		"app_call": fmt.Sprintf("POST /api/app/v1/readings/%d/workflow", id),
	})
	return nil
}

func toStep(task, output, vendor, model, errText string) flowStep {
	return flowStep{Task: task, Vendor: vendor, Model: model, Output: output, Error: errText}
}
