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

const popover = (page: Page) => page.getByRole("dialog", nav("System health"));

test("74. Tray popover", async ({ page, start }) => {
  const fixture = await start("health");
  await fixture.setDiskFree("450G/500G");
  await tray(page).click();
  const dialog = popover(page);

  await expect(dialog.getByText("gx server")).toBeVisible();
  for (const part of ["cpu", "memory", "disk"]) await expect(dialog.locator(`[data-part="${part}"]`)).toBeVisible();
  await expect(dialog.locator('[data-part="cpu"]')).toContainText(/high at \d+%, critical at \d+%/);
  await expect(dialog.locator('[data-part="memory"]')).toContainText(/high at \d+%, critical at \d+%/);

  const cpuCauses = dialog.locator('[data-part="cpu-causes"]');
  const agent = cpuCauses.getByRole("link", { name: "demo:health-epic/01" });
  // The agent row exists once the fake agent runs and the table is tied to it.
  await expect(agent).toHaveAttribute("href", /\/t\/demo:health-epic\/01$/, { timeout: 30_000 });
  await expect(cpuCauses).toContainText("go test");
  await expect(cpuCauses).toContainText("other users");
  await expect(dialog.locator('[data-part="disk-entry"]').first()).toContainText("demo");

  const footer = dialog.locator('[data-part="footer"]');
  const seconds = async () => Number(/Updated (\d+)s ago/.exec((await footer.textContent()) ?? "")?.[1] ?? NaN);
  await expect(footer).toHaveText(/^Updated \d+s ago$/);
  const before = await seconds();
  await expect.poll(seconds, { timeout: 4000 }).toBeGreaterThan(before);

  await fixture.setHealth("cpu=60 mem=10");
  await expect(dialog.locator('[data-part="cpu"]')).toContainText("60% busy", { timeout: 5000 });

  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();

  await page.setViewportSize({ width: 390, height: 800 });
  await tray(page).click();
  await expect(dialog).toBeVisible();
  // One evaluate measures everything at the same instant, so the open animation's scale cancels out.
  await expect
    .poll(() =>
      dialog.evaluate((el) => {
        const outer = el.getBoundingClientRect();
        const rows = [...el.querySelectorAll('[data-part="cpu-causes"] [data-part="cause-row"]')];
        return outer.left >= 0 && outer.right <= window.innerWidth && rows.length > 0 && rows.every((r) => r.getBoundingClientRect().right <= outer.right + 0.5);
      }),
    )
    .toBe(true);
  await expect(cpuCauses.locator('[data-part="cause-label"]').first()).toHaveCSS("text-overflow", "ellipsis");
});

test("75. Tray offline", async ({ page, start }) => {
  const fixture = await start("health");
  await fixture.setHealth("cpu=75 mem=85");
  await expect(meter(page, "C")).toHaveAttribute("data-state", "high");
  await tray(page).click();
  const dialog = popover(page);
  await expect(dialog.locator('[data-part="cpu"]')).toContainText("75% busy");
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();

  await fixture.stopServer();
  await expect(tray(page)).toHaveAttribute("data-stale", "");
  await expect(tray(page)).toHaveCSS("opacity", "0.45");
  await expect(meter(page, "C")).toHaveAttribute("data-state", "high");
  await expect(meter(page, "M")).toHaveAttribute("data-state", "high");

  await tray(page).click();
  await expect(dialog.locator('[data-part="footer"]')).toHaveText(/^Offline\. Values from \d+[sm] ago\.$/);
  await expect(dialog.locator('[data-part="cpu"]')).toContainText("75% busy");
});
