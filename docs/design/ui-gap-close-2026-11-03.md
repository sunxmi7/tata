# 塔塔 App 定稿 ↔ HTTP：范围内无缺 path（2026-11-03 结案）

**结论：** 对照 2026-11-03 定稿画板（浅色/深色 20 模块，v01–v04 切片）与 main `docs/openapi.yaml` + `server/internal/tata/routes.go` 后，**首版范围内没有缺失的 App HTTP API**。本轮 **不新增** 任何 path、不写占位路由、不编造请求/响应字段。

并行结论（PR#13）：[`ui-api-missing-paths-2026-11-03.md`](./ui-api-missing-paths-2026-11-03.md)。本文补 living gap 回填（疑问清单 / PR#12 报告 / README）。

**核验：** App `method + /api/app/v1/…` **57** 条，OpenAPI operation 与 `routes.go` **1:1**（含 `DELETE /readings/{id}`）。另有公开读文件 `GET /uploads/{name}`（非 App Bearer API）。**无** `push/register`、**无** `profile.locale`、**无** 月报/年运/deck/widget 专用 path。

**不做：** 不把原生 P0 UI 字段验收标成 Done（抽牌/牌阵/历史/日历仍是钩子齐、逐屏字段另表）。Admin 仍走 Vben，不对 App 定稿。行为/状态机仍以**原本流程图**为准。

路径前缀均为 `/api/app/v1`，表中省略。

---

## 1. 本轮清单

| 类别 | 条数 | 内容 |
|------|:----:|------|
| **新增 path** | **0** | 无 |
| **已覆盖（首版可联调钩子）** | 18 模块 | 见 §2 |
| **锁死 / 后置（有图也不补）** | 见 §3 | 月报、年运、登录多步 UI、push、locale、删号 UI |
| **可选 / 不挡首发** | 2 | Q-B5 牌组偏好、Q-B14 小组件同步 — **本轮不补** |

---

## 2. 定稿模块 → 已有 path（already-covered）

| 定稿模块 | 已有 App path | 说明 |
|----------|---------------|------|
| 首页 | `GET /banners` `GET /tarotists` `GET /spreads` `GET /quota` `POST /daily` | 聚合壳；抽牌入口走 readings |
| 探索 | `GET /explore/topics` → `POST /readings` | 目录已有；开局字段映射仍是 Q-C7（**不新开 path**） |
| 我的中心 | （无独立 path） | 导航聚合，复用 profile / quota / 各入口 |
| 塔塔币购买 | `GET /iap/skus` `POST /iap/verify` `GET /iap/config` `GET /quota` `GET /wallet/ledger` | 价仍 null（Q-B13）；列字段 Q-B8 |
| 兑换码 | `POST /redeem` `GET /redeem/records` | 状态机屏齐；记录可空 |
| 参与活动 | `GET /campaigns` `{id}` `participations` `rewards` | 详情/记录可空（Q-B6） |
| 功能心愿单 | `POST /wishlist` `POST /uploads` | P0 已有 |
| 帮助中心 | `GET /help/categories` `articles` `articles/{id}`（`q`） | FAQ `body` 仍「待确认」（Q-B6） |
| 反馈意见 | `POST /feedback` `POST /uploads` | P0 已有 |
| 关于塔塔 | — | 版本/协议/分享/检查更新 → 客户端 + 静态 |
| 账号 | `GET/PUT /profile` | 含 name/bio/avatar/gender/birth_date；退出清本地 token（Q-B12） |
| 通知设置 | `GET/PUT /notification-preferences` | **无** push token（Q-B1） |
| 语言 | Header `Accept-Language` / `X-Language` | **无** `profile.locale`（Q-B2） |
| 日历记录 | `GET /calendar` `POST /daily` | 月历钩子齐；详情字段 Q-C4 |
| 历史记录 | `GET /readings` `{id}` `messages`；`DELETE /readings/{id}` | 列表钩子齐；筛选/续做 Q-C6 |
| 会员中心 | `GET /iap/entitlements` `GET /iap/skus` `POST /readings/{id}/paywall` | 权益文案 Q-C3 / 价 Q-B13 |
| 抽牌 | `POST /readings` + `/readings/{id}/question|detect|spread|shuffle|cut|draw|interpret|confirm-quota|use-reward|paywall|followups|messages|workflow` | 状态机钩子齐；字段/态 Q-C2、Q-N2 |
| 牌阵界面 | `GET /spreads` + `POST /readings/{id}/spread` `draw` | 阵位走 Spread schema |

---

## 3. 延后 / 锁死（deferred）— 本轮不实现

| ID | 项 | 原因 |
|----|----|------|
| Q-B3 | **月度报告** | 定稿有 8+ 浅色屏；**已拍首版不做**；无 `/monthly-reports`。不硬套 readings。 |
| Q-B4 | **年度运势** | 定稿有深浅各 3 屏；**已拍首版不做**；无独立年运 path。不硬套 readings。 |
| Q-N1 | 登录/注册/绑定多步 UI | **已拍后置**。`POST /auth/*` 钩子已在，不新开 UI 驱动流、不编合并字段。 |
| Q-B1 | `POST /push/register` | **已拍砍**。本地/OS 通知 + notification-preferences。 |
| Q-B2 | `profile.locale` | **已拍不上云**。仅 Header。 |
| Q-B7 | 删号 UI | **搁置**。`POST /auth/cancel` 钩子保留，不接线。 |
| Q-B5 | 牌组/牌面 preference | **未拍、不挡首发**。服务端固定 `deck()` 78 张；定稿无牌组设置屏。客户端默认牌背即可。 |
| Q-B14 | 小组件同步 path | **不挡首发**。首版纯客户端（日历/一牌本地数据）；要云同步再开。 |
| — | Admin 运营台 | 不跟本 App UI 逐屏。 |

忽略定稿包内空白 `矩形 5732` / 空 `文本`。

---

## 4. 仍待认、但不是缺 path

这些挡的是 **字段/文案/运营数据**，不是 HTTP 路由。不得为此发明新接口。

| ID | 题目 |
|----|------|
| Q-C2 / Q-N2 | 抽牌补抽顺序、AI 会话/付费墙全态 vs 原流程图 |
| Q-C4 | daily/calendar 详情（宜忌等） |
| Q-C6 | readings 列表筛选/未完成续做（现无分页参数） |
| Q-C7 | explore topic → `CreateReadingBody` |
| Q-C3 / Q-B13 | 会员权益文案、IAP 正式价 |
| Q-B6 / Q-B8 / Q-B9 / Q-B10 | CMS 正文、ledger 列、资料字数、提醒时区 |

---

## 5. 修订

| 日期 | 说明 |
|------|------|
| 2026-11-03 | 重读定稿 v01–v04 + OpenAPI/routes；确认范围内 0 条新 path；Q-B5/B14 不挡首发故不补。 |
