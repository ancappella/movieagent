import { expect, test } from "@playwright/test";

test.describe("chat smoke", () => {
  test("loads welcome and composer", async ({ page }) => {
    await page.goto("/");

    await expect(page.getByText("How can I help you today?")).toBeVisible();
    await expect(page.locator('[data-slot="aui_composer-shell"]')).toBeVisible();
    await expect(page.getByLabel("Message input")).toBeVisible();
    await expect(page.getByLabel("Send message")).toBeVisible();
  });
});
