# HTTP API 文档

机器可读契约（OpenAPI 3.0.3）：
- **Apifox（App 端）**：[openapi-app.yaml](./openapi-app.yaml)
- **完整契约（App + 管理端）**：[openapi.yaml](./openapi.yaml)

路径、方法、请求体和响应形状来自 `server/internal/tata/routes.go` 及对应 handler，只收录已注册的接口。

2026-10-28 起 App P0 壳层路径包括：`GET/PUT /api/app/v1/profile`（name/bio/avatar_url、account_display）、`GET /api/app/v1/explore/topics`（11 个探索主题、22 套牌阵）、`POST /api/app/v1/redeem`、`GET /api/app/v1/wallet/ledger`、`POST /api/app/v1/uploads`、`POST /api/app/v1/wishlist`、`POST /api/app/v1/feedback`、`GET/PUT /api/app/v1/notification-preferences`。额度账本的 UI 展示名为「额度」（`display_label`）。塔塔币已下线。

同日 App P1：`GET /api/app/v1/redeem/records`、`GET /api/app/v1/campaigns`、`GET /api/app/v1/campaigns/{id}`、`GET /api/app/v1/campaigns/participations`、`GET /api/app/v1/campaigns/rewards`、`GET /api/app/v1/help/categories`、`GET /api/app/v1/help/articles`（`category`、`q`）、`GET /api/app/v1/help/articles/{id}`。兑换记录、活动详情、参与记录、奖励记录仍是占位：列表可为空，不编造记录。帮助中心按文字稿只保留「使用与占卜」「账号与设置」两类，正文已写入。

2026-11-06 重启推送（Q-B1 作废）：新增 `POST /api/app/v1/push/register`（上报个推 CID）、`GET /api/app/v1/messages`（消息中心/站内信）、`POST /api/app/v1/messages/{id}/read`，以及管理端 `POST /api/admin/v1/push/broadcast`（群发）。今日/明日抽牌仍走 App 本地通知；优惠/活动走个推（GeTui）服务端下发，只推给 `messages_enabled=true` 的设备。个推凭证在 `server/manifest/config/config.yaml` 的 `getui`（`enabled=false` 时只落站内信、不真连）。

2026-11-06 接口完善（进行中）：
- **解读/追问 SSE 流式**：`POST /api/app/v1/readings/{id}/interpret/stream` 与 `.../followups/stream`。另有 `POST /daily/stream`、`POST /readings/{id}/detect/stream`、`POST /yearly-reports/{id}/generate/stream`。非流式 JSON 仍等完整结果；同一条 URL 也可带 `Accept: text/event-stream` 或 `?stream=1` 切到 SSE。事件为 `delta` / `done` / `error`。生成先落 `ai_jobs` 后台任务，SSE 断开不取消；App 再进用 `GET /readings/{id}` 取 INTERPRETING 片段或 COMPLETED 全文。
- **占卜列表分页**：`GET /api/app/v1/readings` 改为游标分页，支持 `limit`（默认 20、上限 100）、`before_id`（游标）、`type` 过滤，按 id 降序，返回 `ReadingPage`（`items/has_more/next_cursor/limit`）。
- **错误码（有界）**：错误包络 `error_code` 供客户端按码分支，不随语言变化。`message` 按 `Accept-Language` / `X-Language` 读 `server/internal/tata/i18n/*.json`，缺省简体中文。已归类 `unauthorized`、`invalid_request_body`、`invalid_path_id`、`reading_not_found`、`reading_deleted`、`reading_forbidden`、`internal`；其余业务错误暂为空字符串，后续增量归类。

App 路由现为 **86** 条（含公开 `GET /uploads/{name}`）、完整契约 **154** 条，与 `routes.go` 仍 1:1。管理端可查 `GET /api/admin/v1/wishlists`（功能心愿单）、`GET /api/admin/v1/feedback`（反馈意见）。

管理端兑换码：`GET/POST /api/admin/v1/redeem-codes`、`PUT /redeem-codes/{id}`。付费模块可写才能生成或停用。空码自动生成 `XXXX-XXXX`，App 仍走 `POST /api/app/v1/redeem` 核销。

下线塔塔币字段：人工执行 [sql/drop_tata_coin_2026-11-17.sql](./sql/drop_tata_coin_2026-11-17.sql)，不随服务启动自动跑。执行前备份。代码只读写 `quota_balance`。

内容导入：`GET /api/app/v1/preset-questions` 读预设题库，**公开，不需要 token**。`featured` 为首屏默认 15 条（与 backup 分开返回，混合排列）；`backup` 为备用题库。`?scene=` 只传场景码（relationship/career/study/growth/social/life）。同样公开的还有牌阵、塔罗师、提示窗、探索目录、活动列表/详情、帮助中心、SKU 目录。创建占卜、额度、档案、今日一牌仍要登录或匿名令牌。78 张韦特单牌写入 `cards` 表（图文件在 `server/uploads/cards/`）。SQL 见 [sql/seed_content_v1.sql](./sql/seed_content_v1.sql)。管理端 `GET/POST /api/admin/v1/cards`、`PUT /cards/{id}`、`PUT /card-settings`。App 文案 i18n 见 `server/internal/tata/i18n/*.json`，请求头 `Accept-Language` / `X-Language`；牌名优先读 `cards` 表多语言列。

首屏 15 条（`featured`，文档《塔塔_预设问题库_V1》02 节）：最近的我最需要关注什么；TA 对这段关系的真实态度；三个月事业新机会；情绪困扰背后隐藏着什么；和 TA 接下来一个月；为了实现这个愿望可以做些什么；最近值得留意的新机会；职业方向是否继续坚持；这段关系还没看清的事；三个月财务变化；如何改善人际关系；主动迈出一步会怎样；尚未被发现的优势；一个月生活新可能；最需要听见什么样的提醒。详细 id/scene 见 `docs/openapi-app.yaml` 该接口说明。

管理端价位在同一张 `iap_skus`：会员月/年卡、额度补充包（`quota_pack`，仅会员可买，种子 `pack_quota_20/60/100`）、年运解锁、牌组（`deck_year` / `deck_bundle` / `deck_single` / `deck_addon`）。种子已填设计价（分）和 App Store 占位商品号。补充包测试额度按设计图写入 50 / 160 / 300。会员开通赠送没有总数，`quota_grant` 仍为 null。App `GET /iap/skus` 对非会员隐藏补充包，也不返回牌组。牌组目录走 `GET /deck-shop`。年运解锁价取已启用且已定价的 `yearly_unlock` SKU。

2026-11-06 对照定稿 UI 补接口（对齐项已确认，见 [design/ui-gap-vs-api-2026-11-06.md](./design/ui-gap-vs-api-2026-11-06.md)）：
- **月度报告**：`GET /api/app/v1/monthly-reports`、`POST /api/app/v1/monthly-reports/save`。统计来自当月 readings；寄语截取已完成解读，不编造。
- **年度运势**：`GET /yearly-reports/{id}`、`POST .../unlock`、`PUT .../profile`（`birth_date` / `birth_time` / `birth_place` 必填）、`POST .../rhythm`（十二宫 12 张：自我到潜意识）、`POST .../domains`（四季 4 张：春夏秋冬）、`POST .../generate`。路径仍用 rhythm/domains，响应同时给 `house_cards` / `season_cards`。年卡免费解锁；单独购买价取 `yearly_unlock` SKU。生成走 interpret 供应商，不编造。
- **灵数抽牌**：`POST /readings/{id}/draw` 接受 `mode=numerology` + `numbers`（1–78 不重复，映射 Rider-Waite）。
- **账号**：`account_display` 脱敏邮箱，无则微信/Apple；新增 6 位 `public_id`。App 不绑、不展示手机号。
- **会员/额度**：订阅 SKU 带 `benefits`（`followup.count` 取 `quota_settings.followup_limit`，种子 5）。塔塔币 6 档已下线，改为额度补充包 `pack_quota_20/60/100`。追问上限管理端 `PUT /quota-params` 可改，null 不限制。免费用户每日免费全局 1 次；会员扣 `quota_balance`。邀请独立账本，最多 5 人、每人 +3。
- **历史列表**：每条带 `short_text` / `card_name` / `status_label` / `card_thumbnail`（单牌 URL → 模板 → 默认图 → 内置 `/uploads/card-default.png`；无牌名才为 null）。
- **牌面风格**：`GET/PUT /api/app/v1/deck-preference`。`available[]` 含 `description` / `tags`（未填为空串与空数组，不编造）。种子仅 `rider-waite` / 莱德韦特；管理端 `PUT /copy/decks` 可改介绍、标签与预览图。未上架 code 仍拒。
- **牌阵推荐 / 示意图**：`GET /spreads` 含 `recommended` 与 `thumbnail`。种子「时间流」为推荐；示意图未配用内置默认牌背。管理端勾选推荐时全表最多一张。
- **日历**：`GET /calendar` 增加 `type` 过滤；日详情项含牌名 / 正逆位 / 短文 / 多张缩略图 / 占卜师 / `created_at`。
- **消息中心**：接口领先于 UI，先行落底。
- **今日一牌文案 / 牌面缩略图**：正位寄语和今日宜、逆位寄语和今日宜在 `cards`。`yi` 是正位今日宜的覆盖，空则用 `upright_yi`。逆位用 `reversed_yi`。响应里 `card_message` 是对应正逆位的寄语。`card_thumbnail` 优先单牌 URL，否则模板，否则 `card.thumbnail_default`，再否则内置默认牌背 `/uploads/card-default.png`。管理端「牌面」维护这些字段。
- **运营文案**：价位文案在 `iap_skus.copy`（年卡→`campaign_perk`）；年运等无独立表的文案在 `app_config`。管理端 `GET/PUT /copy/{kind}`（yearly/cards/help/campaigns/decks）。单牌目录以 `/api/admin/v1/cards` 为准。
- **管理端上传**：`POST /api/admin/v1/uploads?folder=`。后台图片字段走 Vben Form 的 Upload。`folder` 分子目录：牌阵 `spreads`、风格 `decks`、单牌/默认图 `cards`，缺省 `misc`。存哪由 `upload.driver` 决定：`local` 或阿里云 OSS。
- **邮箱验证码登录**：`POST /api/app/v1/auth/email/code` scene=`login` 或 `bind`，再 `POST /auth/login` `{email, code}`。没有单独注册接口；未注册邮箱首次验证码登录即创建账号。绑邮箱同样只验码、不设密码。SMTP 在 `config.yaml` 的 `mail`（163 邮箱填 smtp.163.com:465，密码用客户端授权码）。未配置返回「邮件服务未配置」。
- **微信 / 苹果登录**：`POST /auth/wechat`、`/auth/apple` 以及对应 bind。微信填 `wechat.appId/appSecret`（iOS/Android 共用），客户端传 SDK `code`。换票后拉 `sns/userinfo`：档案名为空则写入微信昵称，头像为空则下载转存到 `uploads/misc`；已有资料不覆盖，userinfo 失败不挡登录。苹果登录仅 iOS，填 `apple.clientId`（Bundle ID），传 `identity_token`。凭证空、`enabled=false` 即可先上代码；填上并把 `enabled` 设为 true 后直接可用。未配置分别返回「微信登录未配置」「苹果登录未配置」。

内容 backlog #1–#10 的 **App HTTP 层已确认**。还剩运营填文案/图、以及上架前替换 Connect 商品号。见 [design/ui-content-backlog-2026-11-06.md](./design/ui-content-backlog-2026-11-06.md)。

2026-11-17：
- **账号中心**：昵称与头像。`GET/PUT /profile` 的 `nickname` 与 `name` 同一列。微信登录优先微信昵称和转存头像；邮箱/苹果生成 4–6 字随机昵称，头像用内置 `/uploads/avatar-default.png`。两种都可以更换。
- **帮助中心**：使用与占卜、账号与设置写入 Pixso `1034:621`。后台运营文案可改分类名、增删问题和排序。
- **注销**：`POST /auth/cancel` 禁止再登录（JWT 立刻失效），同一身份不能重新注册，不物理删除占卜记录。
- **管理端用户**：`GET /users` 返回昵称、头像、性别、生日、探索偏好。`GET /users/stats` 给性别、年龄（9 档 + 未填写）、探索偏好圆形图。已注销/已合并用户不计入占比。

2026-11-16 对照新 UI 补接口：
- **个人档案**：`PUT /profile` 性别入库为女性 / 男性 / 其他 / 不愿透露；另接受「女」「男」。新增 `explore_preferences`（八项多选）。
- **用户协议**：公开 `GET /legal`；管理端 `PUT /copy/legal`。未填 pending，不编造条款。
- **好友邀请**：`GET /invites` 返回 public_id；登录可带 `invite_code`，或 `POST /invites/accept`。成功给邀请人 +3 次。`GET /campaigns/rewards` 返回真实到账，fields_confirmed=true。参与记录仍占位。
- **78 张牌**：`GET /cards?deck=` 对应「查看全部 78 张」。
- **自定义塔罗牌**：`GET/POST /custom-decks`、`GET/PUT /custom-decks/{id}`、`POST .../use`。图片 `POST /uploads?folder=custom`。
- **年运分享**：`GET /yearly-reports/{id}/share` 给分享图品牌/标题/昵称/报告正文，不编造关键词。
- **探索牌阵文字总结**：`GET /spreads` 增加 theme_key / theme_title / theme_subtitle / short_sentence，运营填，不编造。

定稿 HTTP 字段已齐。还剩运营填文案/图与 Connect 商品号。登录多步 UI（Q-N1）后置；删号 UI（Q-B7）占位。邮箱密码登录未做（产品为验证码登录）。

AI 模型调研（产品文档第九条，2026-11-18）：现阶段国内只用 DeepSeek Flash 主 + 百炼备用/看图，海外 Claude / OpenAI 另包再接。详见 [design/ai-models-research-2026-11-18.md](./design/ai-models-research-2026-11-18.md)。

PRD 1.0.4 对照（2026-11-21）：权益差最大，年运 12+4 已对齐。接口、后台、UI 在同一份清单里。见 [design/prd-1.0.4-admin-ui-2026-11-21.md](./design/prd-1.0.4-admin-ui-2026-11-21.md)。

2026-11-17 额度体系调整（对齐《塔塔 App 新人体验、每日免费次数及额度体系调整》一至五）：
- 取消塔塔币与注册奖励。免费 / 会员每次提问按当前身份分流。
- 免费：全局每日 1 次（普通 ≤3 张 + 今日/明日）；探索 / 年运 / AI / 超过三张需会员。邀请独立次数，最多 5×3，不过日清。
- 会员：不享受每日免费，统一扣 `quota_balance`。补充包仅会员可买并入额度；会员过期后额度冻结不扣。
- 新人：设备引导体验与 AI 试用（仅提问，禁止追问）独立，不扣每日/额度。
- 待确认数字未编造：会员开通赠送、每次扣额度、补充包发放额度、剩余百分比分母。`quota_cost` / `quota_grant` 为 null 时停住。

## 导入 Apifox

在 Apifox 中选择 **导入 → OpenAPI**，选择本仓库的 `docs/openapi-app.yaml`（仅 App 端分组与路径）。完整契约（含管理端）见 `docs/openapi.yaml`，给后台对接使用，不要作为 App 导入源。

**分组口径（2026-11-06）：** `openapi-app.yaml` 的 tag 按 Pixso 定稿「手机界面」`619:7` 的 UI 模块划分。导入 Apifox 后文件夹即 UI 模块：登录注册、首页、探索、我的中心、抽牌、牌阵界面、历史记录、日历记录、塔塔币购买、会员中心、兑换码、参与活动、月度报告、年度运势、功能心愿单、帮助中心、反馈意见、通知设置、账号、语言、关于塔塔、我的小组件。共用接口（如 `GET /quota`、`POST /uploads`）会挂到多个模块下；`月度报告`、`年度运势` 已有专用 path，`语言`、`关于塔塔` 仍为空分组（Q-B2 + 客户端静态）。路径、方法、请求/响应形状与 `routes.go` 1:1。

导入后把环境的服务地址设为 `http://127.0.0.1:8080`（`server/manifest/config/config.yaml` 里 `server.address` 的默认值）。公开目录（预设问题、牌阵、塔罗师、提示窗、探索、活动、帮助、SKU）不用带 token。其余需要登录（或匿名令牌）的接口在 **Headers** 加 `Authorization`，值 `Bearer <accessToken>`（登录/注册/匿名返回的 token）。不要把 token 放进请求体。

管理端只出现在完整契约里。登录后同样在 Headers 加 `Authorization: Bearer <accessToken>`。本地种子：`admin` / `admin123`（全部模块可写，可给别人按模块授权），`readonly` / `readonly123`（五个模块只读）。无模块权限调用对应接口得到 `没有该模块权限`；有读无写调用写接口得到 `只读账号不能修改`。只有 `admin` 能新建/停用操作员并分配模块。
