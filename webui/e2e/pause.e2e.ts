import type { Page } from "@playwright/test";
import { realpath } from "node:fs/promises";
import { card, expect, nav, test, toasts, type Fixture } from "./fixture";

interface Row {
  name: string;
  epic: string;
  id: string;
  /** What the Epics page card shows. It names the epic's first pause only. */
  onQueue: string;
  /** What the epic page row and the ticket page show. */
  reason: string;
  /** Acts on the ticket page. */
  recover?: (page: Page) => Promise<void>;
}

const recovery = (page: Page) => page.getByRole("form", { name: "Recovery actions" });

// A hash change keeps the page, since the fixture fails a test whose page reloaded.
const openTicket = (page: Page, id: string) =>
  page.evaluate((hash) => (location.hash = hash), `#/t/demo:paused-epic/${id}`);

const rows: Row[] = [
  {
    name: "Answer",
    epic: "paused-epic",
    id: "03",
    onQueue: "Seeded pause: self-reported",
    reason: "Waiting for your answer",
    recover: async (page) => {
      await page.getByRole("textbox", { name: "Your answer" }).fill("go ahead");
      await page.getByRole("button", nav("Send answer")).click();
    },
  },
  {
    name: "Choose base",
    epic: "paused-epic",
    id: "04",
    onQueue: "Seeded pause: self-reported",
    reason: "Ambiguous base",
    recover: async (page) => {
      await page.getByRole("button", nav("01")).click();
      await page.getByRole("button", nav("Choose base")).click();
    },
  },
  {
    name: "Retry",
    epic: "paused-epic",
    id: "07",
    onQueue: "Seeded pause: self-reported",
    reason: "Run failed",
    recover: async (page) => {
      await recovery(page).getByRole("button", nav("Retry")).click();
    },
  },
  {
    name: "Merge again",
    epic: "paused-epic",
    id: "05",
    onQueue: "Seeded pause: self-reported",
    reason: "Merge conflict",
    recover: async (page) => {
      await recovery(page).getByRole("button", nav("Merge again")).click();
    },
  },
  {
    name: "Start over",
    epic: "paused-epic",
    id: "08",
    onQueue: "Seeded pause: self-reported",
    reason: "Paused by you",
    recover: async (page) => {
      await recovery(page).getByRole("button", nav("Start over")).click();
      const dialog = page.getByRole("dialog", { name: "Start over" });
      await dialog.getByRole("textbox", { name: "Reason" }).fill("wrong base");
      await dialog.getByRole("button", nav("Start over")).click();
    },
  },
  {
    name: "Resume",
    epic: "paused-epic",
    id: "08",
    onQueue: "Seeded pause: self-reported",
    reason: "Paused by you",
    recover: async (page) => {
      await expect(recovery(page).getByRole("button")).toHaveText([/^Resume$/, /^Start over$/]);
      await recovery(page).getByRole("button", nav("Resume")).click();
    },
  },
  {
    name: "Recommended",
    epic: "paused-epic",
    id: "09",
    onQueue: "Seeded pause: self-reported",
    reason: "Run failed",
    recover: async (page) => {
      await expect(page.getByText("The run hit a transient error; retrying should work.")).toBeVisible();
      const buttons = recovery(page).getByRole("button");
      await expect(buttons.filter({ hasText: "Recommended" })).toHaveCount(1);
      await expect(recovery(page).getByRole("button", nav("Start over"))).not.toContainText("Recommended");
      await buttons.filter({ hasText: "Recommended" }).click();
    },
  },
  {
    name: "Approve",
    epic: "approve-epic",
    id: "01",
    onQueue: "wants to run: rg fixture",
    reason: "Needs approval",
    recover: async (page) => {
      await page.getByRole("button", nav("Allow once")).click();
    },
  },
];

const repairs = ["Run failed", "Merge conflict"];
const onEpicPage = (row: Row) => (repairs.includes(row.reason) ? `needs-repair · ${row.reason}` : "needs answer");

test.describe("3. Pause reasons and recovery", () => {
  for (const row of rows) {
    test(row.name, async ({ page, start }) => {
      const fixture = await start("paused");
      await fixture.cli("server", "queue", "add", "demo:paused-epic");

      await expect(card(page, row.epic).getByText(row.onQueue)).toBeVisible();

      await page.getByRole("link", { name: row.epic }).click();
      // The epic page card shows the state and, for repairs, the reason; the ticket page shows the reason.
      if (row.epic === "paused-epic") await expect(page.getByText(onEpicPage(row)).first()).toBeVisible();
      // The row's action link ("Answer →", "Review →") goes to the same page.
      await page.locator(`a[href$="/t/demo:${row.epic}/${row.id}"]`).filter({ hasNotText: "→" }).click();

      await expect(page.getByText(row.reason).first()).toBeVisible();
      await row.recover!(page);
      await expect(page.getByText(row.reason)).toHaveCount(0);
    });
  }

  test("Repair", async ({ page, start }) => {
    const fixture = await start("paused");
    await fixture.cli("server", "queue", "add", "demo:paused-epic");

    await expect(card(page, "paused-epic").getByText(/\+6 more/)).toBeVisible();

    await page.getByRole("link", { name: "paused-epic" }).click();
    await expect(page.getByText("needs-repair · Run failed").first()).toBeVisible();
    await expect(page.getByRole("button", { name: "Fix it with an agent" })).toHaveCount(3);
    // The tinted card keeps "Fix it with an agent" last in its second row.
    const repair = page
      .locator('[data-tone="deadlocked"]')
      .filter({ has: page.locator('a[href$="/t/demo:paused-epic/07"]') })
      .last();
    await expect(repair.getByText("needs-repair · Run failed")).toBeVisible();
    await expect(repair.getByRole("button").last()).toHaveText("Fix it with an agent");
    const link = (await repair.locator("a").first().boundingBox())!;
    const fix = (await repair.getByRole("button", { name: "Fix it with an agent" }).boundingBox())!;
    expect(fix.y).toBeGreaterThanOrEqual(link.y + link.height);

    await page.locator('a[href$="/t/demo:paused-epic/07"]').filter({ hasNotText: "→" }).click();
    await expect(page.getByText("Run failed").first()).toBeVisible();
  });

  const tooltips = [
    ["05", "Merge again", "Merge the ticket's commits onto its target again."],
    ["08", "Resume", "Lift the pause. The ticket waits its turn in the queue."],
    ["09", /^Retry\s*Recommended$/, "Start a fresh agent now. Keeps the branch and its commits."],
    ["08", "Start over", "Throw away the work and run again from the base. A backup branch is kept."],
  ] as const;

  for (const [id, button, title] of tooltips) {
    test(`Tooltip: ${button} on ${id}`, async ({ page, start }) => {
      await start("paused");
      await openTicket(page, id);
      const target = recovery(page).getByRole("button", typeof button === "string" ? nav(button) : { name: button });
      await expect(target).toHaveAttribute("title", title);
    });
  }

  test("Retry is disabled while the call runs", async ({ page, start }) => {
    await start("paused");
    let release!: () => void;
    const gate = new Promise<void>((resolve) => (release = resolve));
    await page.route("**/v1/tickets/retry", async (route) => {
      await gate;
      await route.continue();
    });
    await openTicket(page, "07");
    const retry = recovery(page).getByRole("button", nav("Retry"));
    await retry.click();
    await expect(retry).toBeDisabled();
    release();
    await expect(page.getByText("Run failed")).toHaveCount(0);
  });
});

const fixDialog = (page: Page) => page.getByRole("dialog", { name: "Fix it with an agent" });
const fixPrompt = "/gx-investigate demo:paused-epic/07 is paused for repair: Run failed";

const openFixDialog = async (page: Page) => {
  const row = page.locator('a[href$="/t/demo:paused-epic/07"]').locator("xpath=ancestor::*[.//button][1]");
  // The running fix ticket's timer re-renders the page every second, so the button is never "stable".
  // A fresh page can show the button before React attaches its handler, which drops the first click.
  await expect(async () => {
    await row.getByRole("button", { name: "Fix it with an agent" }).dispatchEvent("click");
    await expect(fixDialog(page)).toBeVisible({ timeout: 1_000 });
  }).toPass({ timeout: 10_000 });
};

const sessions = async (fixture: Fixture) =>
  JSON.parse(await fixture.cli("agents", "list", "--json")) as Array<{ dir: string; harness: string }>;

// The fixture keeps the store at <root>/data/gx/tickets and the project's trunk checkout at <root>/repo.
const trunkDir = async (fixture: Fixture) => {
  const path = (await fixture.cli("tickets", "show", "demo:paused-epic/07")).match(/^path: (.+)$/m)![1];
  return realpath(`${path.split("/data/gx/tickets")[0]}/repo`);
};

test.describe("55. Fix it with an agent", () => {
  test("copy, start, and refuse a second agent", async ({ page, context, start }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    const fixture = await start("paused");
    await fixture.cli("server", "queue", "add", "demo:paused-epic");
    const before = (await sessions(fixture)).length;

    await page.getByRole("link", { name: "paused-epic" }).click();
    await openFixDialog(page);
    await expect(fixDialog(page).getByText(fixPrompt)).toBeVisible();
    await fixDialog(page).getByRole("button", nav("Copy")).click();
    await expect(fixDialog(page).getByRole("button", nav("Copied"))).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(fixPrompt);

    await fixDialog(page).getByRole("button", nav("Start gx agent")).click();
    await expect(fixDialog(page).getByText("Agent started. View it in the gx TUI Agents tab")).toBeVisible();
    await expect(fixDialog(page).getByRole("button", nav("Start gx agent"))).toHaveCount(0);
    await expect.poll(async () => (await sessions(fixture)).length).toBe(before + 1);
    const started = (await sessions(fixture)).at(-1)!;
    expect(started.harness).toBe("claude");
    expect(await realpath(started.dir)).toBe(await trunkDir(fixture));

    // A reload would fail the test, so the second visit is a fresh page on the same server.
    const second = await context.newPage();
    await second.goto(page.url());
    await openFixDialog(second);
    await fixDialog(second).getByRole("button", nav("Start gx agent")).click();
    // The refusal is a toast, outside the dialog.
    await expect(toasts(second).filter({ hasText: "an agent is already fixing demo:paused-epic/07" })).toBeVisible();
    expect((await sessions(fixture)).length).toBe(before + 1);
  });
});

test.describe("56. Fix it with an agent, remote control", () => {
  test("names the Claude app", async ({ page, start }) => {
    const fixture = await start("paused", { config: { "agents.fix-agent-remote-control": "true" } });
    await fixture.cli("server", "queue", "add", "demo:paused-epic");

    await page.getByRole("link", { name: "paused-epic" }).click();
    await openFixDialog(page);
    await fixDialog(page).getByRole("button", nav("Start gx agent")).click();
    await expect(fixDialog(page).getByText("Agent started. View it in the gx TUI Agents tab or in the Claude app")).toBeVisible();
  });
});
