import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Locator, Page } from "@playwright/test";
import { expect, nav, test, toasts } from "./fixture";

const banner = (page: Page) => page.getByRole("region", nav("Missing check config"));

const missing = async (fixture: { cli(...a: string[]): Promise<string> }, project: string) =>
  /"missing":\s*true/.test(await fixture.cli("server", "checks", "--project", project, "--json"));

/** Filters the epics page to one project, or to all of them with "". The picker is a checkbox popover. */
const pickProject = async (page: Page, project: string) => {
  await page.getByRole("button", nav("Project")).click();
  const list = page.getByRole("dialog", nav("Projects"));
  const all = list.getByRole("button", nav("All projects"));
  if (await all.isEnabled()) await all.click();
  if (project) await list.getByRole("checkbox", nav(project)).check();
  await page.keyboard.press("Escape");
  await expect(list).toHaveCount(0);
};

const openSetup = async (page: Page, project: string) => {
  await pickProject(page, project);
  await banner(page).getByRole("button", nav("Set up checks")).click();
  const dialog = page.getByRole("dialog", nav(`Set up checks · ${project}`));
  await expect(dialog).toBeVisible();
  return dialog;
};

const phoneFits = async (page: Page, dialog: Locator) => {
  for (const kind of ["Format", "Lint", "Typecheck", "Test"]) {
    const row = dialog.getByRole("group", nav(kind));
    const label = (await row.locator("b").boundingBox())!;
    const field = (await row.getByRole("combobox").boundingBox())!;
    expect(field.y).toBeGreaterThanOrEqual(label.y + label.height);
  }
  const box = (await dialog.boundingBox())!;
  expect(box.x + box.width).toBeLessThanOrEqual(400);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(400);
  expect(await dialog.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
};

// One server start for what needs the server, git and a real layout. Moved to vitest: banner text and
// buttons (64, 65, 66) in EpicsPageChecks.test.tsx; the dialog's rows and "keep gx defaults" (68) in
// SetupDialog.test.tsx.
test("67. Set up checks, save to repo (and 68. save to project.json, 69. refused commit, 70. phone)", async ({ page, start }) => {
  const fixture = await start("checks-missing");

  // All projects: the server's list order.
  await expect(banner(page).getByRole("listitem")).toHaveText([/^auto/, /^demo/]);

  // Wide screens keep the banner's buttons on the text's row, to its right (a layout measure jsdom lacks).
  await pickProject(page, "demo");
  await page.setViewportSize({ width: 1100, height: 800 });
  const b = banner(page);
  const text = (await b.locator("p").boundingBox())!;
  const notNow = (await b.getByRole("button", nav("Not now")).boundingBox())!;
  expect(notNow.x).toBeGreaterThanOrEqual(text.x + text.width);
  expect(notNow.y).toBeLessThan(text.y + text.height);

  // 70: the dialog at phone width, before anything is saved.
  await page.setViewportSize({ width: 400, height: 800 });
  const projects: { name: string; repo: string }[] = JSON.parse(await fixture.cli("project", "list", "--json"));
  const repo = projects.find((p) => p.name === "demo")!.repo;
  const file = join(repo, ".gx", "checks.json");
  const draft = '{"test":{"full":"make draft"}}\n';
  const head = () => execFileSync("git", ["-C", repo, "rev-parse", "HEAD"]).toString();
  const before = head();

  const dialog = await openSetup(page, "demo");
  await phoneFits(page, dialog);

  // 69: an uncommitted draft file makes the commit refuse; the dialog stays and HEAD does not move.
  mkdirSync(join(repo, ".gx"), { recursive: true });
  writeFileSync(file, draft);
  await dialog.getByRole("button", nav("Save")).click();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(dialog).toBeVisible();
  await expect(banner(page)).toBeVisible();
  expect(head()).toBe(before);
  expect(readFileSync(file, "utf8")).toBe(draft);
  rmSync(file);

  // 67: the real save commits .gx/checks.json on main.
  await expect(dialog.getByRole("radio", { name: /Commit \.gx\/checks\.json on main/ })).toBeChecked();
  await dialog.getByRole("button", nav("Save")).click();
  await expect(toasts(page).filter({ hasText: "Committed .gx/checks.json on main" })).toBeVisible();
  await expect(dialog).toHaveCount(0);
  await expect(banner(page)).toHaveCount(0);
  expect(await missing(fixture, "demo")).toBe(false);
  expect(head()).not.toBe(before);

  // 68: project `auto` saves to gx's project.json.
  const auto = await openSetup(page, "auto");
  await auto.getByRole("radio", { name: /Keep it in gx's project\.json/ }).check();
  await auto.getByRole("button", nav("Save")).click();
  await expect(auto).toHaveCount(0);
  await expect(banner(page)).toHaveCount(0);
  expect(await missing(fixture, "auto")).toBe(false);
});
