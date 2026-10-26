import type { Page } from "@playwright/test";
import { card, expect, nav, section, test, type Fixture } from "./fixture";

// The card's title opens the epic page, whose Start button starts the session and moves on.
// The Design page before the start is reached by its route.
const openDesign = async (page: Page) => {
  await expect(card(page, "design-epic")).toBeVisible();
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await expect(page.getByRole("button", nav("Start design session"))).toBeVisible();
};

const step = (page: Page, name: string, state: string) => page.locator(`li[data-step="${name}"][data-state="${state}"]`);

test.describe("4. Start a design session", () => {
  test("shows the first step", async ({ page, start }) => {
    await start("design");
    await openDesign(page);
    await expect(step(page, "resolve-tickets", "current")).toBeVisible();
    await expect(page.getByRole("link", { name: /^← Epics/ })).toHaveAccessibleName("← Epics");

    await page.getByRole("button", nav("Start design session")).click();

    await expect(page.getByText("Two questions before the spec.")).toBeVisible();
    await expect(page.getByRole("button", nav("Start design session"))).toHaveCount(0);
    await expect(step(page, "resolve-tickets", "current")).toBeVisible();
  });
});

test.describe("5. Answer a round", () => {
  test("ids, answer, Continue and a fresh round", async ({ page, start }) => {
    // A countdown that never ends in the test, so the press is the only Continue.
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    await expect(questions.getByRole("button", nav("Question 8"))).toBeVisible();
    await expect(questions.getByRole("button", nav("Question 8b"))).toBeVisible();
    await expect(questions.getByText("Q8 - Which store?")).toBeVisible();

    await questions.getByRole("button", { name: /SQLite/ }).click();
    await expect(questions.getByText("Q8b - Migrate at startup?")).toBeVisible();
    await questions.getByRole("button", { name: /Yes/ }).click();
    await expect(questions.getByText("2 of 2 answered")).toBeVisible();
    await page.getByRole("button", nav("Send")).click();

    // The agent got the answers: the chat shows them as the person's message.
    await expect(page.getByText("Q8 - Which store?: SQLite")).toBeVisible();
    await expect(page.getByText("Q8b - Migrate at startup?: Yes")).toBeVisible();
    const cont = page.getByRole("button", { name: /^Continue/ });
    await expect(cont).toBeVisible();

    // Under load the run-end update can land after the first press ("the session changed"), so press again.
    await expect(async () => {
      await cont.click({ timeout: 2000 });
      await expect(page.getByText("A fresh round.")).toBeVisible({ timeout: 2000 });
    }).toPass();
    await expect(page.getByRole("group", nav("Questions")).getByText("Q9 - Keep going?")).toBeVisible();
  });
});

test.describe("9. Design progress to the end", () => {
  test("walks every step to Done", async ({ page, start }) => {
    const fixture = await start("design");
    await openDesign(page);
    await expect(page.getByText("Open implementation epic →")).toHaveCount(0);

    const steps = ["spec", "review", "grill-findings", "create-tickets", "branch", "done"];
    for (const [i, name] of steps.entries()) {
      await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", name);
      const state = name === "done" ? "done" : "current";
      await expect(step(page, name, state)).toBeVisible();
      await expect(step(page, steps[i - 1] ?? "resolve-tickets", "done")).toBeVisible();
      const linked = name === "branch" || name === "done";
      await expect(page.getByText("Open implementation epic →")).toHaveCount(linked ? 1 : 0);
    }
    for (const s of ["chart", ...steps]) await expect(step(page, s, "done")).toBeVisible();
  });
});

// Starts the session and answers round one, ending at the run-end line.
const answerFirstRound = async (page: Page) => {
  await openDesign(page);
  await page.getByRole("button", nav("Start design session")).click();
  const questions = page.getByRole("group", nav("Questions"));
  await questions.getByRole("button", { name: /SQLite/ }).click();
  await questions.getByRole("button", { name: /Yes/ }).click();
  await page.getByRole("button", nav("Send")).click();
};

const autoSwitch = (page: Page, state: "on" | "off") => page.getByRole("switch", { name: `Auto-advance ${state}` });

test.describe("10. Auto-advance", () => {
  test("a fresh round follows the run end with no press", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "3s" });
    await answerFirstRound(page);

    await expect(page.getByText(/Continuing in \ds/)).toBeVisible();
    await expect(page.getByRole("button", nav("Cancel"))).toBeVisible();
    await expect(page.getByText("A fresh round.")).toBeVisible();
    await expect(page.getByRole("group", nav("Questions")).getByText("Q9 - Keep going?")).toBeVisible();
  });
});

test.describe("11. Auto-advance, cancel", () => {
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

  test("a note typed during the countdown stops the send", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "5s" });
    await answerFirstRound(page);

    await expect(page.getByText(/Continuing in/)).toBeVisible();
    await page.getByRole("button", nav("Note")).click();
    await page.getByRole("textbox", nav("Message")).fill("wait, one thing");
    await page.getByRole("button", nav("Send")).click();
    await expect(page.getByText(/Continuing in/)).toHaveCount(0);
    await page.waitForTimeout(6000);
    await expect(page.getByText("A fresh round.")).toHaveCount(0);
    await expect(page.getByRole("button", { name: /^Continue/ })).toBeVisible();
  });
});

test.describe("13. Defaulted questions", () => {
  test("one asked question and two defaults: one card, defaults as plain text", async ({ page, start }) => {
    await start("design-mixed", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    await expect(questions.getByText("Q12 - Keep going?")).toBeVisible();
    await expect(questions.getByRole("button", { name: /^Question \d/ })).toHaveCount(1);
    await expect(page.getByText("Q10: keep the current store.")).toBeVisible();
    await expect(page.getByText("Q11: migrate at startup.")).toBeVisible();

    await questions.getByRole("button", { name: /Yes/ }).click();
    await page.getByRole("button", nav("Send")).click();
    await expect(page.getByText("Q12 - Keep going?: Yes")).toBeVisible();
    // Send asked nothing about the defaults.
    await expect(page.getByText(/Q1[01] - /)).toHaveCount(0);
  });

  test("an all-defaulted round: one Accept these defaults card and a grilling Actionable", async ({ page, start }) => {
    const fixture = await start("design-defaulted", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    await expect(questions.getByText("Q12 - Accept these defaults?")).toBeVisible();
    await expect(questions.getByRole("button", { name: /^Question \d/ })).toHaveCount(1);
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
    await page.getByRole("button", nav("Start design session")).click();

    const row = page.getByRole("status").filter({ hasText: "Working:" });
    await expect(row).toHaveText(/Working: Resolve tickets · first/);

    const id = (await fixture.cli("agents", "list")).split("\t")[0].trim();
    await fixture.cliEnv({ GX_AGENT_SESSION: id }, "agent", "step", "spec");
    await expect(row).toHaveText(/Working: Spec/);
  });
});

test.describe("12. The auto-advance switch", () => {
  test("shows while working and idle; off, then on starts the countdown", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "30s" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();
    await expect(autoSwitch(page, "on")).toBeVisible();

    await autoSwitch(page, "on").click();
    await expect(autoSwitch(page, "off")).toBeVisible();

    const questions = page.getByRole("group", nav("Questions"));
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await questions.getByRole("button", { name: /Yes/ }).click();
    await page.getByRole("button", nav("Send")).click();
    await expect(page.getByRole("button", { name: /^Continue/ })).toBeVisible();
    await expect(page.getByText(/Continuing in/)).toHaveCount(0);

    // Under load the run-end update can land after the first press ("the session changed"), so press again.
    await expect(async () => {
      await autoSwitch(page, "off").click();
      await expect(autoSwitch(page, "on")).toBeVisible({ timeout: 2000 });
    }).toPass();
    await expect(page.getByText(/Continuing in/)).toBeVisible();
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

const sessionRows = async (fixture: Fixture, slug: string) =>
  (await fixture.cli("agents", "list")).split("\n").filter((l) => l.includes(slug));

// Stops the session of `slug` through the CLI; its row is found by slug in `gx agents list`.
const stopSession = async (fixture: Fixture, slug: string) => {
  await expect.poll(async () => (await sessionRows(fixture, slug)).length).toBeGreaterThan(0);
  await fixture.cli("agents", "stop", (await sessionRows(fixture, slug))[0].split("\t")[0].trim());
};

const newSession = (page: Page) => page.getByRole("button", nav("New session"));

test.describe("42. Stopped session card", () => {
  test("the card moves to Ready to design as Not started with a Design button", async ({ page, start }) => {
    const fixture = await start("ready");
    await startDesign(page, "stopped-design");
    await page.getByRole("link", nav("← Epics")).click();

    await stopSession(fixture, "stopped-design");
    const ready = await section(page, "Ready to design");
    const stopped = ready.locator("[data-kind]").filter({ has: page.getByRole("link", nav("stopped-design")) });
    await expect(stopped).toContainText("Not started");
    await expect(stopped.getByRole("button", nav("Design"))).toBeVisible();
  });
});

test.describe("43. New session on a stopped Design", () => {
  test("the header starts a new session and no form opens", async ({ page, start }) => {
    const fixture = await start("ready");
    await startDesign(page, "retry-design");
    await stopSession(fixture, "retry-design");

    // The banner and the header each have a "New session"; the header one is first in the page.
    await expect(newSession(page).first()).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("textbox", nav("Epic slug"))).toHaveCount(0);

    await newSession(page).first().click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect.poll(async () => (await sessionRows(fixture, "retry-design")).length).toBe(2);
    await expect(page).toHaveURL(/#\/d\/demo:retry-design$/);
  });
});

test.describe("44. Failure banner New session", () => {
  test("a session that ends without a run-end starts a new one with no form", async ({ page, start }) => {
    const fixture = await start("ready");
    await startDesign(page, "ended-design");
    await stopSession(fixture, "ended-design");

    const failure = page.getByRole("region", nav("Session failure"));
    await expect(failure).toContainText("Session ended");
    await failure.getByRole("button", nav("New session")).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("textbox", nav("Epic slug"))).toHaveCount(0);
    await expect.poll(async () => (await sessionRows(fixture, "ended-design")).length).toBe(2);
  });
});

test.describe("45. Delete design", () => {
  test("appears when the session stops with the page open, and Delete removes the design", async ({ page, start }) => {
    const fixture = await start("ready");
    await startDesign(page, "delete-me");
    const del = page.getByRole("button", nav("Delete design"));
    await expect(del).toHaveCount(0);

    await stopSession(fixture, "delete-me");
    await expect(del).toBeVisible();

    await del.click();
    await expect(page.getByText("Delete this design?")).toBeVisible();
    await page.getByRole("button", nav("Keep")).click();
    await expect(page.getByText("Delete this design?")).toHaveCount(0);
    await expect(del).toBeVisible();

    await del.click();
    await page.getByRole("button", nav("Delete")).click();
    await expect(page).toHaveURL(/#\/epics$/);
    await expect(card(page, "delete-me")).toHaveCount(0);
    await expect(fixture.cli("tickets", "show", "demo:delete-me")).rejects.toBeTruthy();

    await startDesign(page, "delete-me");
  });

  test("a charted design has no Delete design", async ({ page, start }) => {
    await start("design");
    await openDesign(page);
    await expect(page.getByRole("button", nav("Delete design"))).toHaveCount(0);
  });
});

test.describe("46. gx designs delete", () => {
  test("removes an uncharted design", async ({ start }) => {
    const fixture = await start("ready");
    await fixture.cli("designs", "open", "cli-del", "--idea", "x", "--project", "demo", "--json");
    await stopSession(fixture, "cli-del");

    await fixture.cli("designs", "delete", "demo:cli-del");
    await expect(fixture.cli("tickets", "show", "demo:cli-del")).rejects.toBeTruthy();

  });

  test("refuses a charted design with the not-uncharted reason", async ({ start }) => {
    const fixture = await start("design");
    const refused = await fixture
      .cli("designs", "delete", "demo:design-epic")
      .then(() => undefined, (err: { stdout?: string; stderr?: string }) => `${err.stdout}${err.stderr}`);
    expect(refused).toContain("not-uncharted");
    await fixture.cli("tickets", "show", "demo:design-epic");
  });
});
