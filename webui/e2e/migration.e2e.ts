import type { Page } from "@playwright/test";
import { expect, sessionRows, test, type Fixture } from "./fixture";
import { dialog, openDialog, passing, restartButton } from "./restart-helpers";

// Flows for the migration the server runs when it starts (no flag, no command code). They run on the
// migrate-* scenarios: the real `gx server` runs on the store, and `fixture.serverCtl` kills and starts
// a bare `gx server` process the way launchd does (see testutil/gxfixture/migrate.go).

const LIST = "applied-migrations.txt";
const ticket = "demo/demo-epic/issues/01-broken-shape.md";

/** Waits until the handshake no longer says migrating. */
const migrated = (fixture: Fixture) =>
  expect
    .poll(async () => JSON.parse(await fixture.cli("server", "status", "--json")).migrating, { timeout: 30_000 })
    .toBeFalsy();

/** The names in a project's list file, one per line. */
const names = async (fixture: Fixture, project: string) =>
  (await fixture.readStoreFile(`${project}/${LIST}`)).split("\n").filter(Boolean);

/** The server log (server.log, which the bare process appends to across restarts). */
const serverLog = (fixture: Fixture) => fixture.cli("server", "logs");

/** How many migration notes (containing `text`) the server log holds; a step that ran logs its notes. */
const noteCount = async (fixture: Fixture, text = "") =>
  (await serverLog(fixture)).split("\n").filter((l) => l.includes("migration note") && l.includes(text)).length;

/** Restarts the bare server and waits for its migration to end. */
const restartBare = async (fixture: Fixture) => {
  await fixture.serverCtl("kill");
  await fixture.serverCtl("start");
  await expect.poll(() => fixture.cli("server", "status", "--json").then((s) => JSON.parse(s).running).catch(() => false), { timeout: 30_000 }).toBe(true);
  await migrated(fixture);
};

/** The startup steps the server log holds, in order. */
const startupSteps = async (fixture: Fixture) =>
  (await serverLog(fixture))
    .split("\n")
    .map((l) => /startup step.*step=(\S+)/.exec(l)?.[1])
    .filter(Boolean);

const ready = "demo:ready-epic/01";
const stuckEpic = "demo:stuck-epic";
const mergeEpic = "demo:merge-epic";
const mergeLast = "demo:merge-epic/02";
const manual = "manual-agent";
const agentLines = (fixture: Fixture, slug: string) => sessionRows(fixture, slug);
const oldTicket = "demo/old-epic/issues/01-broken-shape.md";

test("169. Migrating state holds scheduling", async ({ start, page }) => {
  const fixture = await start("migrate-held");

  await test.step("the handshake and snapshot say migrating, and the page shows one plain line", async () => {
    expect(JSON.parse(await fixture.cli("server", "status", "--json")).migrating).toBe(true);
    await expect(page.getByText("Migrating tickets", { exact: true })).toBeVisible();
  });

  await test.step("the server says migrating, and `gx server start` returns fast with exit 0", async () => {
    expect(await fixture.cli("server", "status")).toMatch(/migrating/i);
    const began = Date.now();
    await fixture.cli("server", "start");
    expect(Date.now() - began).toBeLessThan(10_000);
  });

  await test.step("a manual agent is not blocked by the hold", async () => {
    await fixture.cli("agents", "create", "--project", "demo", "--name", manual, "--prompt", "hello");
    await expect.poll(() => agentLines(fixture, manual), { timeout: 30_000 }).not.toHaveLength(0);
  });

  await test.step("nothing is claimed, no recovery or merge starts, and no start-up step ran", async () => {
    await page.waitForTimeout(2000);
    expect(await fixture.cli("tickets", "show", ready)).not.toContain("claimed");
    expect(await fixture.cli("tickets", "show", mergeLast)).not.toContain("claimed");
    expect(await fixture.cli("tickets", "show", stuckEpic)).not.toMatch(/^02 /m);
    expect(await startupSteps(fixture)).toEqual([]);
    expect(await fixture.readStoreFile(oldTicket)).not.toContain("status: paused");
  });

  await test.step("when the hold ends the line goes, each step runs once and the ticket is claimed", async () => {
    await fixture.releaseMigration();
    await expect(page.getByText("Migrating tickets", { exact: true })).toBeHidden({ timeout: 30_000 });
    await migrated(fixture);
    await expect.poll(() => startupSteps(fixture), { timeout: 30_000 }).toEqual(["first-rescan", "recover-merges", "reclaim-runs"]);
    expect(await fixture.readStoreFile(oldTicket)).toContain("status: paused");
    await expect.poll(() => fixture.cli("tickets", "show", ready), { timeout: 30_000 }).toContain("claimed");
  });

  await test.step("the recovery starts, the last ticket of merge-epic is claimed and the manual agent still runs", async () => {
    await expect.poll(() => fixture.cli("tickets", "show", stuckEpic), { timeout: 30_000 }).toMatch(/^02 /m);
    await expect.poll(() => fixture.cli("tickets", "show", mergeLast), { timeout: 30_000 }).toContain("claimed");
    expect(await agentLines(fixture, manual)).not.toHaveLength(0);
  });

  await test.step("the merge starts when the held worker finishes, and the manual agent is still running", async () => {
    await fixture.release();
    await expect.poll(() => fixture.cli("tickets", "show", mergeEpic), { timeout: 30_000 }).toContain("merged");
    expect(await agentLines(fixture, manual)).not.toHaveLength(0);
  });
});

test("170. A failed project is rescanned after the migration", async ({ start }) => {
  const fixture = await start("migrate-broken");
  await migrated(fixture);
  await expect.poll(() => startupSteps(fixture), { timeout: 30_000 }).toEqual(["first-rescan", "recover-merges", "reclaim-runs"]);
  expect(await fixture.cli("tickets", "show", "broken:broken-epic/01")).toContain("first");
});

/** Whether a started CLI verb has ended. */
const ended = async (run: { done: Promise<unknown> }) => (await Promise.race([run.done.then(() => true), new Promise((r) => setTimeout(() => r(false), 100))])) as boolean;

test("171. Writes wait for the migration", async ({ start }) => {
  const fixture = await start("migrate-held");

  await test.step("a write to another project returns at once", async () => {
    await fixture.cli("tickets", "epic", "add", "scratch:other-epic", "--kind", "loop");
    await fixture.cli("tickets", "epic", "set", "scratch:other-epic", "--status", "open");
  });

  await test.step("a read during the hold returns a whole old or whole new ticket", async () => {
    const shown = await fixture.cli("tickets", "show", "demo:old-epic/01");
    expect(shown).toContain("old-epic");
  });

  const writes = {
    answer: ["tickets", "answer", "demo:old-epic/01", "--body", "yes"],
    log: ["tickets", "ask", "demo:ready-epic/01", "--body", "## Question\nq\n\n## Handoff\nh\n"],
    "epic setting": ["tickets", "epic", "set", "demo:ready-epic", "--destination-unconfirmed", "true"],
    move: ["tickets", "move", "demo:ready-epic/01", "old-epic"],
    "epic creation": ["tickets", "epic", "add", "demo:new-epic", "--kind", "loop"],
  };
  const serverSide = new Set(["answer", "log"]);
  const runs = Object.entries(writes).map(([name, args]) => ({ name, run: fixture.cliStart(...args) }));

  await test.step("each write to demo waits and says so after one second", async () => {
    await new Promise((r) => setTimeout(r, 2500));
    for (const { name, run } of runs) {
      expect(await ended(run), `${name} ended during the hold`).toBe(false);
      // answer and ask go through the server, which waits without a CLI notice.
      if (!serverSide.has(name)) expect(run.stderr(), name).toContain("waiting for the migration of demo");
    }
  });

  await test.step("when the hold ends the writes land on the new shape", async () => {
    await fixture.releaseMigration();
    await Promise.all(runs.map(({ run }) => run.done));
    await migrated(fixture);
    expect(await fixture.readStoreFile(oldTicket)).toContain("status: paused");
    expect(await fixture.cli("tickets", "show", "demo:new-epic")).toContain("new-epic");
  });
});

const strip = (page: Page) => page.getByRole("status").filter({ hasText: "Migration blocked" });

test("172. A failed step is not recorded, skip notes stay in the log", async ({ start, page }) => {
  const fixture = await start("migrate-pending");
  await migrated(fixture);
  const first = await names(fixture, "demo");

  await test.step("a failing step is left out of the list, the steps before it are in", async () => {
    await fixture.setInject([{ name: "e2e-before" }, { name: "e2e-fails", fail: "e2e step failed" }]);
    await restartBare(fixture);
    const listed = await names(fixture, "demo");
    expect(listed).toContain("e2e-before");
    expect(listed).not.toContain("e2e-fails");
    expect(listed).toEqual([...first, "e2e-before"]);
    await expect(strip(page).filter({ hasText: "demo:" })).toContainText("e2e step failed");
  });

  await test.step("a step with skip notes puts them in the log and shows nothing on the strip", async () => {
    await fixture.setInject([{ name: "e2e-noted", notes: ["e2e skip note"] }]);
    await restartBare(fixture);
    expect(await names(fixture, "demo")).toContain("e2e-noted");
    expect(await serverLog(fixture)).toContain("e2e skip note");
    await expect(strip(page)).toHaveCount(0, { timeout: 15_000 });
  });
});

const brokenTicket = "broken/broken-epic/issues/90-bad.md";
const goodTicket = "---\nid: \"90\"\nstatus: open\ntype: implement\n---\n# Bad\n";
const brokenStrip = (page: Page) => strip(page).filter({ hasText: "broken:" });
const pidOf = (fixture: Fixture) => fixture.serverCtl("pid");

test("173. Failed start and Migration block", async ({ start, page }) => {
  const fixture = await start("migrate-broken");
  await migrated(fixture);

  await test.step("the server runs and the other project works", async () => {
    expect(JSON.parse(await fixture.cli("server", "status", "--json")).running).toBe(true);
    await expect.poll(() => fixture.cli("tickets", "show", "demo:demo-epic/01"), { timeout: 30_000 }).toContain("claimed");
  });

  await test.step("the broken project's ready ticket is not claimed", async () => {
    expect(await fixture.cli("tickets", "show", "broken:broken-epic/01")).not.toContain("claimed");
  });

  await test.step("the strip says Migration blocked with the error, the command and both buttons", async () => {
    await expect(brokenStrip(page)).toBeVisible();
    await expect(brokenStrip(page)).toContainText("broken: Migration blocked");
    await expect(brokenStrip(page).locator("code")).toContainText("gx tickets migrate");
    await expect(brokenStrip(page).locator("p").nth(1)).not.toBeEmpty();
    await expect(brokenStrip(page).getByRole("button", { name: "Retry migration" })).toBeVisible();
    await expect(brokenStrip(page).getByRole("button", { name: "Fix agent" })).toBeVisible();
    await expect(page.getByRole("status").getByText("Paused")).toHaveCount(0);
    await expect(page.getByRole("status").getByText("Missing trunk")).toHaveCount(0);
  });
});

/** A migrate-broken fixture whose strip is showing. */
const startBlocked = async (start: (scenario: string) => Promise<Fixture>, page: Page) => {
  const fixture = await start("migrate-broken");
  await migrated(fixture);
  await expect(brokenStrip(page)).toBeVisible();
  return fixture;
};

/** Ends a block the way `end` says, with no restart. */
const endsBlock = async (name: string, page: Page, start: (scenario: string) => Promise<Fixture>, end: (fixture: Fixture) => Promise<void>) => {
  const fixture = await startBlocked(start, page);
  const pid = await pidOf(fixture);
  await fixture.writeStoreFile(brokenTicket, goodTicket);
  await end(fixture);
  await expect(brokenStrip(page), name).toHaveCount(0, { timeout: 30_000 });
  expect(await pidOf(fixture), `${name}: pid`).toBe(pid);
  await expect.poll(() => fixture.cli("tickets", "show", "broken:broken-epic/01"), { timeout: 30_000 }).toContain("claimed");
  return fixture;
};

test("174. Retry migration button ends the block", async ({ start, page }) => {
  const fixture = await endsBlock("retry", page, start, async () => {
    await brokenStrip(page).getByRole("button", { name: "Retry migration" }).click();
  });
  await test.step("the block comes back on the next failing start", async () => {
    // Applied steps are not run again: the new failure needs a new step.
    await fixture.setInject([{ name: "e2e-fails-again", fail: "e2e step failed again" }]);
    await restartBare(fixture);
    await expect(brokenStrip(page)).toContainText("e2e step failed again");
  });
});

test("175. The retry command by hand ends the block", async ({ start, page }) => {
  await endsBlock("by hand", page, start, async (fixture) => {
    const dir = (await brokenStrip(page).locator("code").textContent())!.match(/gx tickets migrate '(.+)'/)![1];
    await fixture.cli("tickets", "migrate", dir);
  });
});

test("176. The Fix agent route ends the block", async ({ start, page }) => {
  await endsBlock("fix agent", page, start, async (fixture) => {
    const command = (await brokenStrip(page).locator("code").textContent())!;
    await brokenStrip(page).getByRole("button", { name: "Fix agent" }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Start gx agent" }).click();
    await expect(page.getByText("Agent started.")).toBeVisible();
    // The fake agent runs no command: run the one the session is told to run.
    await fixture.cli("tickets", "migrate", command.match(/gx tickets migrate '(.+)'/)![1]);
  });
});

test("177. A stale failure record is cleared by the start-up run", async ({ start, page }) => {
  const fixture = await startBlocked(start, page);
  await fixture.writeStoreFile(brokenTicket, goodTicket);
  await restartBare(fixture);
  await expect(brokenStrip(page)).toHaveCount(0, { timeout: 30_000 });
});

test("178. Restart dialog with and without pending steps", async ({ start, page }) => {
  const fixture = await start("restart-projects");
  const alert = () => dialog(page).getByRole("alert");

  await test.step("a project with a pending step reports its dry-run failure", async () => {
    await openDialog(page);
    await expect(alert()).toContainText("broken: fixture migration failed");
    await expect(restartButton(page)).toHaveText("Restart anyway");
  });

  await test.step("with nothing pending the check is ok at once and the button is plain", async () => {
    await page.keyboard.press("Escape");
    await expect(dialog(page)).toHaveCount(0);
    await fixture.setRestartControl("check.json", passing("fixture-next"));
    const began = Date.now();
    await openDialog(page);
    await expect(restartButton(page)).toHaveText("Restart");
    await expect(restartButton(page)).toBeEnabled();
    expect(Date.now() - began).toBeLessThan(10_000);
    await expect(dialog(page).getByRole("alert")).toHaveCount(0);
    await expect(dialog(page)).not.toContainText("A migration runs while the agents work.");
  });
});

test("161. Server migrates on start",async ({ start }) => {
  const fixture = await start("migrate-pending");
  await migrated(fixture);

  await test.step("the old-shape ticket is in the new shape", async () => {
    expect(await fixture.readStoreFile(ticket)).toContain("status: paused");
    expect(await fixture.cli("tickets", "show", "demo:demo-epic/01")).toContain("paused");
  });

  const listed = await names(fixture, "demo");
  await test.step("the list holds the step names, one per line, with no repeat", async () => {
    expect(listed.length).toBeGreaterThan(0);
    expect(new Set(listed).size).toBe(listed.length);
  });

  await test.step("a second start runs no step", async () => {
    const notes = await noteCount(fixture);
    await restartBare(fixture);
    expect(await noteCount(fixture)).toBe(notes);
    expect(await names(fixture, "demo")).toEqual(listed);
  });
});

test("163. Per-project list", async ({ start }) => {
  const fixture = await start("migrate-pending");
  await migrated(fixture);
  const full = await names(fixture, "demo");

  await test.step("a second project has a ticket in the old shape and no list", async () => {
    await fixture.writeStoreFile("other/project.json", await fixture.readStoreFile("demo/project.json"));
    await fixture.writeStoreFile("other/other-epic/ticket.md", (await fixture.readStoreFile("demo/demo-epic/ticket.md")).replace("demo-epic", "other-epic"));
    await fixture.writeStoreFile("other/other-epic/issues/01-broken-shape.md", (await fixture.readStoreFile(ticket)).replace("status: paused", "status: needs-answer"));
  });

  await restartBare(fixture);

  await test.step("only the project without a list ran its steps", async () => {
    expect(await fixture.readStoreFile("other/other-epic/issues/01-broken-shape.md")).toContain("status: paused");
    expect(await names(fixture, "demo")).toEqual(full);
  });

  await test.step("both lists are full", async () => {
    expect(await names(fixture, "other")).toEqual(full);
  });
});

test("168. Crash restart and login start", async ({ start }) => {
  const fixture = await start("migrate-pending");
  await migrated(fixture);
  const first = await names(fixture, "demo");

  await test.step("the first start migrated", async () => {
    expect(await fixture.readStoreFile(ticket)).toContain("status: paused");
    expect(first.length).toBeGreaterThan(0);
  });

  await test.step("the second start, from a bare process, ran only the added step", async () => {
    const notes = await noteCount(fixture);
    await fixture.setInject([{ name: "e2e-added", notes: ["e2e added step ran"] }]);
    await restartBare(fixture);
    expect(await names(fixture, "demo")).toEqual([...first, "e2e-added"]);
    // The fixture store has a second project, scratch, which also had the full list.
    expect(await noteCount(fixture)).toBe(notes + 2);
    expect(await noteCount(fixture, "e2e added step ran")).toBe(2);
    expect(await names(fixture, "scratch")).toEqual([...first, "e2e-added"]);
    expect(await serverLog(fixture)).toContain("e2e added step ran");
  });
});
