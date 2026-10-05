import { Agent } from "@earendil-works/pi-agent-core";
import { createModels } from "@earendil-works/pi-ai";
import { TaskScheduler, type ScheduledTask } from "./scheduler.ts";
import { createSchedulerTools } from "./tools.ts";

export interface MovieagentRunnerOptions {
  /** movieagent studio base URL, e.g. http://127.0.0.1:18081 */
  baseUrl?: string;
  /** Optional override instead of POSTing /api/v1/tasks */
  run?: (task: ScheduledTask) => Promise<void>;
}

/** Dispatch due tasks to movieagent brand-agent CreateTask API. */
export function createMovieagentTaskRunner(opts: MovieagentRunnerOptions = {}) {
  const baseUrl = (opts.baseUrl ?? process.env.MOVIEAGENT_BASE_URL ?? "http://127.0.0.1:18081").replace(/\/$/, "");
  if (opts.run) return opts.run;

  return async (task: ScheduledTask) => {
    const brandId = typeof task.meta?.brand_id === "string" ? task.meta.brand_id : undefined;
    const agentId = typeof task.meta?.agent_id === "string" ? task.meta.agent_id : "brand-agent";
    const res = await fetch(`${baseUrl}/api/v1/tasks`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        agent_id: agentId,
        prompt: task.prompt,
        brand_id: brandId,
      }),
    });
    const body = (await res.json()) as { code?: number; message?: string };
    if (!res.ok || body.code !== 0) {
      throw new Error(body.message || `movieagent task failed: HTTP ${res.status}`);
    }
  };
}

export interface CreateSchedulingAgentOptions {
  systemPrompt?: string;
  baseUrl?: string;
  /** Provider model id pair for pi-ai, e.g. ["anthropic","claude-sonnet-4-6"] */
  model?: [string, string];
}

/**
 * Build a pi-agent-core Agent with schedule_* tools wired to TaskScheduler.
 * Requires LLM provider env (e.g. ANTHROPIC_API_KEY) when prompting the agent.
 */
export function createSchedulingAgent(opts: CreateSchedulingAgentOptions = {}) {
  const scheduler = new TaskScheduler({
    runner: createMovieagentTaskRunner({ baseUrl: opts.baseUrl }),
  });
  scheduler.start();

  const models = createModels();
  const [provider = "anthropic", modelId = "claude-sonnet-4-6"] = opts.model ?? [];
  const model = models.getModel(provider, modelId);
  if (!model) {
    throw new Error(`model not found: ${provider}/${modelId} (check provider env / catalog)`);
  }

  const agent = new Agent({
    initialState: {
      systemPrompt:
        opts.systemPrompt ??
        `你是 movieagent 的任务调度助手。可用 schedule_create / schedule_list / schedule_cancel / schedule_run_now 管理定时海报任务。
任务的 prompt 会在到期时 POST 到 movieagent /api/v1/tasks（brand-agent）。`,
      model,
      tools: createSchedulerTools(scheduler),
    },
    streamFn: models.streamSimple.bind(models),
  });

  return { agent, scheduler, models };
}

export { TaskScheduler, createSchedulerTools };
export type { ScheduledTask, ScheduleSpec } from "./scheduler.ts";
