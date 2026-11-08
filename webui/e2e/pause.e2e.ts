import type { Page } from "@playwright/test";
import { realpath } from "node:fs/promises";
import { card, expect, nav, test, toasts, type Fixture } from "./fixture";

const recovery = (page: Page) => page.getByRole("form", { name: "Recovery actions" });

// A hash change keeps the page, since the fixture fails a test whose page reloaded.
const openTicket = (page: Page, id: string, epic = "paused-epic") =>
  page.evaluate((hash) => (location.hash = hash), `#/t/demo:${epic}/${id}`);

// The wait loop pauses an agent that stays blocked for its 15 s dwell.
const dwell = { timeout: 30_000 };
// A recovery scenario's agent has to launch, hang (two launches, each with a launch-proof grace) or
// end its turn before the pause appears.
const gateWait = { timeout: 150_000 };
// The header pill; the Runs list repeats the label in a longer line.
const stuck = (page: Page) => page.getByText("Paused · Agent stuck", { exact: true });

// Flows 3 (every pause reason, Repair, the four tooltips) and 115 share one `paused` server.
// Resume and "Retry is disabled while the call runs" are vitest cases (ticket/ActionCard.test.tsx):
// Resume needs ticket 08, which Start over consumes.
test.describe("3. Pause reasons and recovery", () => {
  test("every pause reason recovers", async ({ page, start }) => {
    test.setTimeout(1_200_000);
    const fixture = await start("paused");
    await fixture.cli("server", "queue", "add", "demo:paused-epic");

    // The Approve card waits for a real worker session to ask for permission, which is slow under load.
    await expect(card(page, "approve-epic").getByText("wants to run: rg fixture")).toBeVisible({ timeout: 45_000 });
    const pausedCard = card(page, "paused-epic");
    await expect(pausedCard.getByText("Seeded pause: self-reported")).toBeVisible();

    // Repair: the card counts the other pauses and keeps "Fix it with an agent" below the link.
    await expect(pausedCard.getByText(/\+6 more/)).toBeVisible();
    await page.getByRole("link", { name: "paused-epic", exact: true }).click();
    await expect(page.getByText("needs-repair · Run failed").first()).toBeVisible();
    await expect(page.getByRole("button", { name: "Fix it with an agent" })).toHaveCount(3);
    const repair = page
      .locator('[data-tone="deadlocked"]')
      .filter({ has: page.locator('a[href$="/t/demo:paused-epic/07"]') })
      .last();
    await expect(repair.getByText("needs-repair · Run failed")).toBeVisible();
    await expect(repair.getByRole("button").last()).toHaveText("Fix it with an agent");
    const link = (await repair.locator("a").first().boundingBox())!;
    const fix = (await repair.getByRole("button", { name: "Fix it with an agent" }).boundingBox())!;
    expect(fix.y).toBeGreaterThanOrEqual(link.y + link.height);

    // Every step opens the ticket, acts, and waits for its pause to clear.
    const step = async (epic: string, id: string, reason: string, act: () => Promise<void>) => {
      await openTicket(page, id, epic);
      await expect(page.getByText(reason).first()).toBeVisible();
      await act();
      await expect(page.getByText(reason)).toHaveCount(0, { timeout: 30_000 });
    };
    // Tooltips are server-supplied title attributes, set before any action changes the tickets.
    const tooltips = [
      ["05", "Merge again", "Merge the ticket's commits onto its target again."],
      ["08", "Resume", "Lift the pause. The ticket waits its turn in the queue."],
      ["09", /^Retry\s*Recommended$/, "Start a fresh agent now. Keeps the branch and its commits."],
      ["08", "Start over", "Throw away the work and run again from the base. A backup branch is kept."],
    ] as const;
    for (const [id, button, title] of tooltips) {
      await openTicket(page, id);
      const target = recovery(page).getByRole("button", typeof button === "string" ? nav(button) : { name: button });
      await expect(target).toHaveAttribute("title", title);
    }

    await step("paused-epic", "03", "Waiting for your answer", async () => {
      await page.getByRole("textbox", { name: /^Answer to question/ }).fill("go ahead");
      await page.getByRole("button", nav("Send answer")).click();
    });
    await step("paused-epic", "04", "Ambiguous base", async () => {
      await page.getByRole("button", nav("01")).click();
      await page.getByRole("button", nav("Choose base")).click();
    });
    await step("paused-epic", "07", "Run failed", async () => {
      await recovery(page).getByRole("button", nav("Retry")).click();
    });
    await step("paused-epic", "05", "Merge conflict", async () => {
      await recovery(page).getByRole("button", nav("Merge again")).click();
    });
    await step("paused-epic", "08", "Paused by you", async () => {
      await recovery(page).getByRole("button", nav("Start over")).click();
      const dialog = page.getByRole("dialog", { name: "Start over" });
      await dialog.getByRole("textbox", { name: "Reason" }).fill("wrong base");
      await dialog.getByRole("button", nav("Start over")).click();
    });
    await step("paused-epic", "09", "Run failed", async () => {
      await expect(page.getByText("The run hit a transient error; retrying should work.")).toBeVisible();
      const buttons = recovery(page).getByRole("button");
      await expect(buttons.filter({ hasText: "Recommended" })).toHaveCount(1);
      await expect(recovery(page).getByRole("button", nav("Start over"))).not.toContainText("Recommended");
      await buttons.filter({ hasText: "Recommended" }).click();
    });

    // 115: the blocked worker names the answer button, and Approve clears the pause.
    await step("approve-epic", "01", "Needs approval", async () => {
      await expect(stuck(page)).toBeVisible(dwell);
      await expect(page.getByText(/press Approve or Deny on the card/)).toBeVisible();
      await page.getByRole("button", nav("Allow once")).click();
    });
    await expect(stuck(page)).toHaveCount(0);

    // Each recovery scenario is its own server, started in turn on the same page.
    const recover = async (scenario: string, epic: string, reason: string, buttons: string[], act?: () => Promise<void>) => {
      await start(scenario);
      await openTicket(page, "01", epic);
      await expect(page.getByText(reason).first()).toBeVisible(gateWait);
      // 20: while a repair pause's run is still finishing the card has no buttons; they return when it settles.
      await expect(recovery(page).getByRole("button")).toHaveText(
        buttons.map((b) => new RegExp(`^${b}(\\s*Recommended)?$`)),
        gateWait,
      );
      await act?.();
    };
    const retry = async () => {
      await recovery(page).getByRole("button", nav("Retry")).click();
    };
    // The Runs list keeps a past pause's text, so a lifted pause shows as the recovery card going away.
    const cleared = () => expect(recovery(page)).toHaveCount(0, { timeout: 30_000 });

    // 6: the relaunch hangs too; the pause names the cause and Retry ends it.
    await recover("hung-pane-twice", "hung-epic", "Could not start the agent", ["Retry", "Start over"], async () => {
      await expect(page.getByText(/pane stopped taking keys, twice/).first()).toBeVisible();
      await retry();
      await cleared();
    });

    // 70, 64, 28: a live Claude task. Keep waiting lifts the pause (the short hold cap pauses it again),
    // and Retry keeps the commits and ends it.
    const gate = "Background task still running";
    await recover("live-task-gate", "live-task-epic", gate, ["Keep waiting", "Stop task", "Retry", "Start over"], async () => {
      await expect(page.getByText("run the soak").first()).toBeVisible();
      await recovery(page).getByRole("button", nav("Keep waiting")).click();
      await cleared();
      await expect(recovery(page)).toBeVisible(gateWait);
      await retry();
      await cleared();
    });

    // 64: with a dirty worktree a live task still offers all four, and once the task's process exits
    // the card drops to Retry and Start over.
    await recover("live-task-gate-dirty", "live-task-dirty-epic", gate, ["Keep waiting", "Stop task", "Retry", "Start over"], async () => {
      await expect(recovery(page).getByRole("button")).toHaveText([/^Retry(\s*Recommended)?$/, /^Start over$/], gateWait);
    });

    // 65: a Codex task cannot be stopped; Start over states the discard and ends the pause.
    await recover("live-task-gate-codex", "live-task-codex-epic", gate, ["Keep waiting", "Retry", "Start over"], async () => {
      await recovery(page).getByRole("button", nav("Start over")).click();
      const dialog = page.getByRole("dialog", { name: "Start over" });
      await expect(dialog.getByText(/Start over discards the branch's commits/)).toBeVisible();
      await dialog.getByRole("textbox", { name: "Reason" }).fill("drop the task");
      await dialog.getByRole("button", nav("Start over")).click();
      await cleared();
    });

    // 41: every turn ends on a "no verdict" error until the re-sends run out; Retry prompts the live
    // agent, and the pause stays for the agent's own next move to end.
    await recover("classifier-outage", "classifier-outage-epic", "Classifier gave no verdict", ["Retry"], async () => {
      await retry();
      await expect(toasts(page).getByText(/refused|stale/i)).toHaveCount(0);
    });

    // 16/68: the holder is alive but never answers; the run fails with Retry and Start over.
    await recover("holder-unreachable", "holder-dead-epic", "Run failed", ["Retry", "Start over"]);
  });

  test("Unknown skill", async ({ page, start }) => {
    await start("unknown-skill");
    await page.evaluate((hash) => (location.hash = hash), "#/t/demo:unknown-epic/01");

    // The agent rejects the skill on both sessions, so the pause appears on its own.
    await expect(page.getByText("Could not start the agent").first()).toBeVisible({ timeout: 60_000 });
    await expect(page.getByText(/does not know the skill \/gx-implement/).first()).toBeVisible();
    await expect(page.getByText(/gx skills install/).first()).toBeVisible();

    await recovery(page).getByRole("button", nav("Retry")).click();
    await expect(page.getByText("Could not start the agent")).toHaveCount(0);
  });
});

test.describe("147. Resume on worker", () => {
  test("a model-mismatch pause resumes on the worker tier", async ({ page, start }) => {
    await start("model-mismatch");
    await page.evaluate((hash) => (location.hash = hash), "#/t/demo:mismatch-epic/01");

    await expect(page.getByText("Wrong model").first()).toBeVisible();
    await expect(recovery(page).getByText("Ran on Claude Opus 4, expected Claude Sonnet 4.")).toBeVisible();
    await expect(recovery(page).getByRole("button")).toHaveText([/^Resume on worker$/]);

    await recovery(page).getByRole("button", nav("Resume on worker")).click();
    await expect(page.getByText("Wrong model")).toHaveCount(0);
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

// The Fix-it agent is a session of its own; the queued epic's Queue agents are not.
const sessions = async (fixture: Fixture) =>
  (JSON.parse(await fixture.cli("agents", "list", "--json")) as Array<{ dir: string; harness: string; queue?: boolean }>).filter(
    (s) => !s.queue,
  );

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

    await page.getByRole("link", { name: "paused-epic", exact: true }).click();
    await openFixDialog(page);
    await expect(fixDialog(page).getByText(fixPrompt)).toBeVisible();
    await fixDialog(page).getByRole("button", nav("Copy")).click();
    await expect(fixDialog(page).getByRole("button", nav("Copied"))).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(fixPrompt);

    const epicUrl = page.url();
    await fixDialog(page).getByRole("button", nav("Start gx agent")).click();
    await expect(page).toHaveURL(/#\/agents\//);
    await expect(fixDialog(page)).toHaveCount(0);
    await expect.poll(async () => (await sessions(fixture)).length).toBe(before + 1);
    const started = (await sessions(fixture)).at(-1)!;
    expect(started.harness).toBe("claude");
    expect(await realpath(started.dir)).toBe(await trunkDir(fixture));

    // A reload would fail the test, so the second visit is a fresh page on the same server.
    const second = await context.newPage();
    await second.goto(epicUrl);
    await openFixDialog(second);
    await fixDialog(second).getByRole("button", nav("Start gx agent")).click();
    // The refusal stays in the dialog.
    await expect(fixDialog(second).getByRole("alert")).toContainText("an agent is already fixing demo:paused-epic/07");
    expect((await sessions(fixture)).length).toBe(before + 1);
  });
});

test.describe("116. Retry on a question menu", () => {
  test("Retry lifts the pause", async ({ page, start }) => {
    await start("blocked-menu");

    await page.getByRole("link", { name: "menu-epic", exact: true }).click();
    await expect(page.getByText("needs answer").first()).toBeVisible(dwell);
    await page.locator('a[href$="/t/demo:menu-epic/01"]').filter({ hasNotText: "→" }).click();

    await expect(stuck(page)).toBeVisible();
    await expect(page.getByRole("button", nav("Allow once"))).toHaveCount(0);
    await recovery(page).getByRole("button", nav("Retry")).click();
    await expect(stuck(page)).toHaveCount(0);
    // The agent left the menu and the ticket went on.
    await expect(page.getByText(/^(Paused|needs-answer)/)).toHaveCount(0);
  });
});

test.describe("56. Fix it with an agent, remote control", () => {
  test("opens the new agent's page", async ({ page, start }) => {
    const fixture = await start("paused", { config: { "agents.fix-agent-remote-control": "true" } });
    await fixture.cli("server", "queue", "add", "demo:paused-epic");

    await page.getByRole("link", { name: "paused-epic", exact: true }).click();
    await openFixDialog(page);
    await fixDialog(page).getByRole("button", nav("Start gx agent")).click();
    await expect(page).toHaveURL(/#\/agents\//);
    await expect(fixDialog(page)).toHaveCount(0);
  });
});
