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
