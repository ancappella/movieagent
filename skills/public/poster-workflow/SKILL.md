---
name: poster-workflow
description: 品牌海报工作流。用户要节日海报、营销出图、产品图合成时使用这个 skill：ingest → brief → creative_design → prompt → render → brand_composite。
license: MIT
---

# 海报工作流

从品牌 Agent 拆出的独立 skill。选中本 skill 后，robot session 走 `RunPosterWorkflow`，不读取其它 SKILL.md。

## 何时使用

- 节日 / 活动海报
- 饮品或产品图出图
- 品牌 Logo / 品牌色合成

## 流水线

```text
ingest → brief(gemini-flash) → creative_design(gemini-flash) → prompt → render → brand_composite
```

## 输入

| 输入 | 必须 | 说明 |
| --- | --- | --- |
| 用户 prompt | 是 | 节日、产品、风格 |
| 产品图 | 推荐 | 饮品参考图 |
| 品牌资产 | 可选 | Logo、品牌色 |

## 输出

- 创意简报（思考区）
- 海报图（对话结果卡）
