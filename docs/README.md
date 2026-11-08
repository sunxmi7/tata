# HTTP API 文档

机器可读契约（OpenAPI 3.0.3）：
- **Apifox（App 端）**：[openapi-app.yaml](./openapi-app.yaml)
- **完整契约（App + 管理端）**：[openapi.yaml](./openapi.yaml)

路径、方法、请求体和响应形状来自 `server/internal/tata/routes.go` 及对应 handler，只收录已注册的接口。

2026-10-28 起 App P0 壳层路径包括：`GET/PUT /api/app/v1/profile`（name/bio/avatar_url、account_display）、`GET /api/app/v1/explore/topics`、`POST /api/app/v1/redeem`、`GET /api/app/v1/wallet/ledger`、`POST /api/app/v1/uploads`、`POST /api/app/v1/wishlist`、`POST /api/app/v1/feedback`、`GET/PUT /api/app/v1/notification-preferences`。钻石账本的 UI 展示名为「塔塔币」（`display_label`）。

同日 App P1：`GET /api/app/v1/redeem/records`、`GET /api/app/v1/campaigns`、`GET /api/app/v1/campaigns/{id}`、`GET /api/app/v1/campaigns/participations`、`GET /api/app/v1/campaigns/rewards`、`GET /api/app/v1/help/categories`、`GET /api/app/v1/help/articles`（`category`、`q`）、`GET /api/app/v1/help/articles/{id}`。兑换记录、活动详情、参与记录、奖励记录和 FAQ 正文仍是占位：列表可为空，不编造记录；FAQ `body` 为「待确认」。

2026-11-06 重启推送（Q-B1 作废）：新增 `POST /api/app/v1/push/register`（上报个推 CID）、`GET /api/app/v1/messages`（消息中心/站内信）、`POST /api/app/v1/messages/{id}/read`，以及管理端 `POST /api/admin/v1/push/broadcast`（群发）。今日/明日抽牌仍走 App 本地通知；优惠/活动走个推（GeTui）服务端下发，只推给 `messages_enabled=true` 的设备。个推凭证在 `server/manifest/config/config.yaml` 的 `getui`（`enabled=false` 时只落站内信、不真连）。

2026-11-06 接口完善（进行中）：
- **解读/追问 SSE 流式**：`POST /api/app/v1/readings/{id}/interpret/stream` 与 `.../followups/stream`。另有 `POST /daily/stream`、`POST /readings/{id}/detect/stream`、`POST /yearly-reports/{id}/generate/stream`。非流式 JSON 仍等完整结果；同一条 URL 也可带 `Accept: text/event-stream` 或 `?stream=1` 切到 SSE。事件为 `delta` / `done` / `error`。生成先落 `ai_jobs` 后台任务，SSE 断开不取消；App 再进用 `GET /readings/{id}` 取 INTERPRETING 片段或 COMPLETED 全文。
- **占卜列表分页**：`GET /api/app/v1/readings` 改为游标分页，支持 `limit`（默认 20、上限 100）、`before_id`（游标）、`type` 过滤，按 id 降序，返回 `ReadingPage`（`items/has_more/next_cursor/limit`）。
- **错误码（有界）**：错误包络 `error_code` 供客户端按码分支，不随语言变化。`message` 按 `Accept-Language` / `X-Language` 读 `server/internal/tata/i18n/*.json`，缺省简体中文。已归类 `unauthorized`、`invalid_request_body`、`invalid_path_id`、`reading_not_found`、`reading_deleted`、`reading_forbidden`、`internal`；其余业务错误暂为空字符串，后续增量归类。

App 路由现为 **77** 条（含公开 `GET /uploads/{name}`）、完整契约 **135** 条，与 `routes.go` 仍 1:1。

内容导入：`GET /api/app/v1/preset-questions` 读预设题库；78 张韦特牌图写入 `app_config` 的 `card.{中文名}.thumbnail`（文件在 `server/uploads/cards/`）。SQL 见 [sql/seed_content_v1.sql](./sql/seed_content_v1.sql)。App 文案 i18n 见 `server/internal/tata/i18n/*.json`，请求头 `Accept-Language` / `X-Language`。

管理端价位三类：会员（`month_subscription` / `year_subscription` / `reading_pack`）、塔塔币（`consumable_pack`）、年运解锁（`yearly_unlock`）。种子已填设计价（分）和 App Store 占位商品号（`iap_skus.app_store_product_id`，如 `tata.membership.month`）；管理员改过的值保留。次卡用 `reading_grant` 发解读次数。App `GET /iap/skus?kind=` 按类型过滤。年运解锁价取已启用且已定价的 `yearly_unlock` SKU。

2026-11-06 对照定稿 UI 补接口（对齐项已确认，见 [design/ui-gap-vs-api-2026-11-06.md](./design/ui-gap-vs-api-2026-11-06.md)）：
- **月度报告**：`GET /api/app/v1/monthly-reports`、`POST /api/app/v1/monthly-reports/save`。统计来自当月 readings；寄语截取已完成解读，不编造。
- **年度运势**：`GET /yearly-reports/{id}`、`POST .../unlock`、`PUT .../profile`、`POST .../rhythm`（12 张全年节奏）、`POST .../domains`（6 张：感情/事业/财运/健康/人际/成长）、`POST .../generate`。年卡免费解锁；单独购买价取 `yearly_unlock` SKU。生成走 interpret 供应商，不编造。
- **灵数抽牌**：`POST /readings/{id}/draw` 接受 `mode=numerology` + `numbers`（1–78 不重复，映射 Rider-Waite）。
- **账号**：`account_display` 脱敏邮箱，无则微信/Apple；新增 6 位 `public_id`。App 不绑、不展示手机号。
- **会员/塔塔币**：订阅 SKU 带 `benefits`（`followup.count` 取 `quota_settings.followup_limit`，种子 5）；6 档消耗包 `pack_60`…`pack_3280` 已填设计价。追问上限管理端 `PUT /quota-params` 可改，null 不限制。
- **历史列表**：每条带 `short_text` / `card_name` / `status_label` / `card_thumbnail`（单牌 URL → 模板 → 默认图 → 内置 `/uploads/card-default.png`；无牌名才为 null）。
- **牌面风格**：`GET/PUT /api/app/v1/deck-preference`。`available[]` 含 `description` / `tags`（未填为空串与空数组，不编造）。种子仅 `rider-waite` / 莱德韦特；管理端 `PUT /copy/decks` 可改介绍、标签与预览图。未上架 code 仍拒。
- **牌阵推荐 / 示意图**：`GET /spreads` 含 `recommended` 与 `thumbnail`。种子「时间流」为推荐；示意图未配用内置默认牌背。管理端勾选推荐时全表最多一张。
- **日历**：`GET /calendar` 增加 `type` 过滤；日详情项含牌名 / 正逆位 / 短文 / 多张缩略图 / 占卜师 / `created_at`。
- **消息中心**：接口领先于 UI，先行落底。
- **今日一牌宜 / 牌面缩略图**：`yi` 取 `card.{牌名}.yi`（空则 null）。`card_thumbnail` 优先单牌 URL，否则模板，否则 `card.thumbnail_default`，再否则内置默认牌背 `/uploads/card-default.png`。
- **运营文案**：价位文案在 `iap_skus.copy`（年卡→`campaign_perk`）；其余无独立表的文案在 `app_config`。管理端 `GET/PUT /copy/{kind}`（yearly/cards/help/campaigns/decks）。
- **管理端上传**：`POST /api/admin/v1/uploads?folder=`。后台图片字段走 Vben Form 的 Upload。`folder` 分子目录：牌阵 `spreads`、风格 `decks`、单牌/默认图 `cards`，缺省 `misc`。存哪由 `upload.driver` 决定：`local` 或阿里云 OSS。
- **邮箱验证码**：`POST /api/app/v1/auth/email/code`。注册 / 绑邮箱要先发码再带 `code`。邮箱登录只验密码，不发码。SMTP 在 `config.yaml` 的 `mail`（163 邮箱填 smtp.163.com:465，密码用客户端授权码）。未配置返回「邮件服务未配置」。
- **微信 / 苹果登录**：`POST /auth/wechat`、`/auth/apple` 以及对应 bind。微信填 `wechat.appId/appSecret`（iOS/Android 共用），客户端传 SDK `code`。苹果登录仅 iOS，填 `apple.clientId`（Bundle ID），传 `identity_token`。凭证空、`enabled=false` 即可先上代码；填上并把 `enabled` 设为 true 后直接可用。未配置分别返回「微信登录未配置」「苹果登录未配置」。

内容 backlog #1–#10 的 **App HTTP 层已确认**。还剩运营填文案/图、以及上架前替换 Connect 商品号。见 [design/ui-content-backlog-2026-11-06.md](./design/ui-content-backlog-2026-11-06.md)。

定稿 HTTP 字段已齐。还剩运营填文案/图与 Connect 商品号。登录多步 UI（Q-N1）后置；删号 UI（Q-B7）占位。

## 导入 Apifox

在 Apifox 中选择 **导入 → OpenAPI**，选择本仓库的 `docs/openapi-app.yaml`（仅 App 端分组与路径）。完整契约（含管理端）见 `docs/openapi.yaml`，给后台对接使用，不要作为 App 导入源。

**分组口径（2026-11-06）：** `openapi-app.yaml` 的 tag 按 Pixso 定稿「手机界面」`619:7` 的 UI 模块划分。导入 Apifox 后文件夹即 UI 模块：登录注册、首页、探索、我的中心、抽牌、牌阵界面、历史记录、日历记录、塔塔币购买、会员中心、兑换码、参与活动、月度报告、年度运势、功能心愿单、帮助中心、反馈意见、通知设置、账号、语言、关于塔塔、我的小组件。共用接口（如 `GET /quota`、`POST /uploads`）会挂到多个模块下；`月度报告`、`年度运势` 已有专用 path，`语言`、`关于塔塔` 仍为空分组（Q-B2 + 客户端静态）。路径、方法、请求/响应形状与 `routes.go` 1:1。

导入后把环境的服务地址设为 `http://127.0.0.1:8080`（`server/manifest/config/config.yaml` 里 `server.address` 的默认值）。需要登录的接口在 **Headers** 加 `Authorization`，值 `Bearer <accessToken>`（登录/注册/匿名返回的 token）。不要把 token 放进请求体。

管理端只出现在完整契约里。登录后同样在 Headers 加 `Authorization: Bearer <accessToken>`。本地种子：`admin` / `admin123`（可写），`readonly` / `readonly123`（只读）。只读角色调用写接口会得到 `code=1`、`message=只读账号不能修改`。
