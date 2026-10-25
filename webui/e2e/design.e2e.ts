import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

const nav = (name: string) => ({ name, exact: true });

const openDesign = async (page: Page) => {
  await page.getByRole("link", nav("Designs")).click();
  await expect(page.getByRole("button", nav("Start design session"))).toBeVisible();
};

const step = (page: Page, name: string, state: string) => page.locator(`li[data-step="${name}"][data-state="${state}"]`);

test.describe("4. Start a design session", () => {
  test("shows the first step", async ({ page, start }) => {
    await start("design");
    await openDesign(page);
    await expect(step(page, "resolve-tickets", "current")).toBeVisible();

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

    await cont.click();
    await expect(page.getByText("A fresh round.")).toBeVisible();
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

    await autoSwitch(page, "off").click();
    await expect(autoSwitch(page, "on")).toBeVisible();
    await expect(page.getByText(/Continuing in/)).toBeVisible();
  });
});
