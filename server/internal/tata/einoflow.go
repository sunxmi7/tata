package tata

import (
	"context"
	"net/http"
	"strings"
	"time"

	"tata/server/internal/readingflow"
)

type holdTransport struct {
	base http.RoundTripper
}

func (t holdTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if d := time.Duration(DemoHold.Load()); d > 0 && strings.Contains(req.URL.Path, "/demo/openai") {
		time.Sleep(d)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func (a *App) modelSpec(ctx context.Context, task string) (readingflow.ModelSpec, error) {
	vendorID, name, model, base, key, timeout, err := a.binding(ctx, task)
	if err != nil {
		return readingflow.ModelSpec{}, err
	}
	wait := 30 * time.Second
	if timeout != nil && *timeout > 0 {
		wait = time.Duration(*timeout) * time.Second
	}
	return readingflow.ModelSpec{
		Vendor:    name,
		VendorID:  vendorID,
		Model:     model,
		BaseURL:   base,
		APIKey:    key,
		Timeout:   wait,
		Transport: holdTransport{base: http.DefaultTransport},
	}, nil
}

func (a *App) runEino(ctx context.Context, task, userPrompt string) (string, string, string, error) {
	spec, err := a.modelSpec(ctx, task)
	if err != nil {
		return "", "", "", err
	}
	text, err := readingflow.RunTask(a.watchUsage(ctx), spec, task, map[string]any{"question": userPrompt})
	return spec.Vendor, spec.Model, text, err
}

// runEinoStream 流式版本：每个增量片段回调 onChunk，返回拼好的完整文本。
func (a *App) runEinoStream(ctx context.Context, task, userPrompt string, onChunk func(string) error) (string, string, string, error) {
	spec, err := a.modelSpec(ctx, task)
	if err != nil {
		return "", "", "", err
	}
	text, err := readingflow.RunTaskStream(a.watchUsage(ctx), spec, task, map[string]any{"question": userPrompt}, onChunk)
	return spec.Vendor, spec.Model, text, err
}

func (a *App) einoChat(ctx context.Context, baseURL, apiKey, model string, timeoutSec *int, system, user string) (string, error) {
	wait := 30 * time.Second
	if timeoutSec != nil && *timeoutSec > 0 {
		wait = time.Duration(*timeoutSec) * time.Second
	}
	bill, _ := ctx.Value(aiBillKey{}).(aiBill)
	if bill.Kind == "" {
		ctx = mergeAIBill(ctx, aiBill{Kind: "probe"})
		bill, _ = ctx.Value(aiBillKey{}).(aiBill)
	}
	return readingflow.RunTask(a.watchUsage(ctx), readingflow.ModelSpec{
		Vendor:    bill.Vendor,
		VendorID:  bill.VendorID,
		Model:     model,
		BaseURL:   baseURL,
		APIKey:    apiKey,
		Timeout:   wait,
		Transport: holdTransport{base: http.DefaultTransport},
	}, "interpret", map[string]any{"question": system + "\n" + user})
}

func (a *App) workflowSpecs(ctx context.Context) map[string]readingflow.ModelSpec {
	specs := map[string]readingflow.ModelSpec{}
	for _, task := range []string{"detect", "rewrite", "recommend", "interpret", "tarotist"} {
		spec, err := a.modelSpec(ctx, task)
		if err != nil {
			continue
		}
		specs[task] = spec
	}
	return specs
}
