package tata

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed daily_card_copy.json
var dailyCardCopyJSON []byte

type dailyCardCopy struct {
	Name            string `json:"name"`
	UprightMessage  string `json:"upright_message"`
	UprightYi       string `json:"upright_yi"`
	ReversedMessage string `json:"reversed_message"`
	ReversedYi      string `json:"reversed_yi"`
}

func loadDailyCardCopy() map[string]dailyCardCopy {
	var list []dailyCardCopy
	if err := json.Unmarshal(dailyCardCopyJSON, &list); err != nil {
		panic("daily_card_copy.json: " + err.Error())
	}
	out := make(map[string]dailyCardCopy, len(list))
	for _, item := range list {
		out[item.Name] = item
	}
	return out
}

var dailyCardCopies = loadDailyCardCopy()

func cardCopyKey(name, field string) string {
	return "card." + name + "." + field
}

func dailyLibraryCopy(cfg map[string]string, name, ori string) (message, yi any) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	side := "upright"
	if ori == "reversed" {
		side = "reversed"
	}
	msg := strings.TrimSpace(cfg[cardCopyKey(name, side+"_message")])
	line := strings.TrimSpace(cfg[cardCopyKey(name, side+"_yi")])
	if side == "upright" {
		if override := strings.TrimSpace(cfg[cardYiKey(name)]); override != "" {
			line = override
		}
	}
	return nullableConfig(msg), nullableConfig(line)
}
