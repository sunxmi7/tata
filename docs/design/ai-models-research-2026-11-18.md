# 塔塔 AI 模型调研（2026-11-18）

对照产品文档《塔塔 App 新人体验、每日免费次数及额度体系调整》第九条：竞品 Zaya 披露的国内外模型、官方价、中文效果，以及塔塔现阶段要不要只用国内两家。

**结论：现阶段只用国内两家。** DeepSeek 做解读主模型，阿里云百炼做备用和看图。Claude / GPT 等海外包再接。塔塔已经是 `vendors` + `bindings`，不要把模型名写死在业务代码里。

本文只谈模型接入与成本，不涉及年运牌阵。价格会变，以文末来源页为准。美元按 7.1 折人民币，只作量级比较，不是结算汇率。未实测 Zaya 客户端，Zaya 用途以公开协议为准，不编造其零售价或具体模型版本。

---

## 1. 结论怎么用

| 项 | 决定 |
|---|---|
| 国内现在接谁 | DeepSeek 开放平台 + 阿里云百炼 |
| 主模型 | `deepseek-flash`（DeepSeek-V4.1-Flash-1017）绑 `interpret` / `tarotist` |
| 备用 | 百炼 `qwen-plus`（华北 2，输入 ≤128K：入 ¥0.8 / 出 ¥2） |
| 看图 | 有产品需求再绑百炼 VL；现在没有看图任务 |
| 海外 | 另开供应商。对话用 Claude Sonnet 档；语音用 OpenAI 转写。国内不上 |
| 语音 | Zaya 国内用火山 ASR。塔塔 P0 可空 |
| 切换 | 一次请求只打一家。故障再改 `bindings`，不要四家并行 |

后台模型名必须写 `deepseek-flash`。旧名 `deepseek-chat` / `deepseek-reasoner` 已下线；`deepseek-v4-flash` 只是别名，实际走 Flash 价。

---

## 2. Zaya 披露的四家 vs 塔塔

Zaya 官方说明：按功能选服务商，一次请求不会同时打全部。塔塔也应按任务绑一家。

| 地区 | 服务商 | Zaya 公开用途 | 官方价（核对日 2026-11-18） | 塔塔建议 |
|---|---|---|---|---|
| 国内 | DeepSeek 开放平台 | AI 对话生成 | `deepseek-flash` 谷时 cache miss：入 $0.15 / 出 $0.60（约 ¥1.1 / ¥4.3）；峰时翻倍 | **主模型**：interpret、追问、年运、塔罗师 |
| 国内 | 阿里云百炼 | 对话 + 用户主动分析的图片 | `qwen-plus` ≤128K 非思考：入 ¥0.8 / 出 ¥2；`qwen-max`：入 ¥2.4 / 出 ¥9.6 | **备用 + 看图**（有图才绑 VL） |
| 海外 | Anthropic Claude | 对话生成（未披露具体版本） | Sonnet 5.5：入 $2 / 出 $10（约 ¥14 / ¥71） | 海外主模型，国内不上 |
| 海外 | OpenAI | 语音转文字，不是聊天主模型 | Whisper `$0.006` / 分钟（将于 2027-02-26 下线）；现推荐 `gpt-transcribe` `$0.0045` / 分钟 | 海外 ASR；国内语音走火山，现在可以不做 |

国内版另披露火山引擎做语音识别、Amazon S3 存用户上传。海外版另有 Apple / Google 登录和 Mixpanel，与模型选型无关。

来源：

- Zaya 国内第三方清单：<https://zaya.quinlove.cn/third-party>
- Zaya 海外隐私政策：<https://heyzaya.com/privacy>
- DeepSeek 价目：<https://api-docs.deepseek.com/quick_start/pricing/>
- 阿里云百炼价目：<https://help.aliyun.com/zh/model-studio/model-pricing>
- Anthropic 价目：<https://platform.claude.com/docs/en/about-claude/pricing>
- OpenAI 价目：<https://developers.openai.com/api/docs/pricing>

---

## 3. 中文效果

LMArena Text Arena 中文榜（第三方转载，2026-11）：

| 模型 | Elo（约） | 说明 |
|---|---|---|
| Qwen3.8-Max | 1541 | 国内可走百炼，单价高于 plus |
| Claude Sonnet 5.5（xHigh） | 1541 | 海外对话档 |
| DeepSeek-V4.1-Flash（Max） | 1526 | 建议的国内主模型 |

榜单每天会动，Elo 误差带大约 ±10～20。Flash 与 Sonnet / Qwen Max 差十几个点，撑不起大约 10 倍的聊天单价。

塔塔场景是中文长文 + 塔罗师人设。人设靠 `tarotists.persona` / `voice` / `banned`，不靠换成海外模型。国内外可以绑不同供应商，知晚、隐川等人设必须同一套。

转载参考：<https://www.xingpanai.com/b/arena-text/chinese/>（Arena 中文文字榜）。官方入口是 LMArena Text Arena Chinese。

---

## 4. 一次解读的量级成本

估算口径：日常解读约 **2k 输入 + 1.5k 输出**，不含缓存命中、思考链加长、年运长文。不是账单。

| 模型 | 单价（每百万 token） | 单次约 | 日均 1000 次约 |
|---|---|---|---|
| DeepSeek Flash 谷时 cache miss | 入 $0.15 / 出 $0.60 | $0.0012 ≈ ¥0.009 | $1.2 ≈ ¥8 |
| 百炼 qwen-plus ≤128K 非思考 | 入 ¥0.8 / 出 ¥2 | ¥0.0046 | ¥4.6 |
| Claude Sonnet 5.5 | 入 $2 / 出 $10 | $0.019 ≈ ¥0.13 | $19 ≈ ¥135 |

年运一篇更长，仍远低于单独解锁 ¥28.8。真正贵的是每天的解读次数，不是年运。海外聊天模型相对 Flash 大约 **10～20 倍**。

峰时 Flash 入/出翻倍；人设长 prompt 命中缓存后输入会再便宜。思考模式会拉高输出。后台若误绑 `deepseek-v4-pro`，单价大约是 Flash 的 4 倍，国内主模型不要用 Pro。

---

## 5. 塔塔现在怎么接，建议怎么绑

代码已经按任务绑定，不是写死模型：

- 表：`vendors`（供应商、密钥、OpenAI 兼容 `base_url`）、`bindings`（任务 → 供应商 + 模型名）
- 管理端：`GET/PUT /api/admin/v1/vendors`、`PUT /api/admin/v1/bindings/{task}`
- 任务码：`detect` / `rewrite` / `recommend` / `interpret` / `tarotist`
- 年运生成走 **`interpret`**，没有单独的 yearly 绑定

| 任务 | 现在 | 国内建议 | 海外以后 |
|---|---|---|---|
| interpret（含追问、年运） | 后台可换 | DeepSeek Flash 主，qwen-plus 备 | Claude Sonnet 档 |
| tarotist | 同一套绑定 | 跟主模型，人设不换 | 跟海外主模型，人设不换 |
| detect / rewrite / recommend | 同一套绑定 | 跟主模型 | 跟海外主模型 |
| 看图 | 没有任务 | 有需求再加任务并绑百炼 VL | Claude / GPT 多模态 |
| 语音 | 没有 | Zaya 国内用火山，P0 可空 | OpenAI `gpt-transcribe` |

不要四家一起接。Zaya 也写明一次请求只选一家。塔塔出故障改绑定即可。

合规：国内用户的对话不要送到 Anthropic / OpenAI。海外包再单独配供应商和密钥。

---

## 6. 还没做、也不在这次结论里

产品文档还希望体验 Zaya 客户端（记忆、人物关系、多模态、扣费展示）。这次只核了对公开协议和官方价，**没有下载体验 Zaya**。下面几项仍空，不要当成已经调研完：

1. Zaya 实际用的 Claude 版本和路由规则（公开材料没有）
2. 塔塔按 token 记成本、按用户汇总（`ai_jobs` 目前不记 token 账单）
3. 主备自动故障切换（现在要人手改 `bindings`）
4. 看图、语音接口
5. 海外包供应商账号与数据驻留

会员单次额度、补充包发放次数等产品数字仍待确认，不在本文编造。

---

## 7. 来源

| 内容 | 链接 |
|---|---|
| 产品需求第九条 | 《塔塔 App 新人体验、每日免费次数及额度体系调整》§二 |
| Zaya 国内第三方 | https://zaya.quinlove.cn/third-party |
| Zaya 海外隐私 | https://heyzaya.com/privacy |
| DeepSeek 定价 | https://api-docs.deepseek.com/quick_start/pricing/ |
| 百炼定价 | https://help.aliyun.com/zh/model-studio/model-pricing |
| Claude 定价 | https://platform.claude.com/docs/en/about-claude/pricing |
| OpenAI 定价 | https://developers.openai.com/api/docs/pricing |
| 中文榜转载 | https://www.xingpanai.com/b/arena-text/chinese/ |
| 塔塔绑定实现 | `server/internal/tata/vendor_api.go`、`generate.go`、`aijob.go` |
