import type { Page } from "@playwright/test";
import { card, expect, holdRoute, nav, section, test, type Fixture } from "./fixture";

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

test.describe("4. Start a design session", () => {
  test("shows the first step", async ({ page, start }) => {
    await start("design");
    await openDesign(page);
    await expect(step(page, "resolve-tickets", "current")).toBeVisible();
    await expect(page.getByRole("link", nav("Epics"))).toHaveAttribute("aria-current", "page");
    // The charted design has no session, so its ⋯ menu would be empty and the button is absent.
    await expect(page.getByRole("button", nav("More actions"))).toHaveCount(0);

    await page.getByRole("button", nav("Start design session")).click();

    // The first message arrives with round one, which opens in the Questions view.
    await showChat(page);
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

    // Answered from the Chat view, so the person's message shows once it is sent.
    await showChat(page);
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await expect(questions.getByText("Q8b - Migrate at startup?")).toBeVisible();
    await questions.getByRole("button", { name: /Yes/ }).click();
    await expect(questions.getByText("2 of 2 answered")).toBeVisible();
    const release = await holdRoute(page, "**/v1/sessions/prompt");
    await page.getByRole("button", nav("Send")).click();
    await expect(page.getByRole("button", nav("Send"))).toBeDisabled();
    release();

    // The agent got the answers: the chat shows them as the person's message.
    await expect(page.getByText("Q8 - Which store?: SQLite")).toBeVisible();
    await expect(page.getByText("Q8b - Migrate at startup?: Yes")).toBeVisible();
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

    // A folded sheet opens at the question of the row. Only the Chat view has the fold chevron.
    await showChat(page);
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

    // Escape and a click outside the popover close it and give focus back to Send.
    await expect(popover.getByRole("button", nav("Send anyway"))).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(popover).toHaveCount(0);
    await expect(send).toBeFocused();
    await page.getByRole("group", nav("View")).getByRole("button", { name: /^Questions/ }).click();
    await send.click();
    await expect(popover.getByRole("button", nav("Send anyway"))).toBeFocused();
    await questions.getByText("Q8 - Which store?").click();
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

  for (const mode of ["Questions view", "docked in the Chat view"]) {
  test(`the header and Prev/Next stay put while the body scrolls, ${mode}`, async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    await expect(questions.getByText("Q8 - Which store?")).toBeVisible();
    if (mode !== "Questions view") await showChat(page);
    await questions.getByRole("button", nav("Note on question 8")).click();
    const note = questions.getByRole("textbox", nav("Note text for question 8"));
    await expect(note).toBeVisible();
    await expect(note).toHaveAttribute("rows", "5");

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
    // Opening the note may already have scrolled to the bottom (the Questions view is taller).
    await scroller.evaluate((el) => ((el as HTMLElement).scrollTop = 0));
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

test.describe("49. Questions and Chat views", () => {
  test("the View toggle, Escape from each layer, and the round across switches", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    const view = page.getByRole("group", nav("View"));
    const questionsTab = view.getByRole("button", { name: /^Questions/ });
    const chatTab = view.getByRole("button", nav("Chat"));
    const send = page.getByRole("button", nav("Send"));
    const popover = page.getByRole("alertdialog", nav("Unanswered questions"));
    const jump = page.getByRole("dialog", nav("Jump to epic"));
    const noteModal = page.getByRole("dialog", nav("Note on Q8b - Migrate at startup?"));
    const menu = page.getByRole("menu", nav("More actions"));
    const chat = page.getByText("Two questions before the spec.");
    const viewport = page.viewportSize()!;

    // The Design opens in the Questions view and the sheet fills the column.
    await expect(questionsTab).toHaveAttribute("aria-pressed", "true");
    await expect(questionsTab).toHaveAccessibleName("Questions (0 of 2)");
    await expect(chat).toBeHidden();
    expect((await questions.boundingBox())!.height).toBeGreaterThan(viewport.height / 2);

    await questions.getByRole("button", { name: /SQLite/ }).click();
    await expect(questionsTab).toHaveAccessibleName("Questions (1 of 2)");

    // Chat docks the sheet under the messages, with a fold chevron; Questions has none.
    await chatTab.click();
    await expect(chatTab).toHaveAttribute("aria-pressed", "true");
    await expect(chat).toBeVisible();
    const fold = questions.getByRole("button", nav("Fold questions"));
    await fold.click();
    await expect(questions.getByRole("button", nav("Open questions"))).toBeVisible();
    await questions.getByRole("button", nav("Open questions")).click();
    await expect(fold).toBeVisible();
    await questionsTab.click();
    await expect(chat).toBeHidden();
    await expect(fold).toHaveCount(0);

    // Escape in the Questions view shows the chat.
    await page.keyboard.press("Escape");
    await expect(chatTab).toHaveAttribute("aria-pressed", "true");
    await expect(chat).toBeVisible();

    // Escape closes the layer on top first, a second one shows the chat.
    const layers = [
      { layer: popover, open: () => send.click() },
      { layer: jump, open: () => page.keyboard.press("Control+k") },
      {
        layer: noteModal,
        open: async () => {
          await questions.getByRole("button", nav("Note on question 8b")).click();
          await questions.getByRole("button", nav("Open the note in a large editor")).click();
        },
      },
      { layer: menu, open: () => page.getByRole("button", nav("More actions")).click() },
    ];
    for (const { layer, open } of layers) {
      await questionsTab.click();
      await expect(chat).toBeHidden();
      await open();
      await expect(layer).toBeVisible();
      // Escape goes to the layer once focus is inside it (or on it).
      await expect.poll(() => layer.evaluate((el) => el.contains(document.activeElement))).toBe(true);
      await page.keyboard.press("Escape");
      await expect(layer).toHaveCount(0);
      await expect(questionsTab).toHaveAttribute("aria-pressed", "true");
      await page.keyboard.press("Escape");
      await expect(chat).toBeVisible();
    }

    // The round survives two switches: an answer, the current question and the composer note.
    await expect(questions.getByText("Q8b - Migrate at startup?")).toBeVisible();
    await page.getByRole("button", nav("Note")).click();
    await page.getByRole("textbox", nav("Message")).fill("keep this");
    await questionsTab.click();
    await chatTab.click();
    await questionsTab.click();
    await expect(questionsTab).toHaveAccessibleName("Questions (1 of 2)");
    await expect(questions.getByRole("button", nav("Question 8b"))).toHaveAttribute("aria-current", "step");
    await expect(page.getByRole("textbox", nav("Message"))).toHaveValue("keep this");

    // The focus mode buttons are gone.
    await expect(page.getByRole("button", nav("Enter focus mode"))).toHaveCount(0);
    await expect(page.getByRole("button", nav("Leave focus mode"))).toHaveCount(0);

    // The question title stays at most 820px wide on a wide screen.
    await page.setViewportSize({ width: 1440, height: 900 });
    expect((await questions.getByText("Q8b - Migrate at startup?").boundingBox())!.width).toBeLessThanOrEqual(820);

    // On a phone, Escape closes the steps sheet first, a second one shows the chat.
    await page.setViewportSize({ width: 375, height: 812 });
    const steps = page.getByRole("dialog", nav("Design progress steps"));
    await questionsTab.click();
    await page.getByRole("button", { name: /^Resolve tickets/ }).click();
    await expect(steps).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(steps).toHaveCount(0);
    await expect(questionsTab).toHaveAttribute("aria-pressed", "true");
    await page.keyboard.press("Escape");
    await expect(chat).toBeVisible();
  });

  test("a round sent from Chat shows the answers, and the next round opens in Questions", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "1h" });
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    const questions = page.getByRole("group", nav("Questions"));
    const view = page.getByRole("group", nav("View"));
    await view.getByRole("button", nav("Chat")).click();
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await questions.getByRole("button", { name: /Yes/ }).click();
    await page.getByRole("button", nav("Send")).click();
    await expect(page.getByText("Q8b - Migrate at startup?: Yes")).toBeVisible();

    const cont = page.getByRole("button", { name: /^Continue/ });
    await expect(async () => {
      await cont.click({ timeout: 2000 });
      await expect(questions.getByText("Q9 - Keep going?")).toBeVisible({ timeout: 2000 });
    }).toPass();
    await expect(view.getByRole("button", { name: /^Questions/ })).toHaveAttribute("aria-pressed", "true");
  });

  test("a busy turn with no round has no View group and shows the chat", async ({ page, start }) => {
    await start("design-working");
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();

    await expect(page.getByRole("status").filter({ hasText: "Working:" })).toBeVisible();
    await expect(page.getByRole("group", nav("View"))).toHaveCount(0);
    await expect(page.getByRole("group", nav("Questions"))).toHaveCount(0);
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

  test("1440px: the Progress region shows each step in turn", async ({ page, start }) => {
    const fixture = await start("design");
    await page.setViewportSize({ width: 1440, height: 900 });
    await openDesign(page);
    await expectImplLink(page, "resolve-tickets");

    for (const [i, name] of steps.entries()) {
      await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", name);
      const state = name === "done" ? "done" : "current";
      await expect(step(page, name, state)).toBeVisible();
      await expect(step(page, steps[i - 1] ?? "resolve-tickets", "done")).toBeVisible();
      await expectImplLink(page, name);
    }
    for (const s of ["chart", ...steps]) await expect(step(page, s, "done")).toBeVisible();
  });

  test("375px: the step line follows the step and the link shows at Branch and Done", async ({ page, start }) => {
    const fixture = await start("design");
    await page.setViewportSize({ width: 375, height: 812 });
    await openDesign(page);
    await expectImplLink(page, "resolve-tickets");
    await expect(page.getByRole("button", { name: /^Resolve tickets/ })).toBeVisible();

    for (const name of steps) {
      await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", name);
      await expectImplLink(page, name);
    }
    await expect(page.getByRole("button", { name: /^Resolve tickets/ })).toHaveCount(0);
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

// The Auto button names each switch's state; the switches sit in the popover it opens.
const autoButton = (page: Page, state: "on" | "off") =>
  page.getByRole("button", { name: new RegExp(`^Auto: Auto-advance ${state}`) });
const flipAuto = async (page: Page, from: "on" | "off") => {
  await autoButton(page, from).click();
  await page.getByRole("dialog", nav("Auto")).getByRole("switch", { name: `Auto-advance ${from}` }).click();
  await page.keyboard.press("Escape");
};

test.describe("10. Auto-advance", () => {
  test("a fresh round follows the run end with no press", async ({ page, start }) => {
    await start("design", { autoAdvanceDelay: "3s" });
    await answerFirstRound(page);

    await expect(page.getByText(/Continuing in \ds/)).toBeVisible();
    await expect(page.getByRole("button", nav("Cancel"))).toBeVisible();
    await expect(page.getByRole("group", nav("Questions")).getByText("Q9 - Keep going?")).toBeVisible();
    await showChat(page);
    await expect(page.getByText("A fresh round.")).toBeVisible();
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
    await showChat(page);
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
    await expect(autoButton(page, "on")).toBeVisible();

    await flipAuto(page, "on");
    await expect(autoButton(page, "off")).toBeVisible();

    const questions = page.getByRole("group", nav("Questions"));
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await questions.getByRole("button", { name: /Yes/ }).click();
    await page.getByRole("button", nav("Send")).click();
    await expect(page.getByRole("button", { name: /^Continue/ })).toBeVisible();
    await expect(page.getByText(/Continuing in/)).toHaveCount(0);

    // Under load the run-end update can land after the first press ("the session changed"), so press again.
    await expect(async () => {
      if (await autoButton(page, "off").isVisible()) await flipAuto(page, "off");
      await expect(autoButton(page, "on")).toBeVisible({ timeout: 2000 });
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

// The ⋯ menu's items; opening it is a click on the trigger.
const actionsMenu = async (page: Page) => {
  await page.getByRole("button", nav("More actions")).click();
  return page.getByRole("menu", nav("More actions"));
};

test.describe("42. Stopped session card", () => {
  test("the card moves to Ready to design as Not started with a Design button", async ({ page, start }) => {
    const fixture = await start("ready");
    await startDesign(page, "stopped-design");
    await page.getByRole("link", nav("Epics")).click();

    await stopSession(fixture, "stopped-design");
    const ready = await section(page, "Ready to design");
    const stopped = ready.locator("[data-kind]").filter({ has: page.getByRole("link", nav("stopped-design")) });
    await expect(stopped).toContainText("Not started");
    await expect(stopped.getByRole("button", nav("Design"))).toBeVisible();
  });
});

test.describe("43. New session on a stopped Design", () => {
  test("the ⋯ menu starts a new session and no form opens", async ({ page, start }) => {
    const fixture = await start("ready");
    await startDesign(page, "retry-design");
    await stopSession(fixture, "retry-design");

    const menu = await actionsMenu(page);
    const item = menu.getByRole("menuitem", { name: /^New session/ });
    await expect(item).toHaveAccessibleName("New session Start a fresh session");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("textbox", nav("Epic slug"))).toHaveCount(0);

    await item.click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect.poll(async () => (await sessionRows(fixture, "retry-design")).length).toBe(2);
    await expect(page).toHaveURL(/#\/d\/demo:retry-design$/);
  });

  test("on a live session it asks first: Cancel starts nothing, OK starts a new session", async ({ page, start }) => {
    const fixture = await start("ready");
    await startDesign(page, "live-design");
    await expect.poll(async () => (await sessionRows(fixture, "live-design")).length).toBe(1);
    const [first] = await sessionRows(fixture, "live-design");
    const firstId = first.split("\t")[0].trim();

    const menu = await actionsMenu(page);
    const item = menu.getByRole("menuitem", { name: /^New session/ });
    await expect(item).toHaveAccessibleName("New session Stop this session and start a fresh one");

    page.once("dialog", (d) => d.dismiss());
    await item.click();
    await expect(async () => expect(await sessionRows(fixture, "live-design")).toHaveLength(1)).toPass({ timeout: 2000 });

    await actionsMenu(page);
    page.once("dialog", (d) => d.accept());
    await menu.getByRole("menuitem", { name: /^New session/ }).click();
    await expect.poll(async () => (await sessionRows(fixture, "live-design")).length).toBe(2);
    const rows = await sessionRows(fixture, "live-design");
    expect(rows.find((r) => r.split("\t")[0].trim() === firstId)).toMatch(/exited/);
    expect(rows.filter((r) => !/exited/.test(r))).toHaveLength(1);
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

  test("a charted design has no Delete design", async ({ page, start }) => {
    await start("design");
    await openDesign(page);
    await page.getByRole("button", nav("Start design session")).click();
    const menu = await actionsMenu(page);
    await expect(menu.getByRole("menuitem", { name: /^New session/ })).toBeVisible();
    await expect(menu.getByRole("menuitem", { name: /^Delete design/ })).toHaveCount(0);
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
  const ring = page.locator("[data-kind]").first();

  // Before a session the ring is muted and the stats line names the step, with a time and no cost.
  await expect(ring).toHaveAttribute("data-tone", "muted");
  await expect(ring).toHaveText(/%$/);
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
  // The Design page opens in the Questions view, so the first message shows after a tap on Chat.
  await showChat(page);
  await expect(page.getByText("Two questions before the spec.")).toBeVisible();
  await page.evaluate(() => (location.hash = "#/e/demo:design-epic"));
  await expect(ring).toHaveAttribute("data-tone", "running");
  await expect(page.getByTestId("comet")).toBeVisible();
  // A live session makes a design ticket's meta read "<type> · running" and adds a time.
  await fixture.cli("tickets", "set", "demo:design-epic/01", "--status", "claimed");
  await expect(page.getByRole("link", { name: /01 first implement · running · \d+[smh]$/ })).toBeVisible();
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
