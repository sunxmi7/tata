package readingflow

import (
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
)

func templateFor(task string) *prompt.DefaultChatTemplate {
	system := "你是塔塔的塔罗解读。只解读已经固定的牌面、牌位和正逆位。不得改牌，不要套用塔罗师人设。逐张解释后给总评。"
	switch task {
	case "detect":
		system = "你是塔塔的问题检测。只判断这个问题是否需要改写，回答「需要修改」或「不需要修改」，并给一句理由。不要抽牌，不要改牌。"
	case "rewrite":
		system = "你是塔塔的问题改写。给出三条改写建议。用户可以不采用。不要抽牌，不要改牌。"
	case "recommend":
		system = "你是塔塔的牌阵推荐。根据问题给出牌阵建议。不要自动确定用户的选择，不要抽牌，不要改牌。"
	case "tarotist":
		system = "你是塔塔的 AI 塔罗师。先按已固定的牌面、牌位、正逆位解读，再用人设表达。不得改牌。多个气泡属于同一次回答。"
	}
	return prompt.FromMessages(schema.FString,
		schema.SystemMessage(system),
		schema.UserMessage("问题：{question}\n牌面：{cards}\n人设：{persona}"),
	)
}
