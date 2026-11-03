package tata

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed major_meanings.json
var majorMeaningsJSON []byte

type majorMeaningEntry struct {
	CardID       int      `json:"card_id"`
	Orientation  string   `json:"orientation"`
	Keywords     []string `json:"keywords"`
	MeaningShort string   `json:"meaning_short"`
}

var majorMeanings map[string]majorMeaningEntry

func init() {
	var rows []majorMeaningEntry
	if err := json.Unmarshal(majorMeaningsJSON, &rows); err != nil {
		panic("major_meanings.json: " + err.Error())
	}
	majorMeanings = make(map[string]majorMeaningEntry, len(rows))
	for _, row := range rows {
		key := majorMeaningKey(row.CardID, row.Orientation)
		if _, dup := majorMeanings[key]; dup {
			panic("duplicate major meaning: " + key)
		}
		majorMeanings[key] = row
	}
	if len(majorMeanings) != 44 {
		panic(fmt.Sprintf("major_meanings.json: want 44 rows, got %d", len(majorMeanings)))
	}
}

func majorMeaningKey(cardID int, orientation string) string {
	return fmt.Sprintf("%d:%s", cardID, orientation)
}

// LookupMajorMeaning returns short copy for major arcana card_id 0–21 only.
func LookupMajorMeaning(cardID int, orientation string) (keywords []string, meaningShort string, ok bool) {
	if cardID < 0 || cardID > 21 {
		return nil, "", false
	}
	if orientation != "upright" && orientation != "reversed" {
		return nil, "", false
	}
	entry, ok := majorMeanings[majorMeaningKey(cardID, orientation)]
	if !ok {
		return nil, "", false
	}
	return append([]string(nil), entry.Keywords...), entry.MeaningShort, true
}

func orientationLabel(orientation string) string {
	if orientation == "reversed" {
		return "逆位"
	}
	return "正位"
}

// formatMajorMeaningsBlock appends reference lines for drawn major arcana in cards JSON.
func formatMajorMeaningsBlock(cardsJSON string) string {
	if cardsJSON == "" || cardsJSON == "null" {
		return ""
	}
	var drawn []drawnCard
	if err := json.Unmarshal([]byte(cardsJSON), &drawn); err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("牌意参考（固定牌面，勿改牌义来源）：\n")
	wrote := false
	for _, c := range drawn {
		kw, meaning, ok := LookupMajorMeaning(c.CardID, c.Orientation)
		if !ok {
			continue
		}
		wrote = true
		label := c.Name
		if label == "" {
			label = fmt.Sprintf("ID%d", c.CardID)
		}
		pos := c.PositionName
		if pos == "" {
			pos = fmt.Sprintf("牌位%d", c.Position)
		}
		fmt.Fprintf(&b, "- %s｜%s，%s｜关键词：%s｜简义：%s\n",
			pos, label, orientationLabel(c.Orientation), strings.Join(kw, "、"), meaning)
	}
	if !wrote {
		return ""
	}
	return b.String()
}
