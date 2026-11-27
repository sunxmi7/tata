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
		{"他还会联系我吗", "I01", "S01", false, detectionStatusSpecific},
		{"今晚适合表白吗", "I01", "S01", false, detectionStatusSpecific},
		{"我应该分手吗", "I01", "S01", false, detectionStatusSpecific},
		{"A和B我选哪个", "I11", "S05", true, detectionStatusSpecific},
		{"这段关系的过去现在未来", "I04A", "S02", false, detectionStatusSpecific},
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
	if broadRec["match_status"] != matchStatusFallback || len(broadRec["recommended_spreads"].([]any)) != 1 {
		t.Fatalf("broad should fallback to 1 spread: %#v", broadRec)
	}
	if broadRec["fallback_reason"] != matchStatusInputNotMatchable {
		t.Fatalf("broad fallback_reason: %#v", broadRec["fallback_reason"])
	}

	invalid := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "????", "type": "normal"})
	invalidRec := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/recommend", asID(invalid["id"])), map[string]any{})
	if invalidRec["match_status"] != matchStatusFallback || len(invalidRec["recommended_spreads"].([]any)) != 1 {
		t.Fatalf("invalid should fallback: %#v", invalidRec)
	}
	_ = http.StatusOK
}

func TestSpreadChoicesFallbackKeepsRecommended(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "choicesfallback")
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "最近运势如何", "type": "normal"})
	choices := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/spread-choices", asID(reading["id"])))
	msgs, _ := choices["messages"].([]any)
	spreads, _ := msgs[0].(map[string]any)["spreads"].([]any)
	recCount := 0
	for _, raw := range spreads {
		if raw.(map[string]any)["recommended"] == true {
			recCount++
		}
	}
	if recCount != 1 {
		t.Fatalf("broad spread-choices should still mark 1 recommended: count=%d %#v", recCount, spreads)
	}
}

func TestExploreSpreadChoicesUsesThemeSpreads(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "explorechoices")
	var themeSpreadID int64
	if err := store.QueryRow(t.Context(), `SELECT id FROM spreads WHERE theme_key='yes_or_no' AND enabled=true ORDER BY id LIMIT 1`).Scan(&themeSpreadID); err != nil {
		t.Fatal(err)
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{
		"type": "explore", "source": "explore", "theme_key": "yes_or_no",
	})
	rid := asID(reading["id"])
	choices := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/spread-choices", rid))
	msgs, _ := choices["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages: %#v", choices)
	}
	choose := msgs[0].(map[string]any)
	if choose["draw_modes"] == nil || choose["draw_mode_label"] == nil {
		t.Fatalf("explore should expose draw_modes: %#v", choose)
	}
	if choose["title"] != "先确认牌阵和张数，再选择抽牌方式。" {
		t.Fatalf("explore title: %#v", choose["title"])
	}
	spreads, _ := choose["spreads"].([]any)
	if len(spreads) == 0 {
		t.Fatalf("empty explore spreads: %#v", choose)
	}
	found := false
	for _, raw := range spreads {
		row := raw.(map[string]any)
		if asID(row["id"]) == themeSpreadID {
			found = true
		}
		// Must not leak normal (theme_key='') catalog ids that selectSpread rejects.
		if name, _ := row["name"].(string); name == "时间流" || name == "单张牌占卜" {
			t.Fatalf("explore choices leaked normal spread: %#v", row)
		}
	}
	if !found {
		t.Fatalf("missing yes_or_no spread %d: %#v", themeSpreadID, spreads)
	}
	must(t, ts, token, http.MethodPost, fmt.Sprintf("/api/app/v1/readings/%d/spread", rid), map[string]any{
		"spread_id": themeSpreadID,
	})
}

func TestExploreSelectSpreadAllowsNumerology(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "explorenumerology")
	var themeSpreadID int64
	if err := store.QueryRow(t.Context(), `SELECT id FROM spreads WHERE theme_key='yes_or_no' AND enabled=true ORDER BY id LIMIT 1`).Scan(&themeSpreadID); err != nil {
		t.Fatal(err)
	}
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{
		"type": "explore", "source": "explore", "theme_key": "yes_or_no",
	})
	rid := asID(reading["id"])
	got := create(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/spread", rid), map[string]any{
		"spread_id": themeSpreadID, "draw_mode": "numerology",
	})
	if got["draw_mode"] != "numerology" {
		t.Fatalf("draw_mode: %#v", got["draw_mode"])
	}
	if got["status"] != "SPREAD_SELECTED" {
		t.Fatalf("status: %#v", got["status"])
	}
	if got["continue"] != "numerology" {
		t.Fatalf("continue: %#v want numerology", got["continue"])
	}
	if got["stage"] != "pending_numerology" {
		t.Fatalf("stage: %#v", got["stage"])
	}
}

func TestSpreadChoicesOverlaysLocalRecommend(t *testing.T) {
	store, ts := newMergeServer(t)
	token, _ := registerUser(t, ts, store, "choicesmatch")
	reading := create(t, ts, token, "/api/app/v1/readings", map[string]any{"question": "我应该接受这份工作吗？", "type": "normal"})
	rid := asID(reading["id"])
	choices := listOne(t, ts, token, fmt.Sprintf("/api/app/v1/readings/%d/spread-choices", rid))
	msgs, _ := choices["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatalf("no messages: %#v", choices)
	}
	choose := msgs[0].(map[string]any)
	spreads, _ := choose["spreads"].([]any)
	if len(spreads) == 0 {
		t.Fatalf("empty spreads: %#v", choose)
	}
	first := spreads[0].(map[string]any)
	if first["name"] != "单张牌占卜" || first["recommended"] != true {
		t.Fatalf("matched first: %#v", first)
	}
	if first["positions"] == nil {
		t.Fatalf("positions missing: %#v", first)
	}
	if choose["more_spreads"] != nil {
		t.Fatalf("more_spreads should be omitted when full list returned: %#v", choose["more_spreads"])
	}
	if choices["deck"] == nil && choices["deck_back_url"] == nil {
		t.Fatalf("deck back missing: %#v", choices)
	}
	recCount := 0
	for _, raw := range spreads {
		if raw.(map[string]any)["recommended"] == true {
			recCount++
		}
	}
	if recCount != 1 {
		t.Fatalf("recommended count=%d spreads=%#v", recCount, spreads)
	}
}
