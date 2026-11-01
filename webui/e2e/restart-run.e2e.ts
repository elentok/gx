import { realpath } from "node:fs/promises";
import type { Page } from "@playwright/test";
import { composer, messageBox, sessions } from "./agentsKit";
import { expect, nav, test, toasts, type Fixture } from "./fixture";
import { buildText, dialog, expectReloaded, openDialog, passing, pidLine, restartButton } from "./restart-helpers";

// Flows that run a restart for real: the fake gx binary hands `server restart --continue` to a real
// gx (see testutil/gxfixture/restart.go), so the migration of every project in the store runs.

const brokenTicket = "broken/broken-epic/issues/90-bad.md";
const badTicket = "---\nid: \"90\"\nstatus: bogus\ntype: implement\n---\n# Bad\n";
const goodTicket = "---\nid: \"90\"\nstatus: open\ntype: implement\n---\n# Bad\n";
// The migration renames this legacy epic key to destination_unconfirmed.
const demoEpic = "demo/demo-epic/ticket.md";
const legacyKey = "provisional_destination";

/** Applied steps are not run again: empty the lists so the next server start runs them all on both projects. */
const forgetApplied = async (fixture: Fixture) => {
  for (const project of ["demo", "broken"]) await fixture.writeStoreFile(`${project}/applied-migrations.txt`, "");
};

/** Puts the bad ticket in `broken` and makes the next server start try to migrate it. */
const breakBroken = async (fixture: Fixture) => {
  await fixture.writeStoreFile(brokenTicket, badTicket);
  await forgetApplied(fixture);
};

/** A restart-projects fixture ready to restart through the real gx, with `broken` unable to migrate. */
const startBroken = async (start: (scenario: string, opts?: { reloads?: number }) => Promise<Fixture>, reloads: number) => {
  const fixture = await start("restart-projects", { reloads });
  await fixture.setRestartControl("check.json", passing("fixture-next"));
  await fixture.setRestartControl("start", "up");
  await breakBroken(fixture);
  const epic = await fixture.readStoreFile(demoEpic);
  await fixture.writeStoreFile(demoEpic, epic.replace(/^---\n/, `---\n${legacyKey}: main\n`));
  return fixture;
};

const restartNow = async (page: Page) => {
  await openDialog(page);
  await restartButton(page).click();
  await expectReloaded(page);
};

const strip = (page: Page) => page.getByRole("status").filter({ hasText: "broken: Migration blocked" });

test("139. Restart with a failed migration", async ({ page, start }) => {
  const fixture = await startBroken(start, 4);
  const before = await pidLine(fixture);

  await test.step("the server comes back although broken fails", async () => {
    await restartNow(page);
    expect(await pidLine(fixture)).not.toBe(before);
    expect(await fixture.cli("server", "status")).toMatch(/running/i);
  });

  await test.step("the other project is migrated", async () => {
    const epic = await fixture.readStoreFile(demoEpic);
    expect(epic).toContain("destination_unconfirmed: main");
    expect(epic).not.toContain(legacyKey);
  });

  await test.step("the strip shows on two pages, with Retry migration", async () => {
    await expect(strip(page)).toBeVisible({ timeout: 30_000 });
    await expect(strip(page)).toContainText("status");
    await expect(strip(page).locator("code")).toContainText("gx tickets migrate");
    await expect(strip(page).getByRole("button", nav("Retry migration"))).toBeVisible();
    await page.getByRole("link", { name: "demo-epic" }).first().click();
    await expect(strip(page)).toBeVisible();
    await expect(strip(page).locator("code")).toContainText("gx tickets migrate");
  });

  await test.step("fixing the project and Retry migration clears the strip", async () => {
    await fixture.writeStoreFile(brokenTicket, goodTicket);
    await strip(page).getByRole("button", nav("Retry migration")).click();
    await expect(strip(page)).toHaveCount(0, { timeout: 30_000 });
  });

  await test.step("the command by hand clears it too", async () => {
    await breakBroken(fixture);
    await restartNow(page);
    await expect(strip(page)).toBeVisible({ timeout: 30_000 });
    const dir = (await strip(page).locator("code").textContent())!.match(/gx tickets migrate '(.+)'/)![1];
    await fixture.writeStoreFile(brokenTicket, goodTicket);
    await fixture.cli("tickets", "migrate", dir);
    await expect(strip(page)).toHaveCount(0, { timeout: 30_000 });
  });

  await test.step("a passing restart clears it too", async () => {
    await breakBroken(fixture);
    await restartNow(page);
    await expect(strip(page)).toBeVisible({ timeout: 30_000 });
    await fixture.writeStoreFile(brokenTicket, goodTicket);
    await restartNow(page);
    await expect(strip(page)).toHaveCount(0, { timeout: 30_000 });
  });
});

const fixDialog = (page: Page) => page.getByRole("dialog", nav("Fix it with an agent"));

test("140. Fix agent for a failed migration", async ({ page, context, start }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const fixture = await startBroken(start, 1);
  await restartNow(page);
  await expect(strip(page)).toBeVisible({ timeout: 30_000 });
  const error = (await strip(page).locator("p").nth(1).textContent())!;
  const before = (await sessions(fixture)).length;

  await test.step("the strip button opens the dialog with the prompt and Copy", async () => {
    await strip(page).getByRole("button", nav("Fix agent")).click();
    await expect(fixDialog(page)).toBeVisible();
    await expect(fixDialog(page)).toContainText("The migration of project broken failed with:");
    await expect(fixDialog(page)).toContainText(error);
    await fixDialog(page).getByRole("button", nav("Copy")).click();
    await expect(fixDialog(page).getByRole("button", nav("Copied"))).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toContain("The migration of project broken failed with:");
  });

  await test.step("Start runs the strongest tier in the trunk checkout", async () => {
    await fixDialog(page).getByRole("button", nav("Start gx agent")).click();
    await expect(fixDialog(page).getByText("Agent started. View it in the gx TUI Agents tab")).toBeVisible();
    await expect.poll(async () => (await sessions(fixture)).length).toBe(before + 1);
    const started = (await sessions(fixture)).at(-1)!;
    const path = (await fixture.cli("tickets", "show", "broken:broken-epic/01")).match(/^path: (.+)$/m)![1];
    expect(await realpath(started.dir)).toBe(await realpath(`${path.split("/data/gx/tickets")[0]}/broken-repo`));
    expect(started.tier).toBe("strongest");
    expect(started.project).toBe("broken");
    await expect.poll(async () => (await fixture.launches()).at(-1)!.join(" ")).toContain("--model opus --effort high");
  });

  await test.step("the Agents tab lists it and a second start is refused", async () => {
    const started = (await sessions(fixture)).at(-1)!;
    // A hash change keeps the page: a reload would fail the test.
    await page.evaluate(() => (location.hash = "#/agents"));
    await expect(page.locator(`a[href$="/agents/${started.id}"]`)).toBeVisible();

    const second = await context.newPage();
    await second.goto(new URL(page.url()).origin);
    await strip(second).getByRole("button", nav("Fix agent")).click();
    await fixDialog(second).getByRole("button", nav("Start gx agent")).click();
    await expect(toasts(second).filter({ hasText: "an agent is already fixing broken" })).toBeVisible();
    expect((await sessions(fixture)).length).toBe(before + 1);
  });
});

test("141. Restart server, tab reloads", async ({ page, start }) => {
  const fixture = await start("restart-projects", { reloads: 1 });
  await fixture.setRestartControl("check.json", passing("fixture-next"));
  await fixture.setRestartControl("start", "up");
  await fixture.setRestartControl("gx", process.env.GX_E2E_GX_NEXT!);
  expect(await buildText(page)).not.toContain("fixture-next");

  await openDialog(page);
  await restartButton(page).click();
  await expect(dialog(page)).toContainText("Restarting...");
  await expect(dialog(page)).toHaveCount(0, { timeout: 45_000 });
  // A fresh page can show the menu button before React attaches its handler, which drops a click.
  await expect(async () => expect(await buildText(page)).toContain("fixture-next")).toPass({ timeout: 30_000 });
});

test("142. Restart server, other tab", async ({ page, context, start }) => {
  const fixture = await start("restart-projects", { reloads: 2 });
  await fixture.setRestartControl("check.json", passing("fixture-next"));
  await fixture.setRestartControl("start", "up");
  await fixture.setRestartControl("gx", process.env.GX_E2E_GX_NEXT!);
  await fixture.cli("agents", "create", "--project", "demo", "--prompt", "hello");
  const session = (await sessions(fixture)).at(-1)!;

  await page.evaluate((h) => (location.hash = h), `#/agents/${session.id}`);
  await messageBox(page).fill("unsent words");
  await expect(composer(page)).toBeVisible();

  const other = await context.newPage();
  await other.goto(new URL(page.url()).origin);
  await restartNow(other);

  const newVersion = page.getByRole("status").filter({ hasText: "New version, reload" });
  await expect(newVersion).toBeVisible({ timeout: 30_000 });
  await expect(messageBox(page)).toHaveValue("unsent words");
  await newVersion.getByRole("button", nav("Reload")).click();
  await expect(newVersion).toHaveCount(0, { timeout: 30_000 });
});

test("143. Restart server, server does not return", async ({ page, start }) => {
  test.setTimeout(180_000);
  const fixture = await start("restart-projects", { reloads: 1 });
  await fixture.setRestartControl("check.json", passing("fixture-next"));
  const logs: string[] = [];
  page.on("response", async (res) => {
    if (!/\/v1\/restart(\/check)?$/.test(new URL(res.url()).pathname)) return;
    const log = (await res.json().catch(() => ({})))?.restart_log;
    if (log) logs.push(log);
  });

  await openDialog(page);
  await restartButton(page).click();
  await expect(dialog(page)).toContainText("Restarting...");
  await expect(dialog(page)).toContainText("The server did not return. It may have failed to start.", { timeout: 75_000 });
  await expect(dialog(page)).toContainText("gx server start");
  expect(logs.length).toBeGreaterThan(0);
  const shown = (await dialog(page).textContent())!.match(/restart log at (.+?), or run/)![1];
  for (const log of logs) expect(log).toBe(shown);

  await fixture.setRestartControl("start", "up");
  // The fake start refused, so the old server still runs: start by hand brings up the new one.
  await fixture.cli("server", "restart", "--continue");
  await expect(dialog(page)).toHaveCount(0, { timeout: 45_000 });
  await expect(page.getByText("Restarting...")).toHaveCount(0);
});

test("144. Restart server, binary changed", async ({ page, start }) => {
  const fixture = await start("restart-projects");
  await fixture.setRestartControl("check.json", passing("fixture-next"));
  await fixture.setRestartControl("start", "up");
  const before = await pidLine(fixture);

  await openDialog(page);
  await expect(dialog(page)).toContainText("Next version: fixture-next");
  await fixture.setRestartControl("check.json", passing("fixture-newer"));
  await restartButton(page).click();

  await expect(dialog(page)).toContainText("Next version: fixture-newer");
  await expect(dialog(page)).not.toContainText("Restarting...");
  await expect(restartButton(page)).toBeVisible();
  expect(await pidLine(fixture)).toBe(before);
});

test("145. Restart server, two tabs", async ({ page, context, start }) => {
  const fixture = await start("restart-projects");
  await fixture.setRestartControl("check.json", passing("fixture-next"));
  const origin = new URL(page.url()).origin;
  const open = async () => {
    const tab = await context.newPage();
    await tab.goto(origin);
    await openDialog(tab);
    await expect(restartButton(tab)).toBeEnabled();
    return tab;
  };
  const during = await open();
  const after = await open();
  await openDialog(page);

  await restartButton(page).click();
  await expect(dialog(page)).toContainText("Restarting...");
  await restartButton(during).click();
  await expect(dialog(during)).toContainText("Restarting...");

  // The fake start fails, so the old server stays up; stop it as a real restart would.
  await fixture.stopServer();
  await restartButton(after).click();
  await expect(dialog(after)).toContainText("Restarting...");
});

test("146. Restart dialog, launchd binary", async ({ page, start }) => {
  const fixture = await start("restart-launchd");
  await fixture.setRestartControl(
    "check.json",
    JSON.stringify({ version: "launchd-version", config: { ok: true }, projects: [{ name: "demo", ok: true, files: [] }] }),
  );

  await openDialog(page);
  await expect(dialog(page)).toContainText("Next version: launchd-version");
  await expect(restartButton(page)).toBeEnabled();
});
