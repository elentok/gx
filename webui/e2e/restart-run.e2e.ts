import type { Page } from "@playwright/test";
import { expect, test, type Fixture } from "./fixture";
import { dialog, expectReloaded, openDialog, passing, pidLine, restartButton } from "./restart-helpers";

// Flows that run a restart for real: the fake gx binary hands `server restart --continue` to a real
// gx (see testutil/gxfixture/restart.go), so the migration of every project in the store runs.

const brokenTicket = "broken/broken-epic/issues/90-bad.md";
const badTicket = "---\nid: \"90\"\nstatus: bogus\ntype: implement\n---\n# Bad\n";
const goodTicket = "---\nid: \"90\"\nstatus: open\ntype: implement\n---\n# Bad\n";
// The migration renames this legacy epic key to destination_unconfirmed.
const demoEpic = "demo/demo-epic/ticket.md";
const legacyKey = "provisional_destination";

/** A restart-projects fixture ready to restart through the real gx, with `broken` unable to migrate. */
const startBroken = async (start: (scenario: string, opts?: { reloads?: number }) => Promise<Fixture>, reloads: number) => {
  const fixture = await start("restart-projects", { reloads });
  await fixture.setRestartControl("check.json", passing("fixture-next"));
  await fixture.setRestartControl("start", "up");
  await fixture.writeStoreFile(brokenTicket, badTicket);
  const epic = await fixture.readStoreFile(demoEpic);
  await fixture.writeStoreFile(demoEpic, epic.replace(/^---\n/, `---\n${legacyKey}: main\n`));
  return fixture;
};

const restartNow = async (page: Page) => {
  await openDialog(page);
  await restartButton(page).click();
  await expectReloaded(page);
};

const strip = (page: Page) => page.getByRole("status").filter({ hasText: "broken: migration failed" });

test("139. Restart with a failed migration", async ({ page, start }) => {
  const fixture = await startBroken(start, 3);
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

  await test.step("the strip shows on two pages", async () => {
    await expect(strip(page)).toBeVisible();
    await expect(strip(page)).toContainText("status");
    await expect(strip(page).locator("code")).toContainText("gx tickets migrate");
    await page.getByRole("link", { name: "demo-epic" }).first().click();
    await expect(strip(page)).toBeVisible();
    await expect(strip(page).locator("code")).toContainText("gx tickets migrate");
  });

  await test.step("fixing the project and migrating clears the strip", async () => {
    const dir = (await strip(page).locator("code").textContent())!.match(/gx tickets migrate '(.+)'/)![1];
    await fixture.writeStoreFile(brokenTicket, goodTicket);
    await fixture.cli("tickets", "migrate", dir);
    await expect(strip(page)).toHaveCount(0, { timeout: 15_000 });
  });

  await test.step("a passing restart clears it too", async () => {
    await fixture.writeStoreFile(brokenTicket, badTicket);
    await restartNow(page);
    await expect(strip(page)).toBeVisible();
    await fixture.writeStoreFile(brokenTicket, goodTicket);
    await restartNow(page);
    await expect(strip(page)).toHaveCount(0, { timeout: 15_000 });
  });
});
