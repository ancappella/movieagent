import { expect, test } from "@playwright/test";
import { mockChatFlow } from "./fixtures/chat-mock";

test.describe("chat send message (mocked SSE)", () => {
  test("shows one-line progress ticker then assistant text", async ({ page }) => {
    await mockChatFlow(page, {
      progress: ["开始本轮", "正在思考…", "调用 绘本插画", "本轮完成"],
      delta: "这是一张竖版柠檬茶海报方案。",
    });

    await page.goto("/");
    const input = page.getByLabel("Message input");
    await input.fill("帮我做一张中秋柠檬茶海报");
    await page.getByLabel("Send message").click();

    await expect(page.locator('[data-slot="aui_user-message-root"]')).toContainText(
      "帮我做一张中秋柠檬茶海报",
    );

    const thinking = page.locator('[data-slot="aui_assistant-message-thinking"]');
    await expect(thinking).toBeVisible();
    // Single-line ticker shows the latest progress step.
    await expect(thinking).toContainText("本轮完成");
    await expect(thinking).toHaveCSS("height", "20px");

    const assistant = page.locator('[data-slot="aui_assistant-message-root"]');
    await expect(assistant).toContainText("这是一张竖版柠檬茶海报方案。");
  });

  test("shows illustration skeleton while generating", async ({ page }) => {
    await mockChatFlow(page, {
      thinking: "准备出图",
      status: "正在生成图片",
    });

    await page.goto("/");
    await page.getByLabel("Message input").fill("画一只狐狸");
    await page.getByLabel("Send message").click();

    const placeholder = page.locator(
      '[data-slot="aui_assistant-message-image-placeholder"]',
    );
    await expect(placeholder).toBeVisible();
    await expect(placeholder).toHaveAttribute("data-kind", "pending");
    await expect(placeholder).toContainText("正在生成图片");
    await expect(page.locator('[data-slot="aui_assistant-message-image"]')).toHaveCount(
      0,
    );
  });

  test("two-turn flow: plan then illustrate in chat", async ({ page }) => {
    await mockChatFlow(page, {
      turns: [
        {
          taskId: "e2e-turn-1",
          thinking: "先给三个方向，等用户确认再出图",
          delta:
            "已准备三个方向的清爽竖版海报方案。选一个方向，或直接说口味和主色，我就出图。",
        },
        {
          taskId: "e2e-turn-2",
          progress: ["开始本轮", "调用 绘本插画", "绘本插画 完成"],
          status: "正在生成图片",
          imageUrl: "/media/generated/e2e-poster.png",
          imageTitle: "夏日冰饮海报",
          delta: "已按「水花炸裂」方向生成竖版海报。",
        },
      ],
    });

    await page.goto("/");
    const input = page.getByLabel("Message input");
    const send = page.getByLabel("Send message");

    // Turn 1: ask for a poster → plan only, no image yet.
    await input.fill("为夏日冰饮做一张清爽竖版营销海报");
    await send.click();

    const assistants = page.locator('[data-slot="aui_assistant-message-root"]');
    await expect(assistants).toHaveCount(1);
    await expect(assistants.first()).toContainText("三个方向");
    await expect(page.locator('[data-slot="aui_assistant-message-image"]')).toHaveCount(0);

    // Turn 2: confirm generation → image appears in chat.
    await input.fill("按方向一水花炸裂出图");
    await send.click();

    await expect(assistants).toHaveCount(2);
    await expect(page.locator('[data-slot="aui_user-message-root"]')).toHaveCount(2);

    const figure = page.locator('[data-slot="aui_assistant-message-image"]');
    const image = figure.locator("img");
    await expect(image).toBeVisible();
    await expect(image).toHaveAttribute("src", /e2e-poster\.png/);
    await expect(image).toHaveJSProperty("naturalWidth", 1);
    await expect(figure.locator("figcaption")).toContainText("夏日冰饮海报");
    await expect(page.locator('[data-slot="aui_assistant-message-image-placeholder"]')).toHaveCount(
      0,
    );

    await expect(assistants.nth(1)).toContainText("已按「水花炸裂」方向生成竖版海报");
  });

  test("surfaces chat error from SSE", async ({ page }) => {
    await mockChatFlow(page, {
      error: "模拟失败：配额不足",
    });

    await page.goto("/");
    await page.getByLabel("Message input").fill("触发错误");
    await page.getByLabel("Send message").click();

    await expect(page.getByText(/模拟失败：配额不足|配额不足/)).toBeVisible();
  });

  test("shows error placeholder when illustration fails", async ({ page }) => {
    await mockChatFlow(page, {
      status: "插画生成失败，请稍后重试或换个描述再试。",
    });

    await page.goto("/");
    await page.getByLabel("Message input").fill("出图失败场景");
    await page.getByLabel("Send message").click();

    const placeholder = page.locator(
      '[data-slot="aui_assistant-message-image-placeholder"]',
    );
    await expect(placeholder).toBeVisible();
    await expect(placeholder).toHaveAttribute("data-kind", "error");
    await expect(placeholder).toContainText("失败");
  });

  test("shows animation placeholder while generating", async ({ page }) => {
    await mockChatFlow(page, {
      status: "正在生成动画",
    });

    await page.goto("/");
    await page.getByLabel("Message input").fill("做成动画");
    await page.getByLabel("Send message").click();

    const placeholder = page.locator(
      '[data-slot="aui_assistant-message-image-placeholder"]',
    );
    await expect(placeholder).toBeVisible();
    await expect(placeholder).toHaveAttribute("data-kind", "pending");
    await expect(placeholder).toContainText("正在生成动画");
    await expect(page.locator('[data-slot="aui_assistant-message-video"]')).toHaveCount(
      0,
    );
  });

  test("renders async animation video from SSE (OSS URL)", async ({ page }) => {
    await mockChatFlow(page, {
      progress: ["开始本轮", "调用 插画动画", "插画动画 完成"],
      status: "正在生成动画",
      videoUrl: "https://cdn.example.com/movieagent/chat/e2e-clip.mp4",
      videoTitle: "动画 character_spin",
      delta: "已把插画做成原地旋转短片。",
    });

    await page.goto("/");
    await page.getByLabel("Message input").fill("把刚才的插画做成动画");
    await page.getByLabel("Send message").click();

    const figure = page.locator('[data-slot="aui_assistant-message-video"]');
    const video = figure.locator("video");
    await expect(video).toBeVisible();
    await expect(video).toHaveAttribute(
      "src",
      "https://cdn.example.com/movieagent/chat/e2e-clip.mp4",
    );
    await expect(figure.locator("figcaption")).toContainText("动画 character_spin");
    await expect(page.locator('[data-slot="aui_assistant-message-image-placeholder"]')).toHaveCount(
      0,
    );

    const assistant = page.locator('[data-slot="aui_assistant-message-root"]');
    await expect(assistant).toContainText("原地旋转短片");
  });
});
