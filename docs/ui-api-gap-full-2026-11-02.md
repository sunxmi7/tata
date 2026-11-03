# 塔塔 UI↔API 全量对照（2026-11-02）

**口径（用户拍）：** 设计包必须都对照；没有的先列疑问文档，拍板后再补代码；有疑问不瞎补。  
**依据：** 设计包 `design-pack/`（**170** PNG，本 PR 盘点路径）+ main `docs/openapi.yaml` + `server/internal/tata/routes.go`（PR #6 P0 shell、PR #7 P1）。  
**权威 A/B/C：** 与 Quin 功能结论官稿一致；疑问明细见 [`ui-api-questions-2026-11-02.md`](./ui-api-questions-2026-11-02.md)。

**OpenAPI 核验（2026-11-02）：** 当时口径为 **56** 条；**2026-11-03 复核为 57** 条 App `method+path`（含 `DELETE /readings/{id}`），与 `openapi.yaml` 1:1。占位策略（空列表、help「待确认」、**Q-B1 不注册 push/register**）不变。定稿 HTTP 结案见 [`design/ui-gap-close-2026-11-03.md`](./design/ui-gap-close-2026-11-03.md)（**无新增 path**）。

---

## 0. Executive 总结

| 指标 | 数量 |
|------|-----:|
| 设计屏 PNG 合计 | **170** |
| 其中定稿态画面 | 141 |
| 占位 / 占位屏 | 10 |
| 组件碎片 / 资源（矩形、图标等） | 19 |
| **A · 已定稿且 main 已有 API** | **壳层已齐**（见 §2） |
| **B · 有图待补 / 待拍** | 14 条疑问（Q-B1～Q-B14）；**Q-B1 已拍砍**（无 push/register）；**Q-B2 已拍**（Header 传语言）；**Q-B7 搁置**（删号稍后确认） |
| **C · 包外主流程** | 7 条疑问（Q-C1～Q-C7） |
| **本轮代码变更** | Accept-Language 请求解析 + CORS 允许头；**无** push/register、**无**删号 UI |

---

## 1. 设计屏清单（filename → 模块 → 定稿/占位）

模块列为启发式归类（深浅色变体、状态机变体均单独计行）。组件类 PNG 不参与 API 矩阵，但列入清单以满足「每张都 walk」。

<!-- inventory-start -->
| # | 文件名 | 模块 | 定稿/占位 |
|---:|---|---|---|
| 1 | `calendar-check-line.png` | misc | 组件/资源 |
| 2 | `会员福利详情 占位.png` | campaigns | 占位 |
| 3 | `余额明细 占位.png` | wallet/iap | 占位 |
| 4 | `充值帮助 占位.png` | wallet/iap | 占位 |
| 5 | `兑换码 浅色.png` | redeem | 定稿 |
| 6 | `兑换码 浅色·兑换中.png` | redeem | 定稿 |
| 7 | `兑换码 浅色·失败.png` | redeem | 定稿 |
| 8 | `兑换码 浅色·已输入.png` | redeem | 定稿 |
| 9 | `兑换码 浅色·成功.png` | redeem | 定稿 |
| 10 | `兑换码 浅色·聚焦.png` | redeem | 定稿 |
| 11 | `兑换码 深色.png` | redeem | 定稿 |
| 12 | `兑换码.png` | redeem | 定稿 |
| 13 | `兑换记录 占位.png` | redeem | 占位 |
| 14 | `关于塔塔 浅色.png` | theme | 定稿 |
| 15 | `关于塔塔 浅色·分享占位.png` | about | 占位 |
| 16 | `关于塔塔 浅色·分享失败.png` | about | 定稿 |
| 17 | `关于塔塔 浅色·按下版本信息.png` | about | 定稿 |
| 18 | `关于塔塔 浅色·版本信息.png` | about | 定稿 |
| 19 | `关于塔塔 深色.png` | theme | 定稿 |
| 20 | `关于塔塔 深色·版本信息.png` | theme | 定稿 |
| 21 | `关于塔塔.png` | about | 定稿 |
| 22 | `功能心愿单 浅色.png` | theme | 定稿 |
| 23 | `功能心愿单 浅色·上传中.png` | wishlist | 定稿 |
| 24 | `功能心愿单 浅色·上传失败.png` | wishlist | 定稿 |
| 25 | `功能心愿单 浅色·已选图片.png` | wishlist | 定稿 |
| 26 | `功能心愿单 浅色·提交中.png` | wishlist | 定稿 |
| 27 | `功能心愿单 浅色·提交失败.png` | wishlist | 定稿 |
| 28 | `功能心愿单 浅色·提交成功.png` | wishlist | 定稿 |
| 29 | `功能心愿单 浅色·离开确认.png` | wishlist | 定稿 |
| 30 | `功能心愿单 浅色·输入中.png` | wishlist | 定稿 |
| 31 | `功能心愿单 浅色·达到上限.png` | wishlist | 定稿 |
| 32 | `功能心愿单 深色.png` | theme | 定稿 |
| 33 | `功能心愿单.png` | wishlist | 定稿 |
| 34 | `参与活动 浅色.png` | theme | 定稿 |
| 35 | `参与活动 浅色·加载中.png` | campaigns | 定稿 |
| 36 | `参与活动 浅色·加载失败.png` | campaigns | 定稿 |
| 37 | `参与活动 浅色·暂无活动.png` | campaigns | 定稿 |
| 38 | `参与活动 浅色·活动已结束.png` | campaigns | 定稿 |
| 39 | `参与活动 深色.png` | theme | 定稿 |
| 40 | `参与活动.png` | campaigns | 定稿 |
| 41 | `参与记录 占位.png` | campaigns | 占位 |
| 42 | `反馈意见 浅色.png` | theme | 定稿 |
| 43 | `反馈意见 浅色·上传中.png` | feedback | 定稿 |
| 44 | `反馈意见 浅色·上传失败.png` | feedback | 定稿 |
| 45 | `反馈意见 浅色·已选图片.png` | feedback | 定稿 |
| 46 | `反馈意见 浅色·提交中.png` | feedback | 定稿 |
| 47 | `反馈意见 浅色·提交失败.png` | feedback | 定稿 |
| 48 | `反馈意见 浅色·提交成功.png` | feedback | 定稿 |
| 49 | `反馈意见 浅色·离开确认.png` | feedback | 定稿 |
| 50 | `反馈意见 浅色·结果未确认.png` | feedback | 定稿 |
| 51 | `反馈意见 浅色·输入中.png` | feedback | 定稿 |
| 52 | `反馈意见 浅色·邮箱错误.png` | feedback | 定稿 |
| 53 | `反馈意见 深色.png` | theme | 定稿 |
| 54 | `反馈意见.png` | feedback | 定稿 |
| 55 | `塔塔·年度运势·浅色·四步流程.png` | annual-fortune | 定稿 |
| 56 | `塔塔·年度运势·浅色·密函.png` | annual-fortune | 定稿 |
| 57 | `塔塔·年度运势·浅色·报告内容.png` | annual-fortune | 定稿 |
| 58 | `塔塔·我的·浅色·滚动.png` | mine | 定稿 |
| 59 | `塔塔·我的·浅色·长页.png` | mine | 定稿 |
| 60 | `塔塔·我的·浅色·首屏.png` | mine | 定稿 |
| 61 | `塔塔·我的·深色·滚动.png` | theme | 定稿 |
| 62 | `塔塔·我的·深色·长页.png` | theme | 定稿 |
| 63 | `塔塔·我的·深色·首屏.png` | theme | 定稿 |
| 64 | `塔塔·探索·浅色2·第二屏.png` | explore | 定稿 |
| 65 | `塔塔·探索·浅色·第二屏.png` | explore | 定稿 |
| 66 | `塔塔·探索·浅色·首屏-1.png` | explore | 定稿 |
| 67 | `塔塔·探索·浅色·首屏.png` | explore | 定稿 |
| 68 | `塔塔·探索·深色·第二屏.png` | explore | 定稿 |
| 69 | `塔塔·探索·深色·首屏.png` | explore | 定稿 |
| 70 | `塔塔·首页·暗色.png` | home | 定稿 |
| 71 | `塔塔·首页·浅色-1.png` | home | 定稿 |
| 72 | `塔塔·首页·浅色.png` | home | 定稿 |
| 73 | `塔塔币 浅色.png` | wallet/iap | 定稿 |
| 74 | `塔塔币 浅色·处理中.png` | wallet/iap | 定稿 |
| 75 | `塔塔币 深色.png` | wallet/iap | 定稿 |
| 76 | `塔塔币购买.png` | wallet/iap | 定稿 |
| 77 | `备选.png` | misc | 定稿 |
| 78 | `奖励记录 占位.png` | campaigns | 占位 |
| 79 | `好友邀请详情 占位.png` | campaigns | 占位 |
| 80 | `定稿.png` | misc | 定稿 |
| 81 | `帮助中心 浅色.png` | theme | 定稿 |
| 82 | `帮助中心 浅色·会员与支付.png` | help | 定稿 |
| 83 | `帮助中心 浅色·加载中.png` | help | 定稿 |
| 84 | `帮助中心 浅色·加载失败.png` | help | 定稿 |
| 85 | `帮助中心 浅色·搜索有结果.png` | help | 定稿 |
| 86 | `帮助中心 浅色·无结果.png` | help | 定稿 |
| 87 | `帮助中心 浅色·账号与设置.png` | account | 定稿 |
| 88 | `帮助中心 深色.png` | theme | 定稿 |
| 89 | `帮助中心.png` | help | 定稿 |
| 90 | `年度运势.png` | annual-fortune | 定稿 |
| 91 | `底板.png` | misc | 定稿 |
| 92 | `我的中心.png` | misc | 定稿 |
| 93 | `我的中心主页面.png` | misc | 定稿 |
| 94 | `我的选中.png` | misc | 定稿 |
| 95 | `探索.png` | explore | 定稿 |
| 96 | `文本.png` | misc | 定稿 |
| 97 | `月份选择 浅色.png` | theme | 定稿 |
| 98 | `月度报告 浅色.png` | monthly-report | 定稿 |
| 99 | `月度报告 浅色·保存中.png` | monthly-report | 定稿 |
| 100 | `月度报告 浅色·加载中.png` | monthly-report | 定稿 |
| 101 | `月度报告 浅色·加载失败.png` | monthly-report | 定稿 |
| 102 | `月度报告 浅色·尚未生成.png` | monthly-report | 定稿 |
| 103 | `月度报告 浅色·已保存.png` | monthly-report | 定稿 |
| 104 | `月度报告 浅色·当月无数据.png` | monthly-report | 定稿 |
| 105 | `月度报告 深色.png` | monthly-report | 定稿 |
| 106 | `月度报告.png` | monthly-report | 定稿 |
| 107 | `标签栏-1.png` | misc | 定稿 |
| 108 | `标签栏-探索选中.png` | explore | 定稿 |
| 109 | `标签栏.png` | misc | 定稿 |
| 110 | `椭圆 6580.png` | misc | 定稿 |
| 111 | `矢量 14-1.png` | misc | 定稿 |
| 112 | `矢量 14.png` | misc | 定稿 |
| 113 | `矩形 385.png` | component | 占位/组件 |
| 114 | `矩形 5716.png` | component | 占位/组件 |
| 115 | `矩形 5717.png` | component | 占位/组件 |
| 116 | `矩形 5732.png` | component | 占位/组件 |
| 117 | `矩形 5733.png` | component | 占位/组件 |
| 118 | `矩形 5748.png` | component | 占位/组件 |
| 119 | `矩形 5749.png` | component | 占位/组件 |
| 120 | `矩形 5750.png` | component | 占位/组件 |
| 121 | `矩形 5751.png` | component | 占位/组件 |
| 122 | `矩形 5752.png` | component | 占位/组件 |
| 123 | `矩形 5753.png` | component | 占位/组件 |
| 124 | `矩形 5754.png` | component | 占位/组件 |
| 125 | `矩形 5755.png` | component | 占位/组件 |
| 126 | `矩形 5756.png` | component | 占位/组件 |
| 127 | `矩形 5757.png` | component | 占位/组件 |
| 128 | `矩形 5758.png` | component | 占位/组件 |
| 129 | `矩形 5759.png` | component | 占位/组件 |
| 130 | `矩形 5760.png` | component | 占位/组件 |
| 131 | `语言 浅色.png` | language | 定稿 |
| 132 | `语言 浅色·切换中.png` | language | 定稿 |
| 133 | `语言 浅色·切换失败.png` | language | 定稿 |
| 134 | `语言 浅色·已切换韩语.png` | language | 定稿 |
| 135 | `语言 浅色·待确认韩语.png` | language | 定稿 |
| 136 | `语言 浅色·未修改.png` | language | 定稿 |
| 137 | `语言 浅色·跟随系统.png` | language | 定稿 |
| 138 | `语言 浅色·返回确认.png` | language | 定稿 |
| 139 | `语言 深色.png` | language | 定稿 |
| 140 | `语言.png` | language | 定稿 |
| 141 | `账号 浅色.png` | account | 定稿 |
| 142 | `账号 浅色·ID已复制.png` | account | 定稿 |
| 143 | `账号 浅色·保存失败.png` | account | 定稿 |
| 144 | `账号 浅色·删除账号占位.png` | account | 占位 |
| 145 | `账号 浅色·复制失败.png` | account | 定稿 |
| 146 | `账号 浅色·姓名编辑.png` | account | 定稿 |
| 147 | `账号 浅色·字号放大.png` | account | 定稿 |
| 148 | `账号 浅色·性别选择.png` | account | 定稿 |
| 149 | `账号 浅色·放弃修改.png` | account | 定稿 |
| 150 | `账号 浅色·生日选择.png` | account | 定稿 |
| 151 | `账号 浅色·简介编辑.png` | account | 定稿 |
| 152 | `账号 浅色·账号过长.png` | account | 定稿 |
| 153 | `账号 浅色·资料为空.png` | account | 定稿 |
| 154 | `账号 浅色·退出失败.png` | account | 定稿 |
| 155 | `账号 浅色·退出确认.png` | account | 定稿 |
| 156 | `账号 深色.png` | account | 定稿 |
| 157 | `账号.png` | account | 定稿 |
| 158 | `通知设置 浅色.png` | notification | 定稿 |
| 159 | `通知设置 浅色·今日关闭.png` | notification | 定稿 |
| 160 | `通知设置 浅色·保存中.png` | notification | 定稿 |
| 161 | `通知设置 浅色·保存失败.png` | notification | 定稿 |
| 162 | `通知设置 浅色·系统未授权.png` | notification | 定稿 |
| 163 | `通知设置 浅色·返回后仍未授权.png` | notification | 定稿 |
| 164 | `通知设置 浅色·选择时间.png` | notification | 定稿 |
| 165 | `通知设置 深色.png` | notification | 定稿 |
| 166 | `通知设置.png` | notification | 定稿 |
| 167 | `遮罩-1.png` | misc | 定稿 |
| 168 | `遮罩.png` | misc | 定稿 |
| 169 | `问题详情 占位.png` | misc | 占位 |
| 170 | `首页双主题.png` | home | 定稿 |

**合计：170 张 PNG**

<!-- inventory-end -->

---

## 2. A · 已覆盖（定稿屏 ∩ main 已有 · 核验通过）

下列能力在 **2026-11-02 main** 上可联调；部分 P1 列表/正文字段仍为占位，**不编造数据**（与 openapi 顶栏说明一致）。

| UI 模块 / 代表屏 | App API | 对照 | 备注 |
|------------------|---------|:----:|------|
| 探索目录（塔塔·探索·首屏/第二屏） | `GET /explore/topics` | 有 | P0；spread 可 stub |
| 兑换码（兑换码 浅色*） | `POST /redeem` | 有 | P0 |
| 兑换记录入口（兑换记录 **占位**） | `GET /redeem/records` | 部分 | P1；可空列表，字段待拍 Q-B8 |
| 塔塔币 + 明细入口（塔塔币 浅色*；余额明细 **占位**） | `quota` `iap/*` `GET /wallet/ledger` | 部分 | 展示 `display_label=塔塔币`；价 null；明细字段 Q-B8 |
| 上传（心愿/反馈上传中） | `POST /uploads` | 有 | P0 |
| 功能心愿单 | `POST /wishlist` | 有 | P0 |
| 反馈意见 | `POST /feedback` | 有 | P0 |
| 通知设置 | `GET/PUT /notification-preferences` | 部分 | **Q-B1 已拍砍** — 无 push token；本地/OS 提醒 |
| 参与活动 / 列表 | `GET /campaigns` `{id}` `participations` `rewards` | 部分 | P1；详情/记录占位 Q-B6 |
| 帮助中心 | `GET /help/categories` `articles` `{id}` | 部分 | body「待确认」Q-B6 |
| 账号资料（账号 浅色* 编辑态） | `GET/PUT /profile` | 部分 | name/bio/avatar；展示邮箱非手机 Q-B11；字数 Q-B9 |
| 删号占位 | `POST /auth/cancel` | 部分 | 钩子保留；**Q-B7 搁置〔2026-11-02〕** — 本轮不接 UI，规则稍后确认 |
| 首页壳 / 我的长页（聚合入口） | `banners` `tarotists` `spreads` `quota` 等 | 部分 | 开始抽牌/AI/一牌在 **C** |
| 语言屏 | — | 无 profile.locale | **Q-B2 已拍** — 本地存偏好；请求 Header（`Accept-Language` 等）传语言 |
| 月度报告屏 | — | 无 | V1.3 Q-B3 |
| 年度运势屏 | — | 无 | V1.3 Q-B4 |
| 我的塔罗牌入口 | — | 无 | Q-B5 |
| 关于塔塔 | — | 不必 | 客户端 + 静态链接 |

**登录链 / readings 全链 / paywall / 每日详情：** 设计包 **无逐步逻辑屏** → 归入 **§4 C**，不在此表假装 UI 字段已对照。

---

## 3. B · 本包有图 → 待补 / 待拍（不在本轮编码）

| ID | UI | API 现状 | 矩阵 | 疑问 |
|----|-----|----------|:----:|------|
| Q-B1 | 通知设置 | 无 push/register | 无 | **已拍砍** — 不要服务端推 |
| Q-B2 | 语言 10 屏 | 无 locale | 无* | **已拍** — Header 传语言；不上云 |
| Q-B3 | 月度报告 9+ 屏 | 无月报 path | 无 | **V1.3 延后** |
| Q-B4 | 年度运势 4 屏 | 无年运 path | 无 | **V1.3 延后** |
| Q-B5 | 首页我的塔罗牌 | 无 deck API | 无 | 无设置屏 |
| Q-B6 | 多种 **占位** 正文页 | help/campaign 占位 | 部分 | CMS vs 静态 |
| Q-B7 | 删号 **占位** | cancel 有 | 部分 | **搁置** — 规则 TBD；本轮不接 UI |
| Q-B8 | 明细/兑换记录 **占位** | ledger/records 有 | 部分 | 列字段未拍 |
| Q-B9 | 姓名/简介编辑 | profile 有 | 部分 | 字数未拍 |
| Q-B10 | 通知时间 | preferences 有 | 部分 | 时区策略 |
| Q-B11 | 账号手机排版 | bind-phone 有 | — | AUTH 无手机 |
| Q-B12 | 退出确认 | 无 app logout | — | 建议客户端 |
| Q-B13 | 塔塔币/年运示例价 | SKU null | 部分 | 不挡形状 |
| Q-B14 | 小组件入口 | 无 API | — | 客户端 V1 |

详情见 [`ui-api-questions-2026-11-02.md`](./ui-api-questions-2026-11-02.md)。

---

## 4. C · 包外主流程（钩子对照 · 不编字段）

| 模块 | 已有 App API（摘要） | 对照结论 | 疑问 |
|------|----------------------|----------|------|
| 登录/注册/绑定/合并 | `auth/anonymous` `register` `login` `wechat` `apple` `bind-*` `merge-confirm` | **钩子有**；UI 未到 | Q-C1 |
| 普通占卜全链 | `readings*` + `quota` + `spreads` | **钩子有**；屏未到 | Q-C2 |
| 付费墙 / IAP | `readings/{id}/paywall` + `iap/*` | **钩子有**；独立收银屏未到 | Q-C3 |
| 今日一牌 / 明日 | `POST /daily` `GET /calendar` | **钩子有**；详情屏未到 | Q-C4 |
| AI 塔罗师会话 | `tarotists` + readings | 列表有；会话屏未到 | Q-C5 |
| 历史 / 续做 | `GET /readings` `{id}` `messages` | **钩子有** | Q-C6 |
| 探索下游开局 | `explore/topics` → `POST /readings`（推断） | topics 有 | Q-C7 |

**C 原则：** 补图或流程图定稿前，只核对实现与群内「普通链 / 额度 / 钻石」已拍行为（失败不退次、额度顺序、AI 试用整段等，见仓库外决策笔记 2026-10-25）；**不按想象加 path**。

---

## 5. 能力矩阵（UI 能力 → API → 有/无/部分）

| UI 能力 | 现有 API path | 有/无/部分 | notes |
|---------|---------------|:----------:|-------|
| 匿名/登录/绑定/合并 | `/auth/*` | 有 | UI 在 C；邮箱 AUTH-002 |
| 普通占卜 + 追问 + 荐阵 | `/readings/*` | 有 | UI 在 C |
| 付费墙续跑 | `/readings/{id}/paywall` | 有 | UI 在 C |
| AI 塔罗师列表 | `/tarotists` | 有 | 会话 C |
| 牌阵 | `/spreads` | 有 | |
| 额度 / 塔塔币展示 | `/quota` `/iap/entitlements` | 部分 | display_label |
| SKU / 验单 / 恢复 | `/iap/skus` `verify` `restore` `config` | 部分 | 价 null |
| 塔塔币流水 | `/wallet/ledger` | 部分 | Q-B8 |
| 今日一牌 / 日历 | `/daily` `/calendar` | 部分 | 详情 C Q-C4 |
| 探索主题 | `/explore/topics` | 有 | 下游 C Q-C7 |
| 兑换码 | `/redeem` | 有 | |
| 兑换记录 | `/redeem/records` | 部分 | 占位屏 |
| 上传 | `/uploads` | 有 | |
| 心愿单 | `/wishlist` | 有 | |
| 反馈 | `/feedback` | 有 | |
| 通知偏好 | `/notification-preferences` | 部分 | Q-B1 已砍 |
| 语言 | — | 无 profile.locale | **Q-B2 已拍** — Header |
| 活动 | `/campaigns*` | 部分 | Q-B6 |
| 帮助 | `/help/*` | 部分 | Q-B6 |
| 账号资料 | `/profile` | 部分 | Q-B9/B11 |
| 删号 | `/auth/cancel` | 部分 | **Q-B7 搁置** |
| 退出 | — | 无* | Q-B12 客户端 |
| 月报 | — | 无 | Q-B3 V1.3 |
| 年运 | — | 无 | Q-B4 V1.3 |
| 我的塔罗牌 | — | 无 | Q-B5 |
| 推送 token | — | 无 | **Q-B1 已拍砍** |
| 主题浅/深 | — | 不必 | 本地 |
| 关于/分享/检查更新 | — | 不必 | 客户端 |

---

## 6. 与 2026-10-28 差距文档的关系

2026-10-28 差距 / CRUD 稿 **不可当作当前真相**。P0 shell（#6）与 P1 campaigns/help/redeem（#7）已合 main；本文以 **2026-11-02 openapi + routes** 为准重新盘点。

---

## 7. 建议落地顺序（文档同步）

1. **已拍 / 搁置 / 已砍：** Q-B1（**不**做服务端推）；Q-B2（Header 传语言）；**Q-B7 搁置**（删号稍后确认，本轮不接）  
2. **仍待拍：** Q-B5、Q-B6、Q-B8、Q-B9 等  
3. **本 PR：** Q-B2 — OpenAPI `Accept-Language` + 服务端读 Header（缺省 `zh-Hans`）；**不**补 push/register  
4. **本轮不做：** 删号 UI 对接 `auth/cancel`；月报/年运（V1.3）

---

## 8. 修订

| 日期 | 说明 |
|------|------|
| 2026-11-02 | 170 屏清单 + A/B/C + 矩阵；routes/openapi 核验；首轮仅文档。 |
| 2026-11-02 | Q-B2 已拍、Q-B7 搁置、Q-B1 已拍砍；Accept-Language 文档 + 服务端解析。 |
| 2026-11-03 | 定稿结案：Q-B3/B4 首版不做、Q-N1 后置；57 条 1:1、0 新 path。见 close 文档。 |

