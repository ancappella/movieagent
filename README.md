# movieagent

基于 Gin 的 Go HTTP 服务骨架。

## 环境要求

- Go 1.25+（首次构建会自动下载匹配 toolchain）
- 建议设置：`export GOPROXY=https://goproxy.cn,direct`

## 快速开始

```bash
export PATH="$HOME/.local/go/bin:$PATH"
export GOPROXY=https://goproxy.cn,direct

go run ./cmd/server
```

健康检查：

```bash
curl http://127.0.0.1:8080/api/v1/health
```

## Nano Banana 文生图

默认画图模型：`DRAW_MODEL=nano-banana-pro`（Gemini 3 Pro Image）。

```bash
# CLI
go run ./cmd/nanobanana -p "柠檬茶海报，无文字" -o tmp/nano-banana.png

# HTTP（ReAct / 前端可调用）
curl -X POST http://127.0.0.1:8080/api/v1/images/generate \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"A lemon tea poster, no text","aspect_ratio":"3:4","image_size":"1K"}'
# 结果图：/media/generated/<file>.png
```

品牌 Agent 的 ReAct 工具 `generate_image` 同样走 Nano Banana Pro。

## 目录结构

```text
cmd/server          # 程序入口
internal/config     # 配置加载
internal/handler    # HTTP handlers
internal/middleware # 中间件
internal/router     # 路由注册
pkg/response        # 统一响应体
```

## 常用环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_NAME` | `movieagent` | 应用名 |
| `GIN_MODE` | `debug` | Gin 模式：`debug` / `release` / `test` |
| `APP_HOST` | `0.0.0.0` | 监听地址 |
| `APP_PORT` | `8080` | 监听端口 |
