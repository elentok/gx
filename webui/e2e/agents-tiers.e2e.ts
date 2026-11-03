import type { Page } from "@playwright/test";
// Flow 96 moved to AgentsPage.sessionView.test.tsx; flow 98 merged into flow 85 (agents-create.e2e.ts).
import { created, createButton, go, newForm, projectSelect, promptBox, sessions } from "./agentsKit";
import { expect, flag, test } from "./fixture";

const tierSelect = (page: Page) => newForm(page).getByRole("combobox", { name: "Tier" });

const newSession = async (page: Page) => {
  await page.getByRole("button", { name: "+ New session", exact: true }).click();
  await expect(projectSelect(page)).toHaveValue("demo");
};

test("95. New Agents tab session tiers", async ({ page, start }) => {
  const fixture = await start("design-model-tiers");
  const before = await sessions(fixture);
  const launched = (await fixture.launches()).length;
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/agents");
  await newSession(page);

  // Three tiers, each with the model it runs under; Worker is the default.
  await expect(tierSelect(page).locator("option")).toHaveText(["Light (haiku)", "Worker (sonnet)", "Strongest (opus)"]);
  await expect(tierSelect(page)).toHaveValue("worker");
  await promptBox(page).fill("worker words");
  await createButton(page).click();
  const worker = await created(fixture, before);
  await expect.poll(async () => (await fixture.launches()).length).toBe(launched + 1);
  let args = (await fixture.launches()).at(-1)!;
  expect(flag(args, "model")).toBe("sonnet");
  expect(flag(args, "effort")).toBe("medium");

  // Strongest launches with the strongest model and effort.
  await newSession(page);
  await tierSelect(page).selectOption("strongest");
  await promptBox(page).fill("strongest words");
  await createButton(page).click();
  const strongest = await created(fixture, [...before, worker]);
  expect(strongest.tier).toBe("strongest");
  await expect.poll(async () => (await fixture.launches()).length).toBe(launched + 2);
  args = (await fixture.launches()).at(-1)!;
  expect(flag(args, "model")).toBe("opus");
  expect(flag(args, "effort")).toBe("max");

  // Resume keeps the tier.
  await fixture.cli("agents", "stop", strongest.id);
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === strongest.id)?.status.state).toBe("exited");
  const resumed = (await fixture.cli("agents", "resume", strongest.id)).trim();
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === resumed)?.tier).toBe("strongest");
  await go(page, `#/agents/${resumed}`);
  await expect(page.getByText("opus · max").first()).toBeVisible();
});

test("97. Session model label follows the handoff", async ({ page, start }) => {
  const fixture = await start("charting-handoff", { autoAdvanceDelay: "1s" });
  await page.setViewportSize({ width: 1280, height: 800 });
  const auto = (await sessions(fixture)).find((s) => s.ticket?.includes("handoff-auto"))!;
  await go(page, `#/agents/${auto.id}`);
  await expect(page.getByText("sonnet · medium", { exact: true })).toBeVisible();
});
