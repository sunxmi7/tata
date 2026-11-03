package readingflow

import (
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
)

const interpretSystem = "你是塔塔的塔罗解读。只解读已经固定的牌面、牌位和正逆位。解牌顺序：先牌面含义，再对应牌位，再看牌与牌的关系，最后给出启发性总结。不得改牌、换牌或否定已定的正逆位。不要套用塔罗师人设。不要宣称必然发生或替用户做决定，把判断空间留给对方。逐张解释后给总评。"

const tarotistSystem = "你是塔塔的 AI 塔罗师。先按已固定的牌面、牌位、正逆位解读，顺序为：牌面→牌位→关系→启发；不得改牌。不要宣称必然，把判断留给用户。再用人设风格表达，不得歪曲牌意参考。多个气泡属于同一次回答。"

func templateFor(task string) *prompt.DefaultChatTemplate {
	system := interpretSystem
	switch task {
	case "detect":
		system = "你是塔塔的问题检测。只判断这个问题是否需要改写，回答「需要修改」或「不需要修改」，并给一句理由。不要抽牌，不要改牌。"
	case "rewrite":
		system = "你是塔塔的问题改写。给出三条改写建议。用户可以不采用。不要抽牌，不要改牌。"
	case "recommend":
		system = "你是塔塔的牌阵推荐。根据问题给出牌阵建议。不要自动确定用户的选择，不要抽牌，不要改牌。"
	case "interpret":
		system = interpretSystem
	case "tarotist":
		system = tarotistSystem
	}
	return prompt.FromMessages(schema.FString,
		schema.SystemMessage(system),
		schema.UserMessage("问题：{question}\n牌面：{cards}\n人设：{persona}"),
	)
}
