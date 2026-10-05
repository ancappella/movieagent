import type { AgentTool } from "@earendil-works/pi-agent-core";
import { Type } from "@earendil-works/pi-ai";
import type { ScheduleSpec, TaskScheduler } from "./scheduler.ts";

function asSchedule(raw: unknown): ScheduleSpec {
  const s = raw as ScheduleSpec;
  if (!s || typeof s !== "object" || !s.kind) {
    throw new Error("schedule.kind is required (once|interval|cron)");
  }
  return s;
}

/** LLM-callable scheduler tools for a pi-agent-core Agent. */
export function createSchedulerTools(scheduler: TaskScheduler): AgentTool[] {
  const schedule_create: AgentTool = {
    name: "schedule_create",
    label: "Create scheduled task",
    description:
      "Create a scheduled task. schedule.kind is once|interval|cron. once uses at (+5m or ISO); interval uses every (30s|10m|1h); cron uses 5-field UTC cron.",
    parameters: Type.Object({
      name: Type.String(),
      prompt: Type.String({ description: "Prompt or job payload to run when due" }),
      schedule: Type.Object({
        kind: Type.Union([Type.Literal("once"), Type.Literal("interval"), Type.Literal("cron")]),
        at: Type.Optional(Type.String()),
        every: Type.Optional(Type.String()),
        cron: Type.Optional(Type.String()),
      }),
      brand_id: Type.Optional(Type.String()),
      agent_id: Type.Optional(Type.String()),
    }),
    execute: async (_id, params) => {
      const p = params as {
        name: string;
        prompt: string;
        schedule: ScheduleSpec;
        brand_id?: string;
        agent_id?: string;
      };
      const task = scheduler.schedule({
        name: p.name,
        prompt: p.prompt,
        schedule: asSchedule(p.schedule),
        meta: {
          brand_id: p.brand_id,
          agent_id: p.agent_id ?? "brand-agent",
        },
      });
      return {
        content: [{ type: "text", text: JSON.stringify(task, null, 2) }],
        details: task,
      };
    },
  };

  const schedule_list: AgentTool = {
    name: "schedule_list",
    label: "List scheduled tasks",
    description: "List all scheduled tasks and their next run times.",
    parameters: Type.Object({}),
    execute: async () => {
      const list = scheduler.list();
      return {
        content: [{ type: "text", text: JSON.stringify(list, null, 2) }],
        details: list,
      };
    },
  };

  const schedule_cancel: AgentTool = {
    name: "schedule_cancel",
    label: "Cancel scheduled task",
    description: "Cancel a scheduled task by id.",
    parameters: Type.Object({ id: Type.String() }),
    execute: async (_id, params) => {
      const id = String((params as { id: string }).id);
      const ok = scheduler.cancel(id);
      return {
        content: [{ type: "text", text: ok ? `cancelled ${id}` : `not cancelled ${id}` }],
        details: { id, ok },
      };
    },
  };

  const schedule_run_now: AgentTool = {
    name: "schedule_run_now",
    label: "Run scheduled task now",
    description: "Execute a scheduled task immediately.",
    parameters: Type.Object({ id: Type.String() }),
    execute: async (_id, params) => {
      const id = String((params as { id: string }).id);
      const task = await scheduler.runNow(id);
      return {
        content: [{ type: "text", text: JSON.stringify(task, null, 2) }],
        details: task,
      };
    },
  };

  return [schedule_create, schedule_list, schedule_cancel, schedule_run_now];
}
