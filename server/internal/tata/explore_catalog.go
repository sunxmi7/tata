package tata

import (
	"context"
	"encoding/json"
	"strings"
)

type prdExplore struct {
	Key, Title, Subtitle string
	Name, Short, Desc    string
	Cards, Sort          int
	Positions            []string
	Questions            string
}

func prdExploreSpreads() []prdExplore {
	q := func(s string) string { return s }
	return []prdExplore{
		{"yes_or_no", "Yes or No", "听听此刻的倾向", "单张是非牌阵", "快速看清当下倾向", "看一个明确、范围较小的二元问题，当前倾向是什么。", 1, 101,
			[]string{"当前问题的答案倾向与核心影响因素"},
			q("他近期会主动联系我吗？／这次面试有机会通过吗？／我现在适合换工作吗？／这段关系还有继续发展的可能吗？")},
		{"either_or", "二选一", "换个角度看选择", "双项对照牌阵", "先看两个选项的总体倾向", "两个选项已经明确时，先比一个总体倾向。", 3, 111,
			[]string{"选择 A 的发展倾向", "选择 B 的发展倾向", "综合建议与需要权衡的重点"},
			q("我应该留在现在的公司还是换工作？／我应该选择 A 学校还是 B 学校？／我应该主动联系还是继续等待？／我应该买这款还是另一款？")},
		{"either_or", "二选一", "换个角度看选择", "双路径决策牌阵", "分别看清 A / B 两条路", "分别看 A、B 的优势、挑战和可能发展，不只比表面好坏。", 7, 112,
			[]string{"当前选择的核心问题", "A 路径的优势", "A 路径的挑战或代价", "A 路径的可能发展", "B 路径的优势", "B 路径的挑战或代价", "B 路径的可能发展"},
			q("两个工作机会哪个更适合我？／继续这段关系还是选择放手？／留在现在的城市还是去新的城市？／两个发展方向分别需要我付出什么？")},
		{"relationship", "关系走向", "看见关系状态，找到更适合你的理解方式", "过去·现在·未来", "用时间线看这段关系", "用时间线看关系怎么形成、现在怎样、按现有趋势会怎么变。", 3, 121,
			[]string{"过去：关系形成的基础或重要经历", "现在：双方目前的互动状态", "未来：按照当前趋势可能出现的发展"},
			q("我们接下来三个月会有什么变化？／这段关系目前处于什么阶段？／我和他未来的发展趋势是什么？／我们之间发生的变化意味着什么？")},
		{"relationship", "关系走向", "看见关系状态，找到更适合你的理解方式", "关系互动牌阵", "看双方、连接和阻碍", "看双方状态、连接、助力与阻碍，不把解读收成猜对方怎么想。", 6, 122,
			[]string{"我在这段关系中的状态", "对方在这段关系中的状态或表现", "双方之间的连接与互动", "关系中的有利因素", "关系中的主要挑战", "关系的发展方向与建议"},
			q("他现在是怎么看待这段关系的？／我们之间的连接和阻碍分别是什么？／这段关系还有进一步发展的可能吗？／我可以怎样改善目前的关系？")},
		{"communication", "沟通建议", "找到表达方式", "沟通三张牌阵", "理清这一次怎么说", "聚焦一次具体沟通，梳理要说什么、怎么说、可能带来什么影响。", 3, 131,
			[]string{"当前沟通的核心情况", "我需要表达或理解的重点", "更合适的沟通方式与可能影响"},
			q("我应该如何表达自己的真实想法？／这次谈话有什么需要注意的？／我现在应该主动沟通吗？／怎样才能减少彼此之间的误解？")},
		{"communication", "沟通建议", "找到表达方式", "沟通桥梁牌阵", "找到可执行的沟通入口", "从双方立场、误解和共同点里找一个能执行的沟通入口。", 5, 132,
			[]string{"我的立场与感受", "对方可能呈现的立场", "阻碍沟通的误解或因素", "双方可以建立的共同点", "建议采取的沟通方式"},
			q("我们为什么总是无法好好沟通？／这次争执应该如何化解？／我应该怎样和家人解决长期分歧？／怎样表达才能减少对方的防御感？")},
		{"career", "职业方向", "梳理下一步", "职业发展牌阵", "看清下一步怎么走", "从当前位置、优势、阻碍、机会到下一步行动。", 5, 141,
			[]string{"当前职业位置与状态", "自身优势及可利用资源", "目前的职业阻碍", "潜在机会与发展方向", "下一步行动建议"},
			q("我未来三个月的工作会有哪些机会？／我适合继续留在现在的岗位吗？／我目前最大的职业阻碍是什么？／我应该如何提升职业竞争力？")},
		{"career", "职业方向", "梳理下一步", "凯尔特十字牌阵", "复杂职业问题的全景梳理", "因素多、情况复杂的职业问题。固定十张版本。", 10, 142,
			[]string{"当前核心处境", "横跨当前处境的挑战", "深层基础或根源", "近期过去的影响", "意识目标或关注重点", "近期未来的发展", "我在事件中的状态", "外部环境与他人影响", "希望与担忧", "按当前路径发展的可能结果"},
			q("我应该如何看待目前的职业困境？／未来一段时间我的职业发展会如何变化？／这份工作是否值得我继续投入？／我目前没有看见的职业影响因素是什么？")},
		{"study", "学习状态", "调整学习节奏", "学习三张牌阵", "快速看清学习卡点", "快速看学习状态、主要阻碍和可以怎么调。", 3, 151,
			[]string{"当前学习状态", "目前最大的学习阻碍", "改善学习状态的建议"},
			q("我最近的学习状态怎么样？／我应该如何提高学习效率？／我目前最需要解决的学习问题是什么？／我应该把精力放在哪个知识点上？")},
		{"study", "学习状态", "调整学习节奏", "马蹄形学习牌阵", "把学习进程摊开看", "七张马蹄形，从过去看到可能发展。", 7, 152,
			[]string{"过去对当前学习的影响", "近期学习状态", "尚未察觉的影响因素", "当前主要障碍", "外部环境与可用资源", "应对建议", "按当前状态发展的可能结果"},
			q("我的学习状态接下来会如何变化？／目前的学习困境会怎样发展？／我应该如何调整学习规划？／哪些外部因素正在影响我的学习？")},
		{"interpersonal", "人际相处", "理清相处边界", "人际互动三张牌阵", "看清彼此互动模式", "看自己、对方和互动模式。用于朋友、同事、家庭。", 3, 161,
			[]string{"我在这段关系中的状态", "对方在关系中的表现或立场", "双方目前形成的互动模式"},
			q("我在人际关系中需要注意什么？／我和朋友之间的问题出在哪里？／我应该如何改善与同事的关系？／为什么我总是难以融入一个圈子？")},
		{"interpersonal", "人际相处", "理清相处边界", "冲突解决牌阵", "拆开冲突找到解法", "把冲突拆成双方需求、根源、共同点和解决方式。", 5, 162,
			[]string{"冲突的核心问题", "我的立场与需求", "对方可能的立场与需求", "双方可以建立的共同点", "解决冲突的可行方式"},
			q("我和朋友之间的矛盾应该如何处理？／我与同事之间的关系还有改善空间吗？／这段人际关系让我疲惫，我该怎么做？／怎样在维护关系的同时表达自己的边界？")},
		{"shopping", "购物抉择", "看清购买动机", "购买决策三张牌阵", "分清需求还是冲动", "区分真实需求和冲动。不替代商品信息比较。", 3, 171,
			[]string{"我真正的购买需求", "购买后可能获得的价值或需要注意的代价", "综合建议：现在购买、暂缓或进一步比较"},
			q("这件东西真的值得买吗？／我现在购买合适吗？／这次消费是否符合我的真实需求？／我是不是因为一时冲动才想买？")},
		{"shopping", "购物抉择", "看清购买动机", "购买决策牌阵", "更完整地权衡这笔消费", "需求、价值、成本、替代方案、决策建议。不替代真实比价。", 5, 172,
			[]string{"真实需求与购买动机", "购买可能带来的价值", "潜在成本、限制或风险", "替代选择或暂缓购买的意义", "综合决策建议"},
			q("我应该现在买还是再等等？／这件商品是否值得我投入这笔钱？／有没有更适合我的替代选择？／我需要先确认哪些信息再决定？")},
		{"outfit", "穿搭灵感", "找到今日风格", "今日风格牌阵", "把牌义变成今日穿搭灵感", "转成当天的风格、色彩和搭配重点。给灵感，不做预测。", 3, 181,
			[]string{"今日适合呈现的整体气质", "适合尝试的色彩或风格方向", "值得突出的搭配细节"},
			q("今天适合尝试什么穿搭风格？／今天适合什么颜色？／今天如何搭配更有个人特色？／我想换个风格，可以从哪里开始？")},
		{"outfit", "穿搭灵感", "找到今日风格", "个人风格探索牌阵", "梳理更长期的风格方向", "气质、风格方向、色彩材质、廓形、配饰。", 5, 182,
			[]string{"个人气质与风格基础", "值得探索的风格方向", "适合尝试的色彩与材质", "适合的服装廓形或比例", "配饰与整体搭配建议"},
			q("我适合什么样的穿衣风格？／什么颜色更能体现我的气质？／我应该如何建立自己的穿搭风格？／我适合尝试哪种新的造型方向？")},
		{"wellbeing", "身心关照", "给自己一点照顾", "身心灵牌阵", "关照心智、身体与意义", "心智、身体感受、内在意义。不做医学判断。", 3, 191,
			[]string{"心智：当前想法与精神状态", "身体：身体感受与生活节奏", "内在：精神需求与自我关照方向"},
			q("我目前整体状态怎么样？／我需要关注自己的哪一部分？／如何让生活状态更加平衡？／我现在最需要给自己什么支持？")},
		{"wellbeing", "身心关照", "给自己一点照顾", "身心平衡牌阵", "找到可执行的日常关照", "压力来源、被忽略的需求、支持资源、可执行的日常建议。不做医学判断。", 5, 192,
			[]string{"当前整体状态", "近期压力的主要来源", "被忽略的情绪或生活需求", "可以利用的支持资源", "改善状态的具体行动建议"},
			q("最近为什么总觉得疲惫？／我应该如何调整生活节奏？／我现在最需要给自己什么支持？／怎样为自己留出更好的休息空间？")},
		{"inner", "内心需求", "听见自己的心", "内心三问牌阵", "分清愿望、需求与行动", "分开已经知道的愿望、还没察觉的需求和可以做的行动。", 3, 201,
			[]string{"我目前意识到的需求", "我可能尚未察觉的真实需求", "我可以采取的下一步行动"},
			q("我真正想要的是什么？／我目前最需要什么？／为什么我总觉得不够满足？／有什么需求一直被我忽略了？")},
		{"inner", "内心需求", "听见自己的心", "阴影探索牌阵", "看见反复出现的内在模式", "反复出现的内在模式。不能解释成心理诊断。", 5, 202,
			[]string{"当前反复出现的内在模式", "触发这种模式的因素", "这种模式可能试图保护的部分", "被压抑或忽略的真实需求", "可以开始练习的接纳与整合行动"},
			q("为什么我总在同一件事上反复纠结？／有哪些情绪是我一直没有面对的？／我为什么会反复陷入这种关系模式？／我可以怎样接纳自己不喜欢的部分？")},
	}
}

func namedPositions(names []string) string {
	pos := make([]position, len(names))
	for i, name := range names {
		pos[i] = position{Index: i + 1, Name: name, Meaning: name}
	}
	b, _ := json.Marshal(pos)
	return string(b)
}

func questionList(raw string) string {
	parts := strings.Split(raw, "／")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	b, _ := json.Marshal(items)
	return string(b)
}

func (s *Store) seedExploreSpreads(ctx context.Context) error {
	wanted := map[string]bool{}
	for _, item := range prdExploreSpreads() {
		key := item.Key + "\x00" + item.Name
		wanted[key] = true
		positions := namedPositions(item.Positions)
		questions := questionList(item.Questions)
		var n int
		if err := s.QueryRow(ctx, `SELECT count(*) FROM spreads WHERE theme_key=$1 AND name=$2`, item.Key, item.Name).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := s.Exec(ctx, `INSERT INTO spreads(name, description, card_count, positions, quick_questions, enabled, charged, recommended, theme_key, theme_title, theme_subtitle, short_sentence, depth_group, quota_cost, sort_order)
				VALUES ($1,$2,$3,$4,$5,true,false,false,$6,$7,$8,$9,'',NULL,$10)`,
				item.Name, item.Desc, item.Cards, positions, questions, item.Key, item.Title, item.Subtitle, item.Short, item.Sort); err != nil {
				return err
			}
			continue
		}
		if _, err := s.Exec(ctx, `UPDATE spreads SET description=$1, card_count=$2, positions=$3, quick_questions=$4, theme_title=$5, theme_subtitle=$6, short_sentence=$7, sort_order=$8, enabled=true
			WHERE theme_key=$9 AND name=$10`,
			item.Desc, item.Cards, positions, questions, item.Title, item.Subtitle, item.Short, item.Sort, item.Key, item.Name); err != nil {
			return err
		}
	}
	rows, err := s.Query(ctx, `SELECT theme_key, name FROM spreads WHERE theme_key<>''`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type pair struct{ key, name string }
	var stale []pair
	for rows.Next() {
		var key, name string
		if err := rows.Scan(&key, &name); err != nil {
			return err
		}
		if !wanted[key+"\x00"+name] {
			stale = append(stale, pair{key, name})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range stale {
		if _, err := s.Exec(ctx, `UPDATE spreads SET enabled=false WHERE theme_key=$1 AND name=$2`, item.key, item.name); err != nil {
			return err
		}
	}
	return nil
}
