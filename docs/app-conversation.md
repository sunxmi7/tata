# 普通占卜对话协议

给 App 渲染用。从请求解读开始，对话都放在 `data.messages` 里。App 只看每条的 `kind`、`from`、`style` 决定用哪一块画面。

外层统一是：

```json
{
  "code": 0,
  "message": "",
  "data": {
    "reading_id": 1569,
    "has_more": false,
    "messages": []
  }
}
```

`has_more` 为 `true` 表示这一页后面还有。问题推荐发生在创建占卜之前，那时还没有 `reading_id`，只返回 `messages` 和 `has_more`。

## 常规抽牌怎么走

1. `GET /api/app/v1/preset-questions` 取预设问题。用户选中的那一句，就是这次的问题。
2. 预设都不满意时，用户自己输入。`POST /api/app/v1/question-suggestions` 按这句话做问题推荐。用户选中推荐里的一句，就用那一句。
3. 推荐也不满意，就用用户自己输入的那句。
4. `POST /api/app/v1/readings` 用这句最终问题创建占卜，返回占卜 ID。已经知道抽牌方式时带上 `draw_mode`。
5. `GET /api/app/v1/readings/{id}/spread-choices` 拉牌阵选项，`POST /api/app/v1/readings/{id}/spread` 提交选中的牌阵，并带上最终的 `draw_mode`。
6. 洗牌、抽牌在客户端完成。
7. `POST /api/app/v1/readings/{id}/interpret` 把占卜 ID、牌阵 ID、抽好的牌一起交上来。服务端用占卜 ID 查出问题，再解读。
8. 之后的追问、补抽、历史，都返回同一份 `messages`。

创建占卜之前没有占卜 ID，预设列表和问题推荐不会写进后来的历史。洗牌、抽牌还没随解读请求交上来时，服务端只停在「问题加已选牌阵」。

## 进度

`GET /api/app/v1/readings/{id}` 是这条占卜的记录，用来判断停在哪一步。`progress` 是给界面看的进度文案。对话内容以 `GET /api/app/v1/readings/{id}/messages` 为准。

| 状态 | progress | 已经记下的内容 |
|---|---|---|
| `QUESTION_CREATED` | 问题已提交 | 类型二 |
| `SPREAD_SELECTED` | 牌阵已选择 | 类型二，记录上有牌阵 ID |
| `SHUFFLING` | 洗牌中 | 类型二，记录上有牌阵 ID |
| `CUTTING` | 切牌中 | 类型二，记录上有牌阵 ID |
| `CARDS_FIXED` | 牌面已固定 | 类型二、类型一 |
| `INTERPRETING` | 解读生成中 | 类型二、类型一，已写出的类型三 |
| `FAILED_RETRYABLE` | 失败可重试 | 类型二、类型一 |
| `COMPLETED` | 已完成 | 类型二、类型一、类型三，以及已经发生的追问 |

停在补抽没点时，历史里仍有类型二十，`pending` 为 `true`。记录上同时有 `redraw_pending: true` 和 `redraw`。点过之后 `redraw_pending` 为 `false`。

## 顺序

同一条占卜按发生顺序返回，追问次数再多也不重排。

1. 类型二，最初的问题
2. 类型一，最初交上来的牌阵
3. 类型三，第一次解读
4. 第一轮追问：类型十九、类型三（可多条）、类型二十、点了去抽牌才有类型二十一
5. 第二轮追问接在第一轮后面，以此类推

每一轮追问有自己的 `turn_id`，轮次越大越靠后。同一轮内部按 `seq` 排。分页只是在这个顺序上截取。

追问请求里 `continuation` 为 `false` 时，另开一条新占卜，不写进当前列表。返回：

```json
{
  "new_reading": true,
  "charged_here": false,
  "reading": {}
}
```

`reading` 是新建占卜的记录。

## 历史分页

`GET /api/app/v1/readings/{id}/messages?limit=20&offset=0`

不传 `limit` 时返回全部，`has_more` 为 `false`。`offset` 是已经看过的条数。

## 怎么判断画面

每条消息都有：

| 字段 | 取值 |
|---|---|
| `kind` | `question`、`cards`、`interpretation`、`suggestion` |
| `from` | `user` 或 `server` |
| `style` | 下表里的样式 |

用 `kind + style` 选组件。`from` 区分用户和服务端。十二、十五、十六、十八、二十二由客户端自己画，接口不返回。

| 类型 | kind / style | from | 画面 | 固定字段 |
|---|---|---|---|---|
| 二 | `question` / `labeled` | user | 小标题「你的问题」，下面是问题正文，有边框 | `label`、`text` |
| 十九 | `question` / `bubble` | user | 带尾巴的对话气泡，只有一句用户的话 | `turn_id`、`text` |
| 十三 | `suggestion` / `chip` | server | 白色胶囊，只有一句推荐问题 | `text` |
| 十四 | `suggestion` / `option` | server | 问题加灰色说明，右边空心圆点 | `text`、`hint`、`selected` |
| 四 | `suggestion` / `followup` | server | 星标标题「继续解读」，下面一颗胶囊 | `title`、`text` |
| 五 | `suggestion` / `recommend` | server | 标题按实际条数写成「为你推荐 N 个更清晰的问题」，下面每一项都是类型十四 | `title`、`options` |
| 一 | `cards` / `spread` | server | 星标「牌面预览」，多张牌 | `title`、`cards` |
| 二十一 | `cards` / `single` | server | 标题「牌面」，左边有尾巴，一张牌 | `title`、`turn_id`、`cards` |
| 七 | `cards` / `pick` | server | 牌背按牌位排，上面是用户输入的数字，不对的牌位标红 | `slots` |
| 三 | `interpretation` / `body` | server | 一整段解读，没有标题，没有尾巴 | `text`。追问回答再加 `turn_id`、`seq` |
| 六 | `interpretation` / `detect` | server | 「塔塔 · 问题检测」加「已完成」，再加「为什么建议修改？」和说明 | `title`、`status`、`reason_title`、`text` |
| 八、九 | `interpretation` / `guide` | server | 标题「输入专属数字」、示例输入框、两条灰色规则。两张图是同一个返回 | `title`、`placeholder`、`rules` |
| 十、十一 | `interpretation` / `error` | server | 同一块输入区，数字变红。十和十一只是 `text` 不同 | `text`、`numbers` |
| 十七 | `cards` / `choose` | server | 「先确认牌阵和张数」，牌阵卡。带推荐的一张 `recommended` 为 `true` | `title`、`spreads` |
| 二十 | `suggestion` / `redraw` | server | 标题「补抽牌面」，左边有尾巴，按钮是「去抽牌」「跳过」 | `title`、`turn_id`、`actions`、`pending`。点过之后再加 `choice` |

客户端自己画、不进 `messages` 的：

| 类型 | 画面 |
|---|---|
| 十五 | 「更多问题」按钮 |
| 十八 | 「抽牌方式」里的「常规抽牌」「灵数抽牌」 |
| 十二 | 空灰块 |
| 十六 | 一短横加一个点 |
| 二十二 | 类型二十、二十一外面的空气泡壳 |

类型十九旁边那颗空气泡也是同一个壳，接口不会再返回一条空消息。

## 牌

`cards` 里每一张：

| 字段 | 说明 |
|---|---|
| `position` | 牌位序号，从 1 开始 |
| `position_name` | 牌位名，例如「过去」 |
| `card_id` | 牌编号 |
| `name` | 牌名 |
| `orientation` | `upright` 正位，`reversed` 逆位 |
| `image` | 图片绝对地址。没有图时是空字符串 |

灵数牌多一个 `number`。普通抽牌没有这个字段。

补抽的类型二十一追加在类型一后面，不替换原来的牌阵。类型一始终只含当初牌阵的张数。

## 各接口

### 预设问题

`GET /api/app/v1/preset-questions`

这是题库，不是对话消息。用户点中的 `body` 作为创建占卜的 `question`。

### 问题推荐

`POST /api/app/v1/question-suggestions`

```json
{ "question": "这是我自己输入的问题" }
```

返回的 `messages` 顺序是类型六、类型十三、类型十四、类型四、类型五。推荐句子来自预设题库。用户不选，就仍用自己输入的那句去创建。

```json
{
  "has_more": false,
  "messages": [
    {
      "kind": "interpretation",
      "from": "server",
      "style": "detect",
      "title": "塔塔 · 问题检测",
      "status": "已完成",
      "reason_title": "为什么建议修改？",
      "text": "可以改用下面的问法，也可以继续用你输入的问题。"
    },
    {
      "kind": "suggestion",
      "from": "server",
      "style": "chip",
      "text": "最近的我，最需要关注什么？"
    },
    {
      "kind": "suggestion",
      "from": "server",
      "style": "option",
      "text": "这段关系还值得继续吗？",
      "hint": "感情关系",
      "selected": false
    },
    {
      "kind": "suggestion",
      "from": "server",
      "style": "followup",
      "title": "继续解读",
      "text": "最近的我，最需要关注什么？"
    },
    {
      "kind": "suggestion",
      "from": "server",
      "style": "recommend",
      "title": "为你推荐 3 个更清晰的问题",
      "options": [
        { "text": "最近的我，最需要关注什么？", "hint": "自我成长", "selected": false },
        { "text": "这段关系还值得继续吗？", "hint": "感情关系", "selected": false }
      ]
    }
  ]
}
```

类型五标题里的数字是这次实际返回的条数，不要写死成 3。

### 创建占卜

`POST /api/app/v1/readings`

```json
{ "question": "最近的我，最需要关注什么？", "type": "normal", "source": "preset", "draw_mode": "normal" }
```

`draw_mode` 是抽牌方式：`normal` 常规抽牌，`numerology` 灵数抽牌。还没选可以不传，记录上先是空字符串。`type` 仍是占卜种类，灵数也是 `normal`。探索、塔罗师、今日运势不要带这个字段。

自己输入的问题不要带 `source: "preset"`。返回的记录里有 `id` 和 `draw_mode`，后面的路径都用这个 ID。占卜详情和历史列表同样返回 `draw_mode`。

第一次写成 `numerology` 时，服务端生成这套 78 张的数字和正逆位。之后改回常规再改回灵数，仍用这一套。这个映射不返回给 App。

### 选牌阵

`GET /api/app/v1/readings/{id}/spread-choices`

```json
{
  "reading_id": 1569,
  "has_more": false,
  "messages": [
    {
      "kind": "cards",
      "from": "server",
      "style": "choose",
      "title": "先确认牌阵和张数",
      "spreads": [
        { "id": 1, "name": "时间流", "card_count": 3, "recommended": true, "selected": false },
        { "id": 2, "name": "一张牌", "card_count": 1, "recommended": false, "selected": false }
      ]
    }
  ]
}
```

用户点中后：

`POST /api/app/v1/readings/{id}/spread`

```json
{ "spread_id": 1, "draw_mode": "normal" }
```

这里的 `draw_mode` 覆盖创建时记下的值，扣费按这一次算。普通占卜还是空字符串时不扣，返回「请先选择抽牌方式」。扣过之后不能再改。灵数用 `numerology`，并且走赠送和邀请的适用范围。

这一步返回的是占卜记录，不是 `messages`。记录里的 `draw_mode` 就是这次提交的值。

### 请求解读

`POST /api/app/v1/readings/{id}/interpret`

```json
{
  "spread_id": 1,
  "cards": [
    { "position": 1, "position_name": "过去", "name": "愚者", "orientation": "upright" },
    { "position": 2, "position_name": "现在", "name": "魔术师", "orientation": "reversed" },
    { "position": 3, "position_name": "未来", "name": "女祭司", "orientation": "upright" }
  ]
}
```

牌名必须是牌库里的名字。`orientation` 只能是 `upright` 或 `reversed`。张数必须等于牌阵张数。同一张牌不能出现两次。服务端按牌名补 `card_id`。

成功后 `data.messages`：

```json
[
  {
    "kind": "question",
    "from": "user",
    "style": "labeled",
    "label": "你的问题",
    "text": "最近的我，最需要关注什么？"
  },
  {
    "kind": "cards",
    "from": "server",
    "style": "spread",
    "title": "牌面预览",
    "cards": [
      {
        "position": 1,
        "position_name": "过去",
        "card_id": 0,
        "name": "愚者",
        "orientation": "upright",
        "image": "http://127.0.0.1:8080/uploads/cards/rider_waite_tarot_big_the_fool_00.jpg"
      },
      {
        "position": 2,
        "position_name": "现在",
        "card_id": 1,
        "name": "魔术师",
        "orientation": "reversed",
        "image": "http://127.0.0.1:8080/uploads/cards/rider_waite_tarot_big_the_magician_01.jpg"
      },
      {
        "position": 3,
        "position_name": "未来",
        "card_id": 2,
        "name": "女祭司",
        "orientation": "upright",
        "image": "http://127.0.0.1:8080/uploads/cards/rider_waite_tarot_big_the_high_priestess_02.jpg"
      }
    ]
  },
  {
    "kind": "interpretation",
    "from": "server",
    "style": "body",
    "text": "这里是整段解读。"
  }
]
```

### 追问

`POST /api/app/v1/readings/{id}/followups`

```json
{ "text": "帮我补一张", "continuation": true }
```

常规抽牌追问之后接类型二十。`data.messages` 是到这一步为止的整段对话，新的几条在最后：

```json
[
  {
    "kind": "question",
    "from": "user",
    "style": "bubble",
    "turn_id": 1,
    "text": "帮我补一张"
  },
  {
    "kind": "interpretation",
    "from": "server",
    "style": "body",
    "turn_id": 1,
    "seq": 1,
    "text": "先看清你现在卡住的地方。"
  },
  {
    "kind": "interpretation",
    "from": "server",
    "style": "body",
    "turn_id": 1,
    "seq": 2,
    "text": "再决定要不要补一张牌。"
  },
  {
    "kind": "suggestion",
    "from": "server",
    "style": "redraw",
    "turn_id": 1,
    "title": "补抽牌面",
    "actions": [
      { "action": "draw", "label": "去抽牌" },
      { "action": "skip", "label": "跳过" }
    ],
    "pending": true
  }
]
```

追问回答最多四条类型三。上面只示意两条。

这一步的 `data` 里还会带额度字段：`quota_outcome`、`entitlement`、`charged`，以及一份和最后那条类型二十相同的 `redraw`。画面仍按 `messages` 画。

### 补抽

`POST /api/app/v1/readings/{id}/followups/redraw`

```json
{ "action": "draw" }
```

`action` 为 `skip` 时不增加牌。类型二十的 `pending` 变为 `false`，并带上 `choice`。

去抽牌时，列表末尾多一条类型二十一：

```json
{
  "kind": "cards",
  "from": "server",
  "style": "single",
  "title": "牌面",
  "turn_id": 1,
  "cards": [
    {
      "position": 4,
      "position_name": "",
      "card_id": 71,
      "name": "星币八",
      "orientation": "upright",
      "image": "http://127.0.0.1:8080/uploads/cards/rider_waite_tarot_small_eight_of_pentacles_50.jpg"
    }
  ]
}
```

对应的类型二十变成：

```json
{
  "kind": "suggestion",
  "from": "server",
  "style": "redraw",
  "turn_id": 1,
  "title": "补抽牌面",
  "actions": [
    { "action": "draw", "label": "去抽牌" },
    { "action": "skip", "label": "跳过" }
  ],
  "pending": false,
  "choice": "draw"
}
```

跳过时 `choice` 是 `skip`，没有类型二十一。

`data` 里同样有整段 `messages`。去抽牌时还有一个 `card`，内容和类型二十一相同；`redraw` 是更新后的类型二十。

### 历史详情

`GET /api/app/v1/readings/{id}/messages`

占卜 1569 实际返回的顺序是：

1. 类型二，「最近的我，最需要关注什么？」
2. 类型一，过去愚者正位、现在魔术师逆位、未来女祭司正位
3. 类型三，第一次解读
4. 类型十九，「帮我补一张」
5. 类型三，追问回答第一段，`seq` 为 1
6. 类型三，追问回答第二段，`seq` 为 2
7. 类型二十，`choice` 为 `draw`，`pending` 为 `false`
8. 类型二十一，星币八正位

第二轮追问会从第 9 条开始，`turn_id` 为 2，结构与第 4 到第 8 条相同。

### 灵数抽牌

需求 7.3 和图「灵数抽牌规则」说的是：选完牌阵之后，输入方式和校验都一样，只有张数跟着牌阵走。设计图用三张牌举例，用户就输入 3 个数字。张数小于等于 6 时，数字放在一个输入框；大于 6 时，输入框按每行 6 个分行。`card_count` 是牌阵张数，`input_rows` 是输入框行数。

还没填数字时返回类型八、九。两条规则用设计图上的原文：

`POST /api/app/v1/readings/{id}/numerology`

```json
{ "numbers": [] }
```

```json
{
  "kind": "interpretation",
  "from": "server",
  "style": "guide",
  "title": "输入专属数字",
  "placeholder": "例如：12、58、62",
  "card_count": 3,
  "input_rows": 1,
  "rules": [
    "按牌位的顺序输入，数字之间用顿号隔开。",
    "输入范围：1-78的整数，数字不可重复。"
  ]
}
```

数字按牌位顺序对应。牌位上的 `mark` 用设计图里的字：`待选择`、`已选择`、`重复数字`、`超出范围`。只有重复和超出范围时 `invalid` 为 `true`，牌位标红。

三张牌只交了 `12` 时，返回类型七和类型十一：

```json
[
  {
    "kind": "cards",
    "from": "server",
    "style": "pick",
    "card_count": 3,
    "slots": [
      { "position": 1, "position_name": "过去", "number": "12", "mark": "已选择", "invalid": false },
      { "position": 2, "position_name": "现在", "number": "", "mark": "待选择", "invalid": false },
      { "position": 3, "position_name": "未来", "number": "", "mark": "待选择", "invalid": false }
    ]
  },
  {
    "kind": "interpretation",
    "from": "server",
    "style": "error",
    "text": "请再输入 2 个数字",
    "numbers": ["12"]
  }
]
```

`12、12、67` 时，后一个 12 的 `mark` 是「重复数字」。类型十的 `text` 是「数字 12 已被使用，请更换一个数字。」

`12、79、58` 时，79 的 `mark` 是「超出范围」，这个数字不占张数，所以同时返回「请再输入 1 个数字」。

这条占卜的 `draw_mode` 必须是 `numerology`。确认后的解读、历史顺序和常规抽牌相同。牌上多一个 `number`。需求 7.3 的补抽是再输入 1 个数字，沿用这套 78 张映射，追加到历史里，不覆盖原来的牌。设计图的「去抽牌 / 跳过」没有单独的灵数页，所以类型二十只出现在 `draw_mode` 为 `normal` 的普通占卜追问后面。
