---
name: picturebook-illustrate
description: 儿童绘本水彩彩铅插画。用户要绘本插画、儿童插画、水彩彩铅风格出图、picture book illustration，或明确调用 picturebook_illustrate 时使用。通过 Canvas 2D 绘制 PNG（不使用 SD / LoRA）。
license: MIT
tool: picturebook_illustrate
script: scripts/draw.mjs
---

# Picturebook Illustrate

用服务端 **Canvas 2D** 绘制水彩 + 彩铅风格的儿童绘本插画（程序化场景，不是 Stable Diffusion / LoRA）。

## 何时使用

- 儿童绘本 / 故事插页
- 水彩、彩铅风格插画
- 需要按比例出图（封面、短视频竖图、明信片等）

## 执行方式

必须调用本 skill 绑定的工具 `picturebook_illustrate`。运行时会执行本目录 `scripts/draw.mjs`（Node Canvas）。不要改用其它出图通道，不要自己拼宿主路径或泄露 sandbox 内部细节给用户。

### 调度要点

1. 用户要图 → 本轮就发 toolCall，不要只写方案。
2. `prompt` 写具体：主体、动作、环境；脚本会按关键词选择主题（海/狐/花/森林/女孩等）并上色。
3. `aspect_ratio` 按用途选；默认绘本插页 `2:3`。
4. 收到 ToolResult 后再写简短画面说明；失败则根据错误调整 prompt 再调一次。

### 参数要点

| 参数 | 说明 |
| --- | --- |
| `prompt` | 场景 / 角色描述（中英均可） |
| `aspect_ratio` | 优先用比例选尺寸，默认 `2:3` |
| `output_path` | 沙箱虚拟路径，默认 `/mnt/user-data/outputs/picturebook.png` |
| `width` + `height` | 同时提供时可覆盖比例预设 |
| `seed` | 可选，影响随机笔触与布局 |

比例与像素映射见 `references/aspect-ratios.md`。

## 对用户回复

只展示生成的图片与简短创作说明。不要暴露工具名、sandbox 路径、seed、字节数或思考过程。
