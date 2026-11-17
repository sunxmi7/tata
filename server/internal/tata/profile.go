package tata

import (
	"encoding/json"
	"strings"
)

var explorePreferenceOptions = []string{
	"感情与关系", "工作与学业", "自我成长", "人生选择",
	"日常生活", "财务与规划", "情绪与疗愈", "人际相处",
}

func normalizeGender(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "":
		return "", nil
	case "女", "女性":
		return "女性", nil
	case "男", "男性":
		return "男性", nil
	case "其他", "不愿透露":
		return "不愿透露", nil
	default:
		return "", biz("性别选项不正确")
	}
}

func parseExplorePreferences(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []string{}
	}
	var items []string
	if json.Unmarshal([]byte(raw), &items) != nil {
		return []string{}
	}
	out, err := normalizeExplorePreferences(items)
	if err != nil {
		return []string{}
	}
	return out
}

func normalizeExplorePreferences(items []string) ([]string, error) {
	allowed := map[string]bool{}
	for _, opt := range explorePreferenceOptions {
		allowed[opt] = true
	}
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !allowed[item] {
			return nil, biz("探索偏好不正确")
		}
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out, nil
}

func encodeExplorePreferences(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}
