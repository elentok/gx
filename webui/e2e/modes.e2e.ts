import type { Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";

const goto = (page: Page, hash: string) => page.evaluate((h) => (window.location.hash = h), hash);

const SUMMARY = "Reach the sea by building a canal, a lock and a long harbour wall before the autumn storms.";

// A card's wrapper (the element that carries `data-indent`) holding a live (not done, not cancelled) ticket.
const LIVE_CARDS = "[data-indent]:not([data-tone=done]):not([data-tone=cancelled])";

test.describe("61. Page modes at 760px", () => {
  test.use({ viewport: { width: 760, height: 900 } });

  test("Epic page gets the wide layout and a popover jump menu (and 62. Card columns)",async ({ page, start }) => {
    const fixture = await start("mixed");
    await fixture.cli("tickets", "section", "demo:mixed-epic", "Summary", SUMMARY);
    // The server reads the new section a moment after the CLI wrote it, so the page is opened until it shows.
    await expect(async () => {
      await goto(page, "#/epics");
      await goto(page, "#/e/demo:mixed-epic");
      await expect(page.getByText(SUMMARY, nav(SUMMARY))).toBeVisible({ timeout: 1000 });
    }).toPass();

    const ring = (await page.getByTestId("page-header-actions").locator("[data-kind]").boundingBox())!;
    expect(ring.width).toBe(56);
    const summary = (await page.getByText(SUMMARY, nav(SUMMARY)).boundingBox())!;
    expect(summary.y).toBeLessThan(ring.y + ring.height);

    const stats = (await page.getByTestId("stats-line").boundingBox())!;
    const queue = (await page.getByRole("button", nav("Queue")).boundingBox())!;
    // The stats line stacks in the header, above the divider; the button sits below it.
    expect(queue.y).toBeGreaterThanOrEqual(stats.y + stats.height);

    // The meta sits on the title's line, to the right of it.
    const link = page.getByRole("link", { name: "02 open ready" });
    const title = (await link.getByText("open", nav("open")).boundingBox())!;
    const meta = (await link.getByText("ready", nav("ready")).boundingBox())!;
    expect(meta.y).toBeLessThan(title.y + title.height);
    expect(meta.y + meta.height).toBeGreaterThan(title.y);
    expect(meta.x).toBeGreaterThanOrEqual(title.x + title.width);

    // The title is not a button; Cmd/Ctrl+K opens the jump menu as a popover, not a full-screen sheet.
    await expect(page.getByRole("heading", { level: 1 }).getByRole("button")).toHaveCount(0);
    await page.keyboard.press("ControlOrMeta+k");
    const menu = page.getByRole("dialog", nav("Jump to"));
    await expect(menu).toBeVisible();
    const box = (await menu.boundingBox())!;
    expect(box.width).toBeLessThan(760);
    expect(box.height).toBeLessThan(900);

    // 62. Card columns: same scenario, measured at desktop width.
    await page.keyboard.press("Escape");
    await expect(menu).toHaveCount(0);
    await page.setViewportSize({ width: 1280, height: 900 });
    const long = "A very long ticket title that is wider than its card on a desktop screen, so it must wrap onto further lines. ".repeat(3).trim();
    await fixture.cli("tickets", "add", "demo:mixed-epic", "--slug", "long-title", "--body", `# ${long}\n\n## What to build\n\nx`);
    await expect(page.getByText(long, nav(long))).toBeVisible();

    const cards = page.locator(LIVE_CARDS);
    await expect(cards.first()).toBeVisible();
    const rows = await cards.evaluateAll((els) =>
      els.map((el) => {
        const meta = el.querySelector("[data-meta]")!.getBoundingClientRect();
        let frameEl = el.querySelector("[data-meta]")!.parentElement!;
        while (getComputedStyle(frameEl).borderTopWidth === "0px") frameEl = frameEl.parentElement!;
        const frame = frameEl.getBoundingClientRect();
        const titleEl = el.querySelector("[data-title]")!;
        const title = titleEl.getBoundingClientRect();
        const cs = getComputedStyle(titleEl);
        return {
          text: titleEl.textContent,
          left: title.left,
          height: title.height,
          lineHeight: parseFloat(cs.lineHeight),
          textOverflow: cs.textOverflow,
          gap: frame.right - parseFloat(getComputedStyle(frameEl).borderRightWidth) - meta.right,
        };
      }),
    );
    expect(rows.length).toBeGreaterThan(2);

    // Every title starts in the same column.
    for (const r of rows) expect(r.left).toBe(rows[0].left);

    // The long title wraps onto more lines instead of being cut with an ellipsis.
    const wide = rows.find((r) => r.text === long)!;
    expect(wide.textOverflow).not.toBe("ellipsis");
    expect(wide.height).toBeGreaterThan(wide.lineHeight * 1.5);

    // Each meta ends 16px short of its card's right edge, inside the 1px border.
    for (const r of rows) expect(r.gap).toBeCloseTo(16, 0);
  });

  // 760px is a tablet now, so the phone sheet is measured at 390px.
  test("Design page gets the phone layout", async ({ page, start }) => {
    await start("design");
    await page.setViewportSize({ width: 390, height: 844 });
    await goto(page, "#/d/demo:design-epic");
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await page.keyboard.press("ControlOrMeta+k");
    const sheet = page.getByRole("dialog", nav("Jump to"));
    await expect(sheet).toBeVisible();
    // The phone/wide switch follows a media query, so the box can read as the inset popover until it settles.
    await expect.poll(() => sheet.boundingBox()).toMatchObject({ x: 0, y: 0, width: 390, height: 844 });
  });
});
