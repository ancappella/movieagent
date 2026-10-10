---
name: picturebook-animate
description: 根据用户动画需求，由模型编写 Canvas 动画程序（draw/seek），再用 Playwright 逐帧截图 + ffmpeg 合成 MP4。用户要做成动画、动起来、原地旋转、插画短视频，或明确调用 picturebook_animate 时使用。
license: MIT
tool: picturebook_animate
script: scripts/render.mjs
---

# Picturebook Animate（代码驱动渲染）

流水线：

1. **用户 Prompt**：描述动画需求（如「玩偶原地旋转 10 秒」）。
2. **你写程序**：输出可运行的浏览器 JS（文本代码，不是视频）。必须实现：
   - `draw(t, ctx, canvas, assets)`：`t` 为秒；根据 `t` 画出完整一帧
   - 可选 `seek(t)`：更新内部状态
   - 可选 `async setup({ canvas, ctx, width, height, assets })`
   - 若有静图，`assets.image` 为 `HTMLImageElement`
3. **调用工具** `picturebook_animate`，把代码放进参数 `code`，并设置 `duration_sec` / `fps`；可选 `image_path`。
4. **运行时**：Playwright 无头浏览器对每个时间点 `t` 调用 `draw`，截图帧序列 → ffmpeg → MP4。

## 何时使用

- 用户要「做成动画 / 动起来 / 旋转 / 短视频」
- 已有插画时可传入 `image_path`（默认 `/mnt/user-data/outputs/picturebook.png`）

## 调度要点

1. **先写代码再调工具**：`code` 是模型产物；不要只写文案假装已出片。
2. `draw` 必须幂等：任意 `t` 都能完整重绘，不要依赖上一帧残影（除非刻意拖影）。
3. 时长从用户需求解析（如「10 秒」→ `duration_sec: 10`）；默认 4，最大 30。
4. 若暂时写不出代码，可省略 `code` 并设 `template: character_spin` 或 `character_idle` 作兜底。
5. 工具异步执行：收到后台开始的 ToolResult 后立刻写动效说明；成片由系统稍后推送。

### 参数

| 参数 | 说明 |
| --- | --- |
| `code` | 动画 JS 源码（推荐由你生成） |
| `prompt` | 用户原话（日志 / 兜底） |
| `image_path` | 可选静图，映射为 `assets.image` |
| `duration_sec` | 秒，默认 4 |
| `fps` | 默认 24 |
| `width` / `height` | 默认 512×768 |
| `template` | `code` 为空时的兜底：`character_spin` / `character_idle` |
| `output_path` | 默认 `/mnt/user-data/outputs/picturebook.mp4` |

### `draw` 示例骨架

```javascript
function setup({ assets }) {
  // optional
}

function seek(t) {
  // optional state
}

function draw(t, ctx, canvas, assets) {
  const w = canvas.width, h = canvas.height;
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, w, h);
  const img = assets.image;
  if (!img) return;
  const u = t; // seconds
  const angle = (u / 4) * Math.PI * 2; // example: 4s one turn — use real duration in your math
  const sx = Math.cos(angle);
  ctx.save();
  ctx.translate(w / 2, h / 2);
  ctx.scale(Math.max(0.08, Math.abs(sx)) * Math.sign(sx || 1), 1);
  ctx.drawImage(img, -img.width / 2, -img.height / 2);
  ctx.restore();
}
```

## 对用户回复

只描述动效与成片将单独展示。不要暴露工具名、sandbox 路径或内部脚本细节。
