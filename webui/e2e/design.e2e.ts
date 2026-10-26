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

test.describe("47. Send popover", () => {
  test("lists the gaps above Send, jumps to a question, sends anyway", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    const send = page.getByRole("button", nav("Send"));
    const popover = page.getByRole("alertdialog", nav("Unanswered questions"));
    const composer = page.locator('[aria-label="Composer"]');
    await expect(questions.getByText("Q8 - Which store?")).toBeVisible();

    const heightBefore = (await composer.boundingBox())!.height;
    await send.click();
    await expect(popover).toBeVisible();
    expect((await composer.boundingBox())!.height).toBe(heightBefore);
    await expect(popover.getByText("2 questions are unanswered")).toBeVisible();
    await expect(popover.getByText("0 of 2 answered")).toBeVisible();
    const rowQ8 = popover.getByRole("button", { name: /^Q8 Which store\? ›$/ });
    const rowQ8b = popover.getByRole("button", { name: /^Q8b Migrate at startup\? ›$/ });
    await expect(rowQ8).toBeVisible();
    await expect(rowQ8b).toBeVisible();
    await expect(popover.getByRole("button", nav("Send anyway"))).toBeFocused();

    // The popover sits above Send and ends at its right edge.
    const pop = (await popover.boundingBox())!;
    const btn = (await send.boundingBox())!;
    expect(pop.y + pop.height).toBeLessThan(btn.y);
    expect(Math.abs(pop.x + pop.width - (btn.x + btn.width))).toBeLessThanOrEqual(2);

    // A row closes the popover, makes its question current and focuses its first choice.
    await rowQ8b.click();
    await expect(popover).toHaveCount(0);
    await expect(questions.getByRole("button", nav("Question 8b"))).toHaveAttribute("aria-current", "step");
    await expect(questions.getByRole("group", nav("Question 8b of 2")).getByRole("button").first()).toBeFocused();

    // A folded sheet opens at the question of the row.
    await questions.getByRole("button", nav("Fold questions")).click();
    await expect(questions.getByRole("button", nav("Open questions"))).toBeVisible();
    await send.click();
    await popover.getByRole("button", { name: /^Q8 Which store\? ›$/ }).click();
    await expect(questions.getByRole("button", nav("Fold questions"))).toBeVisible();
    await expect(questions.getByRole("button", nav("Question 8"))).toHaveAttribute("aria-current", "step");

    // One gap left.
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await send.click();
    await expect(popover.getByText("1 question is unanswered")).toBeVisible();
    await expect(popover.getByText("1 of 2 answered")).toBeVisible();

    // Escape and a click on the chat close it and give focus back to Send.
    await expect(popover.getByRole("button", nav("Send anyway"))).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(popover).toHaveCount(0);
    await expect(send).toBeFocused();
    await send.click();
    await expect(popover.getByRole("button", nav("Send anyway"))).toBeFocused();
    await page.getByText("Two questions before the spec.").click();
    await expect(popover).toHaveCount(0);
    await expect(send).toBeFocused();

    // A second press closes what the first opened, and sends nothing.
    await send.click();
    await expect(popover).toBeVisible();
    await send.click();
    await expect(popover).toHaveCount(0);
    await expect(page.getByText("Q8b - Migrate at startup?: (unanswered)")).toHaveCount(0);

    // Enter on the focused Send anyway sends, with no second dialog.
    await send.click();
    await expect(popover.getByRole("button", nav("Send anyway"))).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page.getByText("Q8b - Migrate at startup?: (unanswered)")).toBeVisible();
    await expect(page.getByRole("alertdialog")).toHaveCount(0);
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("button", nav("Answer them"))).toHaveCount(0);
  });
});

test.describe("48. Fixed sheet rows", () => {
  test.use({ viewport: { width: 1280, height: 600 } });

  for (const mode of ["docked", "focus mode"]) {
  test(`the header and Prev/Next stay put while the body scrolls, ${mode}`, async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    await expect(questions.getByText("Q8 - Which store?")).toBeVisible();
    if (mode === "focus mode") await questions.getByRole("button", nav("Enter focus mode")).click();
    await questions.getByRole("button", nav("Note on question 8")).click();
    const note = questions.getByRole("textbox", nav("Note text for question 8"));
    await expect(note).toBeVisible();
    await expect(note).toHaveAttribute("rows", "3");

    const dot = questions.getByRole("button", nav("Question 8"));
    const count = questions.getByText("0 of 2 answered");
    const prev = questions.getByRole("button", nav("← Prev"));
    const next = questions.getByRole("button", nav("Next →"));
    const scroller = await note.evaluateHandle((el) => {
      for (let p = el.parentElement; p; p = p.parentElement) {
        if (getComputedStyle(p).overflowY === "auto" && p.scrollHeight > p.clientHeight) return p;
      }
      return null;
    });
    expect(await scroller.evaluate((el) => el !== null), "the body must overflow at this height").toBe(true);

    const where = async () => ({
      dot: await dot.boundingBox(),
      count: await count.boundingBox(),
      prev: await prev.boundingBox(),
      next: await next.boundingBox(),
      top: (await questions.boundingBox())!.y,
    });
    const before = await where();
    const scrollBefore = await scroller.evaluate((el) => (el as HTMLElement).scrollTop);
    await scroller.evaluate((el) => ((el as HTMLElement).scrollTop = (el as HTMLElement).scrollHeight));
    const scrollAfter = await scroller.evaluate((el) => (el as HTMLElement).scrollTop);

    expect(scrollAfter).toBeGreaterThan(scrollBefore);
    expect(await where()).toEqual(before);
    for (const el of [dot, count, prev, next]) await expect(el).toBeInViewport();
  });
  }
});

test.describe("49. Focus mode", () => {
  test("hides the messages, keeps the round, and every way out leaves", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    const send = page.getByRole("button", nav("Send"));
    const popover = page.getByRole("alertdialog", nav("Unanswered questions"));
    const jump = page.getByRole("dialog", nav("Jump to epic"));
    const enter = questions.getByRole("button", nav("Enter focus mode"));
    const leave = questions.getByRole("button", nav("Leave focus mode"));
    const chat = page.getByText("Two questions before the spec.");
    const q8 = questions.getByText("Q8 - Which store?");
    const viewport = page.viewportSize()!;
    await expect(q8).toBeVisible();
    await expect(enter).toHaveText("⤢");

    // The button sits left of ▾.
    const fold = (await questions.getByRole("button", nav("Fold questions")).boundingBox())!;
    const focusBox = (await enter.boundingBox())!;
    expect(focusBox.x + focusBox.width).toBeLessThanOrEqual(fold.x);

    const dockedSize = await q8.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    await enter.click();
    await expect(chat).toHaveCount(0);
    expect((await questions.boundingBox())!.height).toBeGreaterThan(viewport.height / 2);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(page.getByRole("region", nav("Progress"))).toBeVisible();
    await expect(page.locator('[aria-label="Composer"]')).toBeVisible();
    expect(await q8.evaluate((el) => parseFloat(getComputedStyle(el).fontSize))).toBeGreaterThan(dockedSize);
    await expect(leave).toHaveText("⤡");
    await leave.click();
    await expect(chat).toBeVisible();
    await expect(enter).toBeVisible();

    // Escape from the note field leaves.
    await enter.click();
    await expect(chat).toHaveCount(0);
    await questions.getByRole("button", nav("Note on question 8")).click();
    await questions.getByRole("textbox", nav("Note text for question 8")).focus();
    await page.keyboard.press("Escape");
    await expect(chat).toBeVisible();

    // Escape closes the layer on top first, a second one leaves.
    const layers = [
      { layer: popover, open: () => send.click() },
      { layer: jump, open: () => page.keyboard.press("Control+k") },
    ];
    for (const { layer, open } of layers) {
      await enter.click();
      await expect(chat).toHaveCount(0);
      await open();
      await expect(layer).toBeVisible();
      // Escape goes to the layer once focus is inside it.
      await expect(layer.locator(":focus")).toHaveCount(1);
      await page.keyboard.press("Escape");
      await expect(layer).toHaveCount(0);
      await expect(leave).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(chat).toBeVisible();
    }

    // The round survives entering and leaving: an answer, the current question and the composer note.
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await expect(questions.getByText("Q8b - Migrate at startup?")).toBeVisible();
    await page.getByRole("button", nav("Note")).click();
    await page.getByRole("textbox", nav("Message")).fill("keep this");
    await enter.click();
    await expect(chat).toHaveCount(0);
    await expect(questions.getByRole("button", nav("Question 8b"))).toHaveAttribute("aria-current", "step");
    await expect(page.getByRole("textbox", nav("Message"))).toHaveValue("keep this");
    await leave.click();
    await expect(questions.getByRole("button", nav("Question 8b"))).toHaveAttribute("aria-current", "step");
    await expect(questions.getByText("1 of 2 answered")).toBeVisible();
    await expect(page.getByRole("textbox", nav("Message"))).toHaveValue("keep this");

    // Folding leaves, and ⤢ on the folded sheet unfolds and enters.
    await enter.click();
    await questions.getByRole("button", nav("Fold questions")).click();
    await expect(chat).toBeVisible();
    await expect(questions.getByRole("button", nav("Open questions"))).toBeVisible();
    await enter.click();
    await expect(chat).toHaveCount(0);
    await expect(questions.getByRole("button", nav("Fold questions"))).toBeVisible();
    await expect(leave).toBeVisible();

    // Answering both and sending leaves with the sent round shown; Continue opens the next one docked.
    await questions.getByRole("button", { name: /Yes/ }).click();
    await send.click();
    await expect(page.getByText("Q8b - Migrate at startup?: Yes")).toBeVisible();
    await expect(enter).toHaveCount(0);
    const cont = page.getByRole("button", { name: /^Continue/ });
    await expect(async () => {
      await cont.click({ timeout: 2000 });
      await expect(page.getByText("A fresh round.")).toBeVisible({ timeout: 2000 });
    }).toPass();
    await expect(questions.getByText("Q9 - Keep going?")).toBeVisible();
    await expect(enter).toBeVisible();
    await expect(page.getByRole("dialog", nav("Questions, full screen"))).toHaveCount(0);
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
