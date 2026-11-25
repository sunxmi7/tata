# 塔塔 App 分模块手工测试

按 Pixso 手机界面模块走，和 Apifox 文件夹同名。建议从上到下测，前面拿到的 token / 占卜 id 后面会用到。

**环境：** http://127.0.0.1:8080

**鉴权：** Apifox 在 **Headers** 加参数 `Authorization`，值 `Bearer <accessToken>`（Bearer 后有空格）。不要放进请求体。`Content-Type: application/json`

**包络：** 成功 `code=0`；业务失败仍是 HTTP 200、`code=1`、看 `message`；需要令牌却没带是 HTTP 401「未登录或令牌无效」。

公开目录（预设问题、牌阵、塔罗师、提示窗、活动列表/详情、帮助）**不带 token 就能过**。未登录占卜不带令牌，请求里带 `device_id`。额度、档案、今日一牌、兑换、年运必须登录。没有匿名登录。

---

## 建议顺序

- [ ] 1 登录注册（无依赖，最先测）
- [ ] 2 首页（目录公开；额度/今日一牌要令牌）
- [ ] 3 探索（目录公开；开局要令牌）
- [ ] 4 抽牌（令牌；选阵要用牌阵 id）
- [ ] 5 牌阵界面（跟抽牌同一条记录）
- [ ] 6 历史记录（至少有一条占卜）
- [ ] 7 日历记录（登录后抽过今日一牌）
- [ ] 8 我的中心（登录令牌）
- [ ] 9 账号（登录令牌；注销放到最后）
- [ ] 10 塔塔币购买（登录）
- [ ] 11 会员中心（登录）
- [ ] 12 兑换码（登录）
- [ ] 13 参与活动（列表/详情公开；参与记录/奖励记录要登录）
- [ ] 14 月度报告（登录；当月有占卜更好）
- [ ] 15 年度运势（登录；解锁依赖年卡或年运 SKU）
- [ ] 16 功能心愿单（登录）
- [ ] 17 反馈意见（登录）
- [ ] 18 帮助中心（公开）
- [ ] 19 通知设置（登录）
- [ ] 20 我的小组件（同日历，复用今日一牌）
- [ ] 21 语言（无独立接口）
- [ ] 22 关于塔塔（无独立接口）

## 不是 bug 的预期

- 解读失败「未绑定供应商」：管理端没绑 interpret 供应商，流程仍应停在可重试。
- 钻石单价 `quota_cost=null`：扣到钻石步会停，文案走付费墙 / 未配置，不要当成接口挂了。
- `POST /iap/verify` 不连苹果/谷歌，只做本地映射；商店商品号对不上就是「商品未映射」。
- 活动详情、帮助正文、兑换记录行字段仍是占位。
- 发码同一邮箱同一 scene 60 秒内只能发一次。

---

## 1. 登录注册

登录令牌标 USER。未登录占卜用设备号 DEVICE。把返回值记在文末表格里。没有匿名登录，也没有 `POST /auth/anonymous`。

### 1.1 发码（登录）

`POST /api/app/v1/auth/email/code`

```json
{ "email": "你的邮箱", "scene": "login" }
```

- [ ] `code=0`，`data.expires_in=300`，响应里没有验证码
- [ ] 邮箱收到 6 位码，记为 CODE
- [ ] 立刻再发一次，应「发送过于频繁」
- [ ] scene 乱填（含旧的 register），应「验证码场景不正确」
- [ ] 邮箱格式不对，应「邮箱不正确」
- [ ] 已有账号的邮箱也可以发 login 码，不会「邮箱已注册」

### 1.2 邮箱验证码登录（首次即创建账号）

可先用 DEVICE 走第 4 节建一条占卜，再登录；也可以先登录再抽。没有 `POST /auth/register`。

`POST /api/app/v1/auth/login`

```json
{
  "email": "你的邮箱",
  "code": "邮件里的6位码",
  "device_id": "这台设备的 DEVICE"
}
```

- [ ] `code=0`，有 accessToken、id、login=email
- [ ] 记 USER = accessToken，UID = id
- [ ] 带了 device_id 时，该设备上尚未归属的占卜会出现在登录后的历史里
- [ ] 多传 anonymous_id，应「请求字段不正确」
- [ ] 验证码填错，应「验证码不正确或已过期」
- [ ] 漏填验证码，应「邮箱和验证码必填」
- [ ] 多传 password，应「请求字段不正确」

### 1.3 已有账号再登录

等 60 秒冷却后再发一次 login 码，或换邮箱。`POST /api/app/v1/auth/login`

```json
{
  "email": "你的邮箱",
  "code": "新的6位码",
  "device_id": "这台设备的 DEVICE"
}
```

- [ ] 成功拿到新的 USER 令牌，login=email
- [ ] 验证码用过即作废，再用应「验证码不正确或已过期」

### 1.4 微信 / 苹果（可选）

客户端把微信 SDK 的 `code`、苹果 `identity_token` 交给服务端校验。微信 iOS/Android 共用一套 AppID；苹果登录仅 iOS，安卓没有。`config.yaml` 的 `wechat` / `apple` 未配时接口返回「微信登录未配置」或「苹果登录未配置」，不必等凭证也能测这条错误。凭证到了把 `enabled` 设为 true 再测真登录。请求体可带 `device_id`，用来挂设备上的占卜。

`POST /api/app/v1/auth/wechat`

```json
{ "code": "微信SDK返回的code", "device_id": "这台设备的 DEVICE" }
```

`POST /api/app/v1/auth/apple`

```json
{ "identity_token": "苹果identity_token", "device_id": "这台设备的 DEVICE" }
```

- [ ] 未配置时「微信登录未配置」/「苹果登录未配置」
- [ ] 配置后首次会建用户
- [ ] 同一微信 / 苹果身份再调是登录
- [ ] 多传 anonymous_id，应「请求字段不正确」

---

## 2. 首页

`banners` / `tarotists` / `spreads` 公开，可不带令牌。`quota` / `deck-preference` / `daily` 必须登录。

- [ ] `GET /api/app/v1/banners`：当前有效提示窗，没有就空数组；不带 token 也能过
- [ ] `GET /api/app/v1/tarotists`：至少有知晚 / 明序 / 隐川 / 照野；记下要用的 id（种子知晚一般是 9）
- [ ] `GET /api/app/v1/spreads`：有「时间流」，recommended=true，card_count=3；记下 spread_id（一般是 1）
- [ ] `GET /api/app/v1/quota` 不带令牌：HTTP 401
- [ ] `GET /api/app/v1/quota` 用登录令牌：能看到今日免费或会员额度
- [ ] `GET /api/app/v1/deck-preference`：当前 code=rider-waite，available 含莱德韦特
- [ ] `PUT /api/app/v1/deck-preference` body `{"code":"rider-waite"}`；未上架 code 应被拒绝
- [ ] `POST /api/app/v1/daily` 不带令牌：应「登录后才能抽今日一牌」或 HTTP 401
- [ ] `POST /api/app/v1/daily` 用登录令牌，body `{"target":"today"}`：首次成功或额度拦截；同一天再抽应 code=1 且 data.reused=true

---

## 3. 探索

这期没有 `GET /explore/topics`。开局直接 `POST /readings`，未登录带 `device_id`，已登录带令牌。

- [ ] `GET /api/app/v1/explore/topics`：应 404
- [ ] 探索主题下的牌阵仍可从 `GET /spreads` 或运营配置里认

点进主题后开局走抽牌，不要另找探索专用创建接口。

`POST /api/app/v1/readings`

```json
{
  "question": "这件事该不该做",
  "type": "explore",
  "source": "explore"
}
```

- [ ] code=0，记下 READING_ID
- [ ] 探索这次不在创建时扣费（quota_outcome 为 deferred 一类）

后面选阵 / 抽牌 / 解读与第 4 节相同。

---

## 4. 抽牌

用登录令牌走一条完整「普通占卜」。时间流 3 张。提问页预设题库公开。

- [ ] `GET /api/app/v1/preset-questions`：不带 token；`featured` 15 条、`backup` 36 条

### 4.1 创建

`POST /api/app/v1/readings`

```json
{ "question": "我最近的工作运势如何？", "type": "normal" }
```

- [ ] 状态 QUESTION_CREATED，记下 READING_ID
- [ ] 问题超过 200 字应被拒绝
- [ ] 塔罗师局：type=tarotist 且带 tarotist_id

### 4.2 改问题（抽牌前）

这期没有 `PUT /readings/{id}/question`。问题在创建时一次定好；创建前可用 `POST /question-suggestions` 换更清晰的问法。

### 4.3 选阵 → 客户端抽牌 → 解读

服务端没有 detect / shuffle / cut / draw。洗牌、切牌、抽牌在客户端完成。

1. `GET /api/app/v1/readings/{id}/spread-choices`（未登录带 `?device_id=`）
2. `POST /api/app/v1/readings/{id}/spread` `{"spread_id":1,"draw_mode":"normal"}`
3. 客户端抽好牌后，`POST /api/app/v1/readings/{id}/interpret` 带上牌面（必须登录）

灵数：`draw_mode` 为 `numerology` 后，`POST /api/app/v1/readings/{id}/numerology` 交 `numbers`，个数等于牌阵张数，1–78 不重复。

- [ ] 普通占卜没选 `draw_mode` 就提交牌阵，应「请先选择抽牌方式」
- [ ] 灵数数字不齐或重复时，返回类型九 / 十，不落牌

### 4.4 解读

`POST /api/app/v1/readings/{id}/interpret` 必须登录，body 带 `spread_id` 和 `cards`。

- [ ] 供应商已绑：`data.messages` 含类型二、一、三
- [ ] 未绑：失败可重试，不重复扣费
- [ ] 流式（可选）：`POST /api/app/v1/readings/{id}/interpret/stream`

### 4.5 追问

- [ ] 解读成功后再追问：`POST /api/app/v1/readings/{id}/followups`

```json
{ "text": "能再详细说说吗？", "continuation": true }
```

- [ ] `GET /api/app/v1/readings/{id}` 的 messages 能看到追问对话
- [ ] continuation=false 会另开一条普通占卜
- [ ] 没有 `confirm-quota` / `use-reward` / `paywall` 这些公开接口

---

## 5. 牌阵界面

与抽牌共用接口，重点看选阵屏数据。

- [ ] `GET /api/app/v1/spreads`：推荐角标、示意图 thumbnail（没配则默认牌背）
- [ ] `GET /api/app/v1/readings/{id}/spread-choices`：一条类型十六，含抽牌方式
- [ ] `POST /api/app/v1/readings/{id}/spread`：只接受启用中的牌阵，并带 `draw_mode`

---

## 6. 历史记录

登录令牌。

- [ ] `GET /api/app/v1/readings`：分页，limit 默认 20，可用 before_id 翻下一页
- [ ] 列表项有 short_text / card_name / status_label（没有牌则缩略图走默认）
- [ ] 列表项有 conversation_type（daily/moment/theme）与 stage/stage_label；daily 有 target 今日/明日
- [ ] 探索创建带 theme_key=yes_or_no → moment/此刻抉择；relationship → theme/主题占卜；未带 theme_key 选阵后能回填
- [ ] App 区分：type=explore && conversation_type=moment → 此刻抉择；type=explore && conversation_type=theme → 主题占卜（不能只看 type）
- [ ] `GET /api/app/v1/readings/{id}`：自己的记录能读；别人的或不存在为业务错误；响应含 messages / has_more
- [ ] `DELETE /api/app/v1/readings/{id}`：未登录不能删（「登录后才能删除」）；登录后软删，再读应当不存在

---

## 7. 日历记录

必须登录。先保证第 2 节抽过今日一牌。

- [ ] `POST /api/app/v1/daily` `{"target":"today"}`
- [ ] `POST /api/app/v1/daily` `{"target":"tomorrow"}`：明日运势；再抽应复用、不重复扣
- [ ] target 乱填，应「目标日期不正确」
- [ ] `GET /api/app/v1/calendar`：当月有点
- [ ] `GET /api/app/v1/calendar?type=daily`：能过滤

---

## 8. 我的中心

无独立创建接口，只读聚合。

- [ ] `GET /api/app/v1/profile`：有 public_id（6 位，第一次读会生成）、account_display（脱敏邮箱）；响应里没有 phone
- [ ] `GET /api/app/v1/quota`
- [ ] `GET /api/app/v1/iap/entitlements`：钻石两桶 + 展示名「塔塔币」
- [ ] `GET /api/app/v1/monthly-reports`：入口能打开
- [ ] `GET /api/app/v1/yearly-reports/2026`：入口能打开，未解锁时 unlocked=false

---

## 9. 账号

登录令牌。注销会毁掉当前测试号，放到本模块最后，或换小号测。

- [ ] `GET /api/app/v1/profile`
- [ ] 改头像：先 `POST /api/app/v1/uploads`（folder 缺省 misc），再 `PUT /profile` 把返回的 url 写入 `avatar_url`。没有单独的上传头像接口
- [ ] `PUT /api/app/v1/profile`：`name` 或 `nickname` 均可；GET 两个字段值相同

```json
{
  "name": "塔塔用户",
  "nickname": "塔塔用户",
  "gender": "不愿透露",
  "timezone": "Asia/Shanghai",
  "birth_date": "1990-01-01"
}
```

```json
{ "avatar_url": "/uploads/misc/上一步返回的文件名.png" }
```

- [ ] name 超过 32 字应失败；gender 允许 女性 / 男性 / 其他 / 不愿透露（以及别名 女 / 男）
- [ ] 绑定微信 `POST /api/app/v1/auth/bind-wechat` `{"code":"微信SDK返回的code"}`（未配置应「微信登录未配置」）
- [ ] 绑定苹果 `POST /api/app/v1/auth/bind-apple` `{"identity_token":"苹果identity_token"}`（仅 iOS；未配置应「苹果登录未配置」）
- [ ] 没有绑定手机接口；`GET /profile` 不含 `phone`
- [ ] 绑邮箱：先 `POST /auth/email/code` scene=bind（要带登录令牌），再 `POST /auth/bind-email`

```json
{ "email": "另一个邮箱", "code": "验证码" }
```

- [ ] 身份已被其它账号占用时返回冲突包络，不要直接并账；确认走 `POST /api/app/v1/auth/merge-confirm`
- [ ] 注销 `POST /api/app/v1/auth/cancel`：旧 token 再请求应「账号已注销」；再用该邮箱登录应「该邮箱已注销，不能重复注册」

---

## 10. 塔塔币购买

登录。价格以管理端/种子为准，接口不会编造。

- [ ] `GET /api/app/v1/iap/skus`：有月卡/年卡/次卡/消耗包/年运解锁
- [ ] `GET /api/app/v1/iap/skus?kind=consumable_pack`：只出消耗包
- [ ] `GET /api/app/v1/iap/entitlements`
- [ ] `GET /api/app/v1/iap/config`：恢复购买开关
- [ ] `GET /api/app/v1/wallet/ledger`：没流水就是空数组
- [ ] `GET /api/app/v1/quota`：display_label=塔塔币，quota_cost 未配则为 null
- [ ] `POST /api/app/v1/iap/verify`：用未映射商品号，应「商品未映射」（不连商店）

```json
{
  "platform": "app_store",
  "product_id": "unmapped",
  "transaction_id": "txn-demo",
  "receipt": "demo-receipt"
}
```

- [ ] `POST /api/app/v1/iap/restore`：同样只做本地映射

管理端把某个 SKU 的 App Store 商品号填上之后，再用那个 product_id 验一次，才能测到入账。

---

## 11. 会员中心

- [ ] `GET /api/app/v1/iap/entitlements`
- [ ] `GET /api/app/v1/iap/skus?kind=month_subscription` 与 year_subscription
- [ ] 年卡 SKU 的 benefits 里追问次数应对 quota_settings.followup_limit（种子 5）
- [ ] 抽牌碰到付费墙时：`POST /api/app/v1/readings/{id}/paywall` `{"action":"resume"}` 或 `"cancel"`
- [ ] 验单接口与塔塔币相同，订阅到期字段看 expires_at

---

## 12. 兑换码

必须登录。本地若没有启用中的码，这条可以记「无码可测」或去管理端建一条。

- [ ] `POST /api/app/v1/redeem` `{"code":"实际兑换码"}`：成功则钻石或免费次数到账
- [ ] 无效码「兑换码无效」；用过「兑换码已使用」；过期「兑换码已过期」
- [ ] `GET /api/app/v1/redeem/records`：列表可空；fields_confirmed=false 是占位，不是失败

---

## 13. 参与活动

列表/详情公开。参与记录、奖励记录必须登录。

- [ ] `GET /api/app/v1/campaigns`：种子两条 friend_invite、membership_benefit；不带 token 也能过
- [ ] `GET /api/app/v1/campaigns/friend_invite`：详情 detail_status=pending，不要期望完整规则
- [ ] `GET /api/app/v1/campaigns/participations`：空列表 + 占位说明
- [ ] `GET /api/app/v1/campaigns/rewards`：空列表 + 占位说明

---

## 14. 月度报告

登录。当月有占卜时统计才不是全 0。

- [ ] `GET /api/app/v1/monthly-reports`：探索次数、陪伴天数、寄语（寄语来自当月已完成解读，没有就是空，不编造）
- [ ] `GET /api/app/v1/monthly-reports?month=2026-11`
- [ ] `POST /api/app/v1/monthly-reports/save` `{"month":"2026-11"}`：能保存
- [ ] 再 GET，对应月 saved=true

---

## 15. 年度运势

登录。路径 id 是**年份**，测 2026。

顺序：读卖点 → 解锁 → 资料（出生日期/时间/地点）→ 十二宫 12 张 → 四季 4 张 → 生成。

- [ ] `GET /api/app/v1/yearly-reports/2026`：未解锁 unlocked=false，step 指向卖点
- [ ] `POST /api/app/v1/yearly-reports/2026/unlock`：无年卡时，已定价会提示走内购；未定价「年卡未开通，单独购买价格未配置」
- [ ] 有年卡或解锁成功后再 `PUT /api/app/v1/yearly-reports/2026/profile`

```json
{ "birth_date": "1990-01-01", "birth_time": "08:30", "birth_place": "北京", "gender": "不愿透露" }
```

- [ ] 未解锁就填资料应失败；生日不能是未来；缺出生时间或地点应失败
- [ ] `POST /api/app/v1/yearly-reports/2026/rhythm` `{"mode":"normal"}`：12 张（自我到潜意识）
- [ ] `POST /api/app/v1/yearly-reports/2026/domains` `{"mode":"normal"}`：4 张，春/夏/秋/冬
- [ ] 步序不能跳：没填资料不能抽节奏，没抽节奏不能抽领域
- [ ] `POST /api/app/v1/yearly-reports/2026/generate`：走 interpret 供应商；没绑模型会失败，不编造正文

---

## 16. 功能心愿单

登录。

- [ ] `POST /api/app/v1/uploads?folder=feedback`（心愿/反馈图都走这个上传）

JSON 示例（1×1 png）：

```json
{
  "filename": "demo.png",
  "content_type": "image/png",
  "content_base64": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
}
```

- [ ] 返回可引用 url（本机形如 /uploads/feedback/...）
- [ ] `GET /uploads/card-default.png`：不用令牌也能读默认牌背
- [ ] `POST /api/app/v1/wishlist`

```json
{ "content": "希望增加更多牌阵", "image_url": "上一步的 url" }
```

- [ ] 超过 500 字应失败

---

## 17. 反馈意见

- [ ] `POST /api/app/v1/feedback`

```json
{
  "category": "内容建议",
  "content": "希望解读更具体一些",
  "email": "你的邮箱"
}
```

- [ ] category 只允许：使用问题 / 内容建议 / 其他
- [ ] 图仍走 `POST /uploads?folder=feedback`

---

## 18. 帮助中心

公开，不带 token 可读。

- [ ] `GET /api/app/v1/help/categories`：两类，使用与占卜、账号与设置，都是 ready
- [ ] `GET /api/app/v1/help/articles?category=reading`
- [ ] `GET /api/app/v1/help/articles?q=抽牌`
- [ ] `GET /api/app/v1/help/articles/how_to_start`：正文已写入，body_status=ready
- [ ] `GET /api/app/v1/help/articles/language_theme`：账号与设置正文 ready

---

## 19. 通知设置

登录。今日/明日抽牌提醒是 App 本地通知，不经服务端推。

- [ ] `GET /api/app/v1/notification-preferences`
- [ ] `PUT /api/app/v1/notification-preferences`

```json
{
  "daily_draw_enabled": true,
  "daily_draw_time": "08:00",
  "messages_enabled": true
}
```

- [ ] `POST /api/app/v1/push/register` `{"cid":"cid-demo","platform":"ios"}`：再打一次应幂等
- [ ] cid 为空，应「缺少推送 CID」
- [ ] `GET /api/app/v1/messages`：能列出广播（库里若有「双十一优惠」「新年活动上线」）
- [ ] `POST /api/app/v1/messages/{id}/read`：再拉列表 unread_count 减少
- [ ] 个推 enabled=false 时只落站内信，不算失败

---

## 20. 我的小组件

无同步接口。内容复用今日一牌。

- [ ] 用登录令牌再打一次 `POST /api/app/v1/daily` `{"target":"today"}`
- [ ] 已抽过则复用，不扣第二次

---

## 21. 语言

无 path。客户端本地保存。

- [ ] 任意已登录接口带 `Accept-Language: zh-Hans`（或 zh-Hant / en / ja / ko）
- [ ] 也可带 `X-Language`，语义相同
- [ ] 不带头时按简体；本轮业务文案多半仍是中文，只要接口不 500 即可

---

## 22. 关于塔塔

无 App 接口。版本、协议、分享、检查更新走客户端和静态资源。本模块打勾表示「确认没有漏接口」，不用发 HTTP。

- [ ] 契约里没有 /about、/version、/share

---

## 测完可记的数据

| 项 | 值 |
| --- | --- |
| device_id |  |
| 登录 token |  |
| user id |  |
| 时间流 spread_id |  |
| 塔罗师 tarotist_id |  |
| 普通占卜 reading_id |  |
| 探索占卜 reading_id |  |
| 今日一牌 reading_id |  |
| 站内信 message_id |  |
