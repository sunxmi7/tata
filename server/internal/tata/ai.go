package tata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// DemoHold delays the local demo model so tests can prove a single in-flight generation.
var DemoHold atomic.Int64

func chatURL(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func callChat(ctx context.Context, baseURL, apiKey, model, prompt string, timeoutSec *int) (string, error) {
	timeout := 30 * time.Second
	if timeoutSec != nil && *timeoutSec > 0 {
		timeout = time.Duration(*timeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	body, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{{
			"role": "user", "content": prompt,
		}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL(baseURL), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return "", biz("供应商调用失败")
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Choices) == 0 {
		return "", biz("供应商返回无法解析")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (a *App) demoOpenAI(w http.ResponseWriter, r *http.Request) {
	if d := time.Duration(DemoHold.Load()); d > 0 {
		time.Sleep(d)
	}
	var req struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	content := ""
	promptRunes := 0
	for _, msg := range req.Messages {
		promptRunes += utf8.RuneCountInString(msg.Content)
		content = msg.Content
	}
	reply := "【演示解读】牌面由业务固定，这里只解读，不改牌。\n" + content
	usage := demoUsage(promptRunes, utf8.RuneCountInString(reply))
	if req.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, piece := range chunkRunes(reply, 24) {
			evt := map[string]any{
				"object":  "chat.completion.chunk",
				"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": piece}}},
			}
			b, _ := json.Marshal(evt)
			fmt.Fprintf(w, "data: %s\n\n", b)
			if flusher != nil {
				flusher.Flush()
			}
		}
		usageEvt := map[string]any{
			"object":  "chat.completion.chunk",
			"choices": []any{},
			"usage":   usage,
		}
		b, _ := json.Marshal(usageEvt)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": reply}}},
		"usage":   usage,
	})
}

// demoUsage 按字符数造一份供应商 usage，让演示供应商也能走出记 token 的路径。
// cached 取输入的五分之一，对应缓存命中更便宜的那一部分。
func demoUsage(promptRunes, completionRunes int) map[string]any {
	if promptRunes < 1 {
		promptRunes = 1
	}
	if completionRunes < 1 {
		completionRunes = 1
	}
	return map[string]any{
		"prompt_tokens":             promptRunes,
		"completion_tokens":         completionRunes,
		"total_tokens":              promptRunes + completionRunes,
		"prompt_tokens_details":     map[string]any{"cached_tokens": promptRunes / 5},
		"completion_tokens_details": map[string]any{"reasoning_tokens": 0},
	}
}

// chunkRunes 把文本按 rune 切成每段至多 n 个字符，供演示流式分片下发。
func chunkRunes(s string, n int) []string {
	runes := []rune(s)
	var out []string
	for i := 0; i < len(runes); i += n {
		end := i + n
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[i:end]))
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}
