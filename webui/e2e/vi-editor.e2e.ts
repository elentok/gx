import type { Page } from "@playwright/test";
import { composer, go, messageBox, sessions } from "./agentsKit";
import { card, expect, type Fixture, nav, test, touch } from "./fixture";

const TOLERANCE = 1;

// `gx vimode` changes the server; the page follows, which the menu switch shows.
const setViMode = async (fixture: Fixture, page: Page, on: boolean) => {
  await fixture.cli("vimode", on ? "on" : "off");
  await page.getByRole("button", nav("Menu")).click();
  const item = page.getByRole("menu", nav("Menu")).getByRole("menuitemcheckbox", nav("Vi mode"));
  await expect(item).toHaveAttribute("aria-checked", String(on));
  await page.getByRole("button", nav("Menu")).click();
  await expect(page.getByRole("menu", nav("Menu"))).toHaveCount(0);
};

// Shows the design-working message box; returns the field.
const openMessageBox = async (page: Page) => {
  await expect(card(page, "design-epic")).toBeVisible();
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await page.getByRole("button", nav("Start design session")).click();
  await page.getByRole("button", nav("Note")).click();
  const field = page.getByRole("textbox", nav("Message"));
  // The field stays disabled while the session starts.
  await expect(field).toBeEnabled();
  return field;
};

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

test.describe("124. Vi editor: open, close and submit", () => {
  test("390px: the expand button shows on the message box", async ({ page, start }) => {
    await start("design-working");
    await page.setViewportSize({ width: 390, height: 800 });
    await openMessageBox(page);
    await expect(page.getByRole("button", nav("Open the note in a large editor"))).toBeVisible();
  });

  test("vi on: Cmd-G in the field opens the editor", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const field = await openMessageBox(page);
    await field.focus();
    await page.keyboard.press("ControlOrMeta+g");
    await expect(page.getByRole("dialog", nav("Message to the agent"))).toBeVisible();
  });

  test("vi on: Cmd-G with focus on the page body opens nothing", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    await openMessageBox(page);
    await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
    await page.keyboard.press("ControlOrMeta+g");
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("vi off: Cmd-G in the field opens nothing and is not cancelled", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, false);
    const field = await openMessageBox(page);
    await field.focus();
    await page.evaluate(() => {
      (window as unknown as { gPrevented: boolean | null }).gPrevented = null;
      window.addEventListener("keydown", (e) => {
        if (e.key.toLowerCase() === "g") (window as unknown as { gPrevented: boolean }).gPrevented = e.defaultPrevented;
      });
    });
    await page.keyboard.press("ControlOrMeta+g");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    expect(await page.evaluate(() => (window as unknown as { gPrevented: boolean | null }).gPrevented)).toBe(false);
  });

  test("the editor opens in INSERT at the field's caret", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const field = await openMessageBox(page);
    await field.fill("hello world");
    await field.evaluate((el: HTMLTextAreaElement) => el.setSelectionRange(5, 5));
    await page.keyboard.press("ControlOrMeta+g");
    const dialog = page.getByRole("dialog", nav("Message to the agent"));
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("vi-mode-text")).toHaveText("INSERT");
    // The editor loads lazily; typing before it takes focus would be lost.
    const editable = dialog.getByRole("textbox", nav("Message to the agent text"));
    await expect(editable).toBeFocused();
    await page.keyboard.type("X");
    await expect(editable).toHaveText("helloX world");
  });
});

// Counts the prompts the page posts, so a key that does nothing can be told from one that sent.
const countPrompts = (page: Page) => {
  const seen = { count: 0 };
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().includes("/v1/sessions/prompt")) seen.count++;
  });
  return seen;
};

const MESSAGE = "Message to the agent";

// Opens the design-working editor with `text` in the field and the caret after `caret` characters.
const openWith = async (page: Page, text: string, caret: number) => {
  await expect(card(page, "design-epic")).toBeVisible();
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await page.getByRole("button", nav("Start design session")).click();
  // The session is idle for a moment before its first turn starts, and that turn disables the field.
  // Wait the turn out so the field stays enabled for the whole test.
  const working = page.getByRole("status").filter({ hasText: "Working:" });
  await expect(working).toBeVisible();
  await expect(working).toBeHidden({ timeout: 45_000 });
  await page.getByRole("button", nav("Note")).click();
  const field = page.getByRole("textbox", nav("Message"));
  await expect(field).toBeEnabled();
  await field.fill(text);
  await field.evaluate((el: HTMLTextAreaElement, at) => el.setSelectionRange(at, at), caret);
  await page.keyboard.press("ControlOrMeta+g");
  const dialog = page.getByRole("dialog", nav(MESSAGE));
  await expect(dialog.getByRole("textbox", nav(`${MESSAGE} text`))).toBeFocused();
  return { field, dialog };
};

test.describe("124. Vi editor: open, close and submit", () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test("ZZ in normal mode closes the editor and keeps the text", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const { field } = await openWith(page, "", 0);
    await page.keyboard.type("kept text");
    await page.keyboard.press("Escape");
    await expect(page.getByRole("dialog", nav(MESSAGE))).toBeVisible();
    await page.keyboard.type("ZZ");
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(field).toHaveValue("kept text");
  });

  test("Cmd-G inside the editor closes it", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    await openWith(page, "", 0);
    await page.keyboard.press("ControlOrMeta+g");
    await expect(page.getByRole("dialog")).toHaveCount(0);
  });

  test("a backdrop press closes at desktop width and not at 390px", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    await openWith(page, "", 0);
    await page.mouse.click(5, 5);
    await expect(page.getByRole("dialog")).toHaveCount(0);

    await page.setViewportSize({ width: 390, height: 800 });
    await page.getByRole("button", nav("Open the note in a large editor")).click();
    await expect(page.getByRole("dialog", nav(MESSAGE))).toBeVisible();
    await page.mouse.click(1, 1);
    await expect(page.getByRole("dialog", nav(MESSAGE))).toBeVisible();
  });

  for (const on of [true, false]) {
    test(`1280px mouse, vi ${on ? "on" : "off"}: Done is visible and closes`, async ({ page, start }) => {
      const fixture = await start("design-working");
      await setViMode(fixture, page, on);
      const dialog = await openEditor(page, 1280, 900);
      await expectDoneCloses(page, dialog);
    });
  }

  test("Escape leaves the editor open in insert and in normal mode", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const { dialog } = await openWith(page, "", 0);
    await expect(dialog.getByTestId("vi-mode-text")).toHaveText("INSERT");
    await page.keyboard.press("Escape");
    await expect(dialog.getByTestId("vi-mode-text")).toHaveText("NORMAL");
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
    await page.keyboard.press("i");
    await expect(dialog.getByTestId("vi-mode-text")).toHaveText("INSERT");
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
  });

  // The caret is after the last character (11) on every path but ZZ: Escape in vi steps it back onto the last character.
  const closers: [string, number, (page: Page, dialog: ReturnType<Page["getByRole"]>) => Promise<void>][] = [
    ["Done", 11, (_, dialog) => dialog.getByRole("button", nav("Done")).click()],
    ["Cmd-G", 11, (page) => page.keyboard.press("ControlOrMeta+g")],
    [
      "ZZ",
      10,
      async (page) => {
        await page.keyboard.press("Escape");
        await page.keyboard.type("ZZ");
      },
    ],
    ["the backdrop", 11, (page) => page.mouse.click(5, 5)],
  ];
  for (const [name, caret, close] of closers) {
    test(`closing with ${name} focuses the field at the editor's caret`, async ({ page, start }) => {
      const fixture = await start("design-working", { autoAdvanceDelay: "1h" });
      await setViMode(fixture, page, true);
      const { field, dialog } = await openWith(page, "hello world", 5);
      await page.keyboard.press("End");
      await close(page, dialog);
      await expect(page.getByRole("dialog")).toHaveCount(0);
      await expect(field).toBeFocused();
      expect(await field.evaluate((el: HTMLTextAreaElement) => [el.selectionStart, el.selectionEnd])).toEqual([caret, caret]);
    });
  }

  test("typing in the editor updates the field and the sent request body", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const { field } = await openWith(page, "", 0);
    await page.keyboard.type("typed in the editor");
    await expect(field).toHaveValue("typed in the editor");
    await page.getByRole("dialog", nav(MESSAGE)).getByRole("button", nav("Done")).click();
    const sent = page.waitForRequest((r) => r.method() === "POST" && r.url().includes("/v1/sessions/prompt"));
    await page.getByRole("button", { name: "Send", exact: true }).click();
    expect((await sent).postData()).toContain("typed in the editor");
  });

  test("Cmd-Enter in the agent composer's editor closes it and sends", async ({ page, start }) => {
    const fixture = await start("agents");
    const idle = (await sessions(fixture))[0];
    await setViMode(fixture, page, true);
    await go(page, `#/agents/${idle.id}`);
    await messageBox(page).fill("sent from the editor");
    await composer(page).getByRole("button", nav("Open the note in a large editor")).click();
    const dialog = page.getByRole("dialog", { name: /^Message to / });
    await expect(dialog.getByRole("textbox", { name: /text$/ })).toBeFocused();
    const sent = page.waitForRequest((r) => r.method() === "POST" && r.url().includes("/v1/sessions/prompt"));
    await page.keyboard.press("ControlOrMeta+Enter");
    await sent;
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect.poll(() => fixture.cli("agents", "chat", idle.id)).toContain("sent from the editor");
  });

  test("Cmd-Enter in a question's answer editor does nothing", async ({ page, start }) => {
    const fixture = await start("design-text", { autoAdvanceDelay: "1h" });
    await setViMode(fixture, page, true);
    const prompts = countPrompts(page);
    await expect(card(page, "design-epic")).toBeVisible();
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
    await page.getByRole("button", nav("Start design session")).click();
    await expect(page.getByRole("textbox", nav("Answer to question 13"))).toBeVisible();
    await page.getByRole("group", nav("Questions")).getByRole("button", nav("Open the note in a large editor")).click();
    const answer = page.getByRole("dialog", { name: /^Answer to / });
    await expect(answer.getByRole("textbox")).toBeFocused();
    await page.keyboard.press("ControlOrMeta+Enter");
    await expect(answer).toBeVisible();
    expect(prompts.count).toBe(0);
  });

  test("390px: Cmd-Enter in the composer's editor does nothing", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const prompts = countPrompts(page);
    await page.setViewportSize({ width: 390, height: 800 });
    await openWith(page, "kept", 4);
    await page.keyboard.press("ControlOrMeta+Enter");
    await expect(page.getByRole("dialog", nav(MESSAGE))).toBeVisible();
    expect(prompts.count).toBe(0);
  });
});

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
