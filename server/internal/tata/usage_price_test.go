package tata

import (
	"testing"

	"tata/server/internal/readingflow"
)

func TestUsagePriceMinor(t *testing.T) {
	cache := int64(40_000) // 0.04 元 / 百万 = 40000 微元
	p := modelPrice{currency: "CNY", inputMinor: 2_000_000, cacheMinor: &cache, outputMinor: 8_000_000}
	// 201 prompt, 40 cached, 624 completion → 322 + 1.6 + 4992 = 5315.6 → 5316 微元
	got, cur := usagePriceMinor(p, readingflow.Usage{PromptTokens: 201, CachedTokens: 40, CompletionTokens: 624})
	if cur != "CNY" || got != 5316 {
		t.Fatalf("got %d %s want 5316 CNY micros", got, cur)
	}
	p2 := modelPrice{currency: "CNY", inputMinor: 2_000_000, outputMinor: 8_000_000}
	got2, _ := usagePriceMinor(p2, readingflow.Usage{PromptTokens: 100, CachedTokens: 20, CompletionTokens: 0})
	if got2 != 200 {
		t.Fatalf("small usage: %d", got2)
	}
	got3, _ := usagePriceMinor(p2, readingflow.Usage{PromptTokens: 1_000_000, CachedTokens: 0, CompletionTokens: 0})
	if got3 != 2_000_000 {
		t.Fatalf("1M input: %d", got3)
	}
}
