import { expect, test } from "@playwright/test";

/**
 * Optional live health check against Go backend.
 * Skipped unless E2E_LIVE=1 (requires movieagent on APP_PORT, default 8090).
 */
test.describe("live backend health", () => {
  test.skip(!process.env.E2E_LIVE, "set E2E_LIVE=1 to run against Go");

  test("GET /api/v1/health", async ({ request }) => {
    const origin = process.env.MOVIEAGENT_API_ORIGIN || "http://127.0.0.1:8090";
    const res = await request.get(`${origin}/api/v1/health`);
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.code).toBe(0);
    expect(body.data?.status).toBe("up");
  });
});
