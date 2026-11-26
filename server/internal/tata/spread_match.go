package tata

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

//go:embed spread_match_config.json
var spreadMatchConfigJSON []byte

const (
	matchStatusMatched           = "MATCHED"
	matchStatusNoMatch           = "NO_MATCH"
	matchStatusNoMatchMainIntent = "NO_MATCH_MAIN_INTENT"
	matchStatusInputNotMatchable = "INPUT_NOT_MATCHABLE"
	matchStatusFallback          = "FALLBACK"
	detectionStatusInvalid       = "invalid"
	detectionStatusBroad         = "broad"
	detectionStatusSpecific      = "specific"
	coverageDirect               = "D"
	coverageCombined             = "C"
	mappingValidationSupported   = "supported"
	structureMatchYes            = "MATCH"
	structureMatchNo             = "NO_MATCH"
)

type intentCoverage struct {
	Coverage string `json:"coverage"`
	Status   string `json:"status"`
}

type spreadCatalogEntry struct {
	Code            string                    `json:"code"`
	Names           []string                  `json:"names"`
	Recommendable   bool                      `json:"recommendable"`
	StructuresMatch []string                  `json:"structures_match"`
	Intents         map[string]intentCoverage `json:"intents"`
}

type spreadMatchConfig struct {
	Version             string               `json:"version"`
	MappingStatus       string               `json:"mapping_status"`
	PriorityStatus      string               `json:"priority_status"`
	PriorityByStructure map[string][]string  `json:"priority_by_structure"`
	Spreads             []spreadCatalogEntry `json:"spreads"`
	byName              map[string]*spreadCatalogEntry
}

type questionMatchSnapshot struct {
	FormalQuestion   string
	DetectionStatus  string
	MainIntent       string
	MainStructure    string
	SecondaryIntents []string
	HasDualOptions   bool
	IntentRelation   string
}

type dbSpreadRow struct {
	ID    int64
	Name  string
	Desc  string
	Short string
	Count int
	Thumb string
	Depth string
	DBRec bool
}

type matchCandidate struct {
	row            dbSpreadRow
	cfg            *spreadCatalogEntry
	mainCoverage   string
	secondaryCount int
	structureMatch string
	priorityRank   int
}

var embeddedSpreadMatchConfig = mustLoadSpreadMatchConfig()

func mustLoadSpreadMatchConfig() *spreadMatchConfig {
	cfg, err := parseSpreadMatchConfig(spreadMatchConfigJSON)
	if err != nil {
		panic("spread_match_config: " + err.Error())
	}
	return cfg
}

func parseSpreadMatchConfig(raw []byte) (*spreadMatchConfig, error) {
	var cfg spreadMatchConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	cfg.byName = map[string]*spreadCatalogEntry{}
	for i := range cfg.Spreads {
		entry := &cfg.Spreads[i]
		for _, name := range entry.Names {
			key := normalizeSpreadName(name)
			if key == "" {
				continue
			}
			cfg.byName[key] = entry
		}
	}
	return &cfg, nil
}

func normalizeSpreadName(name string) string {
	s := strings.TrimSpace(name)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "·", "")
	s = strings.ReplaceAll(s, "•", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "／", "")
	return strings.ToLower(s)
}

// buildQuestionMatchSnapshot is the question-detection layer for recommend.
// It reuses detectQuestion for invalid/specific/broad, then applies keyword
// heuristics only for specific questions to emit main_intent / main_structure.
func buildQuestionMatchSnapshot(raw string) questionMatchSnapshot {
	det := detectQuestion(raw)
	snap := questionMatchSnapshot{
		FormalQuestion:  det.Normalized,
		DetectionStatus: string(det.Kind),
		IntentRelation:  "single",
	}
	switch det.Kind {
	case questionInvalid:
		snap.DetectionStatus = detectionStatusInvalid
		return snap
	case questionBroad:
		snap.DetectionStatus = detectionStatusBroad
		return snap
	}
	snap.DetectionStatus = detectionStatusSpecific
	intent, structure, dual := inferIntentStructure(det.Normalized)
	snap.MainIntent = intent
	snap.MainStructure = structure
	snap.HasDualOptions = dual
	return snap
}

// inferIntentStructure is the keyword heuristic layer (question-detection side).
// The matcher only consumes the resulting snapshot fields.
func inferIntentStructure(q string) (intent, structure string, dual bool) {
	lower := strings.ToLower(q)
	switch {
	case hasDualOptionHints(q, lower):
		return "I11", "S05", true
	case containsAny(q, "为什么", "原因", "根源", "起因"):
		if strings.Contains(q, "根源") || strings.Contains(q, "深层") {
			return "I05B", "S03", false
		}
		return "I05A", "S03", false
	case containsAny(q, "怎么办", "如何提升", "怎么提升", "如何改善", "怎么改善", "该怎么", "建议"):
		return "I10A", "S04", false
	case containsAny(q, "过去现在未来", "过去·现在·未来", "会怎样", "接下来", "未来", "发展趋势", "走向"):
		if containsAny(q, "结果", "结局", "最终") {
			return "I04B", "S02", false
		}
		return "I04A", "S02", false
	case containsAny(q, "对我什么感觉", "对我是什么感觉", "对我的感觉", "对我的态度", "怎么看我", "怎么想我", "真实态度"):
		return "I12B", "S06", false
	case containsAny(q, "要不要", "该不该", "应不应该", "能不能", "会不会", "能否", "适合吗", "联系我吗", "表白吗", "分手吗", "复合吗") ||
		(strings.HasSuffix(strings.TrimRight(q, "？?！!。．.、 "), "吗") && containsAny(q, "应该", "接受", "辞", "买", "去", "留", "会", "能", "适合", "联系", "表白", "分手", "复合", "值得")):
		return "I01", "S01", false
	case containsAny(q, "关系", "他", "她", "对方", "男友", "女友", "对象", "伴侣", "感情"):
		return "I13A", "S06", false
	default:
		return "I01", "S01", false
	}
}

func hasDualOptionHints(q, lower string) bool {
	if containsAny(q, "选A还是B", "选a还是b", "两个选项", "选项A", "选项B", "A还是B", "a还是b", "选哪个", "哪个更好", "二选一") {
		return true
	}
	if strings.Contains(q, "还是") && containsAny(q, "还是选", "还是去", "还是留", "还是换", "还是继续") {
		return true
	}
	if strings.Contains(lower, "还是") && (strings.Contains(lower, "选") || strings.Count(q, "还是") >= 1) {
		// "A还是B" style without the specific phrases above.
		if strings.Contains(q, "还是") && !strings.Contains(q, "还是不是") {
			left := strings.ContainsAny(q, "ABCDabcd") || containsAny(q, "留", "走", "去", "换", "继续", "辞职", "接受")
			return left
		}
	}
	return false
}

func containsAny(s string, words ...string) bool {
	for _, w := range words {
		if w != "" && strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func (cfg *spreadMatchConfig) lookupByName(name string) *spreadCatalogEntry {
	if cfg == nil {
		return nil
	}
	return cfg.byName[normalizeSpreadName(name)]
}

func (cfg *spreadMatchConfig) admitCoverage(entry *spreadCatalogEntry, intent string) (string, bool) {
	if entry == nil || intent == "" {
		return "", false
	}
	cov, ok := entry.Intents[intent]
	if !ok {
		return "", false
	}
	if cov.Status != mappingValidationSupported {
		return "", false
	}
	if cov.Coverage != coverageDirect && cov.Coverage != coverageCombined {
		return "", false
	}
	return cov.Coverage, true
}

func (cfg *spreadMatchConfig) structureMatchStatus(entry *spreadCatalogEntry, structure string) string {
	if entry == nil || structure == "" {
		return structureMatchNo
	}
	for _, s := range entry.StructuresMatch {
		if s == structure {
			return structureMatchYes
		}
	}
	return structureMatchNo
}

func (cfg *spreadMatchConfig) priorityRank(structure, code string) int {
	list := cfg.PriorityByStructure[structure]
	for i, id := range list {
		if id == code {
			return i
		}
	}
	return 999
}

func (cfg *spreadMatchConfig) match(snap questionMatchSnapshot, rows []dbSpreadRow) (matchCandidate, string) {
	empty := matchCandidate{}
	if snap.DetectionStatus != detectionStatusSpecific {
		return empty, matchStatusInputNotMatchable
	}
	if strings.TrimSpace(snap.FormalQuestion) == "" || snap.MainIntent == "" || snap.MainStructure == "" {
		return empty, matchStatusInputNotMatchable
	}
	if snap.IntentRelation == "independent_multi_topic" {
		return empty, matchStatusInputNotMatchable
	}

	candidates := make([]matchCandidate, 0, len(rows))
	for _, row := range rows {
		entry := cfg.lookupByName(row.Name)
		if entry == nil || !entry.Recommendable {
			continue
		}
		coverage, ok := cfg.admitCoverage(entry, snap.MainIntent)
		if !ok {
			continue
		}
		if entry.Code == "05" || entry.Code == "17" {
			if snap.MainIntent != "I11" || !snap.HasDualOptions {
				continue
			}
		}
		sec := 0
		for _, intent := range snap.SecondaryIntents {
			if _, ok := cfg.admitCoverage(entry, intent); ok {
				sec++
			}
		}
		candidates = append(candidates, matchCandidate{
			row:            row,
			cfg:            entry,
			mainCoverage:   coverage,
			secondaryCount: sec,
			structureMatch: cfg.structureMatchStatus(entry, snap.MainStructure),
			priorityRank:   cfg.priorityRank(snap.MainStructure, entry.Code),
		})
	}
	if len(candidates) == 0 {
		return empty, matchStatusNoMatchMainIntent
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.mainCoverage != b.mainCoverage {
			return a.mainCoverage == coverageDirect && b.mainCoverage != coverageDirect
		}
		if a.secondaryCount != b.secondaryCount {
			return a.secondaryCount > b.secondaryCount
		}
		if a.structureMatch != b.structureMatch {
			return a.structureMatch == structureMatchYes && b.structureMatch != structureMatchYes
		}
		if a.priorityRank != b.priorityRank {
			return a.priorityRank < b.priorityRank
		}
		return a.row.ID < b.row.ID
	})
	return candidates[0], matchStatusMatched
}

func (a *App) loadOrdinarySpreadsForMatch(ctx context.Context) ([]dbSpreadRow, error) {
	rows, err := a.Store.Query(ctx, `SELECT id, name, description, short_sentence, card_count, recommended, thumbnail, depth_group
		FROM spreads WHERE enabled=true AND theme_key='' ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []dbSpreadRow{}
	for rows.Next() {
		var row dbSpreadRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Desc, &row.Short, &row.Count, &row.DBRec, &row.Thumb, &row.Depth); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func recommendSpreadItem(r *http.Request, row dbSpreadRow, code, coverage string) map[string]any {
	item := map[string]any{
		"id": row.ID, "name": T(r.Context(), row.Name), "description": T(r.Context(), row.Desc),
		"short_sentence": row.Short, "card_count": row.Count, "recommended": true,
		"thumbnail":   absoluteAssetURL(r, deckPreviewURL(row.Thumb)),
		"depth_group": row.Depth, "depth_label": depthLabel(row.Depth),
	}
	if code != "" {
		item["spread_code"] = code
	}
	if coverage != "" {
		item["coverage"] = coverage
	}
	return item
}

// pickFallbackSpread prefers DB recommended, then first quick-group, then first ordinary row.
func pickFallbackSpread(rows []dbSpreadRow) (dbSpreadRow, bool) {
	for _, row := range rows {
		if row.DBRec {
			return row, true
		}
	}
	for _, row := range rows {
		if row.Depth == "quick" {
			return row, true
		}
	}
	if len(rows) > 0 {
		return rows[0], true
	}
	return dbSpreadRow{}, false
}

func (a *App) attachRecommendation(r *http.Request, out map[string]any) error {
	cfg := embeddedSpreadMatchConfig
	out["recommended_spreads"] = []map[string]any{}
	out["match_status"] = matchStatusNoMatch
	out["mapping_status"] = cfg.MappingStatus
	if cfg.PriorityStatus != "" {
		out["priority_status"] = cfg.PriorityStatus
	}

	question, _ := out["question"].(string)
	snap := buildQuestionMatchSnapshot(question)
	out["detection_status"] = snap.DetectionStatus
	out["main_intent"] = snap.MainIntent
	out["main_structure"] = snap.MainStructure
	if snap.HasDualOptions {
		out["has_dual_options"] = true
	}

	rows, err := a.loadOrdinarySpreadsForMatch(r.Context())
	if err != nil {
		return err
	}

	if snap.DetectionStatus != detectionStatusSpecific || snap.MainIntent == "" || snap.MainStructure == "" {
		out["match_status"] = matchStatusInputNotMatchable
		// Broad/invalid: still give one selectable default so App 选阵屏不空。
		if row, ok := pickFallbackSpread(rows); ok {
			out["recommended_spreads"] = []map[string]any{recommendSpreadItem(r, row, "", "")}
			out["match_status"] = matchStatusFallback
			out["fallback_reason"] = matchStatusInputNotMatchable
		}
		return nil
	}

	best, status := cfg.match(snap, rows)
	out["match_status"] = status
	if status == matchStatusMatched {
		out["recommended_spreads"] = []map[string]any{
			recommendSpreadItem(r, best.row, best.cfg.Code, best.mainCoverage),
		}
		return nil
	}
	// Specific but no capability hit: fall back so recommend never returns an empty list when catalog exists.
	if row, ok := pickFallbackSpread(rows); ok {
		out["recommended_spreads"] = []map[string]any{recommendSpreadItem(r, row, "", "")}
		out["match_status"] = matchStatusFallback
		out["fallback_reason"] = status
	}
	return nil
}
