# 解读规则 → Prompt 落地说明

产品原文：`docs/design/interpret-rules-v1.md`（来自《塔塔解读规则.md》）。

## 仓库分层

| 层级 | 位置 | 作用 |
|---|---|---|
| System | `readingflow/prompts/*.md` + `InterpretSystem(mode)` | 普通/今日/明日/探索规则包 |
| User | `tata/generate.go` `buildInterpretPrompt` | 问题/主题/牌阵 + **牌位轻量展开** + 牌面 JSON + 大阿尔卡纳参考 |
| 调用 | `aijob.executeInterpret` | 按 reading type 选 mode，注入 system；加载 `spreads.positions` |
| 供应商 | `bindings` | 只绑 vendor/model；**不做** Admin 热更新 prompt |

## Prompt 拆分

| 文件 | 内容 |
|---|---|
| `readingflow/prompts/common.md` | 原则、正逆位、关联、输出隔离、语言、格式、异常 |
| `readingflow/prompts/normal.md` | 普通占卜 |
| `readingflow/prompts/daily_today.md` | 今日运势 |
| `readingflow/prompts/daily_tomorrow.md` | 明日运势 |
| `readingflow/prompts/explore.md` | 探索 |
| `readingflow.InterpretRulesVersion` | `interpret-rules-v1-2026-11-25` |

塔罗师仍用人设短提示（规则适用范围不含塔罗师）。年报 **不纳入** 本规则包，继续独立 `buildYearlyPrompt`。

## 牌位轻量展开（已定）

- 数据：`spreads.positions` 的 `name` + `meaning`（即 description）。
- 普通占卜 / 探索（及塔罗师共用解牌材料）：按**抽牌顺序**输出  
  `序号. 牌位名：含义 / 牌名 正逆位`。
- 今日 / 明日：只输出 `牌位：槽位名`，不展开含义。
- 不做诉求标签（I 代码）级展开。

## 版本化

- 产品稿改版本：更新 `interpret-rules-v1.md`，同步 `prompts/*.md`，bump `InterpretRulesVersion`。
- 运维方式：embed 文件 + Git 发版（不做 Admin 热更新）。

## 生成后校验

`InterpretationLeakReason` 拦截明显工作性前缀 / 异常格式；异常则 `FAILED_RETRYABLE`。
