package tata

// 帮助中心正式问答。来源 Pixso「帮助中心Q&A」681:7。会员与支付仍未定稿，不写入。
var helpArticleCopy = []struct {
	ID, Category, Title, Body string
	Order                     int
}{
	{
		ID: "how_to_start", Category: "reading", Title: "如何开始一次塔罗占卜？", Order: 1,
		Body: "进入「首页」，点击「开始抽牌」，输入你想咨询的问题并选择牌阵。按照页面指引完成洗牌、切牌和抽牌后，塔塔会为你生成对应的塔罗解读。\n\n你也可以进入「探索」，选择感兴趣的牌阵开始占卜。",
	},
	{
		ID: "where_records", Category: "reading", Title: "在哪里查看占卜记录？", Order: 2,
		Body: "你可以在「首页」中右上角进入占卜历史记录，查看之前的问题、抽到的卡牌及解读内容。",
	},
	{
		ID: "change_deck", Category: "reading", Title: "如何更换使用的牌组？", Order: 3,
		Body: "进入「我的」→「我的塔罗牌」，即可查看和切换喜欢的牌组。\n\n选择牌组后，后续支持该牌组的抽牌界面将使用对应的卡牌样式。部分特殊牌组可能需要单独解锁。",
	},
	{
		ID: "draw_failed", Category: "reading", Title: "抽牌或解读失败怎么办？", Order: 4,
		Body: "如果抽牌中断或解读生成失败，可以先检查网络连接，再尝试重新进入页面。\n\n如果多次尝试仍无法正常使用，请通过「我的」中的反馈入口联系我们，并提供出现问题的页面截图及大致时间，我们会协助排查。",
	},
	{
		ID: "annual_entry", Category: "reading", Title: "年度运势在哪里开启？", Order: 5,
		Body: "进入「我的」→「年度运势」，即可开启专属年度解读。\n\n根据页面提示填写信息后，完成十二宫牌阵和四季牌阵的抽取后，系统将生成你的年度报告。",
	},
	{
		ID: "language_theme", Category: "account", Title: "如何切换语言和主题？", Order: 1,
		Body: "进入「我的」，找到「语言」或「主题」设置。\n\n在「语言」中，可以选择 App 支持的显示语言；在「主题」中，可以切换浅色或深色模式。\n\n选择后，界面将按照对应设置进行调整。",
	},
}
