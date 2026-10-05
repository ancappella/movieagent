---
name: brand-tone
description: （文档参考，运行时不再加载）海报工作流说明：节日 + 产品图 → Gemini Flash 简报/创意 → Nano Banana Pro 出图。实际执行走 brandagent.RunPosterWorkflow，直接转发大模型。
license: MIT
---

# Brand Tone（文档 · 运行时不调用）

> **注意**：agent / `RunBrandTone` / `RunPosterWorkflow` **不再读取本 SKILL.md**。  
> 请求直接进入 Gemini 工作流：`brief → creative_design → prompt → render`。

保留本文仅作产品说明参考。

## 运行时流水线

```text
ingest → brief(gemini-flash) → creative_design(gemini-flash) → prompt → render(nano-banana-pro)
```

入口：

```bash
go run ./internal/service/agents/brandagent/example
# 或代码：agent.RunPosterWorkflow(... RequireGemini: true)
```

## 输入

| 输入 | 必须 | 说明 |
| --- | --- | --- |
| 节日名称 | 是 | 国庆节 / 中秋节… |
| 产品图 | 是 | 饮品参考图 |
| 产品提示 | 推荐 | 柠檬茶新品 |

## 节点产物

| 节点 | 产物 |
| --- | --- |
| brief | `festival-brief.md` |
| creative_design | `creative-design.md` |
| prompt | `brand-prompt.md` |
| render | `outputs/brand-campaign.png` |
