package tata

import (
	"regexp"
	"strings"
	"unicode"
)

type questionKind string

const (
	questionInvalid  questionKind = "invalid"
	questionSpecific questionKind = "specific"
	questionBroad    questionKind = "broad"
)

type questionDetectResult struct {
	Kind       questionKind
	Normalized string
	TimeScope  string
	Scenes     []string
	DetectText string
}

type presetRow struct {
	body, category, scene string
}

var reTimeScope = regexp.MustCompile(`(?i)(\d{1,2}\s*月|今年|明年|本月|这个月|下个月|最近|这几天|这周|本周|下周|这三个月|接下来三个月|接下来\s*\d+\s*个?月|未来一年|半年|年底)`)

var sceneKeywords = []struct {
	scene string
	words []string
}{
	{"relationship", []string{"感情", "恋爱", "分手", "复合", "相亲", "暗恋", "喜欢的人", "男友", "女友", "男朋友", "女朋友", "对象", "伴侣", "结婚", "婚姻", "桃花", "前任"}},
	{"career", []string{"工作", "事业", "职场", "升职", "加薪", "跳槽", "面试", "老板", "同事", "创业", "项目", "业绩", "裁员", "求职"}},
	{"study", []string{"学业", "考试", "学习", "功课", "作业", "考研", "高考", "留学", "成绩", "论文", "学校", "老师"}},
	{"growth", []string{"自我", "成长", "心态", "情绪", "焦虑", "迷茫", "动力", "习惯", "疗愈", "自信"}},
	{"social", []string{"人际", "朋友", "社交", "同事关系", "家人", "父母", "亲戚", "闺蜜", "兄弟"}},
	{"life", []string{"生活", "运势", "财运", "财富", "金钱", "理财", "买房", "搬家", "健康", "出行", "选择", "日常"}},
}

func normalizeQuestion(raw string) string {
	q := strings.TrimSpace(raw)
	if q == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(q))
	prevSpace := false
	prevPunct := rune(0)
	for _, r := range q {
		switch {
		case unicode.IsSpace(r):
			if !prevSpace && b.Len() > 0 {
				b.WriteRune(' ')
				prevSpace = true
			}
			prevPunct = 0
		case strings.ContainsRune(`。！？!?；;，,、：:…·~～`, r):
			if prevPunct == r {
				continue
			}
			b.WriteRune(r)
			prevPunct = r
			prevSpace = false
		default:
			b.WriteRune(r)
			prevPunct = 0
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

func isInvalidQuestion(q string) bool {
	if q == "" {
		return true
	}
	hasLetter := false
	hasDigit := false
	for _, r := range q {
		switch {
		case unicode.Is(unicode.Han, r) || unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter && !hasDigit {
		return true
	}
	if !hasLetter && hasDigit {
		return true
	}
	compact := strings.ReplaceAll(q, " ", "")
	runes := []rune(compact)
	if len(runes) >= 4 {
		allSame := true
		for i := 1; i < len(runes); i++ {
			if runes[i] != runes[0] {
				allSame = false
				break
			}
		}
		if allSame {
			return true
		}
	}
	return false
}

func isSpecificQuestion(q string) bool {
	specificHints := []string{
		"要不要", "该不该", "应不应该", "能不能", "会不会", "能否", "可不可以", "有没有可能",
		"选a还是b", "选A还是B", "选一还是", "还是选", "选哪个", "哪个更好", "二选一",
		"接受这份", "辞掉", "辞职", "跳槽吗", "分手吗", "复合吗",
		"通过吗", "成功吗", "顺利吗", "适合吗", "联系我吗", "表白吗",
		"对我什么感觉", "对我是什么感觉", "对我的感觉", "对我的态度", "怎么看我", "怎么想我",
		"真实态度", "心里怎么想", "什么感觉",
		"如何提升", "怎么提升", "该如何提升", "如何改善", "怎么改善", "该怎么办", "怎么办",
		"有没有新工作", "会有新的工作机会", "有没有机会",
		"该买吗", "要不要买", "该不该买",
		"过去现在未来", "过去·现在·未来", "发展趋势", "会怎样",
	}
	lower := strings.ToLower(q)
	for _, h := range specificHints {
		if strings.Contains(lower, strings.ToLower(h)) {
			return true
		}
	}
	if strings.Contains(q, "还是") && (strings.Contains(q, "还是选") || strings.Contains(q, "还是去") || strings.Contains(q, "还是留") || strings.Contains(q, "还是继续") || strings.Contains(q, "还是换")) {
		return true
	}
	// Yes/no style: clear ask ending with 吗/麼 and a decision verb.
	trimmed := strings.TrimRight(q, "？?！!。．.、 ")
	if strings.HasSuffix(trimmed, "吗") || strings.HasSuffix(trimmed, "麼") || strings.HasSuffix(trimmed, "么") {
		if containsAny(q, "会", "能", "该", "要", "适合", "应该", "可以", "愿意", "想", "敢", "值得", "联系", "表白", "分手", "复合", "接受", "去", "留", "买", "换", "表白") {
			return true
		}
	}
	return false
}

func detectScenes(q string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, item := range sceneKeywords {
		for _, w := range item.words {
			if strings.Contains(q, w) {
				if !seen[item.scene] {
					seen[item.scene] = true
					out = append(out, item.scene)
				}
				break
			}
		}
	}
	return out
}

func detectTimeScope(q string) string {
	m := reTimeScope.FindString(q)
	return strings.TrimSpace(m)
}

func detectQuestion(raw string) questionDetectResult {
	normalized := normalizeQuestion(raw)
	res := questionDetectResult{Normalized: normalized, TimeScope: detectTimeScope(normalized), Scenes: detectScenes(normalized)}
	if isInvalidQuestion(normalized) {
		res.Kind = questionInvalid
		res.DetectText = "这个问题还看不出你想问什么。请用一句话写出你关心的事情或选择，再试一次。"
		return res
	}
	if isSpecificQuestion(normalized) {
		res.Kind = questionSpecific
		res.DetectText = "你的问题已经足够具体，可以直接进入选阵占卜。"
		return res
	}
	res.Kind = questionBroad
	switch {
	case res.TimeScope != "" && len(res.Scenes) > 0:
		res.DetectText = "你的问题偏宽泛。已保留「" + res.TimeScope + "」这个时间范围，建议先明确更具体的方向，解读会更聚焦。"
	case len(res.Scenes) == 1:
		title := sceneTitle(res.Scenes[0])
		res.DetectText = "你的问题偏宽泛，目前主要落在「" + title + "」。明确更具体的子方向，可以帮助解读更聚焦。"
	case len(res.Scenes) > 1:
		res.DetectText = "你的问题同时涉及多个方向。建议先选一个最想问清的主题，解读会更具体。"
	default:
		res.DetectText = "你的问题涉及的范围比较广，明确你最关注的方向，可以帮助解读更聚焦，也更容易获得具体的建议。"
	}
	return res
}

func sceneTitle(scene string) string {
	for _, d := range presetSceneDefs {
		if d.scene == scene {
			return d.title
		}
	}
	return scene
}

func refineBroadSuggestions(det questionDetectResult, presets []presetRow) []presetRow {
	const want = 3
	out := make([]presetRow, 0, want)
	seen := map[string]bool{}
	add := func(body, category, scene string) {
		body = strings.TrimSpace(body)
		if body == "" || seen[body] || len(out) >= want {
			return
		}
		seen[body] = true
		if category == "" {
			category = sceneTitle(scene)
		}
		out = append(out, presetRow{body: body, category: category, scene: scene})
	}

	scenes := det.Scenes
	if len(scenes) == 0 {
		scenes = []string{"relationship", "career", "life"}
	}
	if det.TimeScope != "" {
		templates := map[string]string{
			"relationship": det.TimeScope + "的感情运势怎么样？",
			"career":       det.TimeScope + "的工作运势怎么样？",
			"study":        det.TimeScope + "的学业运势怎么样？",
			"growth":       det.TimeScope + "我在自我成长上最该关注什么？",
			"social":       det.TimeScope + "的人际运势怎么样？",
			"life":         det.TimeScope + "的整体运势怎么样？",
		}
		for _, scene := range scenes {
			if t, ok := templates[scene]; ok {
				add(t, sceneTitle(scene), scene)
			}
		}
		for _, scene := range []string{"relationship", "career", "life", "growth", "study", "social"} {
			if len(out) >= want {
				break
			}
			if t, ok := templates[scene]; ok {
				add(t, sceneTitle(scene), scene)
			}
		}
	}

	sceneSet := map[string]bool{}
	for _, s := range scenes {
		sceneSet[s] = true
	}
	for _, p := range presets {
		if len(out) >= want {
			break
		}
		if len(sceneSet) > 0 && !sceneSet[p.scene] {
			continue
		}
		add(p.body, p.category, p.scene)
	}
	for _, p := range presets {
		if len(out) >= want {
			break
		}
		add(p.body, p.category, p.scene)
	}
	return out
}
