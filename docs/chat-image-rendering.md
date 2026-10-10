# 技术方案：Chat 页面渲染生成图片

| 项 | 内容 |
|----|------|
| 状态 | 已落地（movieagent chat-ui） |
| 范围 | 绘本 / 插画生图在对话气泡内回显 |
| 前端 | Next.js + assistant-ui（`useLocalRuntime`） |
| 后端 | Gin + SSE（`chathub`）+ `picturebook_illustrate` |

---

## 1. 目标

在 Chat 对话中，当模型调度生图 skill 后：

1. 等待阶段展示左对齐灰色文案「正在生成图片」
2. 完成后在**同一条助手消息**内展示图片（左对齐、无边框、原尺寸，仅限制不超过消息区宽度）
3. 说明文字可在图后出现；历史消息可回看图片

不采用：仅 Markdown `![](url)`、独立画廊页、新标签页下载作为主路径。

---

## 2. 总体架构

```text
用户发消息
    │
    ▼
POST /api/v1/chat  →  { task_id, events }
    │
    ▼
GET  /api/v1/chat/tasks/:id/events   (SSE)
    │
    ├─ thinking_delta
    ├─ status          「正在生成图片」
    ├─ image           { url, title }     ← 生图完成
    ├─ delta           方案/说明正文
    └─ done | error
    │
    ▼
ChatModelAdapter 组装 ThreadAssistantMessagePart[]
    │
    ▼
Thread MessagePrimitive.Parts
    ├─ reasoning → Thinking
    ├─ image     → <img>（GeneratedIllustration）
    └─ text      → Markdown 或 status 占位行
```

**原则：图片走独立 SSE 事件 + message `image` part，不塞进模型正文。**

---

## 3. 协议设计

### 3.1 创建任务

`POST /api/v1/chat`

```json
{
  "code": 0,
  "data": {
    "task_id": "task-xxx",
    "status": "pending",
    "events": "/api/v1/chat/tasks/task-xxx/events"
  }
}
```

### 3.2 SSE 事件（`chathub.Event`）

| type | 字段 | 含义 |
|------|------|------|
| `task` | `status` | 任务状态 |
| `thinking_delta` | `delta` | 思考增量 |
| `status` | `message` | 等待/失败提示（如「正在生成图片」） |
| `image` | `url`, `title` | 可访问图片 URL + 标题 |
| `delta` | `delta` | 助手正文增量 |
| `done` | `model` | 正常结束 |
| `error` | `message` | 失败结束 |

示例：

```text
data: {"type":"status","task_id":"...","message":"正在生成图片"}

data: {"type":"image","task_id":"...","url":"/media/generated/chat-....png","title":"插画 512x768"}

data: {"type":"delta","task_id":"...","delta":"已按夜色方向生成横版氛围图。"}

data: {"type":"done","task_id":"...","model":"deepseek-chat"}
```

### 3.3 静态资源 / OSS

| 模式 | 行为 |
|------|------|
| **OSS 已配置** | PNG 上传阿里云 OSS，SSE `image.url` 为公网/CDN 地址 |
| **未配置（默认）** | 落盘 `tmp/generated/`，SSE `url` = `/media/generated/<name>.png` |

环境变量（见 `.env.example`）：

```bash
OSS_ENDPOINT=oss-cn-hangzhou.aliyuncs.com
OSS_ACCESS_KEY_ID=...
OSS_ACCESS_KEY_SECRET=...
OSS_BUCKET=...
OSS_PUBLIC_BASE_URL=https://cdn.example.com   # 可选
OSS_KEY_PREFIX=movieagent/chat/
```

对象 Key：`{OSS_KEY_PREFIX}{yyyy/mm/dd}/chat-<timestamp>-<suffix>.png`  
上传失败时自动回退本地 `/media/generated/...`。

---

## 4. 后端职责

| 步骤 | 位置 | 行为 |
|------|------|------|
| skill 开始 | `tool_start` + `picturebook_illustrate` | `publish(status: 正在生成图片)` |
| skill 开始 | `picturebook_illustrate` | **异步出图**：先推 `status`/正文 `delta`，不阻塞等扩散完成 |
| skill 成功 | `tool_end`（后台完成） | `publishPicturebook`：沙箱 PNG → 本地/OSS → `publish(image)`，再 `done` |
| skill 失败 | `tool_end` IsError | `publish(status: 失败文案)` |
| 发布失败 | `url == ""` | `publish(status: 发布失败提示)`，不静默丢弃 |

图片 URL **只通过 SSE 推送**；模型 system prompt 要求正文不写内部路径、不声称「已保存到…」。

---

## 5. 前端职责

### 5.1 关键模块

| 文件 | 职责 |
|------|------|
| `lib/movieagent/api.ts` | `startChatTask` / `subscribeChatEvents`；`apiURL`；SSE 断线重连一次 |
| `lib/movieagent/adapter.ts` | SSE → `ChatModelRunResult.content` parts |
| `lib/movieagent/status-part.ts` | status 编码进 text part，供 UI 识别 |
| `components/.../chat-image.tsx` | 等待行 / 成图 `<figure>` |
| `components/.../thread.aui.tsx` | `MessagePrimitive.Parts` 分发渲染 |
| `next.config.ts` | `NEXT_PUBLIC_MOVIEAGENT_API_ORIGIN` 直连 Go，避免 rewrite 掐断长 SSE |

### 5.2 Adapter 组装顺序

同一条助手消息内 parts 顺序：

1. `reasoning`（Thinking）
2. `image`（可多张）
3. `text`（正文 Markdown）
4. status 文本（仅当尚无 image：等待或失败）

对应类型：

```ts
{ type: "image", image: string /* url */, filename?: string }
{ type: "text", text: "⟦ma:status:pending⟧正在生成图片" }  // UI 解析后不走 Markdown
```

### 5.3 渲染规则

| part | UI |
|------|-----|
| `image` | 左对齐、`w-auto h-auto max-w-full`、无边框；可点开新标签 |
| status pending | 灰色左对齐一行「正在生成图片」 |
| status error | 红色左对齐失败文案 |
| `text` | 常规 Markdown |

成图到达后清除 status，占位行消失，图片淡入。

### 5.4 开发环境 SSE

Next.js `rewrites` 代理长连接约 30s 会断开，生图常超过该时间。

- **Dev**：浏览器直连 `NEXT_PUBLIC_MOVIEAGENT_API_ORIGIN=http://127.0.0.1:8090`（CORS 放行 `:3000`）
- **Prod（Go 托管 static export）**：同域相对路径，无代理超时问题
- **兜底**：SSE 未收到 `done`/`error` 时重连一次，依赖 hub 回放 history（含晚到的 `image`）

---

## 6. 交互时序（两轮常见路径）

```text
Turn 1  「做一张夜色氛围图」
  → 仅方案文字（可能无 tool）

Turn 2  「按这个出图」
  → thinking（可选）
  → status: 正在生成图片
  → image: /media/generated/...
  → delta: 简短说明
  → done
```

图片落在**本轮**助手气泡，不回写上一轮消息。

---

## 7. 方案对比（为何不用其它做法）

| 方案 | 结论 |
|------|------|
| Markdown `![](url)` | 依赖模型格式；等待态难做；与「系统单独展示图」冲突 |
| Tool UI 卡片 | 适合展示调用过程，不适合作为成图主展示 |
| Base64 进 SSE | 体积大、延迟高 |
| **SSE `image` + image part** | 可控、可回放、与 assistant-ui 模型一致（采用） |

---

## 8. 测试

| 类型 | 覆盖 |
|------|------|
| E2E（Playwright mock SSE） | 等待文案、两轮出图、失败 status、错误 SSE |
| Go | `publishPicturebook` 拷贝到 media 路径 |

命令：

```bash
cd web/chat-ui && npm run test:e2e -- e2e/send-message.spec.ts
go test ./internal/handler/ -count=1
```

---

## 9. 风险与约束

1. **长耗时生图**：必须直连 API 或保证 SSE 代理不超时  
2. **URL 可达**：`/media/generated` 与前端 origin 配置一致  
3. **并发同秒文件名**：已用时间戳 + 纳秒后缀避免覆盖  
4. **历史会话持久化**：当前 runtime 为本地会话；若以后落库，需同时存 image part 的 URL  

---

## 10. 验收标准

- [x] 生图过程中出现「正在生成图片」（左对齐灰字）  
- [x] 完成后同一气泡展示图片（左对齐、无边框、原尺寸 / 宽度自适应）  
- [x] 点击可打开大图  
- [x] 失败有可读 status，不出现空白  
- [x] Dev 下生图 >30s 不因代理导致 `network error`  

---

## 11. 相关代码索引

```text
internal/handler/chat.go              # status / image 发布
internal/service/chathub/hub.go       # SSE fan-out
internal/router/router.go             # /media/generated、CORS
web/chat-ui/lib/movieagent/api.ts
web/chat-ui/lib/movieagent/adapter.ts
web/chat-ui/lib/movieagent/status-part.ts
web/chat-ui/components/assistant-ui/elements/chat-image.tsx
web/chat-ui/components/assistant-ui/elements/thread.aui.tsx
web/chat-ui/e2e/send-message.spec.ts
```
