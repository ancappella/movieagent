/**
 * Lightweight task scheduler used with @earendil-works/pi-agent-core.
 * pi-agent-core itself is the agent loop; scheduling lives here so brand /
 * poster jobs can be queued once, on an interval, or at a cron-like pattern.
 */

export type ScheduleKind = "once" | "interval" | "cron";

export type TaskStatus = "pending" | "running" | "done" | "failed" | "cancelled";

export interface ScheduleSpec {
  kind: ScheduleKind;
  /** once: ISO time or "+5m" / "+30s" */
  at?: string;
  /** interval: "30s" | "10m" | "1h" */
  every?: string;
  /** cron: minute hour day-of-month month day-of-week (5 fields) */
  cron?: string;
}

export interface ScheduledTask {
  id: string;
  name: string;
  prompt: string;
  schedule: ScheduleSpec;
  status: TaskStatus;
  createdAt: string;
  nextRunAt?: string;
  lastRunAt?: string;
  lastError?: string;
  meta?: Record<string, unknown>;
}

export type TaskRunner = (task: ScheduledTask) => Promise<void>;

export interface TaskSchedulerOptions {
  runner: TaskRunner;
  /** Poll / tick resolution in ms (default 1000). */
  tickMs?: number;
  now?: () => Date;
}

function parseDuration(raw: string): number {
  const m = String(raw).trim().match(/^(\d+(?:\.\d+)?)(ms|s|m|h|d)$/i);
  if (!m) throw new Error(`invalid duration: ${raw}`);
  const n = Number(m[1]);
  switch (m[2].toLowerCase()) {
    case "ms":
      return n;
    case "s":
      return n * 1000;
    case "m":
      return n * 60_000;
    case "h":
      return n * 3_600_000;
    case "d":
      return n * 86_400_000;
    default:
      throw new Error(`invalid duration unit: ${raw}`);
  }
}

function parseOnce(at: string, now: Date): Date {
  const s = at.trim();
  if (s.startsWith("+")) {
    return new Date(now.getTime() + parseDuration(s.slice(1)));
  }
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) throw new Error(`invalid once time: ${at}`);
  return d;
}

/** Minimal 5-field cron matcher (UTC). Supports *, N, and *\/N. */
function cronMatches(expr: string, d: Date): boolean {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) throw new Error(`cron needs 5 fields: ${expr}`);
  const fields = [
    d.getUTCMinutes(),
    d.getUTCHours(),
    d.getUTCDate(),
    d.getUTCMonth() + 1,
    d.getUTCDay(),
  ];
  return parts.every((part, i) => matchCronField(part, fields[i]!));
}

function matchCronField(part: string, value: number): boolean {
  if (part === "*") return true;
  if (part.startsWith("*/")) {
    const step = Number(part.slice(2));
    return Number.isFinite(step) && step > 0 && value % step === 0;
  }
  return Number(part) === value;
}

function nextInterval(from: Date, every: string): Date {
  return new Date(from.getTime() + parseDuration(every));
}

function nextCron(from: Date, cron: string): Date {
  const cursor = new Date(from.getTime());
  cursor.setUTCSeconds(0, 0);
  for (let i = 0; i < 60 * 24 * 8; i++) {
    cursor.setUTCMinutes(cursor.getUTCMinutes() + 1);
    if (cronMatches(cron, cursor)) return new Date(cursor);
  }
  throw new Error(`no next cron match within 8 days: ${cron}`);
}

function computeNext(schedule: ScheduleSpec, from: Date, afterRun = false): Date | undefined {
  switch (schedule.kind) {
    case "once": {
      if (afterRun) return undefined;
      if (!schedule.at) throw new Error("once schedule requires at");
      return parseOnce(schedule.at, from);
    }
    case "interval": {
      if (!schedule.every) throw new Error("interval schedule requires every");
      return nextInterval(from, schedule.every);
    }
    case "cron": {
      if (!schedule.cron) throw new Error("cron schedule requires cron");
      return nextCron(from, schedule.cron);
    }
    default:
      throw new Error(`unknown schedule kind`);
  }
}

export class TaskScheduler {
  private readonly runner: TaskRunner;
  private readonly tickMs: number;
  private readonly now: () => Date;
  private readonly tasks = new Map<string, ScheduledTask>();
  private timer: ReturnType<typeof setInterval> | undefined;
  private running = false;
  private seq = 0;

  constructor(opts: TaskSchedulerOptions) {
    this.runner = opts.runner;
    this.tickMs = opts.tickMs ?? 1000;
    this.now = opts.now ?? (() => new Date());
  }

  start(): void {
    if (this.timer) return;
    this.timer = setInterval(() => {
      void this.tick();
    }, this.tickMs);
    if (typeof this.timer.unref === "function") this.timer.unref();
  }

  stop(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = undefined;
  }

  list(): ScheduledTask[] {
    return [...this.tasks.values()].sort((a, b) => a.createdAt.localeCompare(b.createdAt));
  }

  get(id: string): ScheduledTask | undefined {
    return this.tasks.get(id);
  }

  schedule(input: {
    name: string;
    prompt: string;
    schedule: ScheduleSpec;
    meta?: Record<string, unknown>;
  }): ScheduledTask {
    const now = this.now();
    const id = `task-${Date.now()}-${++this.seq}`;
    const next = computeNext(input.schedule, now, false);
    const task: ScheduledTask = {
      id,
      name: input.name,
      prompt: input.prompt,
      schedule: input.schedule,
      status: "pending",
      createdAt: now.toISOString(),
      nextRunAt: next?.toISOString(),
      meta: input.meta,
    };
    this.tasks.set(id, task);
    return task;
  }

  cancel(id: string): boolean {
    const task = this.tasks.get(id);
    if (!task) return false;
    if (task.status === "done" || task.status === "cancelled") return false;
    task.status = "cancelled";
    task.nextRunAt = undefined;
    return true;
  }

  async runNow(id: string): Promise<ScheduledTask> {
    const task = this.tasks.get(id);
    if (!task) throw new Error(`task not found: ${id}`);
    await this.execute(task);
    return task;
  }

  private async tick(): Promise<void> {
    if (this.running) return;
    const now = this.now();
    const ready = this.list().filter((t) => {
      if (t.status === "cancelled" || t.status === "failed" || t.status === "running") return false;
      if (!t.nextRunAt) return false;
      return new Date(t.nextRunAt).getTime() <= now.getTime();
    });
    for (const task of ready) {
      await this.execute(task);
    }
  }

  private async execute(task: ScheduledTask): Promise<void> {
    if (this.running) return;
    this.running = true;
    task.status = "running";
    task.lastError = undefined;
    try {
      await this.runner(task);
      const now = this.now();
      task.lastRunAt = now.toISOString();
      const next = computeNext(task.schedule, now, true);
      if (next) {
        task.status = "pending";
        task.nextRunAt = next.toISOString();
      } else {
        task.status = "done";
        task.nextRunAt = undefined;
      }
    } catch (err) {
      task.status = "failed";
      task.lastError = err instanceof Error ? err.message : String(err);
      task.nextRunAt = undefined;
    } finally {
      this.running = false;
    }
  }
}
