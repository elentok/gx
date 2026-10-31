import { expect, test, type Fixture } from "./fixture";
import type { Page } from "@playwright/test";

const nowSec = () => Math.floor(Date.now() / 1000);

// The server drops a report whose reset is earlier than the stored one, so these resets sit past the seeded ones.
const payload = (fiveHour: number, sevenDay: number) =>
  JSON.stringify({
    model: { display_name: "Claude" },
    rate_limits: {
      five_hour: { used_percentage: fiveHour, resets_at: nowSec() + 4 * 3600 },
      seven_day: { used_percentage: sevenDay, resets_at: nowSec() + 5 * 86400 },
    },
  });

const report = (fixture: Fixture, body: string) => fixture.cliInput(body, "claude", "statusline");

const openMenu = async (page: Page) => {
  await page.getByRole("button", { name: "Menu" }).click();
  const menu = page.getByRole("menu");
  await expect(menu).toBeVisible();
  return menu;
};

const row = (page: Page, name: string) => page.getByRole("group", { name });

test.describe("117. Limits block", () => {
  test("rows, live update, tooltip, narrow", async ({ page, start }) => {
    const fixture = await start("limits", { shape: "fresh" });
    const menu = await openMenu(page);

    await expect(menu.getByText("Claude limits")).toBeVisible();
    const five = row(page, "5h limit");
    const week = row(page, "Weekly limit");
    await expect(five.getByText("34%")).toBeVisible();
    await expect(week.getByText("81%")).toBeVisible();
    await expect(five.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "34");
    await expect(week.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "81");
    await expect(week.getByText("81%")).toHaveAttribute("data-tone", "waiting");
    await expect(five.getByText("34%")).toHaveAttribute("data-tone", "running");
    await expect(five.getByText(/resets in \d+[dhm]/)).toBeVisible();
    await expect(week.getByText(/resets in \d+[dhm]/)).toBeVisible();

    // Menu order: Settings, AFK, block, build line.
    const y = async (loc: ReturnType<Page["locator"]>) => (await loc.boundingBox())!.y;
    const ys = [
      await y(menu.getByRole("menuitem", { name: "Settings" })),
      await y(menu.getByRole("menuitemcheckbox", { name: "AFK" })),
      await y(menu.getByText("Claude limits")),
      await y(five),
      await y(week),
      await y(menu.getByTestId("server-build")),
    ];
    expect(ys).toEqual([...ys].sort((a, b) => a - b));

    // A new report reaches the open menu without a reload.
    await report(fixture, payload(55, 96));
    await expect(five.getByText("55%")).toBeVisible();
    await expect(week.getByText("96%")).toBeVisible();
    await expect(week.getByText("96%")).toHaveAttribute("data-tone", "deadlocked");

    // Tooltip by hover.
    const tip = week.getByRole("button", { name: "Weekly limit reset time" });
    await tip.hover();
    await expect(page.getByRole("tooltip")).toContainText(/\d{2}:\d{2}/);
    await page.mouse.move(0, 0);
    await expect(page.getByRole("tooltip")).toBeHidden();
  });

  test.describe("tap", () => {
    test.use({ hasTouch: true });
    test("tooltip opens on tap", async ({ page, start }) => {
      await start("limits", { shape: "fresh" });
      await openMenu(page);
      await row(page, "5h limit").getByRole("button", { name: "5h limit reset time" }).tap();
      await expect(page.getByRole("tooltip")).toContainText(/\d{2}:\d{2}/);
    });
  });

  test.describe("narrow", () => {
    test.use({ viewport: { width: 320, height: 700 } });
    test("320px menu stays inside the viewport", async ({ page, start }) => {
      await start("limits", { shape: "fresh" });
      const scrollWidth = () => page.evaluate(() => document.documentElement.scrollWidth);
      // The page already scrolls sideways by a pixel at 320 before the menu opens, so the menu is held to adding none.
      const before = await scrollWidth();
      const menu = await openMenu(page);
      await expect(menu.getByText("Claude limits")).toBeVisible();
      const box = (await menu.boundingBox())!;
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width).toBeLessThanOrEqual(320);
      expect(await scrollWidth()).toBeLessThanOrEqual(before);
    });
  });
});

test.describe("118. Limits states", () => {
  test("stale", async ({ page, start }) => {
    await start("limits", { shape: "stale" });
    await openMenu(page);
    await expect(row(page, "5h limit").getByText(/^seen /)).toBeVisible();
    await expect(row(page, "Weekly limit").getByText(/^seen /)).toBeVisible();
  });

  test("past-reset", async ({ page, start }) => {
    await start("limits", { shape: "past-reset" });
    await openMenu(page);
    const five = row(page, "5h limit");
    await expect(five.getByText("waiting", { exact: true })).toBeVisible();
    await expect(five.getByText(/reset passed/)).toBeVisible();
    await expect(five.getByRole("progressbar")).toHaveCount(0);
    await expect(five.getByText(/seen/)).toHaveCount(0);
    const week = row(page, "Weekly limit");
    await expect(week.getByRole("progressbar")).toBeVisible();
    await expect(week.getByText(/resets in \d+[dhm]/)).toBeVisible();
  });

  test("one-window", async ({ page, start }) => {
    await start("limits", { shape: "one-window" });
    const menu = await openMenu(page);
    await expect(menu.getByRole("group")).toHaveCount(1);
    await expect(row(page, "Weekly limit")).toBeVisible();
  });
});
