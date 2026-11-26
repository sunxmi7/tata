package tata

import (
	"fmt"
	"net/http"
	"testing"
)

func TestBuildQuestionMatchSnapshotHeuristics(t *testing.T) {
	statusCases := []struct {
		q, intent, structure string
		dual                 bool
		status               string
	}{
		{"!!!", "", "", false, detectionStatusInvalid},
		{"工作运势怎么样", "", "", false, detectionStatusBroad},
		{"我应该接受这份工作吗？", "I01", "S01", false, detectionStatusSpecific},
		{"要不要换工作", "I01", "S01", false, detectionStatusSpecific},
		{"接下来三个月工作会顺利吗", "I04A", "S02", false, detectionStatusSpecific},
		{"我该如何提升职场表现", "I10A", "S04", false, detectionStatusSpecific},
		{"他对我是什么感觉", "I12B", "S06", false, detectionStatusSpecific},
		{"选A还是B比较好", "I11", "S05", true, detectionStatusSpecific},
	}
	for _, tc := range statusCases {
		snap := buildQuestionMatchSnapshot(tc.q)
		if snap.DetectionStatus != tc.status || snap.MainIntent != tc.intent || snap.MainStructure != tc.structure || snap.HasDualOptions != tc.dual {
			t.Fatalf("%q => %#v want status=%s intent=%s structure=%s dual=%v",
				tc.q, snap, tc.status, tc.intent, tc.structure, tc.dual)
		}
	}

	intent, structure, dual := inferIntentStructure("这件事的根源是什么")
	if intent != "I05B" || structure != "S03" || dual {
		t.Fatalf("infer root: %s %s %v", intent, structure, dual)
	}
}

func TestSpreadMatchPicksByIntent(t *testing.T) {
	cfg := embeddedSpreadMatchConfig
	rows := []dbSpreadRow{
		{ID: 1, Name: "单张牌占卜"},
		{ID: 2, Name: "过去·现在·未来"},
		{ID: 3, Name: "时间流"},
		{ID: 11, Name: "问题解决牌阵"},
		{ID: 13, Name: "六张关系牌阵"},
		{ID: 8, Name: "简单十字牌阵"},
	}

	pick := func(intent, structure string) string {
		best, status := cfg.match(questionMatchSnapshot{
			FormalQuestion:  "x",
			DetectionStatus: detectionStatusSpecific,
			MainIntent:      intent,
			MainStructure:   structure,
			IntentRelation:  "single",
		}, rows)
		if status != matchStatusMatched {
			t.Fatalf("%s/%s status=%s", intent, structure, status)
		}
		return best.row.Name
	}
	if got := pick("I01", "S01"); got != "单张牌占卜" {
		t.Fatalf("I01 => %s", got)
	}
	if got := pick("I04A", "S02"); got != "过去·现在·未来" && got != "时间流" {
		t.Fatalf("I04A => %s", got)
	}
	if got := pick("I12B", "S06"); got != "六张关系牌阵" {
		t.Fatalf("I12B => %s", got)
	}
	if got := pick("I10A", "S04"); got != "问题解决牌阵" {
		t.Fatalf("I10A => %s", got)
	}
}

func TestSpreadMatchRejectsPendingRuntimeI11(t *testing.T) {
	cfg := embeddedSpreadMatchConfig
	rows := []dbSpreadRow{
		{ID: 5, Name: "选择A·选择B·综合建议"},
		{ID: 17, Name: "七张双路径选择牌阵"},
		{ID: 1, Name: "单张牌占卜"},
	}
	_, status := cfg.match(questionMatchSnapshot{
		FormalQuestion:  "A还是B",
		DetectionStatus: detectionStatusSpecific,
		MainIntent:      "I11",
		MainStructure:   "S05",
		HasDualOptions:  true,
		IntentRelation:  "single",
	}, rows)
	if status != matchStatusNoMatchMainIntent {
		t.Fatalf("pending I11-C should not admit: %s", status)
	}
}

func TestRecommendLocalMatchAPI(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "spreadmatch")

	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "我应该接受这份工作吗？", "type": "normal"})
	payload := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/recommend", asID(reading["id"])), map[string]any{})
	items, _ := payload["recommended_spreads"].([]any)
	if payload["match_status"] != matchStatusMatched || len(items) != 1 {
		t.Fatalf("specific recommend: %#v", payload)
	}
	item := items[0].(map[string]any)
	if item["name"] != "单张牌占卜" || item["recommended"] != true || item["card_count"] == nil {
		t.Fatalf("matched item: %#v", item)
	}
	if payload["mapping_status"] != "draft_v1" || payload["main_intent"] != "I01" || payload["main_structure"] != "S01" {
		t.Fatalf("snapshot fields: %#v", payload)
	}
	if payload["reason"] != "local_match" && payload["reason"] != "no_entitlement" {
		// member may be false for fresh user; matched still returns local_match
		t.Fatalf("reason: %#v", payload["reason"])
	}

	broad := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "最近运势如何", "type": "normal"})
	broadRec := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/recommend", asID(broad["id"])), map[string]any{})
	if broadRec["match_status"] != matchStatusInputNotMatchable || len(broadRec["recommended_spreads"].([]any)) != 0 {
		t.Fatalf("broad: %#v", broadRec)
	}

	invalid := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "????", "type": "normal"})
	invalidRec := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/recommend", asID(invalid["id"])), map[string]any{})
	if invalidRec["match_status"] != matchStatusInputNotMatchable {
		t.Fatalf("invalid: %#v", invalidRec)
	}
	_ = http.StatusOK
}
