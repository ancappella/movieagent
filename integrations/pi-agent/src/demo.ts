/**
 * Demo: schedule a one-shot brand-agent poster job via TaskScheduler
 * (no LLM required). Start movieagent on :18081 first.
 *
 *   npm run scheduler:demo
 */
import { TaskScheduler } from "./scheduler.ts";
import { createMovieagentTaskRunner } from "./agent.ts";

const scheduler = new TaskScheduler({
  runner: createMovieagentTaskRunner(),
  tickMs: 500,
});

scheduler.start();

const task = scheduler.schedule({
  name: "midautumn-poster",
  prompt: "为中秋节柠檬茶新品做一张竖版海报，氛围团圆温馨，保留饮品主体。",
  schedule: { kind: "once", at: "+3s" },
  meta: { brand_id: "ningdao", agent_id: "brand-agent" },
});

console.log("scheduled:", task.id, "next:", task.nextRunAt);

const iv = setInterval(() => {
  const t = scheduler.get(task.id);
  if (!t) return;
  console.log("status:", t.status, "lastError:", t.lastError ?? "-");
  if (t.status === "done" || t.status === "failed" || t.status === "cancelled") {
    clearInterval(iv);
    scheduler.stop();
    process.exit(t.status === "done" ? 0 : 1);
  }
}, 1000);
