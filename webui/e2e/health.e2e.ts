import type { Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";

const tray = (page: Page) => page.getByRole("button", { name: /^System health:/ });
const meter = (page: Page, letter: string) => tray(page).locator(`[data-meter="${letter}"]`);

const popover = (page: Page) => page.getByRole("dialog", nav("System health"));

// Moved to vitest: 79 (Tray states) in ui/tray/Tray.test.tsx, 80 (Held disk in the popover) in
// health/HealthPopover.test.tsx. 82 (Tray offline) runs as the last step here, on the same scenario.
test("81. Tray popover (and 82. Tray offline)", async ({ page, start }) => {
  const fixture = await start("health");
  await fixture.setDiskFree("450G/500G");
  await tray(page).click();
  const dialog = popover(page);

  await expect(dialog.getByText("gx server")).toBeVisible();
  for (const part of ["cpu", "memory", "disk"]) await expect(dialog.locator(`[data-part="${part}"]`)).toBeVisible();

  const cpuCauses = dialog.locator('[data-part="cpu-causes"]');
  const agent = cpuCauses.getByRole("link", { name: "demo:health-epic/01" });
  // The agent row exists once the fake agent runs and the table is tied to it.
  await expect(agent).toHaveAttribute("href", /\/t\/demo:health-epic\/01$/, { timeout: 30_000 });
  await expect(cpuCauses).toContainText("go test");
  await expect(cpuCauses).toContainText("other users");
  await expect(dialog.locator('[data-part="disk-entry"]').first()).toContainText("fixture disk");

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
  // The mock's busy-phone popover spans x 42 to 388 of 390.
  await expect
    .poll(() => dialog.evaluate((el) => [el.getBoundingClientRect().left, el.getBoundingClientRect().right].map(Math.round)))
    .toEqual([42, 388]);
  await page.keyboard.press("Escape");
  await expect(dialog).toBeHidden();

  // 82: the Tray keeps its last values and fades when the server goes away.
  await fixture.setHealth("cpu=75 mem=85");
  await expect(meter(page, "C")).toHaveAttribute("data-state", "high");
  await expect(meter(page, "M")).toHaveAttribute("data-state", "high");
  await tray(page).click();
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
