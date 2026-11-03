# 定稿 App UI 复阅：缺失 App API 路径结论（2026-11-03）

**任务：** 用户「重新读一下今天我导出的 UI 图，然后如果有缺少的接口就补上」。  
**依据：** 定稿画板切片（`assets/dinggao_ui_*.jpg`）、[`ui-gap-vs-api-2026-11-03.md`](./ui-gap-vs-api-2026-11-03.md)（PR#12）、`docs/openapi.yaml`、`server/internal/tata/routes.go`（57 条 `/api/app/v1/*` 注册，与 OpenAPI 一致）。  
**结论：** **范围内 V1 无新增 App path**；缺口在 **响应字段 / 验收表 / 占位数据**（见 §4），不在路由层。

---

## 1. 复阅方法

1. 按画板 **20 个业务模块**（不含「主题-浅色/深色」标题行）逐块对照附件 UI 切片。  
2. 每个模块列出 UI 需要的 **网络读写**（列表、提交、状态机步进），映射到现有 path。  
3. 排除 **产品已拍板不做或搁置** 的项（§3）。  
4. 机器核对：`routes.go` 与 `openapi.yaml` 的 App 路由集合对齐；**未发现「server 有而 OpenAPI 无」或「UI 必需而两者皆无」的 path**。

---

## 2. 模块 → 现有 path → 仍缺 path？

| # | 模块 | 定稿 UI 需要的 App 网络能力 | 现有 path（`/api/app/v1`） | 仍缺 path？ |
|---|------|------------------------------|-----------------------------|:-----------:|
| 1 | 首页 | Banner、塔罗师、牌阵目录、额度、每日一牌 | `GET /banners` `GET /tarotists` `GET /spreads` `GET /quota` `POST /daily`；开局走 readings | **否** |
| 2 | 探索 | 主题/分类目录 | `GET /explore/topics`；点进后 `POST /readings`（映射 Q-C7，非新 path） | **否** |
| 3 | 我的中心 | 聚合入口 | 上列 + profile/quota/设置类 path；无单独「我的中心」path | **否** |
| 4 | 塔塔币购买 | SKU、验单、余额、流水 | `GET /iap/skus` `POST /iap/verify` `GET /quota` `GET /wallet/ledger` | **否** |
| 5 | 兑换码 | 核销 + 记录 | `POST /redeem` `GET /redeem/records` | **否** |
| 6 | 参与活动 | 活动列表/详情/参与与奖励 | `GET /campaigns` `GET /campaigns/{id}` `GET /campaigns/participations` `GET /campaigns/rewards` | **否** |
| 7 | 月度报告 | 月报图表与锁态 | — | **不适用**（Q-B3 V1 skip，不补 path） |
| 8 | 语言 | 语言切换 | —（Q-B2：`Accept-Language` / `X-Language`，无 `profile.locale`） | **否**（按约束无需 path） |
| 9 | 功能心愿单 | 提交心愿（含图） | `POST /wishlist` `POST /uploads` | **否** |
| 10 | 帮助中心 | 分类、文章、搜索 | `GET /help/categories` `GET /help/articles` `GET /help/articles/{id}` | **否** |
| 11 | 反馈意见 | 提交 + 附图 | `POST /feedback` `POST /uploads` | **否** |
| 12 | 关于塔塔 | 版本、协议、分享 | 客户端 + 静态（`GET /iap/config` 等可选） | **否** |
| 13 | 账号 | 资料读写；删号占位 | `GET/PUT /profile`；`POST /auth/cancel`（Q-B7 不接 UI） | **否** |
| 14 | 通知设置 | 偏好开关 | `GET/PUT /notification-preferences` | **否** |
| 15 | 年度运势 | 年运长文 | — | **不适用**（Q-B4 V1 skip） |
| 16 | 日历记录 | 月历打点、日详情 | `GET /calendar` `POST /daily` | **否** |
| 17 | 历史记录 | 列表、详情、删除、续做 | `GET /readings` `GET /readings/{id}` `DELETE /readings/{id}` + readings 子资源 | **否** |
| 18 | 会员中心 | 权益、SKU、付费墙 | `GET /iap/entitlements` `GET /iap/skus` `POST /readings/{id}/paywall` | **否** |
| 19 | 我的小组件 | 桌面组件预览 | — | **否**（Q-B14：V1 可纯客户端，不补 sync path） |
| 20 | 抽牌 | 提问→阵→洗→切→抽→解读→追问 | `POST /readings` 及 `readings/{id}/*` 全链 | **否** |
| 21 | 牌阵界面 | 阵位与选阵 | `GET /spreads` `POST /readings/{id}/spread` `POST …/draw` | **否** |

（抽牌 / 牌阵在画板上为独立红标题，与 PR#12 的 20 模块计数一致：抽牌+牌阵计入占卜链路，不单独计第 21 条产品模块；上表拆行仅为对照清晰。）

**汇总：** 在 scope 内 **0** 个待补 path；**2** 个模块因产品延后 **故意无 path**（月报、年运）；**2** 个模块按约束 **不应** 有 path（语言 Header、小组件 Q-B14）。

---

## 3. 明确不实现（与 PR#12 / 用户约束一致）

| ID | 内容 |
|----|------|
| Q-B1 | 无 `POST /push/register` |
| Q-B2 | 无 `profile.locale` |
| Q-B3 / Q-B4 | 无月度报告 / 年度运势 path |
| Q-B5 | 无 deck / 牌组 preference path（首页牌面视觉可客户端） |
| Q-B7 | 删号 UI 搁置；保留 `POST /auth/cancel` |
| Q-N1 | 登录/绑定多步流不扩 auth path |
| Q-B14 | 小组件不同步后端 |

---

## 4. 仍待收口（非「缺 path」）

下列为 **字段、正文、定价、验收表** 问题；OpenAPI **已有** 对应 route，本轮 **不编造** CMS 正文或 IAP 单价：

| 区域 | 跟踪 ID | 说明 |
|------|---------|------|
| 抽牌 / 付费墙 / AI 试用态 | Q-C2、Q-N2、Q-C5 | readings + quota 字段对照定稿 |
| 牌阵 UI ↔ `Spread.positions` | P0-2 | schema 已有 |
| 历史列表列 | Q-C6 | `GET /readings` 字段验收 |
| 日历日详情 | Q-C4 | `CalendarView` / daily 字段 |
| 塔塔币流水列 | Q-B8 | `WalletLedgerItem` 已有 time/type/amount/balance/note/bucket |
| 帮助 / 活动正文 | Q-B6 | path 有，内容占位 |
| 会员 SKU 价 | Q-B13 | `iap/skus` 价可为 null |
| 探索 → 开局 body | Q-C7 | `POST /readings` 映射 |

P0 路径层验收表见 uploads / 后续 `tata-p0-ui-field-acceptance-2026-11-03.md` 口径。

---

## 5. 实施决定

- **未新增** `server/` handler、**未改** `openapi.yaml` 路由表。  
- 与 PR#12 一致：**定稿 UI 所需 App 钩子已在 main**；下一步是字段验收与占位数据拍板，而非开新 path。

---

## 6. 修订

| 日期 | 说明 |
|------|------|
| 2026-11-03 | Cloud Agent 复阅定稿切片 + routes/OpenAPI 对齐；零缺失 in-scope path。 |
