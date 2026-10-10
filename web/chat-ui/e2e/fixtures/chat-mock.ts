import type { Page, Route } from "@playwright/test";

export type MockChatTurn = {
  taskId?: string;
  /** Single-line activity ticker steps (turn / tool / thinking). */
  progress?: string[];
  /** @deprecated use progress; kept for short tests */
  thinking?: string;
  status?: string;
  delta?: string;
  imageUrl?: string;
  imageTitle?: string;
  videoUrl?: string;
  videoTitle?: string;
  error?: string;
  /** Delay before SSE starts (ms). */
  delayMs?: number;
};

export type MockChatOptions = MockChatTurn & {
  /** Multi-turn script; each POST /api/v1/chat advances one turn. */
  turns?: MockChatTurn[];
};

function sseLine(payload: unknown): string {
  return `data: ${JSON.stringify(payload)}\n\n`;
}

function buildSSE(taskId: string, turn: MockChatTurn): string {
  const chunks: string[] = [
    sseLine({ type: "task", task_id: taskId, status: "running" }),
  ];
  const progress =
    turn.progress && turn.progress.length > 0
      ? turn.progress
      : turn.thinking
        ? ["开始本轮", "正在思考…", turn.thinking]
        : [];
  for (const line of progress) {
    chunks.push(sseLine({ type: "progress", message: line }));
  }
  if (turn.status) {
    chunks.push(sseLine({ type: "status", message: turn.status }));
  }
  if (turn.imageUrl) {
    chunks.push(
      sseLine({
        type: "image",
        url: turn.imageUrl,
        title: turn.imageTitle || "插画",
      }),
    );
  }
  if (turn.videoUrl) {
    chunks.push(
      sseLine({
        type: "video",
        url: turn.videoUrl,
        title: turn.videoTitle || "动画",
      }),
    );
  }
  if (turn.delta) {
    chunks.push(sseLine({ type: "delta", delta: turn.delta }));
  }
  if (turn.error) {
    chunks.push(sseLine({ type: "error", message: turn.error }));
  } else {
    chunks.push(sseLine({ type: "done", model: "e2e-mock" }));
  }
  return chunks.join("");
}

/** 1×1 PNG so <img src="/media/generated/..."> can load in the chat UI. */
const STUB_PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==";

/** Mock POST /api/v1/chat + SSE progress for deterministic E2E. */
export async function mockChatFlow(page: Page, opts: MockChatOptions = {}) {
  const script: MockChatTurn[] =
    opts.turns && opts.turns.length > 0
      ? opts.turns
      : [
          {
            taskId: opts.taskId,
            progress: opts.progress,
            thinking: opts.thinking,
            status: opts.status,
            delta: opts.delta,
            imageUrl: opts.imageUrl,
            imageTitle: opts.imageTitle,
            videoUrl: opts.videoUrl,
            videoTitle: opts.videoTitle,
            error: opts.error,
            delayMs: opts.delayMs,
          },
        ];

  let turnIndex = 0;

  await page.route("**/api/v1/chat", async (route: Route) => {
    if (route.request().method() !== "POST") {
      await route.fallback();
      return;
    }
    const turn = script[Math.min(turnIndex, script.length - 1)];
    const taskId = turn.taskId || `e2e-task-${turnIndex + 1}`;
    turnIndex += 1;

    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        code: 0,
        message: "ok",
        data: {
          task_id: taskId,
          status: "pending",
          events: `/api/v1/chat/tasks/${taskId}/events`,
        },
      }),
    });
  });

  await page.route("**/api/v1/chat/tasks/*/events", async (route: Route) => {
    const url = new URL(route.request().url());
    const parts = url.pathname.split("/");
    const taskId = parts[parts.indexOf("tasks") + 1] || "e2e-task-1";
    const turn =
      script.find((t, i) => (t.taskId || `e2e-task-${i + 1}`) === taskId) ||
      script[Math.max(0, turnIndex - 1)] ||
      script[0];

    if (turn.delayMs) {
      await new Promise((r) => setTimeout(r, turn.delayMs));
    }

    await route.fulfill({
      status: 200,
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        Connection: "keep-alive",
      },
      body: buildSSE(taskId, turn),
    });
  });

  await page.route("**/media/generated/**", async (route: Route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith(".mp4")) {
      // Minimal ftyp box so <video src> can bind without decode errors in CI.
      await route.fulfill({
        status: 200,
        contentType: "video/mp4",
        body: Buffer.from([
          0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6f, 0x6d,
          0x00, 0x00, 0x00, 0x01, 0x69, 0x73, 0x6f, 0x6d, 0x61, 0x76, 0x63, 0x31,
        ]),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "image/png",
      body: Buffer.from(STUB_PNG_BASE64, "base64"),
    });
  });
}
