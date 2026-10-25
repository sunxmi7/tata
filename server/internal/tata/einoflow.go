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
	name, model, base, key, timeout, err := a.binding(ctx, task)
	if err != nil {
		return readingflow.ModelSpec{}, err
	}
	wait := 30 * time.Second
	if timeout != nil && *timeout > 0 {
		wait = time.Duration(*timeout) * time.Second
	}
	return readingflow.ModelSpec{
		Vendor:    name,
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
	text, err := readingflow.RunTask(ctx, spec, task, map[string]any{"question": userPrompt})
	return spec.Vendor, spec.Model, text, err
}

func (a *App) einoChat(ctx context.Context, baseURL, apiKey, model string, timeoutSec *int, system, user string) (string, error) {
	wait := 30 * time.Second
	if timeoutSec != nil && *timeoutSec > 0 {
		wait = time.Duration(*timeoutSec) * time.Second
	}
	return readingflow.RunTask(ctx, readingflow.ModelSpec{
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
