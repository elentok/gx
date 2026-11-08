import { appendFile, glob, mkdir, mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { expect, nav, test, type Fixture } from "./fixture";
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

const report = (fixture: Fixture, body: string) =>
  fixture.cliInput(body, "claude", "statusline");

// The limits tray popover; the tray sits beside the app menu button, so it opens the sidebar the same way.
const openMenu = async (page: Page) => {
  const show = page.getByRole("button", nav("Show sidebar"));
  const tray = page.getByRole("button", nav("Usage limits"));
  await tray.or(show).first().waitFor();
  if (await show.isVisible()) await show.click();
  await tray.click();
  const popover = page.getByRole("dialog", nav("Usage limits"));
  await expect(popover).toBeVisible();
  return popover;
};

const bar = (page: Page, letter: string) => page.getByRole("button", nav("Usage limits")).locator(`[data-meter=${letter}]`);

test.describe("117b. Limits tray bars", () => {
  test("letters C and X; fill and tone follow usage", async ({ page, start }) => {
    const fixture = await start("limits", { shape: "fresh" });
    await expect(bar(page, "C")).toHaveText("C");
    await expect(bar(page, "X")).toHaveText("X");
    await expect(bar(page, "C")).toHaveAttribute("data-tone", "done");
    await expect(bar(page, "C")).toHaveAttribute("data-percent", "34");

    await report(fixture, payload(85, 90));
    await expect(bar(page, "C")).toHaveAttribute("data-tone", "waiting");
    await expect(bar(page, "C")).toHaveAttribute("data-percent", "85");
  });
});

const row = (page: Page, name: string) => page.getByRole("group", { name });

// Rows, tones, order, tooltips and the other render states live in webui/src/usage/*.test.tsx.
test.describe("117. Limits block", () => {
  test("a statusline report reaches the open menu", async ({ page, start }) => {
    const fixture = await start("limits", { shape: "fresh" });
    await openMenu(page);
    await expect(row(page, "5h limit").getByText("34%")).toBeVisible();

    await report(fixture, payload(55, 96));
    await expect(row(page, "5h limit").getByText("55%")).toBeVisible();
    await expect(row(page, "Weekly limit").getByText("96%")).toBeVisible();
  });
});

const dollars = (n: number) => `$${n.toFixed(2)}`;

const amount = async (page: Page, name: string) => {
  const text = (await row(page, name).textContent()) ?? "";
  return Number(/\$(\d+\.\d{2})/.exec(text)?.[1]);
};

test.describe("120. Menu refresh", () => {
  // The ledger event is throttled to once per 60 s; the tray's own 60 s tick is the backstop.
  test("amount rises while the popover is open", async ({
    page,
    start,
  }) => {
    // The tray's 60 s tick starts at page load, so the fake clock must exist before the page loads.
    await page.clock.install();
    const fixture = await start("spend", { shape: "normal" });
    const menu = await openMenu(page);
    await expect(menu.getByText("gx spend (est.)")).toBeVisible();
    await expect(row(page, "Today").getByText(dollars(4.82))).toBeVisible();
    const before = await amount(page, "Today");

    await mkdir(dirname(fixture.agentTranscript), { recursive: true });
    const line = {
      type: "assistant",
      timestamp: new Date().toISOString(),
      message: {
        model: "claude-sonnet-4-5",
        usage: { input_tokens: 1_000_000, output_tokens: 0 },
      },
    };
    await appendFile(fixture.agentTranscript, JSON.stringify(line) + "\n");

    // The open menu refetches every 60 seconds; the page clock runs that interval, and the
    // server may take a poll or two to see the line, so each try moves the clock again.
    await expect(async () => {
      await page.clock.fastForward(60_000);
      await expect(row(page, "Today").getByText(dollars(before + 3))).toBeVisible({ timeout: 2_000 });
    }).toPass({ timeout: 30_000 });
  });
});

test.describe("121. Hook report", () => {
  test("open menu flips to limits; output matches the no-server run", async ({
    page,
    start,
  }) => {
    const fixture = await start("spend", { shape: "normal" });
    const menu = await openMenu(page);
    await expect(menu.getByText("gx spend (est.)")).toBeVisible();

    const body = payload(34, 81);
    const withServer = await report(fixture, body);
    await expect(menu.getByText("Claude limits")).toBeVisible();
    await expect(row(page, "5h limit").getByText("34%")).toBeVisible();
    await expect(row(page, "Weekly limit").getByText("81%")).toBeVisible();

    const emptyState = await mkdtemp(join(tmpdir(), "gx-no-server-"));
    const withoutServer = await fixture.cliEnvInput(
      { XDG_STATE_HOME: emptyState },
      body,
      "claude",
      "statusline",
    );
    expect(withServer).toBe(withoutServer);
  });
});

// `row` matches names by substring, so "Weekly limit" also matches "Codex Weekly limit".
const exactRow = (page: Page, name: string) =>
  page.getByRole("group", { name, exact: true });
const codexRow = (page: Page, name: string) => exactRow(page, `Codex ${name}`);

const appendCodexSample = async (fixture: Fixture, usedPercent: number) => {
  let rollout = "";
  await expect
    .poll(async () => {
      for await (const f of glob("**/rollout-*.jsonl", {
        cwd: fixture.codexSessionsDir,
      }))
        rollout = join(fixture.codexSessionsDir, f);
      return rollout;
    })
    .not.toBe("");
  const resetsAt = nowSec() + 73 * 3600;
  const line = `{"timestamp":"${new Date().toISOString()}","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":${usedPercent},"window_minutes":10080,"resets_at":${resetsAt}}}}}\n`;
  await appendFile(rollout, line);
};

test.describe("180. Menu limit burn", () => {
  test("narrow menu fits; live updates", async ({ page, start }) => {
    const fixture = await start("limit-burn", { shape: "menu" });
    await page.setViewportSize({ width: 320, height: 700 });

    // The widest menu (Claude + Codex) must stay inside a 320 px viewport.
    const scrollWidth = () =>
      page.evaluate(() => document.documentElement.scrollWidth);
    // The page already scrolls sideways by a pixel at 320 before the menu opens, so the menu is held to adding none.
    const before = await scrollWidth();
    const menu = await openMenu(page);
    await expect(menu.getByText("Codex limits")).toBeVisible();
    const box = (await menu.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(320);
    expect(await scrollWidth()).toBeLessThanOrEqual(before);

    // A new statusline payload reaches the open menu.
    await report(fixture, payload(55, 96));
    await expect(exactRow(page, "5h limit").getByText("55%")).toBeVisible();
    await expect(exactRow(page, "Weekly limit").getByText("96%")).toBeVisible();

    // A Codex scan that saves a new sample reaches the open menu.
    await appendCodexSample(fixture, 55);
    await expect(
      codexRow(page, "Weekly limit").getByText("55%"),
    ).toBeVisible({ timeout: 60_000 });
  });
});
