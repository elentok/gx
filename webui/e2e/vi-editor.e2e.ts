import type { Page } from "@playwright/test";
import { card, expect, nav, test, touch } from "./fixture";

const TOLERANCE = 1;

// Opens the design-working message box in the large editor.
const openEditor = async (page: Page, width: number, height = 900) => {
  await page.setViewportSize({ width, height });
  await expect(card(page, "design-epic")).toBeVisible();
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await page.getByRole("button", nav("Start design session")).click();
  await page.getByRole("button", nav("Note")).click();
  await page.getByRole("button", nav("Open the note in a large editor")).click();
  const dialog = page.getByRole("dialog", nav("Message to the agent"));
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("textbox", nav("Message to the agent text"))).toBeVisible();
  return dialog;
};

const expectDoneCloses = async (page: Page, dialog: ReturnType<Page["getByRole"]>) => {
  const done = dialog.getByRole("button", nav("Done"));
  await expect(done).toBeVisible();
  await done.click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
};

test.describe("123. Vi editor: layouts and touch bar", () => {
  test.describe("touch", () => {
    test.use(touch);

    test("a touch device reports a coarse pointer", async ({ page, start }) => {
      await start("design-working");
      expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    });

    test("390px touch, vi off: Done closes the editor; the bar has Done only", async ({ page, start }) => {
      await start("design-working");
      const dialog = await openEditor(page, 390, 800);
      const bar = dialog.getByTestId("editor-bar");
      await expect(bar).toBeVisible();
      await expect(bar.getByRole("button")).toHaveCount(1);
      await expect(bar.getByRole("button", nav("Done"))).toBeVisible();
      await expect(bar.getByText(/Esc/)).toHaveCount(0);
      await expect(dialog.getByTestId("vi-mode-pill")).toHaveCount(0);
      await expectDoneCloses(page, dialog);
    });

    test("1280px touch: the bar shows and the modal is at most 680px high", async ({ page, start }) => {
      await start("design-working");
      const dialog = await openEditor(page, 1280, 900);
      await expect(dialog.getByTestId("editor-bar")).toBeVisible();
      const box = (await dialog.boundingBox())!;
      expect(box.height).toBeLessThanOrEqual(680 + TOLERANCE);
    });
  });

  test.describe("mouse", () => {
    test("1280px mouse, vi off: Done closes the editor", async ({ page, start }) => {
      await start("design-working");
      const dialog = await openEditor(page, 1280, 900);
      await expectDoneCloses(page, dialog);
    });

    test("1280px mouse: no touch bar", async ({ page, start }) => {
      await start("design-working");
      const dialog = await openEditor(page, 1280, 900);
      await expect(dialog.getByTestId("editor-bar")).toHaveCount(0);
    });

    test("1280px mouse: a 860 by 680 modal with title, footer, Done and no x button", async ({ page, start }) => {
      await start("design-working");
      const dialog = await openEditor(page, 1280, 900);
      const box = (await dialog.boundingBox())!;
      expect(Math.abs(box.width - 860)).toBeLessThanOrEqual(TOLERANCE);
      expect(Math.abs(box.height - 680)).toBeLessThanOrEqual(TOLERANCE);
      await expect(dialog.getByText("Message to the agent", { exact: true })).toBeVisible();
      await expect(dialog.getByText("Saved as you type")).toBeVisible();
      await expect(dialog.getByRole("button", nav("Done"))).toBeVisible();
      await expect(dialog.getByRole("button")).toHaveCount(1);
    });
  });
});
