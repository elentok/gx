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
    await start("design");
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
  // Blocked: setting design_step on the epic publishes no ticket-changed event, so Progress never refetches.
  test.fixme("walks every step to Done", async ({ page, start }) => {
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
