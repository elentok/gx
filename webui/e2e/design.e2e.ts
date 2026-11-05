import type { Page } from "@playwright/test";
import { card, expect, holdRoute, nav, section, sessionRows, test, type Fixture } from "./fixture";

// The card's title opens the epic page, whose Start button starts the session and moves on.
// The Design page before the start is reached by its route.
const openDesign = async (page: Page) => {
  await expect(card(page, "design-epic")).toBeVisible();
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await expect(page.getByRole("button", nav("Start design session"))).toBeVisible();
};

// A round opens in the Questions view; the chat messages show after a tap on Chat.
const showChat = (page: Page) => page.getByRole("group", nav("View")).getByRole("button", nav("Chat")).click();

const step = (page: Page, name: string, state: string) => page.locator(`li[data-step="${name}"][data-state="${state}"]`);

// The Design page ring: a Design badge (role img) inside the element that carries data-tone.
const ring = (page: Page) => page.locator("[data-kind='design']").first();

test.describe("5. Start a design session and answer a round", () => {
  test("first step, ids, answer, Continue and a fresh round", async ({ page, start }) => {
    // A countdown that never ends in the test, so the press is the only Continue.
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await expect(step(page, "resolve-tickets", "current")).toBeVisible();
    await expect(page.getByRole("link", nav("Epics"))).toHaveAttribute("aria-current", "page");
    // The charted design has no session, so its ⋯ menu holds only Copy design ID.
    await expect(page.getByRole("button", nav("More actions"))).toHaveCount(1);
    await page.getByRole("button", nav("Start design session")).click();
    await expect(page.getByRole("button", nav("Start design session"))).toHaveCount(0);

    const questions = page.getByRole("group", nav("Questions"));
    await expect(questions.getByRole("button", nav("Question 8"))).toBeVisible();
    await expect(questions.getByRole("button", nav("Question 8b"))).toBeVisible();
    await expect(questions.getByText("Q8 - Which store?")).toBeVisible();

    // The first message arrives with round one, which opens in the Questions view; the sheet is answered there.
    await showChat(page);
    await expect(page.getByText("Two questions before the spec.")).toBeVisible();
    await expect(step(page, "resolve-tickets", "current")).toBeVisible();
    await page.getByRole("group", nav("View")).getByRole("button", { name: /^Questions/ }).click();
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await expect(questions.getByText("Q8b - Migrate at startup?")).toBeVisible();
    await questions.getByRole("button", { name: /Yes/ }).click();
    await expect(questions.getByText("2 of 2 answered")).toBeVisible();
    const release = await holdRoute(page, "**/v1/sessions/prompt");
    await page.getByRole("button", nav("Send answers")).click();
    await expect(page.getByRole("button", nav("Send answers"))).toBeDisabled();
    release();

    // The agent got the answers: the chat shows them as the person's message.
    await expect(page.getByText("Q8 - Which store?: SQLite").first()).toBeVisible();
    await expect(page.getByText("Q8b - Migrate at startup?: Yes").first()).toBeVisible();
    const cont = page.getByRole("button", { name: /^Continue/ });
    await expect(cont).toBeVisible();

    // Under load the run-end update can land after the first press ("the session changed"), so press again.
    // Round Q9 arrives with "A fresh round." and opens in the Questions view, which hides the chat.
    await expect(async () => {
      await cont.click({ timeout: 2000 });
      await expect(page.getByRole("group", nav("Questions")).getByText("Q9 - Keep going?")).toBeVisible({ timeout: 2000 });
    }).toPass();
    await showChat(page);
    await expect(page.getByText("A fresh round.")).toBeVisible();
  });
});

test.describe("9. Design progress to the end", () => {
  const steps = ["spec", "review", "grill-findings", "create-tickets", "branch", "done"];

  // The link shows only at Branch and Done, at the top of the Chat region.
  const expectImplLink = async (page: Page, name: string) => {
    const link = page.getByRole("region", nav("Chat")).getByRole("link", { name: /^Open implementation epic/ });
    const linked = name === "branch" || name === "done";
    await expect(link).toHaveCount(linked ? 1 : 0);
    if (!linked) return;
    const region = (await page.getByRole("region", nav("Chat")).boundingBox())!;
    expect((await link.boundingBox())!.y - region.y).toBeLessThan(80);
  };

  test("1440px: the Progress region and the ring show each step in turn", async ({ page, start }) => {
    const fixture = await start("design");
    await page.setViewportSize({ width: 1440, height: 900 });

    // The Epic page ring is the reference for the label.
    await expect(card(page, "design-epic")).toBeVisible();
    await page.evaluate(() => (location.hash = "#/e/demo:design-epic"));
    // innerText does not wait: under load it read the ring before its label rendered.
    await expect(ring(page)).toHaveText(/%\s*$/);
    const epicLabel = (await ring(page).innerText()).trim();

    await page.evaluate(() => (location.hash = "#/epics"));
    await openDesign(page);
    const badge = ring(page);
    await expect(badge.getByRole("img", nav("Design"))).toBeVisible();
    const ringBox = (await badge.boundingBox())!;
    const nameBox = (await page.getByRole("heading", { level: 1 }).boundingBox())!;
    expect(ringBox.x + ringBox.width).toBeLessThanOrEqual(nameBox.x);
    expect(Math.abs(ringBox.y + ringBox.height / 2 - (nameBox.y + nameBox.height / 2))).toBeLessThan(nameBox.height);
    await expect(badge).toContainText(epicLabel);
    await expect(badge).toHaveAttribute("data-tone", "muted");

    // Round one up: waiting, no comet. The Chat view has the link region the loop below reads.
    await page.getByRole("button", nav("Start design session")).click();
    await expect(page.getByRole("group", nav("Questions"))).toBeVisible();
    await expect(badge).toHaveAttribute("data-tone", "waiting");
    await expect(page.getByTestId("comet")).toHaveCount(0);
    await showChat(page);
    await expectImplLink(page, "resolve-tickets");

    for (const [i, name] of steps.entries()) {
      await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", name);
      const state = name === "done" ? "done" : "current";
      await expect(step(page, name, state)).toBeVisible();
      await expect(step(page, steps[i - 1] ?? "resolve-tickets", "done")).toBeVisible();
      await expectImplLink(page, name);
    }
    for (const s of ["chart", ...steps]) await expect(step(page, s, "done")).toBeVisible();
    await expect(badge).toHaveAttribute("data-tone", "done");
  });
});

// Starts the session and answers round one, ending at the run-end line.
const answerFirstRound = async (page: Page) => {
  await openDesign(page);
  await page.getByRole("button", nav("Start design session")).click();
  const questions = page.getByRole("group", nav("Questions"));
  await questions.getByRole("button", { name: /SQLite/ }).click();
  await questions.getByRole("button", { name: /Yes/ }).click();
  await page.getByRole("button", nav("Send answers")).click();
};

// The Auto button names each switch's state; the switches sit in the popover it opens.
const autoButton = (page: Page, state: "on" | "off") =>
  page.getByRole("button", { name: new RegExp(`^Auto: Auto-advance ${state}`) });
const flipAuto = async (page: Page, from: "on" | "off") => {
  await autoButton(page, from).click();
  await page.getByRole("dialog", nav("Auto")).getByRole("switch", { name: `Auto-advance ${from}` }).click();
  await page.keyboard.press("Escape");
};

test.describe("11. Auto-advance, cancel", () => {
  // The server's "a note typed during the countdown stops the send" is proven in
  // server/sessions_autoadvance_countdown_test.go (PageNoteStopsTheSendAndKeepsContinue).
  test("Cancel stops the send and Continue stays", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "5s" });
    await answerFirstRound(page);

    await page.getByRole("button", nav("Cancel")).click();
    await expect(page.getByText(/Continuing in/)).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Continue/ })).toBeVisible();
    // Wait past the old deadline: nothing was sent.
    await page.waitForTimeout(6000);
    await expect(page.getByText("A fresh round.")).toHaveCount(0);
  });
});

test.describe("13. Defaulted questions", () => {
  test("an all-defaulted round: one Accept these defaults card and a grilling Actionable", async ({ page, start }) => {
    const fixture = await start("design-defaulted", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    await expect(questions.getByText("Q12 - Accept these defaults?")).toBeVisible();
    await expect(questions.getByRole("button", { name: /^Question \d/ })).toHaveCount(1);
    await showChat(page);
    await expect(page.getByText("Q10: keep the current store.")).toBeVisible();
    await expect
      .poll(async () => /"grilling":\s*true/.test(await fixture.cli("agents", "list", "--json")))
      .toBe(true);
  });
});

test.describe("14. Working row", () => {
  test("shows the step and ticket, then follows the step report", async ({ page, start }) => {
    const fixture = await start("design-working");
    await openDesign(page);
    await expect(ring(page)).toHaveAttribute("data-tone", "muted");
    await page.getByRole("button", nav("Start design session")).click();

    const row = page.getByRole("status").filter({ hasText: "Working:" });
    await expect(row).toHaveText(/Working: Resolve tickets · first/);
    // A busy turn with no round has no View group and no sheet; the ring runs with the comet.
    await expect(page.getByRole("group", nav("View"))).toHaveCount(0);
    await expect(page.getByRole("group", nav("Questions"))).toHaveCount(0);
    await expect(ring(page)).toHaveAttribute("data-tone", "running");
    await expect(page.getByTestId("comet")).toBeVisible();

    const id = (await fixture.cli("agents", "list")).split("\t")[0].trim();
    await fixture.cliEnv({ GX_AGENT_SESSION: id }, "agent", "step", "spec");
    await expect(row).toHaveText(/Working: Spec/);
  });
});

test.describe("12. The auto-advance switch", () => {
  test("shows while working and idle; off, then on starts the countdown and a fresh round follows", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "3s" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();
    await expect(autoButton(page, "on")).toBeVisible();

    await flipAuto(page, "on");
    await expect(autoButton(page, "off")).toBeVisible();

    const questions = page.getByRole("group", nav("Questions"));
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await questions.getByRole("button", { name: /Yes/ }).click();
    await page.getByRole("button", nav("Send answers")).click();
    await expect(page.getByRole("button", { name: /^Continue/ })).toBeVisible();
    await expect(page.getByText(/Continuing in/)).toHaveCount(0);

    // Under load the run-end update can land after the first press ("the session changed"), so press again.
    await expect(async () => {
      if (await autoButton(page, "off").isVisible()) await flipAuto(page, "off");
      await expect(autoButton(page, "on")).toBeVisible({ timeout: 2000 });
    }).toPass();
    await expect(page.getByText(/Continuing in \ds/)).toBeVisible();
    await expect(page.getByRole("button", nav("Cancel"))).toBeVisible();

    // The countdown ends with no press: the fresh round follows the run end.
    await expect(questions.getByText("Q9 - Keep going?")).toBeVisible();
    await showChat(page);
    await expect(page.getByText("A fresh round.")).toBeVisible();
  });
});

// Starts a Design through the New Design dialog, which lands on its Design page.
const startDesign = async (page: Page, slug: string) => {
  await page.getByRole("button", nav("New Design")).click();
  const dialog = page.getByRole("dialog", nav("New Design"));
  await dialog.getByRole("textbox", nav("Epic slug")).fill(slug);
  await dialog.getByRole("textbox", nav("Rough idea")).fill("An idea.");
  await dialog.getByRole("button", nav("Start design")).click();
  await expect(page).toHaveURL(new RegExp(`#/d/demo:${slug}$`));
};

const isLive = (row: string) => !/exited/.test(row);
const rowId = (row: string) => row.split("\t")[0].trim();

// Stops the live session of `slug` through the CLI; its row is found by slug in `gx agents list`.
const stopSession = async (fixture: Fixture, slug: string) => {
  await expect.poll(async () => (await sessionRows(fixture, slug)).filter(isLive).length).toBeGreaterThan(0);
  await fixture.cli("agents", "stop", rowId((await sessionRows(fixture, slug)).filter(isLive)[0]));
};

// The ⋯ menu's items; opening it is a click on the trigger.
const actionsMenu = async (page: Page) => {
  await page.getByRole("button", nav("More actions")).click();
  return page.getByRole("menu", nav("More actions"));
};

test.describe("43. New session on a Design", () => {
  test("live asks first, a stopped card moves to Ready to design, the menu and the banner start a new session", async ({ page, start }) => {
    const fixture = await start("ready");
    const slug = "stopped-design";
    await startDesign(page, slug);
    await expect.poll(async () => (await sessionRows(fixture, slug)).length).toBe(1);
    const firstId = rowId((await sessionRows(fixture, slug))[0]);

    // On a live session the menu asks first: Cancel starts nothing, OK starts a new session.
    const menu = await actionsMenu(page);
    const item = menu.getByRole("menuitem", { name: /^New session/ });
    await expect(item).toHaveAccessibleName("New session Stop this session and start a fresh one");
    page.once("dialog", (d) => d.dismiss());
    await item.click();
    await expect(async () => expect(await sessionRows(fixture, slug)).toHaveLength(1)).toPass({ timeout: 2000 });

    await actionsMenu(page);
    page.once("dialog", (d) => d.accept());
    await menu.getByRole("menuitem", { name: /^New session/ }).click();
    await expect.poll(async () => (await sessionRows(fixture, slug)).length).toBe(2);
    const rows = await sessionRows(fixture, slug);
    expect(rows.find((r) => rowId(r) === firstId)).toMatch(/exited/);
    expect(rows.filter(isLive)).toHaveLength(1);

    // The stopped session's card moves to Ready to design as Not started with a Design button.
    await stopSession(fixture, slug);
    await page.getByRole("link", nav("Epics")).click();
    const ready = await section(page, "Ready to design");
    const stopped = ready.locator("[data-kind]").filter({ has: page.getByRole("link", nav(slug)) });
    await expect(stopped).toContainText("Not started");
    await expect(stopped.getByRole("button", nav("Design"))).toBeVisible();

    // On the stopped Design the ⋯ menu starts a new session and no form opens.
    await page.evaluate((s) => (location.hash = `#/d/demo:${s}`), slug);
    const failure = page.getByRole("region", nav("Session failure"));
    await expect(failure).toContainText("Session ended");
    const stoppedMenu = await actionsMenu(page);
    const fresh = stoppedMenu.getByRole("menuitem", { name: /^New session/ });
    await expect(fresh).toHaveAccessibleName("New session Start a fresh session");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("textbox", nav("Epic slug"))).toHaveCount(0);
    await fresh.click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect.poll(async () => (await sessionRows(fixture, slug)).length).toBe(3);
    await expect(page).toHaveURL(new RegExp(`#/d/demo:${slug}$`));

    // A session that ends without a run-end: the failure banner starts a new one with no form.
    await stopSession(fixture, slug);
    await expect(failure).toContainText("Session ended");
    await failure.getByRole("button", nav("New session")).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("textbox", nav("Epic slug"))).toHaveCount(0);
    await expect.poll(async () => (await sessionRows(fixture, slug)).length).toBe(4);
  });
});

test.describe("45. Delete design", () => {
  test("appears when the session stops with the page open, and Delete removes the design", async ({ page, start }) => {
    const fixture = await start("ready");
    await startDesign(page, "delete-me");
    const menu = await actionsMenu(page);
    const del = menu.getByRole("menuitem", { name: /^Delete design/ });
    await expect(menu.getByRole("menuitem", { name: /^New session/ })).toBeVisible();
    await expect(del).toHaveCount(0);
    // Escape goes to the menu once focus is inside it.
    await expect.poll(() => menu.evaluate((el) => el.contains(document.activeElement))).toBe(true);
    await page.keyboard.press("Escape");
    await expect(menu).toHaveCount(0);

    await stopSession(fixture, "delete-me");
    // The page has settled once the failure banner shows; a menu opened before that is remounted shut.
    await expect(page.getByRole("region", nav("Session failure"))).toBeVisible();
    await actionsMenu(page);
    await expect(del).toBeVisible();

    await del.click();
    await expect(page.getByText("Delete this design?")).toBeVisible();
    await page.getByRole("button", nav("Keep")).click();
    await expect(page.getByText("Delete this design?")).toHaveCount(0);
    await expect(del).toBeVisible();

    await del.click();
    const release = await holdRoute(page, "**/v1/designs/delete");
    const confirm = page.getByRole("button", nav("Delete"));
    await confirm.click();
    await expect(confirm).toBeDisabled();
    release();
    await expect(page).toHaveURL(/#\/epics$/);
    await expect(card(page, "delete-me")).toHaveCount(0);
    await expect(fixture.cli("tickets", "show", "demo:delete-me")).rejects.toBeTruthy();

    await startDesign(page, "delete-me");
  });
});

test("50. Design epic page", async ({ page, start }) => {
  const fixture = await start("design");
  const summary = "Reach the sea by building a canal, a lock and a long harbour wall before the autumn storms.";
  await fixture.cli("tickets", "section", "demo:design-epic", "Destination", summary);
  // The server reads the new section a moment after the CLI wrote it, so the page is opened until it shows.
  await expect(async () => {
    await page.evaluate(() => (location.hash = "#/epics"));
    await page.evaluate(() => (location.hash = "#/e/demo:design-epic"));
    await expect(page.getByText(summary, nav(summary))).toBeVisible({ timeout: 1000 });
  }).toPass();
  const epicRing = page.locator("[data-kind]").first();

  // Before a session the ring is muted and the stats line names the step, with a time and no cost.
  await expect(epicRing).toHaveAttribute("data-tone", "muted");
  await expect(epicRing).toHaveText(/%$/);
  await expect(page.getByText(summary, nav(summary))).toBeVisible();
  const stats = page.getByTestId("stats-line");
  await expect(stats).toHaveText(/^Resolve tickets/);
  await expect(stats).not.toContainText("$");
  await expect(stats).toContainText("2m");
  await expect(page.getByRole("link", { name: /^01 first .*implement.* ready$/ })).toBeVisible();
  await expect(page.getByRole("link", { name: /^02 second .*implement.* after 01$/ })).toBeVisible();
  const timed = page.getByRole("link", { name: /03 timed/ });
  await expect(timed).toContainText("2m");
  await expect(timed).not.toContainText("$");

  await page.getByRole("button", nav("Start design session")).click();
  // Once the first round lands the session waits for answers and the ring turns "waiting", so the
  // running ring is read first. The epic page is reopened for it; the Design page has no ring.
  await page.evaluate(() => (location.hash = "#/e/demo:design-epic"));
  await expect(epicRing).toHaveAttribute("data-tone", "running");
  await expect(page.getByTestId("comet")).toBeVisible();
  // A live session makes a design ticket's meta read "<type> · running" and adds a time.
  await fixture.cli("tickets", "set", "demo:design-epic/01", "--status", "claimed");
  // The server reads the claim a moment after the CLI wrote it, so the page is opened until it shows.
  await expect(async () => {
    await page.evaluate(() => (location.hash = "#/epics"));
    await page.evaluate(() => (location.hash = "#/e/demo:design-epic"));
    await expect(page.getByRole("link", { name: /01 first implement · running · \d+[smh]$/ })).toBeVisible({ timeout: 1000 });
  }).toPass();
});

const longSummary = "Reach the sea by building a canal, a lock and a long harbour wall before the autumn storms. ".repeat(12).trim();

// The server reads a new section a moment after the CLI wrote it, so the page is opened until it shows.
async function openEpicWithSummary(page: Page, fixture: Fixture, summary: string) {
  await fixture.cli("tickets", "section", "demo:design-epic", "Destination", summary);
  await expect(async () => {
    await page.evaluate(() => (location.hash = "#/epics"));
    await page.evaluate(() => (location.hash = "#/e/demo:design-epic"));
    await expect(page.getByRole("button", nav("Show full description"))).toBeVisible({ timeout: 1000 });
  }).toPass();
}

test("Header shape: Design and Epic share the left edge and row at 1440px, and neither scrolls sideways at 375px", async ({ page, start }) => {
  const fixture = await start("design");
  await openEpicWithSummary(page, fixture, "A short destination.");
  const edges = async () => ({
    title: (await page.getByRole("heading", { level: 1 }).boundingBox())!,
    ring: (await page.getByTestId("page-header-ring").boundingBox())!,
  });
  const epic = await edges();
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await expect(page.getByRole("button", nav("Start design session"))).toBeVisible();
  const design = await edges();
  expect(design.ring.x).toBeCloseTo(epic.ring.x, 0);
  expect(design.title.x).toBeCloseTo(epic.title.x, 0);
  // The two pages' containers differ by a sub-pixel or a border, so the row may sit 1px apart.
  expect(Math.abs(design.ring.y - epic.ring.y)).toBeLessThanOrEqual(1);

  await page.setViewportSize({ width: 375, height: 812 });
  const sideways = () => page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
  expect(await sideways()).toBe(false);
  await page.evaluate(() => (location.hash = "#/e/demo:design-epic"));
  await expect(page.getByRole("button", nav("Show full description"))).toBeVisible();
  expect(await sideways()).toBe(false);
});

test("Epic description popover: click, Enter, Space and Escape, and it stays inside the viewport at 375px", async ({ page, start }) => {
  const fixture = await start("design");
  await openEpicWithSummary(page, fixture, longSummary);
  const button = page.getByRole("button", nav("Show full description"));
  const popover = page.getByTestId("description-popover");
  for (const open of [() => button.click(), () => button.press("Enter"), () => button.press("Space")]) {
    await open();
    await expect(popover).toBeVisible();
    // Zag arms its Escape listener a frame after the popover mounts, so a first press may be missed.
    await expect(async () => {
      await page.keyboard.press("Escape");
      await expect(popover).toHaveCount(0, { timeout: 500 });
    }).toPass();
  }

  await page.setViewportSize({ width: 375, height: 812 });
  await button.click();
  await expect(popover).toBeVisible();
  const box = (await popover.boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(375);
  expect(box.y + box.height).toBeLessThanOrEqual(812);
});

test.describe("46. gx designs open and delete", () => {
  test("open refuses an existing epic, delete removes an uncharted design and refuses a charted one", async ({ start }) => {
    const fixture = await start("design");
    const open = () => fixture.cli("designs", "open", "cli-del", "--idea", "x", "--project", "demo", "--json");
    await open();
    expect(await fixture.cli("tickets", "show", "demo:cli-del")).toContain("kind: map");

    // With --json a refusal is printed as JSON; the exit code may or may not be non-zero.
    const exists = await open().catch((err: { stdout?: string; stderr?: string }) => `${err.stdout}${err.stderr}`);
    expect(exists).toContain("epic-exists");

    await stopSession(fixture, "cli-del");
    await fixture.cli("designs", "delete", "demo:cli-del");
    await expect(fixture.cli("tickets", "show", "demo:cli-del")).rejects.toBeTruthy();

    const refused = await fixture
      .cli("designs", "delete", "demo:design-epic")
      .then(() => undefined, (err: { stdout?: string; stderr?: string }) => `${err.stdout}${err.stderr}`);
    expect(refused).toContain("not-uncharted");
    await fixture.cli("tickets", "show", "demo:design-epic");
  });
});
