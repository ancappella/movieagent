# movieagent — Agent Instructions

Go + Gin 后端的图文/视频创意助手。Chat 走 claw 式 Thinking/Action 循环；绘本插画与短动画走 skills；运行时状态落在 `agentruntime` + `task`。

## 环境与命令

- Go：`go.mod` 要求的 toolchain（建议 `GOPROXY=https://goproxy.cn,direct`）
- 配置：复制 `.env.example` → `.env`；**不要提交或改写密钥**
- 启动：`go run ./cmd/server`（默认 `http://127.0.0.1:8080`）
- 测试：`go test ./internal/...`；改 skill 脚本时在对应 `scripts/` 下测
- Chat UI：`web/chat-ui`（Next 静态导出到 `out/`）；该目录另有 `AGENTS.md`
- 国内代理：Go 不走系统代理时设 `HTTPS_PROXY` / `GEMINI_HTTP_PROXY`（见 `.env.example`）

## 目录地图

```text
cmd/server                     # HTTP 入口
internal/
  handler/                     # Chat SSE、pause/resume、出图等
  context/                     # Session / systemprompt / skills 调度文案
  service/
    claw/                      # 引擎 loop + schema + tool registry
    agentruntime/              # Runtime：context / memory / infer / checkpoint
    task/                      # AgentTask → Turn → SkillRun
    tools/                     # picturebook_illustrate / picturebook_animate
    storage/                   # 阿里云 OSS（chat 图视频）
    providers/                 # deepseek / gemini …
    sandbox/                   # LocalSandbox 虚拟路径
skills/public/
  picturebook-illustrate/      # Canvas 绘本静图（draw.mjs）
  picturebook-animate/         # LLM draw(t) → Playwright 帧 → ffmpeg MP4（render.mjs）
docs/chat-image-rendering.md   # SSE 出图/出视频约定
integrations/pi-agent/         # 可选：pi-agent-core 调度桥
```

运行时数据默认在 `{project}/.deer-flow`（或 `DEER_FLOW_HOME`）：

- `tasks/{taskID}/` — task.json、turns、skills、`checkpoint.json` + `checkpoint.full.json`
- `infer/{taskID}/messages.jsonl` — 推理 transcript
- `sessions/{sessionID}/memory.json` — 长期记忆

## Chat 主链路（改行为时优先看这些）

1. `POST /api/v1/chat` → `task.Scheduler.Start` → 后台 `runChatTask`
2. `agentruntime.New` 建 Runtime；`Session.WireMessages` → `claw/engine.Run`
3. SSE：`GET /api/v1/chat/tasks/:id/events`（chathub）
4. Checkpoint：`pause` / `resume` / `checkpoint` / `runtime` 同前缀
5. 异步 skill（绘本/动画）：后台跑完后推 `image` / `video` 事件；OSS 配置时以 OSS URL 为准

对用户可见文案禁止泄露：`/mnt/`、sandbox、tool 名、字节数、seed、内部路径（见 `handler` 里 scrub）。

## Skills

| Skill | Tool | 产物 | 说明 |
|-------|------|------|------|
| picturebook-illustrate | `picturebook_illustrate` | PNG | Canvas 2D，不是 SD/LoRA |
| picturebook-animate | `picturebook_animate` | MP4 | 模型写含 `draw(t,…)` 的 JS → Playwright 逐帧 → ffmpeg；无 code 时用 `template` |

- Skill 元数据：各目录 `SKILL.md` frontmatter（`tool` / `script`）
- 动画入口脚本：`skills/public/picturebook-animate/scripts/render.mjs`（需 `npm install` + Playwright Chromium）
- 调度文案：`internal/context/systemprompt.md`、`ImageSkillSchedule()`

## agentruntime / checkpoint

- **Context**：`internal/context.Session`（system/user/assistant/tool_result）
- **Infer**：按 task 追加 JSONL，不要把完整引擎历史扔掉却又不落盘
- **Checkpoint**：`checkpoint.full.json` 含 phase、continuation、messages、pending_tools（`replay: safe|never`）
- 出图/出视频工具默认 `replay=never`；Resume 先看 `PlanResume`，勿盲目重跑副作用工具
- 轻量游标仍写 `task.Checkpoint`；完整恢复用 `AgentCheckpoint`

## 编码约定

- 新能力优先落在现有包（`agentruntime` / `task` / `tools` / skill `scripts/`），避免平行再造一套 Message/Session
- Canonical 会话消息用 `agentctx.Message`；与 claw 边界用转换，不要在 handler 里堆业务逻辑
- 持久化写文件：tmp + rename；JSON/JSONL 放 RuntimeHome 下，不进 git
- 改 Go API 或 SSE 字段时同步 `docs/chat-image-rendering.md` 与 chat-ui 消费方
- 前端视觉任务遵守用户既有设计规则；不要无故改 `web/chat-ui` 依赖大版本
- 不要擅自 `git commit` / `push`；不要改 `.env` 里的密钥值

## 常用自检

```bash
curl -s http://127.0.0.1:8080/api/v1/health
go test ./internal/service/agentruntime/ ./internal/service/task/ ./internal/service/tools/ -count=1
# 动画 smoke（需已 npm install）
node skills/public/picturebook-animate/scripts/render.mjs \
  --template character_spin --output /tmp/anim-smoke.mp4 --duration 1 --fps 12
```
