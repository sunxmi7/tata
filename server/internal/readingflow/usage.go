package readingflow

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// Usage is the token count the vendor returned for one call.
// Prompt is input, completion is output. Cached is the cached part of the input.
// Reasoning is the thinking part of the output. Prices differ, so they stay separate.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
	ReasoningTokens  int
}

// Billable reports whether the vendor sent a token count we can cost.
func (u Usage) Billable() bool {
	return u.PromptTokens > 0 || u.CompletionTokens > 0 || u.TotalTokens > 0
}

// merge keeps the larger count. Vendors send cumulative totals, often on a final empty chunk.
func (u Usage) merge(v Usage) Usage {
	return Usage{
		PromptTokens:     max(u.PromptTokens, v.PromptTokens),
		CompletionTokens: max(u.CompletionTokens, v.CompletionTokens),
		TotalTokens:      max(u.TotalTokens, v.TotalTokens),
		CachedTokens:     max(u.CachedTokens, v.CachedTokens),
		ReasoningTokens:  max(u.ReasoningTokens, v.ReasoningTokens),
	}
}

func usageFrom(msg *schema.Message) Usage {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return Usage{}
	}
	u := msg.ResponseMeta.Usage
	return Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
		CachedTokens:     u.PromptTokenDetails.CachedTokens,
		ReasoningTokens:  u.CompletionTokensDetails.ReasoningTokens,
	}
}

// UsageEvent is one finished model call. Err is the call error; usage may still be set.
type UsageEvent struct {
	Task     string
	Vendor   string
	VendorID int64
	Model    string
	Usage    Usage
	Err      error
}

type usageHookKey struct{}

// WithUsageHook records vendor token usage when a task finishes.
func WithUsageHook(ctx context.Context, hook func(UsageEvent)) context.Context {
	if hook == nil {
		return ctx
	}
	return context.WithValue(ctx, usageHookKey{}, hook)
}

func notifyUsage(ctx context.Context, spec ModelSpec, task string, usage Usage, err error) {
	if !usage.Billable() {
		return
	}
	hook, _ := ctx.Value(usageHookKey{}).(func(UsageEvent))
	if hook == nil {
		return
	}
	hook(UsageEvent{Task: task, Vendor: spec.Vendor, VendorID: spec.VendorID, Model: spec.Model, Usage: usage, Err: err})
}
