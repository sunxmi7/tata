# 解读规则 → Prompt 落地说明

产品原文：`docs/design/interpret-rules-v1.md`（来自《塔塔解读规则.md》）。

## 仓库现状（改前）

| 层级 | 位置 | 作用 |
|---|---|---|
| System | `server/internal/readingflow/prompt.go` 常量 | 按 binding task（detect/rewrite/recommend/interpret/tarotist）写死短系统提示 |
| User | `server/internal/tata/generate.go` `buildInterpretPrompt` | 拼「不得改牌 + 问题/主题 + 牌面 JSON + 大阿尔卡纳参考」 |
| 调用 | `aijob.executeInterpret` → `runEinoStream(task, userPrompt)` | task 对 normal/daily/explore 都是 `interpret`，系统提示相同 |
| 供应商 | `bindings` 表 | 只绑 vendor/model，**没有**可编辑 prompt 配置表/Admin 页 |

旧 interpret 系统提示要求「逐张解释后给总评」，与规则文档「整体叙事、不拆重复模块」冲突。

## 本次拆分

| 文件 | 内容 |
|---|---|
| `readingflow/prompts/common.md` | 原则、正逆位、关联、输出隔离、语言、格式、异常 |
| `readingflow/prompts/normal.md` | 普通占卜 |
| `readingflow/prompts/daily_today.md` | 今日运势 |
| `readingflow/prompts/daily_tomorrow.md` | 明日运势 |
| `readingflow/prompts/explore.md` | 探索 |
| `readingflow.InterpretRulesVersion` | 版本钉：`interpret-rules-v1-2026-11-25` |

组合：`InterpretSystem(mode) = common + mode 模块 + 版本行`。塔罗师仍用人设短提示（规则适用范围不含塔罗师）。

## 如何接入

1. `executeInterpret` 按 `readings.type` + 问题文案选 mode，把 `InterpretSystem(mode)` 放进 Eino `{system}`。
2. User 侧仍用 `buildInterpretPrompt`（已去掉「逐张总评」措辞）。
3. 生成后用 `InterpretationLeakReason` 拦明显指令泄漏；异常则 `FAILED_RETRYABLE`。

## 版本化

- 产品稿改版本：更新 `docs/design/interpret-rules-v1.md`，同步改 `prompts/*.md`，bump `InterpretRulesVersion`。
- 先走 **embed 文件 + Git**（同 `major_meanings.json` / `legal/*.md`）。Admin DB 可编辑 prompt **未做**，需产品确认是否要运营热更新后再加表。

## 后续（需拍板）

1. 是否把 yearly 也纳入同一规则包。
2. 是否做 Admin「解读 Prompt」热更新（库表 + 版本 + 灰度）。
3. 牌位含义是否在 user prompt 中结构化展开（现主要靠牌面 JSON 里的 `position_name`）。
4. 更强的输出清洗 / 自动重试策略与额度关系。
