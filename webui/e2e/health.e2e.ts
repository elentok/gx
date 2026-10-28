import type { Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";

const tray = (page: Page) => page.getByRole("button", { name: /^System health:/ });
const meter = (page: Page, letter: string) => tray(page).locator(`[data-meter="${letter}"]`);

// The bar is 15px tall and a level fills a fifth more each, from 20% at level 0.
const FILL_PX: Record<string, string> = { "0": "3px", "3": "12px", "4": "15px" };

const STEPS = [
  { values: "cpu=10 mem=10", cpu: "normal", mem: "normal", cpuLevel: "0", memLevel: "0" },
  { values: "cpu=75 mem=85", cpu: "high", mem: "high", cpuLevel: "3", memLevel: "4" },
  { values: "cpu=95 mem=95", cpu: "critical", mem: "critical", cpuLevel: "4", memLevel: "4" },
  // The fixture reader fails the whole reading when the CPU is unavailable, so memory goes unknown too.
  { values: "cpu=unavailable mem=10", cpu: "unknown", mem: "unknown", cpuLevel: "0", memLevel: "0" },
] as const;

test("79. Tray states", async ({ page, start }) => {
  const fixture = await start("health");
  // 10% used on the disk keeps the D meter at level 0.
  await fixture.setDiskFree("450G/500G");

  const pages = [
    async () => {
      await page.evaluate(() => (window.location.hash = "#/epics"));
      await expect(page.getByRole("link", { name: "health-epic", exact: true })).toBeVisible();
    },
    async () => {
      await page.getByRole("link", { name: "health-epic", exact: true }).click();
      await expect(page.locator('a[href$="/t/demo:health-epic/01"]')).toBeVisible();
    },
    async () => {
      await page.locator('a[href$="/t/demo:health-epic/01"]').click();
      await expect(tray(page)).toBeVisible();
    },
  ];

  for (const width of [1100, 390]) {
    await page.setViewportSize({ width, height: 800 });
    for (const open of pages) {
      await open();
      for (const step of STEPS) {
        await fixture.setHealth(step.values);
        await expect(meter(page, "C")).toHaveAttribute("data-state", step.cpu);
        await expect(meter(page, "M")).toHaveAttribute("data-state", step.mem);
        await expect(meter(page, "D")).toHaveAttribute("data-state", "normal");
        for (const [letter, state, level] of [
          ["C", step.cpu, step.cpuLevel],
          ["M", step.mem, step.memLevel],
        ]) {
          const fill = meter(page, letter).locator('[data-part="fill"]');
          if (state === "unknown") await expect(fill).toHaveCount(0);
          else await expect(fill).toHaveCSS("height", FILL_PX[level]);
        }
      }
      await fixture.setHealth("cpu=10 mem=10");
      await expect(meter(page, "C")).toHaveAttribute("data-state", "normal");
    }
  }
});

test("80. Held disk in the popover", async ({ page, start }) => {
  const fixture = await start("health");
  await fixture.setDiskFree("5G");
  await expect(meter(page, "D")).toHaveAttribute("data-state", "critical");

  await tray(page).click();
  const entry = page.getByRole("dialog", nav("System health")).locator('[data-part="disk-entry"]');
  await expect(entry).toHaveAttribute("data-state", "critical");
  await expect(entry).toContainText("Dispatch held");
  await expect(entry).toContainText("Clean now");
});
