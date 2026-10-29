import type { Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { expect, test, type Fixture } from "./fixture";

const sessions = async (fixture: Fixture) => JSON.parse(await fixture.cli("agents", "list", "--json")) as SessionInfo[];

const isActionable = (s: SessionInfo) => !!s.actionable.reason || s.status.state === "blocked";

const row = (page: Page, s: SessionInfo) => page.locator(`a[href$="/agents/${s.id}"]`);

const sidebar = (page: Page) => page.getByRole("complementary");

// A hash change keeps the page, since the fixture fails a test whose page reloaded.
const go = (page: Page, hash: string) => page.evaluate((h) => (location.hash = h), hash);

/** A second visit is a fresh page on the same server and context, since a reload would fail the test. */
async function freshVisit(page: Page, hash: string) {
  const other = await page.context().newPage();
  await other.goto(new URL(page.url()).origin + hash);
  return other;
}

test("72. Agents tab", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const actionable = list.filter(isActionable).length;
  expect(actionable).toBeGreaterThan(0);

  const tabs = page.getByRole("navigation").first().getByRole("link");
  await expect(tabs.nth(0)).toHaveText("Epics");
  await expect(tabs.nth(1)).toHaveText(`Agents${actionable}`);
  await expect(tabs.nth(1)).toHaveAccessibleName(`Agents · ${actionable} ${actionable === 1 ? "needs" : "need"} you`);
  await expect(tabs.nth(2)).toHaveText("Stats");

  await tabs.nth(1).click();
  await expect(tabs.nth(1)).toHaveAttribute("aria-current", "page");
  await expect(page).toHaveURL(/#\/agents/);

  // The deep link selects the session, in this page and in a fresh one.
  const target = list[1];
  await go(page, `#/agents/${target.id}`);
  await expect(row(page, target)).toHaveAttribute("aria-current", "page");
  const second = await freshVisit(page, `/#/agents/${target.id}`);
  await expect(row(second, target)).toHaveAttribute("aria-current", "page");
  await expect(second.getByRole("navigation").first().getByRole("link").nth(1)).toHaveText(`Agents${actionable}`);

  // The count follows gx agents list and hides when no session is left.
  await second.close();
  const empty = await start("empty");
  expect(await sessions(empty)).toHaveLength(0);
  await expect(page.getByRole("navigation").first().getByRole("link").nth(1)).toHaveText("Agents");
});

test("73. Agents sidebar", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  await go(page, "#/agents");
  const side = sidebar(page);

  // Projects in order of their first session; the design sessions start hidden.
  const projects = [...new Set(list.map((s) => s.project))];
  await expect(side.locator("p, span, div").filter({ hasText: /^(demo|other|site)$/ })).toHaveText(projects);
  for (const s of list) {
    if (s.design) await expect(row(page, s)).toHaveCount(0);
    else await expect(row(page, s)).toHaveCount(1);
  }
  const visible = list.filter((s) => !s.design);
  const order = await side.locator('a[href*="/agents/"]').evaluateAll((els) => els.map((el) => el.getAttribute("href")!.split("/agents/")[1]));
  expect(order).toEqual(visible.map((s) => s.id));

  // Marks.
  for (const s of visible) {
    const mark = s.exit === "crashed" ? "crashed" : s.status.state;
    await expect(row(page, s).getByRole("img", { name: mark, exact: true })).toBeVisible();
    await expect(row(page, s).getByRole("img", { name: "Unread" })).toHaveCount(s.unread.unread ? 1 : 0);
    await expect(row(page, s).getByRole("img", { name: "Actionable" })).toHaveCount(isActionable(s) ? 1 : 0);
  }
  const waiting = list.find((s) => s.actionable.reason)!;
  await expect(row(page, waiting).getByRole("img", { name: "Actionable" })).toHaveAttribute("title", waiting.actionable.reason!);

  const live = list.filter((s) => s.status.state !== "exited").length;
  await expect(side.getByText(`${live} live`)).toBeVisible();

  // Live updates: a removed Exited session leaves, a new one arrives.
  const exited = list.find((s) => s.status.state === "exited")!;
  await fixture.cli("agents", "remove", exited.id);
  await expect(row(page, exited)).toHaveCount(0);
  const before = new Set(list.map((s) => s.id));
  await fixture.cli("agents", "create", "--project", "demo", "--prompt", "hello");
  await expect.poll(async () => (await sessions(fixture)).filter((s) => !before.has(s.id)).length).toBe(1);
  const added = (await sessions(fixture)).find((s) => !before.has(s.id))!;
  await expect(row(page, added)).toBeVisible();
  await expect(side.getByText(`${live + 1} live`)).toBeVisible();
});

test("74. Design sessions toggle", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const designs = list.filter((s) => s.design);
  expect(designs.length).toBe(2);
  await go(page, "#/agents");
  const side = sidebar(page);
  const toggle = side.getByRole("checkbox", { name: /^Design sessions/ });

  // Hidden by default, with the count.
  await expect(toggle).not.toBeChecked();
  await expect(side.getByText(`· ${designs.length} hidden`)).toBeVisible();
  for (const s of designs) await expect(row(page, s)).toHaveCount(0);

  // Remembered in a fresh page of the same browser.
  await toggle.check();
  for (const s of designs) await expect(row(page, s)).toBeVisible();
  await expect(side.getByText("hidden")).toHaveCount(0);
  const second = await freshVisit(page, "/#/agents");
  await expect(sidebar(second).getByRole("checkbox", { name: /^Design sessions/ })).toBeChecked();
  for (const s of designs) await expect(row(second, s)).toBeVisible();
  await second.close();

  // A selected design session keeps its row while the rest hide.
  await toggle.uncheck();
  for (const s of designs) await expect(row(page, s)).toHaveCount(0);
  await go(page, `#/agents/${designs[0].id}`);
  await expect(row(page, designs[0])).toHaveAttribute("aria-current", "page");
  await expect(row(page, designs[1])).toHaveCount(0);
  await expect(side.getByText("· 1 hidden")).toBeVisible();
});

test("92. Last viewed session", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const viewed = list[1];

  // With none viewed, the wide page asks for a pick.
  await go(page, "#/agents");
  await expect(page.getByRole("heading", { name: "Pick a session" })).toBeVisible();

  await go(page, `#/agents/${viewed.id}`);
  await expect(row(page, viewed)).toHaveAttribute("aria-current", "page");

  // Opening #/agents in a fresh page reopens it.
  const second = await freshVisit(page, "/#/agents");
  await expect(row(second, viewed)).toHaveAttribute("aria-current", "page");
  await expect(second).toHaveURL(new RegExp(`#/agents/${viewed.id}$`));
  await expect(second.getByRole("heading", { name: "Pick a session" })).toHaveCount(0);
});
