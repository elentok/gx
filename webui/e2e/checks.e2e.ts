import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, nav, test, toasts } from "./fixture";

const banner = (page: Page) => page.getByRole("region", nav("Missing check config"));

const demoMissing = async (fixture: { cli(...a: string[]): Promise<string> }) =>
  /"missing":\s*true/.test(await fixture.cli("server", "checks", "--project", "demo", "--json"));

const openSetup = async (page: Page) => {
  await page.getByRole("combobox", nav("Project")).selectOption("demo");
  await banner(page).getByRole("button", nav("Set up checks")).click();
  const dialog = page.getByRole("dialog", nav("Set up checks · demo"));
  await expect(dialog).toBeVisible();
  return dialog;
};

test("64. Check config banner", async ({ page, start }) => {
  await start("checks-missing");
  await page.getByRole("combobox", nav("Project")).selectOption("demo");

  const b = banner(page);
  await expect(b).toContainText("demo has no check config.");
  await expect(b).toContainText("Agents run gx's built-in checks until you set it up.");
  await expect(b).toContainText("gx found 1 command to suggest.");
  await expect(b).not.toContainText("auto");
  await expect(b.getByRole("button", nav("Not now"))).toBeVisible();
  await expect(b.getByRole("button", nav("Set up checks"))).toBeVisible();
});

test("65. Check config banner, all projects", async ({ page, start }) => {
  await start("checks-missing");
  await page.getByRole("combobox", nav("Project")).selectOption("");

  const b = banner(page);
  await expect(b).toContainText("2 projects have no check config.");
  await expect(b).toContainText("Agents run gx's built-in checks until you set them up.");
  await expect(b.getByRole("listitem")).toHaveText([/^auto/, /^demo/]);
  await expect(b.getByRole("button", nav("Set up checks for demo"))).toBeVisible();
  await expect(b.getByRole("button", nav("Set up checks for auto"))).toBeVisible();
  await expect(b.getByRole("button", nav("Not now"))).toHaveCount(1);
});

test("66. Not now", async ({ page, start }) => {
  await start("checks-missing");
  const writes: string[] = [];
  page.on("request", (r) => {
    if (r.url().includes("/v1/checks/save")) writes.push(r.url());
  });
  await expect(banner(page)).toBeVisible();

  await banner(page).getByRole("button", nav("Not now")).click();

  await expect(banner(page)).toHaveCount(0);
  await expect(page.getByRole("link", nav("demo-epic"))).toBeVisible();
  expect(writes).toEqual([]);
});

test("67. Set up checks, save to repo", async ({ page, start }) => {
  const fixture = await start("checks-missing");
  const dialog = await openSetup(page);

  const testRow = dialog.getByRole("group", nav("Test"));
  await expect(testRow.getByRole("radio", nav("Use command"))).toBeChecked();
  await expect(testRow.getByRole("combobox")).toHaveValue("make test");
  await expect(testRow).toContainText("Recommended");
  await expect(testRow).toContainText("Found in Makefile · test");
  for (const kind of ["Format", "Lint", "Typecheck"]) {
    const row = dialog.getByRole("group", nav(kind));
    await expect(row.getByRole("radio", nav("gx default"))).toBeChecked();
    await expect(row.getByRole("combobox")).toBeDisabled();
    await expect(row.getByRole("combobox")).toHaveValue("");
    await expect(row).toContainText("Nothing found");
  }
  await expect(dialog.locator("datalist option[value*='build']")).toHaveCount(0);

  await expect(dialog.getByRole("radio", { name: /Commit \.gx\/checks\.json on main/ })).toBeChecked();
  await dialog.getByText("Show the file").click();
  await expect(dialog.locator("pre")).toHaveText(
    `// .gx/checks.json (committed on main)\n${JSON.stringify({ test: { full: "make test" } }, null, 2)}`,
  );

  await dialog.getByRole("button", nav("Save")).click();

  await expect(toasts(page).filter({ hasText: "Committed .gx/checks.json on main" })).toBeVisible();
  await expect(dialog).toHaveCount(0);
  await expect(banner(page)).toHaveCount(0);
  expect(await demoMissing(fixture)).toBe(false);
});

test("68. Set up checks, save to project.json", async ({ page, start }) => {
  const fixture = await start("checks-missing");
  const dialog = await openSetup(page);
  await dialog.getByRole("radio", { name: /Keep it in gx's project\.json/ }).check();
  await dialog.getByRole("button", nav("Save")).click();
  await expect(dialog).toHaveCount(0);
  await expect(banner(page)).toHaveCount(0);
  expect(await demoMissing(fixture)).toBe(false);
});

test("68. Set up checks, keep gx defaults", async ({ page, start }) => {
  const fixture = await start("checks-missing");
  const dialog = await openSetup(page);
  await dialog.getByRole("button", nav("Keep gx defaults")).click();
  await expect(dialog).toHaveCount(0);
  await expect(banner(page)).toHaveCount(0);
  expect(await demoMissing(fixture)).toBe(false);
});

test("69. Set up checks, refused commit", async ({ page, start }) => {
  const fixture = await start("checks-missing");
  const projects: { name: string; repo: string }[] = JSON.parse(await fixture.cli("project", "list", "--json"));
  const repo = projects.find((p) => p.name === "demo")!.repo;
  const file = join(repo, ".gx", "checks.json");
  const draft = '{"test":{"full":"make draft"}}\n';
  const head = () => execFileSync("git", ["-C", repo, "rev-parse", "HEAD"]).toString();
  const before = head();

  const dialog = await openSetup(page);
  mkdirSync(join(repo, ".gx"), { recursive: true });
  writeFileSync(file, draft);
  await dialog.getByRole("button", nav("Save")).click();

  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(dialog).toBeVisible();
  await expect(banner(page)).toBeVisible();
  expect(head()).toBe(before);
  expect(readFileSync(file, "utf8")).toBe(draft);
});

test.describe("phone", () => {
  test.use({ viewport: { width: 400, height: 800 } });

  test("70. Phone, Set up checks", async ({ page, start }) => {
    await start("checks-missing");
    const dialog = await openSetup(page);

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
  });
});
