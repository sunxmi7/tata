# 塔塔 App 定稿 UI ↔ HTTP（2026-11-06）

**设计：** Pixso「手机界面」`619:7`（浅色主屏；深色 / 平板 / 折叠为同一流程的主题与设备变体）。  
**代码：** `server/internal/tata/routes.go`，App **73** 条 path（含 `GET /uploads/{name}`），与 `docs/openapi-app.yaml` 1:1。

**结论：** 定稿各模块 HTTP path 已对齐，下表标 **已确认**。还没对齐的只剩 §2。

运营填文案/图见 [ui-content-backlog-2026-11-06.md](./ui-content-backlog-2026-11-06.md)。

---

## 1. 已确认

| 模块 | 定稿节点 | 接口 | 状态 |
|------|----------|------|------|
| 首页 | `565:33` / 未抽 `685:8` | `GET /banners` `GET /tarotists` `GET /spreads` `GET /quota` `POST /daily` `GET/PUT /deck-preference` | 已确认 |
| 探索 | `565:997` | `GET /explore/topics` → `POST /readings`（客户端带 question / source=explore / 再选阵） | 已确认 |
| 我的中心 | `565:1551` | 无独立 path，复用 profile / quota / 各入口 | 已确认 |
| 抽牌 / 灵数 | `595:868`…`595:637` | `POST /readings` 及子资源；`draw` 接受 `mode=numerology` | 已确认 |
| 牌阵 | `595:5054` | `GET /spreads` + `POST /readings/{id}/spread` | 已确认 |
| 历史记录 | `574:1627` | `GET /readings`（预览 / 缩略图 / status_label）`GET/DELETE /readings/{id}` | 已确认 |
| 日历筛选 | `574:306` | `GET /calendar?type=` | 已确认 |
| 今日一牌 | 首页 / `POST /daily` | `card_name` / `yi` / `card_thumbnail` / `short_text`；没配图出默认牌背 | 已确认 |
| 塔塔币 / 会员 | `565:1904` / `593:1905` | `GET /iap/skus` 档位与权益结构已齐；价为种子/管理端值 | 已确认 |
| 兑换码 | `565:2024` | `POST /redeem` `GET /redeem/records` | 已确认（记录行未定稿，可空） |
| 参与活动 | `565:6104` | `GET /campaigns` `{id}` `participations` `rewards` | 已确认（详情占位） |
| 月度报告 | `565:7013` | `GET /monthly-reports` `POST .../save` | 已确认 |
| 年运解锁卖点 | `574:2376` / `574:2423` / `574:2469` | `GET /yearly-reports/{year}` `POST .../unlock`；年卡免费、单独解锁价取 SKU | 已确认 |
| 年运四步（12+6） | `574:2469` | `PUT .../profile` → `POST .../rhythm`（12）→ `POST .../domains`（6：感情/事业/财运/健康/人际/成长）→ `POST .../generate` | 已确认 |
| 账号 | `572:1877` | `GET/PUT /profile`：`account_display` 脱敏手机优先，`public_id` 6 位 | 已确认 |
| 通知设置 | `572:2735` | `GET/PUT /notification-preferences`；`POST /push/register` | 已确认 |
| 心愿单 / 帮助 / 反馈 | `572:40` / `572:544` / `572:805` | 对应 path 已齐；帮助正文空则 pending | 已确认 |
| 语言 | `565:7744` | 不上云，请求头 `Accept-Language` | 已确认（无 path） |
| 关于塔塔 | `572:1536` | 客户端 + 静态 | 已确认（无 path） |
| 小组件 | `610:82` | 复用 `POST /daily`，不新开 sync | 已确认（无 path） |
| 消息中心 | 定稿无入口 | `GET /messages` `POST /messages/{id}/read` 已有 | 已确认（入口客户端可选） |
| 登录多步 / 删号 | — | 钩子在，UI 后置 / 占位 | 已确认（不补 path） |

---

## 2. 未对齐

### 日历日详情字段

`574:2` 日详情要：今日一牌图 / 正逆位 / 短文、普通占卜多张缩略图、占卜师名、时分。  
`GET /calendar` 只有 `id/type/question/status/business_date/progress`。历史列表已有预览字段，日历没有。不新开 path，差的是日历项字段。

### 牌组介绍字段

`693:39`「我的塔罗牌」有套装描述和标签。  
`GET /deck-preference` 只有 code / 名称 / 预览图。78 张牌名客户端可本地拼缩略图，不必新开 path。

---

## 3. 修订

| 日期 | 说明 |
|------|------|
| 2026-11-06 | 对照 `619:7` 全模块；月报/年运/灵数等由「首版不做」改为补接口。 |
| 2026-11-06 | 已对齐项标已确认。未对齐收成：年运生成流程、日历日详情字段、牌组介绍字段。11-03 过期 gap 文档已删。 |
| 2026-11-06 | 年运按定稿 12+6 四步落地：profile / rhythm / domains / generate。 |
