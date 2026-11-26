package tata

import (
	"strings"
	"testing"
)

func TestDetectQuestionKinds(t *testing.T) {
	cases := []struct {
		q    string
		kind questionKind
	}{
		{"", questionInvalid},
		{"！！！", questionInvalid},
		{"12345", questionInvalid},
		{"啊啊啊啊啊", questionInvalid},
		{"我应该接受这份工作吗？", questionSpecific},
		{"他现在对我是什么感觉？", questionSpecific},
		{"这次考试能顺利通过吗？", questionSpecific},
		{"我接下来三个月会有新的工作机会吗？", questionSpecific},
		{"工作", questionBroad},
		{"我最近的工作怎么样？", questionBroad},
		{"我想问11月份的运势", questionBroad},
		{"我和男朋友最近总吵架，还要不要继续", questionSpecific},
	}
	for _, tc := range cases {
		got := detectQuestion(tc.q)
		if got.Kind != tc.kind {
			t.Fatalf("%q: kind=%s want %s text=%s", tc.q, got.Kind, tc.kind, got.DetectText)
		}
		if got.DetectText == "" {
			t.Fatalf("%q: empty detect text", tc.q)
		}
	}
	broad := detectQuestion("我想问11月份的运势")
	if broad.TimeScope == "" || len(broad.Scenes) == 0 {
		t.Fatalf("time/scenes: %+v", broad)
	}
	if broad.DetectText == "你的问题涉及的范围比较广，明确你最关注的方向，可以帮助解读更聚焦，也更容易获得具体的建议。" {
		t.Fatalf("broad detect text stayed generic: %s", broad.DetectText)
	}
}

func TestQuestionSuggestionsVaryByInput(t *testing.T) {
	_, ts := newMergeServer(t)

	invalid := create(t, ts, "", "/api/app/v1/question-suggestions", map[string]any{"question": "！！！"})
	invalidMsgs := messageList(t, invalid)
	if len(invalidMsgs) != 1 || invalidMsgs[0]["style"] != "detect" {
		t.Fatalf("invalid messages: %#v", invalidMsgs)
	}
	if text, _ := invalidMsgs[0]["text"].(string); !strings.Contains(text, "看不出你想问什么") {
		t.Fatalf("invalid detect: %#v", invalidMsgs[0])
	}

	specific := create(t, ts, "", "/api/app/v1/question-suggestions", map[string]any{"question": "我应该接受这份工作吗？"})
	specificMsgs := messageList(t, specific)
	if len(specificMsgs) != 1 || specificMsgs[0]["style"] != "detect" {
		t.Fatalf("specific messages: %#v", specificMsgs)
	}
	if text, _ := specificMsgs[0]["text"].(string); text != "你的问题已经足够具体，可以直接进入选阵占卜。" {
		t.Fatalf("specific detect: %#v", specificMsgs[0])
	}

	love := create(t, ts, "", "/api/app/v1/question-suggestions", map[string]any{"question": "我和他接下来会怎样？"})
	career := create(t, ts, "", "/api/app/v1/question-suggestions", map[string]any{"question": "我最近的工作怎么样？"})
	loveRec := recommendTexts(t, love)
	careerRec := recommendTexts(t, career)
	if len(loveRec) == 0 || len(careerRec) == 0 {
		t.Fatalf("missing recommendations love=%v career=%v", loveRec, careerRec)
	}
	if sameStringSlice(loveRec, careerRec) {
		t.Fatalf("recommendations identical across topics: %v", loveRec)
	}

	timed := create(t, ts, "", "/api/app/v1/question-suggestions", map[string]any{"question": "我想问11月份的运势"})
	timedRec := recommendTexts(t, timed)
	if len(timedRec) != 3 {
		t.Fatalf("timed recommendations: %v", timedRec)
	}
	keptTime := false
	for _, text := range timedRec {
		if strings.Contains(text, "11月") {
			keptTime = true
			break
		}
	}
	if !keptTime {
		t.Fatalf("time scope lost: %v", timedRec)
	}
}

func recommendTexts(t *testing.T, payload map[string]any) []string {
	t.Helper()
	for _, item := range messageList(t, payload) {
		if item["style"] != "recommend" {
			continue
		}
		opts, _ := item["options"].([]any)
		out := make([]string, 0, len(opts))
		for _, raw := range opts {
			out = append(out, raw.(map[string]any)["text"].(string))
		}
		return out
	}
	return nil
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
