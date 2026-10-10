# chat-ui (assistant-ui)

Official [assistant-ui](https://github.com/assistant-ui/assistant-ui) scaffold (`create -t minimal`), wired to movieagent via `useLocalRuntime`.

## Dev

```bash
# terminal 1 — Go API
APP_PORT=8090 /tmp/ma-bin   # or go run ./cmd/server

# terminal 2 — Next.js UI (proxies /api/v1 → :8090)
cd web/chat-ui && npm run dev
# open http://localhost:3000
```

## Production (served by Go on :8090)

```bash
cd web/chat-ui && npm run build   # writes ./out (NEXT_EXPORT=1)
# restart movieagent — serves web/chat-ui/out at /
```

Open http://127.0.0.1:8090/
