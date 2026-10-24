import type { Page } from "@playwright/test";
import { expect, test } from "./fixture";

const nav = (name: string) => ({ name, exact: true });

interface Row {
  name: string;
  epic: string;
  id: string;
  /** What the queue card shows for the ticket. */
  onQueue: string;
  /** What the epic page row and the ticket page show. */
  reason: string;
  /** Acts on the ticket page. */
  recover?: (page: Page) => Promise<void>;
}

const rows: Row[] = [
  {
    name: "Answer",
    epic: "paused-epic",
    id: "03",
    onQueue: "Seeded pause: self-reported",
    reason: "Waiting for your answer",
    recover: async (page) => {
      await page.getByRole("textbox", { name: "Your answer" }).fill("go ahead");
      await page.getByRole("button", nav("Send answer")).click();
    },
  },
  {
    name: "Choose base",
    epic: "paused-epic",
    id: "04",
    onQueue: "Seeded pause: ambiguous-base",
    reason: "Ambiguous base",
    recover: async (page) => {
      await page.getByRole("button", nav("01")).click();
      await page.getByRole("button", nav("Choose base")).click();
    },
  },
  {
    name: "Retry",
    epic: "paused-epic",
    id: "06",
    onQueue: "Seeded pause: zero-commit",
    reason: "Agent stopped without a result",
    recover: async (page) => {
      await page.getByRole("button", nav("Retry")).click();
    },
  },
  {
    name: "Approve",
    epic: "approve-epic",
    id: "01",
    onQueue: "needs approval",
    reason: "Needs approval",
    recover: async (page) => {
      await page.getByRole("button", nav("Allow once")).click();
    },
  },
];

test.describe("3. Pause reasons and recovery", () => {
  for (const row of rows) {
    test(row.name, async ({ page, start }) => {
      const fixture = await start("paused");
      await fixture.cli("server", "queue", "add", "demo:paused-epic");

      await page.getByRole("link", nav("Queue")).click();
      await expect(page.getByText(row.onQueue)).toBeVisible();

      await page.getByRole("link", { name: row.epic }).click();
      if (row.epic === "paused-epic") await expect(page.getByText(row.reason)).toBeVisible();
      // The row's action link ("Answer →", "Review →") goes to the same page.
      await page.locator(`a[href$="/t/demo:${row.epic}/${row.id}"]`).filter({ hasNotText: "→" }).click();

      await expect(page.getByText(row.reason).first()).toBeVisible();
      await row.recover!(page);
      await expect(page.getByText(row.reason)).toHaveCount(0);
    });
  }

  test("Repair", async ({ page, start }) => {
    const fixture = await start("paused");
    await fixture.cli("server", "queue", "add", "demo:paused-epic");

    await page.getByRole("link", nav("Queue")).click();
    await expect(page.getByText("Run failed")).toBeVisible();
    await expect(page.getByText("Fix it in the TUI")).toHaveCount(2);

    await page.getByRole("link", { name: "paused-epic" }).click();
    await expect(page.getByText("repair · needs-repair · Run failed")).toBeVisible();
    await expect(page.getByText("Fix it in the TUI")).toHaveCount(2);

    await page.locator('a[href$="/t/demo:paused-epic/07"]').click();
    await expect(page.getByText("Run failed").first()).toBeVisible();
  });
});
