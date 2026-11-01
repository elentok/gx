import { appendFile, glob, mkdir, mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
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

const report = (fixture: Fixture, body: string) =>
  fixture.cliInput(body, "claude", "statusline");

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
    await expect(five.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "34",
    );
    await expect(week.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "81",
    );
    await expect(week.getByText("81%")).toHaveAttribute("data-tone", "waiting");
    await expect(five.getByText("34%")).toHaveAttribute("data-tone", "running");
    await expect(five.getByText(/resets in \d+[dhm]/)).toBeVisible();
    await expect(week.getByText(/resets in \d+[dhm]/)).toBeVisible();

    // Menu order: Settings, AFK, block, build line.
    const y = async (loc: ReturnType<Page["locator"]>) =>
      (await loc.boundingBox())!.y;
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
    await expect(week.getByText("96%")).toHaveAttribute(
      "data-tone",
      "deadlocked",
    );

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
      await row(page, "5h limit")
        .getByRole("button", { name: "5h limit reset time" })
        .tap();
      await expect(page.getByRole("tooltip")).toContainText(/\d{2}:\d{2}/);
    });
  });

  test.describe("narrow", () => {
    test.use({ viewport: { width: 320, height: 700 } });
    test("320px menu stays inside the viewport", async ({ page, start }) => {
      await start("limits", { shape: "fresh" });
      const scrollWidth = () =>
        page.evaluate(() => document.documentElement.scrollWidth);
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

const caveat =
  "Estimated cost of Claude runs started by gx. Not the bill. Codex runs and Claude sessions outside gx are not counted.";
const dollars = (n: number) => `$${n.toFixed(2)}`;
const sinceToday = () =>
  `since ${new Date().toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric" })}`;

test.describe("119. Spending block", () => {
  test("normal", async ({ page, start }) => {
    await start("spend", { shape: "normal" });
    const menu = await openMenu(page);
    await expect(menu.getByText("gx spend (est.)")).toBeVisible();

    const now = new Date();
    await expect(row(page, "Today").getByText(dollars(4.82))).toBeVisible();
    await expect(
      row(page, "Since Sunday").getByText(dollars(4.82 + 3 * now.getDay())),
    ).toBeVisible();
    await expect(
      row(page, "Since the 1st").getByText(
        dollars(4.82 + 3 * (now.getDate() - 1)),
      ),
    ).toBeVisible();
    await expect(menu.getByText(/^since /)).toHaveCount(0);

    const y = async (loc: ReturnType<Page["locator"]>) =>
      (await loc.boundingBox())!.y;
    const ys = [
      await y(menu.getByRole("menuitem", { name: "Settings" })),
      await y(menu.getByRole("menuitemcheckbox", { name: "AFK" })),
      await y(menu.getByText("gx spend (est.)")),
      await y(row(page, "Today")),
      await y(row(page, "Since Sunday")),
      await y(row(page, "Since the 1st")),
      await y(menu.getByTestId("server-build")),
    ];
    expect(ys).toEqual([...ys].sort((a, b) => a - b));

    await menu.getByRole("button", { name: "About gx spend" }).hover();
    await expect(page.getByRole("tooltip")).toHaveText(caveat);
    await page.mouse.move(0, 0);
    await expect(page.getByRole("tooltip")).toBeHidden();
  });

  test.describe("tap", () => {
    test.use({ hasTouch: true });
    test("tooltip opens on tap", async ({ page, start }) => {
      await start("spend", { shape: "normal" });
      const menu = await openMenu(page);
      await menu.getByRole("button", { name: "About gx spend" }).tap();
      await expect(page.getByRole("tooltip")).toHaveText(caveat);
    });
  });

  test("short", async ({ page, start }) => {
    await start("spend", { shape: "short" });
    await openMenu(page);
    const now = new Date();
    const note = sinceToday();
    await expect(row(page, "Today").getByText(/^since /)).toHaveCount(0);
    for (const [name, startsToday] of [
      ["Since Sunday", now.getDay() === 0],
      ["Since the 1st", now.getDate() === 1],
    ] as const) {
      const notes = row(page, name).getByText(/^since /);
      if (startsToday) await expect(notes).toHaveCount(0);
      else await expect(notes).toHaveText(note);
    }
  });

  test("empty", async ({ page, start }) => {
    await start("spend", { shape: "empty" });
    const menu = await openMenu(page);
    for (const name of ["Today", "Since Sunday", "Since the 1st"]) {
      await expect(row(page, name).getByText("$0.00")).toBeVisible();
    }
    await expect(menu.getByText(/^since /)).toHaveCount(0);
  });

  test.describe("narrow", () => {
    test.use({ viewport: { width: 320, height: 700 } });
    test("320px menu stays inside the viewport", async ({ page, start }) => {
      await start("spend", { shape: "normal" });
      const scrollWidth = () =>
        page.evaluate(() => document.documentElement.scrollWidth);
      const before = await scrollWidth();
      const menu = await openMenu(page);
      await expect(menu.getByText("gx spend (est.)")).toBeVisible();
      const box = (await menu.boundingBox())!;
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width).toBeLessThanOrEqual(320);
      expect(await scrollWidth()).toBeLessThanOrEqual(before);
    });
  });
});

const amount = async (page: Page, name: string) => {
  const text = (await row(page, name).textContent()) ?? "";
  return Number(/\$(\d+\.\d{2})/.exec(text)?.[1]);
};

test.describe("120. Menu refresh", () => {
  // The ledger event is throttled to once per 60 s; the menu's own 60 s tick is the backstop.
  test("amount rises while open; closed menu stops fetching", async ({
    page,
    start,
  }) => {
    test.setTimeout(180_000);
    const fixture = await start("spend", { shape: "normal" });
    await openMenu(page);
    await expect(row(page, "Today").getByText(/\$\d+\.\d{2}/)).toBeVisible();
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

    await expect(row(page, "Today").getByText(dollars(before + 3))).toBeVisible(
      { timeout: 120_000 },
    );

    await page.keyboard.press("Escape");
    await expect(page.getByRole("menu")).toBeHidden();
    let summaries = 0;
    page.on("request", (req) => {
      if (req.url().includes("/v1/usage/summary")) summaries++;
    });
    await report(fixture, payload(40, 50));
    await page.waitForTimeout(4000);
    expect(summaries).toBe(0);
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

const color = (loc: ReturnType<Page["locator"]>) =>
  loc.evaluate((el) => getComputedStyle(el).color);

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
  test("chips, forecasts, Codex block, live updates", async ({
    page,
    start,
  }) => {
    const fixture = await start("limit-burn", { shape: "menu" });
    const menu = await openMenu(page);

    const week = exactRow(page, "Weekly limit");
    const codexWeek = codexRow(page, "Weekly limit");
    await expect(menu.getByText("Claude limits")).toBeVisible();
    await expect(menu.getByText("Codex limits")).toBeVisible();
    expect(
      await (await menu.getByText("Codex limits").boundingBox())!.y,
    ).toBeGreaterThan(
      await (await menu.getByText("Claude limits").boundingBox())!.y,
    );

    // Chips: amber up on Claude, green down on Codex.
    const claudeChip = week.getByRole("button", { name: "Weekly limit burn" });
    const codexChip = codexWeek.getByRole("button", {
      name: "Codex Weekly limit burn",
    });
    await expect(claudeChip).toHaveText("▲ 7 vs 5 pts");
    await expect(codexChip).toContainText("▼");
    expect(await color(claudeChip)).toBe(await color(week.getByText("81%")));
    expect(await color(codexChip)).not.toBe(await color(claudeChip));

    await expect(week.getByText("today so far")).toBeVisible();
    await expect(codexWeek.getByText("today so far")).toBeVisible();
    await expect(
      week.getByText(/^Runs out ~.*, before the reset$/),
    ).toBeVisible();
    // The projection depends on the hour (days left to the reset), so the percentage is not pinned.
    await expect(codexWeek.getByText(/^~\d+% at reset$/)).toBeVisible();

    // 5-hour rows: no chip, no forecast.
    for (const name of ["5h limit", "Codex 5h limit"]) {
      const five = exactRow(page, name);
      await expect(five).toBeVisible();
      await expect(five.getByRole("button", { name: /burn$/ })).toHaveCount(0);
      await expect(
        five.getByText(/Runs out|at reset|Forecast needs|today so far/),
      ).toHaveCount(0);
    }

    // Tooltip by hover.
    await claudeChip.hover();
    await expect(page.getByRole("tooltip")).toContainText(
      "Burn today 7 pts so far, yesterday 5",
    );
    await page.mouse.move(0, 0);
    await expect(page.getByRole("tooltip")).toBeHidden();

    // A new statusline payload reaches the open menu.
    await report(fixture, payload(55, 96));
    await expect(exactRow(page, "5h limit").getByText("55%")).toBeVisible();
    await expect(week.getByText("96%")).toBeVisible();

    // A Codex scan that saves a new sample reaches the open menu.
    await appendCodexSample(fixture, 55);
    await expect(codexWeek.getByText("55%")).toBeVisible({ timeout: 60_000 });
  });

  test.describe("tap", () => {
    test.use({ hasTouch: true });
    test("chip tooltip opens on tap", async ({ page, start }) => {
      await start("limit-burn", { shape: "menu" });
      await openMenu(page);
      await exactRow(page, "Weekly limit")
        .getByRole("button", { name: "Weekly limit burn" })
        .tap();
      await expect(page.getByRole("tooltip")).toContainText(
        "Burn today 7 pts so far, yesterday 5",
      );
    });
  });

  test.describe("narrow", () => {
    test.use({ viewport: { width: 320, height: 700 } });
    test("320px menu stays inside the viewport", async ({ page, start }) => {
      await start("limit-burn", { shape: "menu" });
      await expect(page.getByRole("button", { name: "Menu" })).toBeVisible();
      const scrollWidth = () =>
        page.evaluate(() => document.documentElement.scrollWidth);
      const before = await scrollWidth();
      const menu = await openMenu(page);
      await expect(menu.getByText("Codex limits")).toBeVisible();
      const box = (await menu.boundingBox())!;
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width).toBeLessThanOrEqual(320);
      expect(await scrollWidth()).toBeLessThanOrEqual(before);
    });
  });
});

test.describe("181. Menu limit burn states", () => {
  test("few-days", async ({ page, start }) => {
    await start("limit-burn", { shape: "few-days" });
    const menu = await openMenu(page);
    const week = exactRow(page, "Weekly limit");
    await expect(week.getByText("Forecast needs 3 days of data")).toBeVisible();
    await expect(week.getByRole("button", { name: /burn$/ })).toHaveCount(0);
    await expect(menu.getByText("Codex limits")).toHaveCount(0);
  });

  test("stale", async ({ page, start }) => {
    await start("limit-burn", { shape: "stale" });
    await openMenu(page);
    for (const [name, pct] of [
      ["Weekly limit", "52%"],
      ["Codex Weekly limit", "40%"],
    ]) {
      const week = exactRow(page, name);
      await expect(week.getByText(/seen /)).toBeVisible();
      await expect(week.getByText(pct)).toBeVisible();
      await expect(week.getByText(/Runs out|at reset/)).toBeVisible();
    }
  });

  test("past-reset", async ({ page, start }) => {
    await start("limit-burn", { shape: "past-reset" });
    await openMenu(page);
    const week = exactRow(page, "Weekly limit");
    await expect(week.getByText(/reset passed/)).toBeVisible();
    await expect(
      week.getByText(/Runs out|at reset|Forecast needs/),
    ).toHaveCount(0);
    await expect(
      week.getByRole("button", { name: "Weekly limit burn" }),
    ).toBeVisible();
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
