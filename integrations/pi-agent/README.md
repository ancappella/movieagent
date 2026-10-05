# movieagent × pi-agent-core

Installs [@earendil-works/pi-agent-core](https://www.npmjs.com/package/@earendil-works/pi-agent-core) and adds a **TaskScheduler** so brand-agent poster jobs can run on a schedule.

`pi-agent-core` is the agent loop (tools + state + streaming). Scheduling is implemented in this folder on top of it.

## Install

```bash
cd integrations/pi-agent
npm install
```

## Schedule API (no LLM)

```ts
import { TaskScheduler, createMovieagentTaskRunner } from "./src/index.ts";

const scheduler = new TaskScheduler({
  runner: createMovieagentTaskRunner({ baseUrl: "http://127.0.0.1:18081" }),
});
scheduler.start();

scheduler.schedule({
  name: "midautumn",
  prompt: "为中秋节做一张竖版海报",
  schedule: { kind: "once", at: "+5m" },       // or { kind: "interval", every: "1h" }
  meta: { brand_id: "ningdao", agent_id: "brand-agent" },
});
```

Due tasks `POST` to `http://127.0.0.1:18081/api/v1/tasks`.

## Agent tools (with LLM)

```ts
import { createSchedulingAgent } from "./src/index.ts";

const { agent, scheduler } = createSchedulingAgent();
await agent.prompt("每小时生成一张中秋海报，品牌用茶抱抱");
```

Tools: `schedule_create` / `schedule_list` / `schedule_cancel` / `schedule_run_now`.

## Demo

```bash
# terminal 1: movieagent server
APP_PORT=18081 /tmp/movieagent-server

# terminal 2
cd integrations/pi-agent
npm run scheduler:demo
```
