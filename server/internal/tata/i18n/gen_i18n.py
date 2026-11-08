#!/usr/bin/env python3
"""Generate App i18n JSON catalogs. Run from repo: python3 server/internal/tata/i18n/gen_i18n.py"""
from __future__ import annotations

import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
TATA = HERE.parent
SEED = json.loads((TATA / "seed_content.json").read_text())

S2T = str.maketrans({
    "个": "個", "为": "為", "买": "買", "于": "於", "仅": "僅", "从": "從", "传": "傳",
    "会": "會", "价": "價", "余": "餘", "冲": "衝", "减": "減", "击": "擊", "删": "刪",
    "务": "務", "动": "動", "励": "勵", "势": "勢", "区": "區", "单": "單", "历": "歷",
    "参": "參", "发": "發", "变": "變", "号": "號", "后": "後", "启": "啟", "员": "員",
    "图": "圖", "场": "場", "复": "復", "奖": "獎", "对": "對", "币": "幣", "师": "師",
    "应": "應", "开": "開", "当": "當", "录": "錄", "径": "徑", "态": "態", "愿": "願",
    "户": "戶", "拟": "擬", "择": "擇", "换": "換", "据": "據", "数": "數", "无": "無",
    "时": "時", "权": "權", "来": "來", "标": "標", "档": "檔", "检": "檢", "测": "測",
    "点": "點", "独": "獨", "码": "碼", "确": "確", "称": "稱", "签": "簽", "类": "類",
    "经": "經", "绑": "綁", "结": "結", "继": "繼", "续": "續", "编": "編", "罗": "羅",
    "节": "節", "获": "獲", "补": "補", "认": "認", "计": "計", "记": "記", "许": "許",
    "设": "設", "评": "評", "词": "詞", "试": "試", "读": "讀", "调": "調", "败": "敗",
    "账": "賬", "货": "貨", "购": "購", "费": "費", "达": "達", "过": "過", "运": "運",
    "还": "還", "这": "這", "连": "連", "选": "選", "邮": "郵", "钥": "鑰", "钩": "鉤",
    "钻": "鑽", "销": "銷", "锁": "鎖", "错": "錯", "长": "長", "问": "問", "间": "間",
    "阵": "陣", "页": "頁", "须": "須", "领": "領", "频": "頻", "题": "題", "额": "額",
    "馈": "饋", "验": "驗", "骤": "驟", "灵": "靈", "术": "術", "隐": "隱", "轮": "輪",
    "义": "義", "恶": "惡", "阳": "陽", "审": "審", "圣": "聖", "剑": "劍", "币": "幣",
    "从": "從", "骑": "騎", "恋": "戀", "战": "戰", "运": "運", "韦": "韋", "特": "特",
    "与": "與", "关": "關", "系": "係", "业": "業", "学": "學", "习": "習", "际": "際",
    "现": "現", "则": "則", "该": "該", "请": "請", "么": "麼", "们": "們", "这": "這",
    "里": "裡", "为": "為", "里": "裏", "冲": "沖", "复": "複", "只": "隻", "准": "準",
    "并": "並", "注": "註", "松": "鬆", "占": "佔",
})

# Explicit traditional for tarot / copy where char map is wrong or incomplete.
TRAD_OVERRIDE = {
    "魔术师": "魔術師",
    "女祭司": "女祭司",
    "女皇": "女皇",
    "恋人": "戀人",
    "战车": "戰車",
    "隐者": "隱者",
    "命运之轮": "命運之輪",
    "正义": "正義",
    "节制": "節制",
    "恶魔": "惡魔",
    "太阳": "太陽",
    "审判": "審判",
    "权杖": "權杖",
    "圣杯": "聖杯",
    "宝剑": "寶劍",
    "星币": "星幣",
    "侍从": "侍從",
    "骑士": "騎士",
    "莱德韦特": "萊德韋特",
    "经典韦特塔罗": "經典韋特塔羅",
    "塔塔币": "塔塔幣",
    "钻石": "鑽石",
    "次数": "次數",
    "正位": "正位",
    "逆位": "逆位",
    "今日一牌": "今日一牌",
    "普通占卜": "普通占卜",
    "占卜师": "占卜師",
    "探索": "探索",
    "已解读": "已解讀",
    "待继续": "待繼續",
    "感情关系": "感情關係",
    "事业与工作": "事業與工作",
    "学业与考试": "學業與考試",
    "自我成长": "自我成長",
    "人际关系": "人際關係",
    "生活与选择": "生活與選擇",
    "时间流": "時間流",
    "过去": "過去",
    "现在": "現在",
    "未来": "未來",
    "过去 / 现在 / 未来": "過去 / 現在 / 未來",
}


def trad(s: str) -> str:
    if s in TRAD_OVERRIDE:
        return TRAD_OVERRIDE[s]
    return s.translate(S2T)


# msgid (zh-Hans) -> (en, ja, ko)
CATALOG: dict[str, tuple[str, str, str]] = {}


def add(zh: str, en: str, ja: str, ko: str) -> None:
    CATALOG[zh] = (en, ja, ko)


# --- cards from seed ---
for c in SEED["cards"]:
    add(c["name"], c["en"], c["ja"], c["ko"])

# suit/rank fragments (for position-independent lookup)
add("权杖", "Wands", "ワンド", "완드")
add("圣杯", "Cups", "カップ", "컵")
add("宝剑", "Swords", "ソード", "검")
add("星币", "Pentacles", "ペンタクル", "펜타클")
add("王牌", "Ace", "エース", "에이스")
add("侍从", "Page", "ペイジ", "시종")
add("骑士", "Knight", "ナイト", "기사")
add("王后", "Queen", "クイーン", "여왕")
add("国王", "King", "キング", "왕")

add("莱德韦特", "Rider-Waite Tarot", "ライダー・ウェイト・タロット", "라이더 웨이트 타로")
add("经典韦特塔罗", SEED["deck"]["name_en"], SEED["deck"]["name_ja"], SEED["deck"]["name_ko"])

# --- labels ---
add("塔塔币", "Tata Coins", "タタコイン", "타타코인")
add("钻石", "Diamonds", "ダイヤモンド", "다이아")
add("次数", "Reads", "回数", "횟수")
add("正位", "Upright", "正位置", "정위치")
add("逆位", "Reversed", "逆位置", "역위치")
add("今日一牌", "Daily card", "今日の一枚", "오늘의 한 장")
add("普通占卜", "Reading", "通常占い", "일반 점술")
add("占卜师", "Tarotist", "タロティスト", "타로티스트")
add("探索", "Explore", "探索", "탐색")
add("已解读", "Interpreted", "解読済み", "해석 완료")
add("待继续", "Continue", "続きから", "이어서")
add("问题已提交", "Question submitted", "質問を送信済み", "질문 제출됨")
add("牌阵已选择", "Spread selected", "スプレッド選択済み", "스프레드 선택됨")
add("洗牌中", "Shuffling", "シャッフル中", "셔플 중")
add("切牌中", "Cutting", "カット中", "컷 중")
add("牌面已固定", "Cards fixed", "カード確定", "카드 확정")
add("解读生成中", "Interpreting", "解読生成中", "해석 생성 중")
add("已完成", "Completed", "完了", "완료")
add("失败可重试", "Failed, retry", "失敗（再試行可）", "실패, 재시도 가능")
add("北京时间 08:00", "08:00 Beijing time", "北京時間 08:00", "베이징 시간 08:00")
add("注册奖励 → 今日免费次数 → 人工补发免费次数 → 钻石 → 付费",
    "Registration reward → daily free reads → admin grant → diamonds → paywall",
    "登録特典 → 本日の無料回数 → 運営付与 → ダイヤモンド → 課金",
    "가입 보상 → 오늘 무료 횟수 → 운영 지급 → 다이아 → 결제")
add("全玩法共用：注册奖励，今日免费次数，人工补发免费次数，然后钻石",
    "Shared across modes: registration reward, daily free reads, admin grant, then diamonds",
    "全モード共通：登録特典、本日の無料回数、運営付与、その後ダイヤモンド",
    "모든 플레이 공용: 가입 보상, 오늘 무료 횟수, 운영 지급, 그다음 다이아")
add("已按优先级处理：注册奖励，今日免费次数，人工补发免费次数，然后钻石",
    "Settled in order: registration reward, daily free reads, admin grant, then diamonds",
    "優先順で処理：登録特典、本日の無料回数、運営付与、その後ダイヤモンド",
    "우선순위로 처리: 가입 보상, 오늘 무료 횟수, 운영 지급, 그다음 다이아")
add("注册奖励优先于今日免费次数，提问时自动扣减",
    "Registration reward is used before daily free reads, deducted when you ask",
    "登録特典は本日の無料回数より優先し、質問時に自動控除します",
    "가입 보상이 오늘 무료 횟수보다 우선이며, 질문 시 자동 차감됩니다")
add("一开始提问就消耗本次次数；中途退出可从历史继续，不再扣",
    "This read is used when you ask. You can resume from history without another charge.",
    "質問開始時に今回の回数を消費します。途中退出後は履歴から再開でき、再課金しません。",
    "질문 시작 시 이번 횟수가 차감됩니다. 중간에 나가도 기록에서 이어가며 다시 차감되지 않습니다.")
add("心愿已提交", "Wish submitted", "願いを送信しました", "소원이 제출되었습니다")
add("反馈已提交", "Feedback submitted", "フィードバックを送信しました", "피드백이 제출되었습니다")
add("已经扣过，不再扣", "Already charged, not charged again", "すでに控除済みです", "이미 차감되어 다시 차감하지 않습니다")
add("未确认，不扣额度，记录已保留", "Not confirmed. Quota unchanged, record kept.", "未確認のため控除せず、記録を保持します", "확인하지 않아 차감하지 않았고, 기록은 유지됩니다")
add("当日今日一牌名额已占用", "Today's daily card slot is already used", "本日の今日の一枚枠は使用済みです", "오늘의 한 장 횟수를 이미 사용했습니다")
add("今日免费次数已用完，钻石消耗未配置，本次未扣钻石，记录已保留",
    "Daily free reads used up. Diamond cost is not configured, so nothing was charged. Record kept.",
    "本日の無料回数を使い切りました。ダイヤモンド消費が未設定のため未控除、記録は保持します。",
    "오늘 무료 횟수를 모두 썼습니다. 다이아 소모량이 설정되지 않아 차감하지 않았고, 기록은 유지됩니다.")
add("今日免费次数已用完，钻石不足，请付费后自动继续；取消则保留记录且不继续生成",
    "Daily free reads used up and diamonds are insufficient. Pay to continue, or cancel to keep the record without generating.",
    "本日の無料回数を使い切り、ダイヤモンド不足です。課金すると自動継続、キャンセルすると記録保持・生成なし。",
    "오늘 무료 횟수를 모두 썼고 다이아가 부족합니다. 결제하면 이어서 진행되고, 취소하면 기록만 남고 생성하지 않습니다.")
add("试用整段含追问，本次不另扣次数或钻石",
    "Trial covers this reading including follow-ups; no extra reads or diamonds this time",
    "試用は追問を含み、今回は回数・ダイヤモンドを追加控除しません",
    "체험은 추가 질문까지 포함되며, 이번엔 횟수나 다이아를 더 차감하지 않습니다")
add("消耗注册奖励次数", "Used a registration reward", "登録特典回数を消費", "가입 보상 횟수 사용")
add("消耗今日免费次数", "Used a daily free read", "本日の無料回数を消費", "오늘 무료 횟수 사용")
add("消耗人工补发免费次数", "Used an admin-granted free read", "運営付与の無料回数を消費", "운영 지급 무료 횟수 사용")
add("消耗钻石", "Used diamonds", "ダイヤモンドを消費", "다이아 사용")
add("待确认", "To be confirmed", "確認待ち", "확인 예정")
add("请先洗牌", "Shuffle first", "先にシャッフルしてください", "먼저 셔플하세요")
add("今日免费体验额度已用完，本次记录已保存到历史记录",
    "Today's free trial is used up. This record was saved to history.",
    "本日の無料体験は使い切りました。この記録は履歴に保存されています。",
    "오늘 무료 체험을 모두 썼습니다. 이 기록은 히스토리에 저장되었습니다.")
add("该日已有结果，不重复扣次数或钻石",
    "That day already has a result. Reads and diamonds were not charged again.",
    "その日は既に結果があります。回数とダイヤモンドは再控除しません。",
    "그날 결과가 이미 있어 횟수와 다이아를 다시 차감하지 않았습니다.")
add("充值入账", "Top-up credited", "チャージ入金", "충전 입고")
add("会员钻石刷新", "Membership diamonds refreshed", "会員ダイヤモンド更新", "멤버십 다이아 갱신")
add("占卜消耗", "Reading spend", "占い消費", "점술 소모")
add("人工补发", "Admin grant", "運営補填", "운영 지급")
add("兑换码", "Redeem code", "交換コード", "교환 코드")
add("深入追问", "Follow-up questions", "深い追問", "추가 질문")
add("专属活动权益", "Exclusive campaign perks", "限定キャンペーン特典", "전용 캠페인 혜택")
add("每日塔罗解读", "Daily tarot reading", "毎日のタロット解読", "매일 타로 해석")
add("年度运势解锁", "Yearly fortune unlock", "年間運勢の解除", "연간 운세 잠금 해제")
add("已扣不退", "Charged amounts are not refunded", "控除後の返金なし", "차감 후 환불 없음")
add("暂时没有可参与的活动", "No campaigns right now", "参加できるキャンペーンはまだありません", "참여할 캠페인이 없습니다")
add("稍后再来看看", "Check back later", "あとでまた見てください", "나중에 다시 봐 주세요")
add("活动规则与奖励以活动详情页为准", "Rules and rewards follow the campaign details page", "ルールと特典はキャンペーン詳細に従います", "규칙과 보상은 캠페인 상세 페이지 기준입니다")
add("查看活动", "View campaign", "キャンペーンを見る", "캠페인 보기")
add("活动已结束", "Campaign ended", "キャンペーンは終了しました", "캠페인이 종료되었습니다")
add("已结束", "Ended", "終了", "종료")
add("暂无兑换记录", "No redeem records", "交換記録はまだありません", "교환 기록이 없습니다")
add("记录字段待产品确认。成功权益以实际到账结果为准。", "Record fields are pending product confirmation. Benefits follow what was actually credited.", "記録項目は未確定です。特典は実際の入金結果に従います。", "기록 필드는 제품 확인 전입니다. 혜택은 실제 입고 결과를 따릅니다.")
add("暂无参与记录", "No participation records", "参加記録はまだありません", "참여 기록이 없습니다")
add("列表字段与空状态待产品确认。此处不展示伪造记录。", "List fields and empty state are pending confirmation. No fake records are shown.", "リスト項目と空状態は未確定です。偽造記録は出しません。", "목록 필드와 빈 상태는 확인 전입니다. 가짜 기록은 보여 주지 않습니다.")
add("暂无奖励记录", "No reward records", "特典記録はまだありません", "보상 기록이 없습니다")
add("奖励发放与记录字段待产品确认。集合页不写未确认的奖励承诺。", "Reward grant and record fields are pending confirmation. This page does not invent promises.", "特典付与と記録項目は未確定です。未確認の約束は書きません。", "보상 지급과 기록 필드는 확인 전입니다. 확정되지 않은 약속은 쓰지 않습니다.")
add("暂时没有找到相关问题", "No matching articles", "関連する質問は見つかりませんでした", "관련 질문을 찾지 못했습니다")
add("换个关键词，或向我们反馈", "Try another keyword, or send us feedback", "キーワードを変えるか、フィードバックしてください", "다른 키워드를 치거나 의견을 보내 주세요")
add("填写基本信息", "Enter your details", "基本情報を入力", "기본 정보 입력")
add("选12张牌，看全年节奏", "Draw 12 cards for the year", "12枚選んで一年のリズムを見る", "12장 뽑아 한 해 리듬 보기")
add("抽6张牌，看六大领域", "Draw 6 cards for six areas", "6枚引いて六つの領域を見る", "6장 뽑아 여섯 영역 보기")
add("生成年度报告", "Generate yearly report", "年間レポートを生成", "연간 리포트 생성")
add("年度主题", "Yearly theme", "年間テーマ", "연간 테마")
add("领域启示", "Area insights", "領域の示唆", "영역 통찰")
add("节奏变化", "Rhythm", "リズムの変化", "리듬 변화")
add("行动建议", "Suggested actions", "行動の提案", "행동 제안")
add("时间流", "Timeline", "時間の流れ", "시간 흐름")
add("过去 / 现在 / 미래", "Past / Present / Future", "過去 / 現在 / 未来", "과거 / 현재 / 미래")
add("过去 / 现在 / 未来", "Past / Present / Future", "過去 / 現在 / 未来", "과거 / 현재 / 미래")
add("过去", "Past", "過去", "과거")
add("现在", "Present", "現在", "현재")
add("未来", "Future", "未来", "미래")
add("与当前问题相关的既往经历、背景或已经形成的影响",
    "Past events, background, or influence related to this question",
    "この質問に関わる過去の経緯・背景・すでに生じた影響",
    "이 질문과 관련된 과거 경험, 배경, 이미 생긴 영향")
add("当前处境、核心矛盾或主要影响因素",
    "Current situation, core tension, or main influence",
    "今の状況、中心的な対立、主な影響",
    "현재 상황, 핵심 갈등, 주요 영향")
add("当前发展趋势及潜在走向，不代表必然发生",
    "Likely trend and possible direction; not a certainty",
    "現在の流れと起こりうる方向。必ず起きるとは限りません",
    "현재 흐름과 가능한 방향이며, 반드시 일어난다는 뜻은 아닙니다")

# categories
add("感情关系", "Love & relationships", "恋愛・関係", "연애와 관계")
add("事业与工作", "Career & work", "仕事・キャリア", "커리어와 일")
add("学业与考试", "Study & exams", "学業・試験", "학업과 시험")
add("自我成长", "Personal growth", "自己成長", "자기 성장")
add("人际关系", "Relationships", "人間関係", "인간관계")
add("生活与选择", "Life & choices", "生活と選択", "삶과 선택")

# --- preset questions ---
QUESTIONS = {
    "最近的我，最需要关注什么？": ("What do I most need to pay attention to right now?", "今の私は何に一番注目すべき？", "지금 내가 가장 주목해야 할 것은?"),
    "TA 对这段关系的真实态度是什么？": ("How does this person really feel about the relationship?", "相手はこの関係を本当はどう思っている？", "그 사람은 이 관계를 정말 어떻게 생각할까?"),
    "接下来三个月，我的事业会有哪些新机会？": ("What new career opportunities may come in the next three months?", "今後3か月、仕事にどんな新しい機会がある？", "앞으로 석 달, 커리어에 어떤 새 기회가 있을까?"),
    "我现在的情绪困扰，背后隐藏着什么？": ("What is behind the emotional difficulty I feel now?", "今の情緒的な悩みの奥には何がある？", "지금 감정의 어려움 뒤에는 무엇이 있을까?"),
    "我和 TA 接下来一个月的关系会如何发展？": ("How will this relationship develop over the next month?", "これから1か月、二人の関係はどう進む？", "앞으로 한 달, 그 사람과의 관계는 어떻게 될까?"),
    "为了实现这个愿望，我接下来可以做些什么？": ("What can I do next to move this wish forward?", "この願いを叶えるために次にできることは？", "이 바람을 이루려면 다음에 무엇을 할 수 있을까?"),
    "最近有什么值得我留意的新机会？": ("What new opportunities are worth noticing lately?", "最近、注目すべき新しい機会は？", "최근에 눈여겨볼 새 기회는?"),
    "我目前的职业方向适合继续坚持吗？": ("Should I keep going in my current career direction?", "今のキャリア方針は続けてよい？", "지금 커리어 방향을 계속 가도 될까?"),
    "这段关系中，有哪些我还没有看清的事情？": ("What have I not yet seen clearly in this relationship?", "この関係でまだ見えていないことは？", "이 관계에서 아직 보지 못한 것은?"),
    "接下来三个月，我的财务状况会有什么变化？": ("How may my finances change in the next three months?", "今後3か月、お金の状況はどう変わる？", "앞으로 석 달, 재정은 어떻게 바뀔까?"),
    "我应该如何改善目前的人际关系？": ("How can I improve my current relationships?", "今の人間関係をどう改善できる？", "지금 인간관계를 어떻게 개선할 수 있을까?"),
    "如果我主动迈出一步，事情会有什么变化？": ("What would change if I take the first step?", "自分から一歩出たら何が変わる？", "내가 먼저 한 걸음 내디디면 무엇이 바뀔까?"),
    "我身上有哪些尚未被发现的优势？": ("What strengths of mine are still unseen?", "まだ気づいていない自分の強みは？", "아직 발견하지 못한 나의 강점은?"),
    "接下来一个月，我的生活会出现什么新的可能？": ("What new possibilities may appear in my life next month?", "これから1か月、生活にどんな新しい可能性が？", "앞으로 한 달, 삶에 어떤 새로운 가능성이 있을까?"),
    "现在的我，最需要听见什么样的提醒？": ("What reminder do I most need to hear right now?", "今の私にいちばん必要なリマインドは？", "지금 내가 가장 들어야 할 알림은?"),
    "TA 现在是如何看待我们的关系的？": ("How does this person see our relationship now?", "相手は今、私たちの関係をどう見ている？", "그 사람은 지금 우리 관계를 어떻게 볼까?"),
    "我们之间最大的阻碍是什么？": ("What is the biggest block between us?", "二人のあいだの最大の障害は？", "우리 사이 가장 큰 걸림돌은?"),
    "这段关系还有继续发展的空间吗？": ("Is there still room for this relationship to grow?", "この関係はまだ伸びる余地がある？", "이 관계는 아직 더 나아갈 여지가 있을까?"),
    "我应该主动表达自己的感受吗？": ("Should I speak up about how I feel?", "自分の気持ちを先に伝えた方がいい？", "내 감정을 먼저 표현해야 할까?"),
    "我在这段感情中真正需要的是什么？": ("What do I truly need in this relationship?", "この恋愛で本当に必要なものは？", "이 연애에서 내가 정말 필요한 것은?"),
    "接下来三个月，我的感情生活会有什么变化？": ("How may my love life change in the next three months?", "今後3か月、恋愛はどう変わる？", "앞으로 석 달, 연애에는 어떤 변화가 있을까?"),
    "我目前的职业选择适合长期发展吗？": ("Is my current career choice good for the long term?", "今の仕事選びは長期的に合っている？", "지금 직업 선택이 장기적으로 맞을까?"),
    "如果我考虑换工作，应该关注哪些方面？": ("If I consider changing jobs, what should I watch?", "転職を考えるなら何に注目すべき？", "이직을 생각한다면 어디에 주목해야 할까?"),
    "接下来三个月，我的事业发展重点是什么？": ("What should be my career focus for the next three months?", "今後3か月の仕事の重点は？", "앞으로 석 달, 커리어의 초점은?"),
    "我应该如何提升自己在工作中的竞争力？": ("How can I become more competitive at work?", "仕事での競争力をどう高める？", "일에서 경쟁력을 어떻게 높일 수 있을까?"),
    "目前有哪些职业机会值得我关注？": ("Which career opportunities are worth my attention now?", "今注目すべき仕事の機会は？", "지금 눈여겨볼 커리어 기회는?"),
    "我现在的努力会在哪些方面带来改变？": ("Where will my current effort make a difference?", "今の努力はどこに変化をもたらす？", "지금 노력은 어디에 변화를 만들까?"),
    "我接下来的学习状态会如何变化？": ("How will my study state change next?", "これからの学習状態はどう変わる？", "앞으로 학습 상태는 어떻게 바뀔까?"),
    "我应该如何调整目前的学习方法？": ("How should I adjust my current study method?", "今の勉強法をどう調整する？", "지금 공부법을 어떻게 조정해야 할까?"),
    "这段时间，我最需要突破的学习瓶颈是什么？": ("What study bottleneck do I most need to break now?", "今いちばん突破すべき学習の壁は？", "지금 가장 뚫어야 할 학습 병목은?"),
    "面对即将到来的考试，我应该重点准备什么？": ("What should I focus on for the upcoming exam?", "近づく試験で重点的に準備すべきことは？", "다가오는 시험에서 무엇에 집중해 준비해야 할까?"),
    "我适合继续目前的专业方向吗？": ("Should I stay on my current major/path?", "今の専攻・方向を続けてよい？", "지금 전공/방향을 계속해도 될까?"),
    "如何才能更好地平衡学习与生活？": ("How can I better balance study and life?", "学びと生活をどうバランスする？", "학업과 삶을 어떻게 더 잘 균형 잡을까?"),
    "我为什么总是容易陷入自我怀疑？": ("Why do I so easily fall into self-doubt?", "なぜすぐに自己否定してしまう？", "왜 자꾸 자기 의심에 빠질까?"),
    "我现在最需要放下的是什么？": ("What do I most need to let go of now?", "今いちばん手放すべきものは？", "지금 내가 가장 놓아야 할 것은?"),
    "我有哪些尚未发现的潜在优势？": ("What hidden strengths have I not found yet?", "まだ見つけていない潜在的な強みは？", "아직 찾지 못한 잠재 강점은?"),
    "我应该如何建立更稳定的内心状态？": ("How can I build a more stable inner state?", "より安定した内面をどう作る？", "더 안정된 마음 상태를 어떻게 만들까?"),
    "最近反复出现的情绪在提醒我什么？": ("What is the recurring emotion trying to tell me?", "繰り返し現れる感情は何を知らせている？", "자꾸 반복되는 감정은 무엇을 알려줄까?"),
    "未来三个月，我最值得投入精力的方向是什么？": ("Where is my energy best spent in the next three months?", "今後3か月、一番力を入れるべき方向は？", "앞으로 석 달, 가장 힘을 쏟을 방향은?"),
    "我应该如何处理目前的人际关系困扰？": ("How should I handle my current relationship trouble?", "今の人間関係の悩みをどう扱う？", "지금 인간관계 고민을 어떻게 다루면 좋을까?"),
    "身边哪些关系值得我花更多精力维护？": ("Which relationships around me are worth more care?", "周りでより丁寧に保つべき関係は？", "주변에 더 공을 들여 지켜야 할 관계는?"),
    "我在人际交往中容易忽略什么？": ("What do I tend to miss in relating to others?", "人付き合いで見落としがちなことは？", "사람들과 지낼 때 놓치기 쉬운 것은?"),
    "我应该如何表达自己的真实想法？": ("How should I express what I really think?", "本当の考えをどう伝える？", "진짜 생각을 어떻게 표현해야 할까?"),
    "这段友谊接下来会如何发展？": ("How will this friendship develop from here?", "この友情はこれからどう進む？", "이 우정은 앞으로 어떻게 될까?"),
    "我该如何面对与他人的意见分歧？": ("How should I face disagreement with others?", "意見の食い違いにどう向き合う？", "다른 사람과의 의견 차이를 어떻게 마주할까?"),
    "如果我选择另一条道路，会有哪些不同？": ("If I choose another path, what would be different?", "別の道を選んだら何が違う？", "다른 길을 고르면 무엇이 달라질까?"),
    "目前摆在我面前的两个选择，哪个更值得考虑？": ("Of the two choices in front of me, which is more worth considering?", "目の前の二つの選択、どちらを検討すべき？", "앞에 놓인 두 선택 중 어느 쪽이 더 고려할 만할까?"),
    "我现在最应该优先处理的事情是什么？": ("What should I prioritize right now?", "今いちばん優先すべきことは？", "지금 가장 우선해야 할 일은?"),
    "接下来一个月，我的生活重心应该放在哪里？": ("Where should I place my life focus next month?", "これから1か月、生活の重心はどこに？", "앞으로 한 달, 삶의 중심은 어디에 두어야 할까?"),
    "如果我想做出改变，第一步应该从哪里开始？": ("If I want change, where should the first step start?", "変わりたいなら最初の一歩はどこから？", "변화를 원한다면 첫 걸음은 어디서 시작할까?"),
}
for zh, tri in QUESTIONS.items():
    add(zh, *tri)

# --- errors (biz / writeErr) ---
ERRORS = {
    "App Store 商品号已占用": ("This App Store product ID is already in use", "この App Store 商品IDは使用済みです", "이 App Store 상품 ID는 이미 사용 중입니다"),
    "OSS 未配置": ("OSS is not configured", "OSS が未設定です", "OSS가 설정되지 않았습니다"),
    "Play 商品号已占用": ("This Play product ID is already in use", "この Play 商品IDは使用済みです", "이 Play 상품 ID는 이미 사용 중입니다"),
    "audience 不正确": ("audience is invalid", "audience が正しくありません", "audience가 올바르지 않습니다"),
    "before_id 不正确": ("before_id is invalid", "before_id が正しくありません", "before_id가 올바르지 않습니다"),
    "id 不正确": ("id is invalid", "id が正しくありません", "id가 올바르지 않습니다"),
    "type 不正确": ("type is invalid", "type が正しくありません", "type이 올바르지 않습니다"),
    "一次恢复的交易过多": ("Too many transactions in one restore", "一度の復元に含まれる取引が多すぎます", "한 번의 복원에 거래가 너무 많습니다"),
    "上传到 OSS 失败": ("Upload to OSS failed", "OSS へのアップロードに失敗しました", "OSS 업로드에 실패했습니다"),
    "上传方式不正确": ("Upload method is invalid", "アップロード方法が正しくありません", "업로드 방식이 올바르지 않습니다"),
    "上传目录不正确": ("Upload folder is invalid", "アップロード先が正しくありません", "업로드 폴더가 올바르지 않습니다"),
    "不能停用自己": ("You cannot disable your own account", "自分自身は無効化できません", "자기 계정은 비활성화할 수 없습니다"),
    "交易号不正确": ("Transaction ID is invalid", "取引番号が正しくありません", "거래 번호가 올바르지 않습니다"),
    "交易号已使用": ("Transaction ID already used", "取引番号は使用済みです", "거래 번호가 이미 사용되었습니다"),
    "今日一牌次数须为正整数": ("Daily card count must be a positive integer", "今日の一枚回数は正の整数である必要があります", "오늘의 한 장 횟수는 양의 정수여야 합니다"),
    "付费动作不正确": ("Payment action is invalid", "課金アクションが正しくありません", "결제 동작이 올바르지 않습니다"),
    "价格须为正整数（分）": ("Price must be a positive integer (cents)", "価格は正の整数（分）である必要があります", "가격은 양의 정수(분)여야 합니다"),
    "供应商不存在": ("Vendor not found", "ベンダーがありません", "공급자가 없습니다"),
    "供应商不存在或未启用": ("Vendor not found or disabled", "ベンダーがないか無効です", "공급자가 없거나 비활성입니다"),
    "供应商仍被调用绑定引用，先改绑定再停用": ("Vendor is still bound to a task. Rebind first, then disable.", "ベンダーはまだ呼び出しに紐づいています。先に紐づけを変更してください。", "공급자가 아직 호출에 연결되어 있습니다. 먼저 연결을 바꾼 뒤 비활성화하세요."),
    "供应商仍被调用绑定引用，只能停用": ("Vendor is still bound to a task and can only be disabled", "ベンダーはまだ呼び出しに紐づいているため、無効化のみ可能です", "공급자가 아직 호출에 연결되어 있어 비활성화만 가능합니다"),
    "供应商地址只允许 HTTPS；本机演示可使用 http://127.0.0.1 또는 http://localhost": ("Vendor URL must be HTTPS; local demo may use http://127.0.0.1 or http://localhost", "ベンダーURLは HTTPS のみ。ローカルデモは http://127.0.0.1 または http://localhost 可", "공급자 주소는 HTTPS만 허용합니다. 로컬 데모는 http://127.0.0.1 또는 http://localhost"),
    "供应商密钥无法读取": ("Vendor API key cannot be read", "ベンダー鍵を読み取れません", "공급자 키를 읽을 수 없습니다"),
    "供应商未启用": ("Vendor is disabled", "ベンダーは無効です", "공급자가 비활성입니다"),
    "供应商调用失败": ("Vendor call failed", "ベンダー呼び出しに失敗しました", "공급자 호출에 실패했습니다"),
    "供应商返回无法解析": ("Vendor response could not be parsed", "ベンダー応答を解析できません", "공급자 응답을 파싱할 수 없습니다"),
    "先抽牌，再解读": ("Draw cards before interpreting", "先にカードを引いてから解読してください", "먼저 카드를 뽑은 뒤 해석하세요"),
    "兑换码已使用": ("Redeem code already used", "交換コードは使用済みです", "교환 코드가 이미 사용되었습니다"),
    "兑换码已过期": ("Redeem code expired", "交換コードの期限が切れています", "교환 코드가 만료되었습니다"),
    "兑换码无效": ("Redeem code is invalid", "交換コードが無効です", "교환 코드가 유효하지 않습니다"),
    "全年节奏牌已固定": ("Year rhythm cards are already fixed", "年間リズムのカードは確定済みです", "연간 리듬 카드가 이미 확정되었습니다"),
    "公开 ID 生成失败": ("Failed to generate public ID", "公開IDの生成に失敗しました", "공개 ID 생성에 실패했습니다"),
    "六大领域牌已固定": ("Six-area cards are already fixed", "六領域のカードは確定済みです", "여섯 영역 카드가 이미 확정되었습니다"),
    "内容不能超过 500 字": ("Content cannot exceed 500 characters", "内容は500字以内です", "내용은 500자를 넘을 수 없습니다"),
    "内部错误": ("Internal error", "内部エラー", "내부 오류"),
    "出生日期不正确": ("Birth date is invalid", "生年月日が正しくありません", "생년월일이 올바르지 않습니다"),
    "出生日期不能是未来": ("Birth date cannot be in the future", "生年月日は未来にできません", "생년월일은 미래일 수 없습니다"),
    "出生日期格式应为 YYYY-MM-DD": ("Birth date must be YYYY-MM-DD", "生年月日は YYYY-MM-DD 形式です", "생년월일은 YYYY-MM-DD 형식이어야 합니다"),
    "刷新令牌无效": ("Refresh token is invalid", "リフレッシュトークンが無効です", "리프레시 토큰이 유효하지 않습니다"),
    "匿名用户没有档案": ("Anonymous users have no profile", "匿名ユーザーにプロフィールはありません", "익명 사용자에게는 프로필이 없습니다"),
    "原始交易号不正确": ("Original transaction ID is invalid", "元取引番号が正しくありません", "원거래 번호가 올바르지 않습니다"),
    "反馈分类不正确": ("Feedback category is invalid", "フィードバック分類が正しくありません", "피드백 분류가 올바르지 않습니다"),
    "发送过于频繁": ("Sent too frequently", "送信が多すぎます", "너무 자주 보냈습니다"),
    "只支持 jpg、png、webp、gif": ("Only jpg, png, webp, and gif are supported", "jpg / png / webp / gif のみ対応", "jpg, png, webp, gif만 지원합니다"),
    "只有失败可重试的记录能重试": ("Only failed-retryable records can be retried", "失敗して再試行可能な記録のみ再試行できます", "실패 후 재시도 가능한 기록만 다시 시도할 수 있습니다"),
    "只有次卡可以发放解读次数": ("Only reading packs can grant reads", "回数パックのみ解読回数を付与できます", "횟수팩만 해석 횟수를 지급할 수 있습니다"),
    "只能确认自己发起的合并": ("You can only confirm a merge you started", "自分が開始した統合のみ確認できます", "직접 시작한 병합만 확인할 수 있습니다"),
    "只读账号不能修改": ("Read-only accounts cannot make changes", "閲覧専用アカウントは変更できません", "읽기 전용 계정은 수정할 수 없습니다"),
    "可用牌不足": ("Not enough cards left", "使えるカードが足りません", "사용 가능한 카드가 부족합니다"),
    "合并结果缺失": ("Merge result is missing", "統合結果がありません", "병합 결과가 없습니다"),
    "合并请求不可确认": ("Merge request cannot be confirmed", "統合リクエストは確認できません", "병합 요청을 확인할 수 없습니다"),
    "合并请求不存在": ("Merge request not found", "統合リクエストがありません", "병합 요청이 없습니다"),
    "名称和模型必填": ("Name and model are required", "名称とモデルは必須です", "이름과 모델은 필수입니다"),
    "名称必填": ("Name is required", "名称は必須です", "이름은 필수입니다"),
    "启用前必须设置密钥": ("Set an API key before enabling", "有効化の前に鍵を設定してください", "활성화하기 전에 키를 설정하세요"),
    "周期数量须为正整数": ("Period count must be a positive integer", "周期数は正の整数である必要があります", "주기 수는 양의 정수여야 합니다"),
    "商品不存在": ("Product not found", "商品がありません", "상품이 없습니다"),
    "商品不正确": ("Product is invalid", "商品が正しくありません", "상품이 올바르지 않습니다"),
    "商品名称不正确": ("Product name is invalid", "商品名が正しくありません", "상품 이름이 올바르지 않습니다"),
    "商品文案过长": ("Product copy is too long", "商品文案が長すぎます", "상품 문구가 너무 깁니다"),
    "商品类型不正确": ("Product type is invalid", "商品タイプが正しくありません", "상품 유형이 올바르지 않습니다"),
    "商品类型不能修改": ("Product type cannot be changed", "商品タイプは変更できません", "상품 유형은 바꿀 수 없습니다"),
    "商品编码不正确": ("Product code is invalid", "商品コードが正しくありません", "상품 코드가 올바르지 않습니다"),
    "商品编码已存在": ("Product code already exists", "商品コードは既にあります", "상품 코드가 이미 있습니다"),
    "图片内容不正确": ("Image content is invalid", "画像内容が正しくありません", "이미지 내용이 올바르지 않습니다"),
    "图片地址不正确": ("Image URL is invalid", "画像アドレスが正しくありません", "이미지 주소가 올바르지 않습니다"),
    "图片地址过长": ("Image URL is too long", "画像アドレスが長すぎます", "이미지 주소가 너무 깁니다"),
    "图片过大": ("Image is too large", "画像が大きすぎます", "이미지가 너무 큽니다"),
    "塔罗师不可用": ("This tarotist is unavailable", "このタロティストは利用できません", "이 타로티스트는 사용할 수 없습니다"),
    "塔罗师不存在": ("Tarotist not found", "タロティストがありません", "타로티스트가 없습니다"),
    "姓名过长": ("Name is too long", "氏名が長すぎます", "이름이 너무 깁니다"),
    "密钥无法读取": ("API key cannot be read", "鍵を読み取れません", "키를 읽을 수 없습니다"),
    "对方账号已合并": ("The other account is already merged", "相手アカウントは既に統合済みです", "상대 계정은 이미 병합되었습니다"),
    "对方账号已注销，不能合并": ("The other account is cancelled and cannot be merged", "相手アカウントは退会済みで統合できません", "상대 계정은 탈퇴되어 병합할 수 없습니다"),
    "展示时间不正确": ("Display time is invalid", "表示時間が正しくありません", "표시 시간이 올바르지 않습니다"),
    "平台不正确": ("Platform is invalid", "プラットフォームが正しくありません", "플랫폼이 올바르지 않습니다"),
    "年份不正确": ("Year is invalid", "年が正しくありません", "연도가 올바르지 않습니다"),
    "年卡未开通，单独购买价格未配置": ("Yearly plan is not active, and a one-off price is not configured", "年額は未加入で、単体購入価格も未設定です", "연간권이 없고 단독 구매 가격도 설정되지 않았습니다"),
    "年卡未开通，请通过内购单独解锁": ("Yearly plan is not active. Unlock with in-app purchase.", "年額は未加入です。アプリ内購入で解除してください。", "연간권이 없습니다. 인앱 결제로 잠금 해제하세요."),
    "年运段落不正确": ("Yearly section is invalid", "年運セクションが正しくありません", "연운 구간이 올바르지 않습니다"),
    "年运解锁不能发放钻石": ("Yearly unlock cannot grant diamonds", "年運解除ではダイヤモンドを付与できません", "연운 해제는 다이아를 지급할 수 없습니다"),
    "当前状态不能生成解读": ("Cannot interpret in the current status", "現在の状態では解読できません", "현재 상태에서는 해석할 수 없습니다"),
    "当前状态不能选择牌阵": ("Cannot select a spread in the current status", "現在の状態ではスプレッドを選べません", "현재 상태에서는 스프레드를 선택할 수 없습니다"),
    "当前记录不能追问": ("This record cannot take follow-ups", "この記録には追問できません", "이 기록에는 추가 질문할 수 없습니다"),
    "微信授权失败": ("WeChat authorization failed", "WeChat 認可に失敗しました", "WeChat 인증에 실패했습니다"),
    "微信授权码不能为空": ("WeChat authorization code is required", "WeChat 認可コードは必須です", "WeChat 인증 코드가 필요합니다"),
    "微信授权码无效": ("WeChat authorization code is invalid", "WeChat 認可コードが無効です", "WeChat 인증 코드가 유효하지 않습니다"),
    "微信登录失败": ("WeChat login failed", "WeChat ログインに失敗しました", "WeChat 로그인에 실패했습니다"),
    "微信登录未配置": ("WeChat login is not configured", "WeChat ログインは未設定です", "WeChat 로그인이 설정되지 않았습니다"),
    "性别选项不正确": ("Gender option is invalid", "性別の選択肢が正しくありません", "성별 옵션이 올바르지 않습니다"),
    "恢复购买钩子已关闭": ("Restore-purchase hook is disabled", "購入復元フックは無効です", "구매 복원 훅이 꺼져 있습니다"),
    "抽牌后不能修改问题": ("The question cannot be edited after cards are drawn", "カードを引いた後は質問を変更できません", "카드를 뽑은 뒤에는 질문을 수정할 수 없습니다"),
    "抽牌失败": ("Draw failed", "カードを引けませんでした", "카드 뽑기에 실패했습니다"),
    "抽牌方式不正确": ("Draw mode is invalid", "抽選方法が正しくありません", "뽑기 방식이 올바르지 않습니다"),
    "抽牌步骤不正确": ("Draw step is invalid", "抽選ステップが正しくありません", "뽑기 단계가 올바르지 않습니다"),
    "提示窗不存在": ("Banner not found", "バナーがありません", "배너가 없습니다"),
    "搜索关键词过长": ("Search query is too long", "検索語が長すぎます", "검색어가 너무 깁니다"),
    "操作员不存在": ("Operator not found", "オペレーターがありません", "운영자가 없습니다"),
    "操作员字段不正确": ("Operator fields are invalid", "オペレーター項目が正しくありません", "운영자 항목이 올바르지 않습니다"),
    "文件过大或格式不正确": ("File is too large or the format is invalid", "ファイルが大きすぎるか形式が正しくありません", "파일이 너무 크거나 형식이 올바르지 않습니다"),
    "文案必填": ("Copy is required", "文案は必須です", "문구는 필수입니다"),
    "文案类型不正确": ("Copy type is invalid", "文案タイプが正しくありません", "문구 유형이 올바르지 않습니다"),
    "时区不正确": ("Timezone is invalid", "タイムゾーンが正しくありません", "시간대가 올바르지 않습니다"),
    "时间格式应为 HH:MM": ("Time must be HH:MM", "時刻は HH:MM 形式です", "시간은 HH:MM 형식이어야 합니다"),
    "时间格式应为 RFC3339": ("Time must be RFC3339", "時刻は RFC3339 形式です", "시간은 RFC3339 형식이어야 합니다"),
    "月份格式应为 YYYY-MM": ("Month must be YYYY-MM", "月は YYYY-MM 形式です", "월은 YYYY-MM 형식이어야 합니다"),
    "未登录或令牌无效": ("Not logged in or token invalid", "未ログイン、またはトークンが無効です", "로그인되지 않았거나 토큰이 유효하지 않습니다"),
    "未知步骤": ("Unknown step", "未知のステップです", "알 수 없는 단계입니다"),
    "未知调用": ("Unknown task", "未知の呼び出しです", "알 수 없는 호출입니다"),
    "未确认，不会合并账号": ("Not confirmed; accounts will not be merged", "未確認のためアカウントは統合しません", "확인하지 않아 계정을 병합하지 않습니다"),
    "未绑定供应商": ("No vendor is bound", "ベンダーが紐づいていません", "공급자가 연결되어 있지 않습니다"),
    "未进入问题检测，不能修改问题": ("Question detect has not started, so the question cannot be edited", "質問検出前のため質問を変更できません", "질문 감지 전이라 질문을 수정할 수 없습니다"),
    "本期不可修改重置时刻或每日次数": ("Reset time and daily counts cannot be changed this period", "今期はリセット時刻と毎日回数を変更できません", "이번 주기에는 리셋 시각과 매일 횟수를 바꿀 수 없습니다"),
    "本条解读追问已达上限": ("Follow-up limit reached for this reading", "この解読の追問上限に達しました", "이 해석의 추가 질문 한도에 도달했습니다"),
    "标签过多": ("Too many tags", "タグが多すぎます", "태그가 너무 많습니다"),
    "标签过长": ("Tag is too long", "タグが長すぎます", "태그가 너무 깁니다"),
    "模型不属于该供应商": ("Model does not belong to this vendor", "モデルはこのベンダーに属しません", "모델이 이 공급자에 속하지 않습니다"),
    "模拟流程不应写入占卜记录": ("Simulation must not write reading records", "シミュレーションは占い記録に書いてはいけません", "시뮬레이션은 점술 기록에 쓰면 안 됩니다"),
    "次卡不能发放钻石": ("Reading packs cannot grant diamonds", "回数パックはダイヤモンドを付与できません", "횟수팩은 다이아를 지급할 수 없습니다"),
    "每日免费次数须为正整数": ("Daily free count must be a positive integer", "毎日の無料回数は正の整数である必要があります", "매일 무료 횟수는 양의 정수여야 합니다"),
    "没有可修改的额度参数": ("No quota parameters to update", "変更できるクォータ項目がありません", "수정할 한도 항목이 없습니다"),
    "没有可取消的付费等待": ("No pending payment to cancel", "キャンセルできる課金待ちがありません", "취소할 결제 대기가 없습니다"),
    "没有可恢复的交易": ("No transactions to restore", "復元できる取引がありません", "복원할 거래가 없습니다"),
    "没有模型": ("No model", "モデルがありません", "모델이 없습니다"),
    "活动不存在": ("Campaign not found", "キャンペーンがありません", "캠페인이 없습니다"),
    "活动状态不正确": ("Campaign status is invalid", "キャンペーン状態が正しくありません", "캠페인 상태가 올바르지 않습니다"),
    "流式仅用于延续追问；新建占卜请用非流式接口": ("Streaming is only for continuing a follow-up. Start a new reading with the non-stream API.", "ストリームは継続追問のみ。新規占いは非ストリームAPIを使ってください。", "스트리밍은 이어지는 추가 질문만 가능합니다. 새 점술은 비스트리밍 API를 쓰세요."),
    "流水不能新增或修改": ("Ledger entries cannot be created or edited", "台帳は追加・変更できません", "장부는 추가하거나 수정할 수 없습니다"),
    "消息不存在": ("Message not found", "メッセージがありません", "메시지가 없습니다"),
    "灵数不能重复": ("Numerology numbers cannot repeat", "霊数は重複できません", "영수는 중복될 수 없습니다"),
    "灵数个数必须与牌阵牌位数一致": ("Numerology count must match the spread size", "霊数の個数はスプレッド枚数と一致する必要があります", "영수 개수는 스프레드 장 수와 같아야 합니다"),
    "灵数须为 1 到 78 的整数": ("Numerology numbers must be integers from 1 to 78", "霊数は 1〜78 の整数です", "영수는 1부터 78까지의 정수여야 합니다"),
    "场景不正确": ("Scene is invalid", "シーンが正しくありません", "장면이 올바르지 않습니다"),
    "牌名不正确": ("Card name is invalid", "カード名が正しくありません", "카드 이름이 올바르지 않습니다"),
    "牌组介绍过长": ("Deck description is too long", "デッキ紹介が長すぎます", "덱 소개가 너무 깁니다"),
    "牌阵不可用": ("This spread is unavailable", "このスプレッドは利用できません", "이 스프레드는 사용할 수 없습니다"),
    "牌阵不存在": ("Spread not found", "スプレッドがありません", "스프레드가 없습니다"),
    "牌阵名称、牌数和牌位不一致": ("Spread name, card count, and positions do not match", "スプレッド名、枚数、ポジションが一致しません", "스프레드 이름, 장 수, 포지션이 일치하지 않습니다"),
    "牌阵已被占卜使用，不能修改牌数或牌位": ("This spread is in use; card count and positions cannot change", "使用中のスプレッドは枚数とポジションを変更できません", "사용 중인 스프레드는 장 수와 포지션을 바꿀 수 없습니다"),
    "牌阵已被引用，只能停用": ("This spread is in use and can only be disabled", "参照中のスプレッドは無効化のみ可能です", "참조 중인 스프레드는 비활성화만 가능합니다"),
    "牌面尚未固定": ("Cards are not fixed yet", "カードがまだ確定していません", "카드가 아직 확정되지 않았습니다"),
    "牌面已固定或状态已变化": ("Cards are already fixed or the status changed", "カードは確定済みか、状態が変わっています", "카드가 이미 확정되었거나 상태가 바뀌었습니다"),
    "牌面编码不正确": ("Deck code is invalid", "デッキコードが正しくありません", "덱 코드가 올바르지 않습니다"),
    "生成进行中": ("Generation is already in progress", "生成は進行中です", "생성이 이미 진행 중입니다"),
    "用户 id 不正确": ("User id is invalid", "ユーザーIDが正しくありません", "사용자 id가 올바르지 않습니다"),
    "用户不存在": ("User not found", "ユーザーがありません", "사용자가 없습니다"),
    "用户名已存在": ("Username already exists", "ユーザー名は既にあります", "사용자 이름이 이미 있습니다"),
    "用户名或密码错误": ("Incorrect username or password", "ユーザー名またはパスワードが違います", "사용자 이름 또는 비밀번호가 올바르지 않습니다"),
    "登录后才会扣次数或钻石": ("Reads or diamonds are only charged after login", "ログイン後に回数またはダイヤモンドを控除します", "로그인 후에만 횟수나 다이아가 차감됩니다"),
    "登录后才能上传": ("Log in to upload", "アップロードするにはログインしてください", "업로드하려면 로그인하세요"),
    "登录后才能使用注册奖励": ("Log in to use the registration reward", "登録特典を使うにはログインしてください", "가입 보상을 쓰려면 로그인하세요"),
    "登录后才能保存通知设置": ("Log in to save notification settings", "通知設定の保存にはログインが必要です", "알림 설정을 저장하려면 로그인하세요"),
    "登录后才能兑换": ("Log in to redeem", "交換するにはログインしてください", "교환하려면 로그인하세요"),
    "登录后才能删除": ("Log in to delete", "削除するにはログインしてください", "삭제하려면 로그인하세요"),
    "登录后才能处理付费": ("Log in to handle payment", "課金処理にはログインが必要です", "결제를 처리하려면 로그인하세요"),
    "登录后才能恢复购买": ("Log in to restore purchases", "購入復元にはログインが必要です", "구매를 복원하려면 로그인하세요"),
    "登录后才能抽今日一牌": ("Log in to draw the daily card", "今日の一枚を引くにはログインしてください", "오늘의 한 장을 뽑으려면 로그인하세요"),
    "登录后才能提交反馈": ("Log in to submit feedback", "フィードバック送信にはログインが必要です", "피드백을 보내려면 로그인하세요"),
    "登录后才能提交心愿": ("Log in to submit a wish", "願いの送信にはログインが必要です", "소원을 보내려면 로그인하세요"),
    "登录后才能操作消息": ("Log in to manage messages", "メッセージ操作にはログインが必要です", "메시지를 처리하려면 로그인하세요"),
    "登录后才能查看": ("Log in to view", "表示するにはログインしてください", "보려면 로그인하세요"),
    "登录后才能查看余额流水": ("Log in to view the ledger", "残高明細の表示にはログインが必要です", "잔액 내역을 보려면 로그인하세요"),
    "登录后才能查看兑换记录": ("Log in to view redeem records", "交換記録の表示にはログインが必要입니다", "교환 기록을 보려면 로그인하세요"),
    "登录后才能查看参与记录": ("Log in to view participation records", "参加記録の表示にはログインが必要です", "참여 기록을 보려면 로그인하세요"),
    "登录后才能查看奖励记录": ("Log in to view reward records", "特典記録の表示にはログインが必要です", "보상 기록을 보려면 로그인하세요"),
    "登录后才能查看消息": ("Log in to view messages", "メッセージ表示에는ログインが必要です", "메시지를 보려면 로그인하세요"),
    "登录后才能查看通知设置": ("Log in to view notification settings", "通知設定の表示にはログインが必要です", "알림 설정을 보려면 로그인하세요"),
    "登录后才能查看钻石余额": ("Log in to view diamond balance", "ダイヤモンド残高の表示にはログインが必要です", "다이아 잔액을 보려면 로그인하세요"),
    "登录后才能注册推送": ("Log in to register push", "プッシュ登録にはログインが必要です", "푸시를 등록하려면 로그인하세요"),
    "登录后才能注销": ("Log in to cancel the account", "退会するにはログインしてください", "탈퇴하려면 로그인하세요"),
    "登录后才能看日历": ("Log in to view the calendar", "カレンダー表示にはログインが必要です", "캘린더를 보려면 로그인하세요"),
    "登录后才能确认合并": ("Log in to confirm the merge", "統合の確認にはログインが必要です", "병합을 확인하려면 로그인하세요"),
    "登录后才能确认额度": ("Log in to confirm quota", "クォータ確認にはログインが必要です", "한도를 확인하려면 로그인하세요"),
    "登录后才能绑定": ("Log in to bind", "連携するにはログインしてください", "연결하려면 로그인하세요"),
    "登录后才能验单": ("Log in to verify the receipt", "レシート検証にはログインが必要です", "영수증을 검증하려면 로그인하세요"),
    "目标日期不正确": ("Target date is invalid", "対象日が正しくありません", "대상 날짜가 올바르지 않습니다"),
    "票据为空": ("Receipt is empty", "レシートが空です", "영수증이 비어 있습니다"),
    "票据过长": ("Receipt is too long", "レシートが長すぎます", "영수증이 너무 깁니다"),
    "简介过长": ("Bio is too long", "紹介文が長すぎます", "소개가 너무 깁니다"),
    "缺少 user_id": ("user_id is required", "user_id が必要です", "user_id가 필요합니다"),
    "缺少推送 CID": ("Push CID is required", "プッシュ CID が必要です", "푸시 CID가 필요합니다"),
    "缺少标题": ("Title is required", "タイトルが必要です", "제목이 필요합니다"),
    "至少保留一套启用的牌面风格": ("Keep at least one deck style enabled", "有効なデッキスタイルを少なくとも1つ残してください", "활성화된 덱 스타일을 하나 이상 남겨 두세요"),
    "苹果登录失败": ("Apple login failed", "Apple ログインに失敗しました", "Apple 로그인에 실패했습니다"),
    "苹果登录未配置": ("Apple login is not configured", "Apple ログインは未設定です", "Apple 로그인이 설정되지 않았습니다"),
    "苹果身份令牌不能为空": ("Apple identity token is required", "Apple ID トークンは必須です", "Apple ID 토큰이 필요합니다"),
    "苹果身份令牌无效": ("Apple identity token is invalid", "Apple ID トークンが無効です", "Apple ID 토큰이 유효하지 않습니다"),
    "苹果身份校验失败": ("Apple identity verification failed", "Apple ID の検証に失敗しました", "Apple ID 검증에 실패했습니다"),
    "获取答案前必须登录": ("Log in before requesting an answer", "回答を得る前にログインしてください", "답을 받기 전에 로그인하세요"),
    "补发原因类型只允许 admin_grant": ("Grant reason type must be admin_grant", "補填理由タイプは admin_grant のみです", "지급 사유 유형은 admin_grant만 가능합니다"),
    "补发原因过长": ("Grant reason is too long", "補填理由が長すぎます", "지급 사유가 너무 깁니다"),
    "补发数量不正确": ("Grant amount is invalid", "補填数量が正しくありません", "지급 수량이 올바르지 않습니다"),
    "补发数量超出单次上限": ("Grant amount exceeds the per-grant cap", "補填数量が1回の上限を超えています", "지급 수량이 1회 한도를 넘었습니다"),
    "解读失败，可重试": ("Interpretation failed. You can retry.", "解読に失敗しました。再試行できます。", "해석에 실패했습니다. 다시 시도할 수 있습니다."),
    "解读失败，可重试，未重新扣费": ("Interpretation failed. You can retry without another charge.", "解読に失敗しました。再課金なしで再試行できます。", "해석에 실패했습니다. 추가 차감 없이 다시 시도할 수 있습니다."),
    "解读完成前不能追问": ("Follow-ups are unavailable until interpretation is complete", "解読完了前は追問できません", "해석이 끝나기 전에는 추가 질문할 수 없습니다"),
    "解读次数须为正整数": ("Reading grant must be a positive integer", "解読回数は正의 정수여야 합니다", "해석 횟수는 양의 정수여야 합니다"),
    "记录不存在": ("Record not found", "記録がありません", "기록이 없습니다"),
    "记录不属于当前用户": ("This record does not belong to the current user", "この記録は現在のユーザーのものではありません", "이 기록은 현재 사용자 것이 아닙니다"),
    "记录已删除": ("Record deleted", "記録は削除済みです", "기록이 삭제되었습니다"),
    "记录类型不正确": ("Record type is invalid", "記録タイプが正しくありません", "기록 유형이 올바르지 않습니다"),
    "该微信已注销，不能重复领取注册奖励": ("This WeChat account was cancelled and cannot claim the signup reward again", "この WeChat は退会済みで、登録特典を再取得できません", "이 WeChat은 탈퇴되어 가입 보상을 다시 받을 수 없습니다"),
    "该牌面风格尚未开放": ("This deck style is not available yet", "このデッキスタイルはまだ開放されていません", "이 덱 스타일은 아직 열려 있지 않습니다"),
    "该登录方式已绑定，换绑未开放": ("This login is already bound. Rebinding is not available.", "このログイン方法は既に連携済みで、付け替えは未開放です", "이 로그인 방식은 이미 연결되어 있으며 재연결은 아직 없습니다"),
    "该苹果账号已注销，不能重复领取注册奖励": ("This Apple account was cancelled and cannot claim the signup reward again", "この Apple アカウントは退会済みで、登録特典を再取得できません", "이 Apple 계정은 탈퇴되어 가입 보상을 다시 받을 수 없습니다"),
    "该身份已绑定其他账号": ("This identity is already bound to another account", "この身分は他のアカウントに連携済みです", "이 신원은 다른 계정에 이미 연결되어 있습니다"),
    "请先填写基本信息": ("Enter your details first", "先に基本情報を入力してください", "먼저 기본 정보를 입력하세요"),
    "请先填写问题": ("Enter a question first", "先に質問を入力してください", "먼저 질문을 입력하세요"),
    "请先完成切牌并点击下一步": ("Finish the cut, then tap next", "先にカットを終えて次へを押してください", "먼저 컷을 끝낸 뒤 다음을 누르세요"),
    "请先抽完六大领域牌": ("Finish drawing the six area cards first", "先に六領域のカードを引き終えてください", "먼저 여섯 영역 카드를 모두 뽑으세요"),
    "请先解锁年度运势": ("Unlock yearly fortune first", "先に年間運勢を解除してください", "먼저 연간 운세를 잠금 해제하세요"),
    "请先选完 12 张全年节奏牌": ("Finish selecting the 12 year-rhythm cards first", "先に年間リズム12枚を選び終えてください", "먼저 연간 리듬 12장을 모두 고르세요"),
    "请先选择牌阵": ("Select a spread first", "先にスプレッドを選んでください", "먼저 스프레드를 선택하세요"),
    "请填写出生日期": ("Enter a birth date", "生年月日を入力してください", "생년월일을 입력하세요"),
    "请填写反馈内容": ("Enter feedback", "フィードバック内容を入力してください", "피드백 내용을 입력하세요"),
    "请填写年运段落": ("Enter yearly sections", "年運セクションを入力してください", "연운 구간을 입력하세요"),
    "请填写心愿内容": ("Enter a wish", "願いの内容を入力してください", "소원 내용을 입력하세요"),
    "请填写牌面名称": ("Enter a deck name", "デッキ名を入力してください", "덱 이름을 입력하세요"),
    "请填写补发原因": ("Enter a grant reason", "補填理由を入力してください", "지급 사유를 입력하세요"),
    "请填写钻石或免费次数": ("Enter diamonds or free reads", "ダイヤモンドまたは無料回数を入力してください", "다이아 또는 무료 횟수를 입력하세요"),
    "请指定用户": ("Specify a user", "ユーザーを指定してください", "사용자를 지정하세요"),
    "请标明是否延续原问题": ("Say whether this continues the original question", "元の質問の継続かどうかを指定してください", "원래 질문을 이어가는지 표시하세요"),
    "请求字段不正确": ("Request fields are invalid", "リクエスト項目が正しくありません", "요청 필드가 올바르지 않습니다"),
    "请选择图片": ("Choose an image", "画像を選んでください", "이미지를 선택하세요"),
    "请选择塔罗师": ("Choose a tarotist", "タロティストを選んでください", "타로티스트를 선택하세요"),
    "读取图片失败": ("Failed to read the image", "画像の読み取りに失敗しました", "이미지를 읽지 못했습니다"),
    "账号已停用": ("Account is disabled", "アカウントは停止されています", "계정이 정지되었습니다"),
    "账号已合并": ("Account already merged", "アカウントは統合済みです", "계정이 이미 병합되었습니다"),
    "账号已合并到其他账号": ("Account was merged into another account", "アカウントは他のアカウントに統合されました", "계정이 다른 계정으로 병합되었습니다"),
    "账号已合并，请补发到主账号": ("Account was merged. Grant to the primary account.", "アカウントは統合済みです。主アカウントへ補填してください。", "계정이 병합되었습니다. 주 계정으로 지급하세요."),
    "账号已注销": ("Account cancelled", "アカウントは退会済みです", "계정이 탈퇴되었습니다"),
    "货币只支持 CNY 或 USD": ("Currency must be CNY or USD", "通貨は CNY または USD のみです", "통화는 CNY 또는 USD만 가능합니다"),
    "还不能看未来月份": ("Future months are not available yet", "未来の月はまだ見られません", "미래 달은 아직 볼 수 없습니다"),
    "这张牌已经用过": ("This card was already used", "このカードは使用済みです", "이 카드는 이미 사용되었습니다"),
    "连通性探测失败": ("Connectivity probe failed", "疎通確認に失敗しました", "연결 확인에 실패했습니다"),
    "追问不能为空": ("Follow-up cannot be empty", "追問は空にできません", "추가 질문은 비울 수 없습니다"),
    "追问次数上限须为正整数，或 null 表示不限制": ("Follow-up limit must be a positive integer, or null for unlimited", "追問上限は正의 정수、または null で無制限", "추가 질문 한도는 양의 정수이거나, null이면 제한 없음"),
    "追问额度键不正确": ("Follow-up quota key is invalid", "追問クォータキーが正しくありません", "추가 질문 한도 키가 올바르지 않습니다"),
    "邮件服务未配置": ("Mail is not configured", "メールサービスが未設定です", "메일 서비스가 설정되지 않았습니다"),
    "邮箱不正确": ("Email is invalid", "メールアドレスが正しくありません", "이메일이 올바르지 않습니다"),
    "邮箱和密码必填": ("Email and password are required", "メールとパスワードは必須です", "이메일과 비밀번호는 필수입니다"),
    "邮箱已注册": ("Email already registered", "このメールは登録済みです", "이미 등록된 이메일입니다"),
    "邮箱已绑定或账号已注销": ("Email is already bound or the account was cancelled", "メールは連携済み、またはアカウント退会済みです", "이메일이 이미 연결되어 있거나 계정이 탈퇴되었습니다"),
    "邮箱或密码错误": ("Incorrect email or password", "メールまたはパスワード가 올바르지 않습니다", "이메일 또는 비밀번호가 올바르지 않습니다"),
    "邮箱格式不正确": ("Email format is invalid", "メール形式が正しくありません", "이메일 형식이 올바르지 않습니다"),
    "重复请求处理中": ("A duplicate request is already in progress", "重複リクエストを処理中です", "중복 요청이 처리 중입니다"),
    "重置时刻不可修改": ("Reset time cannot be changed", "リセット時刻は変更できません", "리셋 시각은 바꿀 수 없습니다"),
    "钻石发放须为正整数": ("Diamond grant must be a positive integer", "ダイヤモンド付与は正의 정수여야 합니다", "다이아 지급은 양의 정수여야 합니다"),
    "钻石桶只允许 pack 或 subscription，默认 pack（pack_diamonds）": ("Diamond bucket must be pack or subscription; default pack (pack_diamonds)", "ダイヤモンドバケットは pack 또는 subscription。기본 pack（pack_diamonds）", "다이아 버킷은 pack 또는 subscription이며, 기본은 pack(pack_diamonds)"),
    "问题不存在": ("Article not found", "記事がありません", "글이 없습니다"),
    "问题不能为空": ("Question cannot be empty", "質問は空에 できません", "질문은 비울 수 없습니다"),
    "问题分类不正确": ("Help category is invalid", "質問分類が正しくありません", "질문 분류가 올바르지 않습니다"),
    "问题检测失败": ("Question detect failed", "質問検出に失敗しました", "질문 감지에 실패했습니다"),
    "问题超过 200 个码点，正式计数口径尚未确认": ("Question exceeds 200 code points. Official counting is not finalized.", "質問が200コードポイントを超えています。正式な数え方は未確定です。", "질문이 200 코드포인트를 넘었습니다. 공식 집계 기준은 아직 확정되지 않았습니다."),
    "额度参数不正确": ("Quota parameters are invalid", "クォータ項目が正しくありません", "한도 항목이 올바르지 않습니다"),
    "额度扣减冲突": ("Quota debit conflict", "クォータ控除が衝突しました", "한도 차감이 충돌했습니다"),
    "额度未扣减，记录已保留，不继续生成": ("Quota was not charged. Record kept; generation will not continue.", "クォータ未控除。기록은 유지되며 생성은 계속하지 않습니다.", "한도를 차감하지 않았습니다. 기록은 유지되고 생성은 이어지지 않습니다."),
    "验单状态不正确": ("Receipt status is invalid", "検証ステータスが正しくありません", "검증 상태가 올바르지 않습니다"),
    "验单进行中": ("Receipt verification is in progress", "レシート検証は進行中です", "영수증 검증이 진행 중입니다"),
    "验证码不正确或已过期": ("Verification code is incorrect or expired", "確認コードが違うか期限切れです", "인증 코드가 틀리거나 만료되었습니다"),
    "验证码场景不正确": ("Verification scene is invalid", "確認コードのシーンが正しくありません", "인증 코드 장면이 올바르지 않습니다"),
    "验证码必填": ("Verification code is required", "確認コードは必須です", "인증 코드는 필수입니다"),
    "请填写补发原因": ("Enter a grant reason", "補填理由を入力してください", "지급 사유를 입력하세요"),
}

# Fix a few strings that I accidentally mixed languages while drafting.
ERRORS["供应商地址只允许 HTTPS；本机演示可使用 http://127.0.0.1 或 http://localhost"] = (
    "Vendor URL must be HTTPS; local demo may use http://127.0.0.1 or http://localhost",
    "ベンダーURLは HTTPS のみ。ローカルデモは http://127.0.0.1 または http://localhost 可",
    "공급자 주소는 HTTPS만 허용합니다. 로컬 데모는 http://127.0.0.1 또는 http://localhost",
)
ERRORS["登录后才能查看兑换记录"] = ("Log in to view redeem records", "交換記録の表示にはログインが必要です", "교환 기록을 보려면 로그인하세요")
ERRORS["登录后才能查看消息"] = ("Log in to view messages", "メッセージの表示にはログインが必要です", "메시지를 보려면 로그인하세요")
ERRORS["解读次数须为正整数"] = ("Reading grant must be a positive integer", "解読回数は正の整数である必要があります", "해석 횟수는 양의 정수여야 합니다")
ERRORS["追问次数上限须为正整数，或 null 表示不限制"] = (
    "Follow-up limit must be a positive integer, or null for unlimited",
    "追問上限は正の整数、または null で無制限です",
    "추가 질문 한도는 양의 정수이거나, null이면 제한 없음입니다",
)
ERRORS["邮箱或密码错误"] = ("Incorrect email or password", "メールまたはパスワードが違います", "이메일 또는 비밀번호가 올바르지 않습니다")
ERRORS["问题不能为空"] = ("Question cannot be empty", "質問は空にできません", "질문은 비울 수 없습니다")
ERRORS["钻石发放须为正整数"] = ("Diamond grant must be a positive integer", "ダイヤモンド付与は正の整数である必要があります", "다이아 지급은 양의 정수여야 합니다")
ERRORS["钻石桶只允许 pack 或 subscription，默认 pack（pack_diamonds）"] = (
    "Diamond bucket must be pack or subscription; default pack (pack_diamonds)",
    "ダイヤモンドバケットは pack または subscription。デフォルトは pack（pack_diamonds）",
    "다이아 버킷은 pack 또는 subscription이며, 기본은 pack(pack_diamonds)",
)
ERRORS["额度未扣减，记录已保留，不继续生成"] = (
    "Quota was not charged. Record kept; generation will not continue.",
    "クォータは未控除です。記録は保持し、生成は続けません。",
    "한도를 차감하지 않았습니다. 기록은 유지되고 생성은 이어지지 않습니다.",
)

for zh, tri in ERRORS.items():
    add(zh, *tri)

# zh-Hans identity for every msgid so missing keys still round-trip.
zh_hans = {k: k for k in CATALOG}
en = {k: v[0] for k, v in CATALOG.items()}
ja = {k: v[1] for k, v in CATALOG.items()}
ko = {k: v[2] for k, v in CATALOG.items()}
zh_hant = {k: trad(k) for k in CATALOG}
# Prefer explicit traditional overrides already in trad().
for k, v in TRAD_OVERRIDE.items():
    if k in zh_hant:
        zh_hant[k] = v

# Question bodies get character conversion via trad().

def dump(name: str, data: dict[str, str]) -> None:
    path = HERE / f"{name}.json"
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(name, len(data))

dump("zh-Hans", zh_hans)
dump("zh-Hant", zh_hant)
dump("en", en)
dump("ja", ja)
dump("ko", ko)
