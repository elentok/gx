import type { Page } from "@playwright/test";
import { composer, go, messageBox, sessions } from "./agentsKit";
import { card, expect, type Fixture, nav, section, test, touch } from "./fixture";

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
  await expect(field).toBeEnabled({ timeout: 45_000 });
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
    await expect(field).toHaveValue("hello world");
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
  await expect(field).toBeEnabled({ timeout: 45_000 });
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

    test("390px mouse, vi on: title header, footer with Done and mode text, no bar and no x button", async ({ page, start }) => {
      const fixture = await start("design-working");
      await setViMode(fixture, page, true);
      const dialog = await openEditor(page, 390, 800);
      await expect(dialog.getByText("Message to the agent", { exact: true })).toBeVisible();
      await expect(dialog.getByTestId("editor-bar")).toHaveCount(0);
      await expect(dialog.getByRole("status")).toHaveText("INSERT");
      await expect(dialog.getByRole("button")).toHaveCount(1);
      await expect(dialog.getByRole("button", nav("Done"))).toBeVisible();
    });

    test("1280px mouse: the mode text sits left of the footer and follows the mode; vi off has none", async ({ page, start }) => {
      const fixture = await start("design-working");
      await setViMode(fixture, page, true);
      const dialog = await openEditor(page, 1280, 900);
      const text = dialog.getByRole("status");
      await expect(text).toHaveText("INSERT");
      const saved = (await dialog.getByText("Saved as you type").boundingBox())!;
      expect((await text.boundingBox())!.x).toBeLessThan(saved.x);
      const editable = dialog.getByRole("textbox", nav("Message to the agent text"));
      await editable.click();
      await page.keyboard.press("Escape");
      await expect(text).toHaveText("NORMAL");
      await page.keyboard.press("i");
      await expect(text).toHaveText("INSERT");
      await expectDoneCloses(page, dialog);
    });

    test("1280px mouse, vi off: no mode text", async ({ page, start }) => {
      await start("design-working");
      const dialog = await openEditor(page, 1280, 900);
      await expect(dialog.getByRole("status")).toHaveCount(0);
    });
  });

  test.describe("touch, vi on", () => {
    test.use(touch);

    test("390px touch, vi on: the bar has the mode text, Esc and Done; Esc leaves insert; Done closes", async ({ page, start }) => {
      const fixture = await start("design-working");
      await setViMode(fixture, page, true);
      const dialog = await openEditor(page, 390, 800);
      const bar = dialog.getByTestId("editor-bar");
      const pill = bar.getByTestId("vi-mode-pill");
      const esc = bar.getByRole("button", nav("Esc"));
      const done = bar.getByRole("button", nav("Done"));
      await expect(pill).toHaveText("INSERT");
      await expect(bar.getByRole("button")).toHaveCount(2);
      await expect(esc).toBeVisible();
      await expect(done).toBeVisible();
      for (const target of [pill, esc, done]) {
        const box = (await target.boundingBox())!;
        expect(Math.abs(box.height - 44)).toBeLessThanOrEqual(TOLERANCE);
      }
      await esc.click();
      await expect(pill).toHaveText("NORMAL");
      await expectDoneCloses(page, dialog);
    });

    test("1280px touch, vi on: title header and bar, no footer, one Done and one mode text", async ({ page, start }) => {
      const fixture = await start("design-working");
      await setViMode(fixture, page, true);
      const dialog = await openEditor(page, 1280, 900);
      await expect(dialog.getByText("Message to the agent", { exact: true })).toBeVisible();
      await expect(dialog.getByTestId("editor-bar")).toBeVisible();
      await expect(dialog.getByText("Saved as you type")).toHaveCount(0);
      await expect(dialog.getByRole("button", nav("Done"))).toHaveCount(1);
      await expect(dialog.getByRole("status")).toHaveCount(1);
    });
  });
});

// The editor text, one line per `.cm-line`.
const editorText = async (editable: ReturnType<Page["getByRole"]>) => {
  await expect(editable).toBeVisible();
  return editable.evaluate((el) => [...el.querySelectorAll(".cm-line")].map((l) => l.textContent).join("\n"));
};

// [anchor, head] of the editor's selection as character offsets in the text; `\n` counts as one.
const editorSelection = async (editable: ReturnType<Page["getByRole"]>) => {
  await expect(editable).toBeVisible();
  return editable.evaluate((el) => {
    const lines = [...el.querySelectorAll(".cm-line")];
    const offset = (node: Node, at: number) => {
      const line = lines.findIndex((l) => l.contains(node));
      let n = 0;
      for (let i = 0; i < line; i++) n += (lines[i].textContent ?? "").length + 1;
      const range = document.createRange();
      range.setStart(lines[line], 0);
      range.setEnd(node, at);
      return n + range.toString().length;
    };
    const s = document.getSelection()!;
    return [offset(s.anchorNode!, s.anchorOffset), offset(s.focusNode!, s.focusOffset)];
  });
};

const head = async (editable: ReturnType<Page["getByRole"]>) => (await editorSelection(editable))[1];

// Opens the editor in vi mode on `text`, in NORMAL mode with the caret at the start.
const openNormal = async (page: Page, fixture: Fixture, text: string) => {
  await setViMode(fixture, page, true);
  const { dialog } = await openWith(page, text, 0);
  const editable = dialog.getByRole("textbox", nav(`${MESSAGE} text`));
  // The mode shows as text, or as the pill on a touch device.
  const mode = dialog.locator('[data-testid^="vi-mode-"]');
  await page.keyboard.press("Escape");
  await expect(mode).toHaveText("NORMAL");
  return { dialog, editable, mode };
};

test.describe("125. Vi editor: commands", () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test("the mode text follows INSERT, NORMAL, VISUAL", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const { dialog } = await openWith(page, "hello", 0);
    const mode = dialog.getByTestId("vi-mode-text");
    await expect(mode).toHaveText("INSERT");
    await page.keyboard.press("Escape");
    await expect(mode).toHaveText("NORMAL");
    await page.keyboard.press("v");
    await expect(mode).toHaveText("VISUAL");
    await page.keyboard.press("Escape");
    await page.keyboard.press("i");
    await expect(mode).toHaveText("INSERT");
  });

  test("h j k l w e 0 f{char} gg G move the caret as vim does", async ({ page, start }) => {
    const fixture = await start("design-working");
    const { editable } = await openNormal(page, fixture, "alpha beta gamma\ndelta epsilon\nzeta");
    // j and k follow the pixel column in a proportional font, so they land within a character of col 11.
    const steps: [string, number, number][] = [
      ["l", 1, 1],
      ["h", 0, 0],
      ["w", 6, 6],
      ["e", 9, 9],
      ["0", 0, 0],
      ["fg", 11, 11],
      ["j", 27, 29],
      ["k", 10, 12],
      ["G", 31, 31],
      ["gg", 0, 0],
    ];
    for (const [keys, min, max] of steps) {
      await page.keyboard.type(keys);
      await expect.poll(() => head(editable), { message: keys }).toBeGreaterThanOrEqual(min);
      expect(await head(editable), keys).toBeLessThanOrEqual(max);
    }
  });

  test("390px: j moves one screen line inside a wrapped paragraph", async ({ page, start }) => {
    const fixture = await start("design-working");
    await page.setViewportSize({ width: 390, height: 800 });
    const text = "word ".repeat(100).trim();
    const { dialog, editable } = await openNormal(page, fixture, text);
    const cursor = dialog.locator(".cm-cursor, .cm-fat-cursor").first();
    const before = (await cursor.boundingBox())!;
    await page.keyboard.press("j");
    await expect.poll(() => head(editable)).toBeGreaterThan(0);
    expect(await head(editable)).toBeLessThan(text.length / 2);
    const lineHeight = await editable.locator(".cm-line").evaluate((l) => parseFloat(getComputedStyle(l).lineHeight));
    // The cursor element repaints after the selection changes, so wait for it to move.
    await expect
      .poll(async () => Math.abs((await cursor.boundingBox())!.y - before.y - lineHeight))
      .toBeLessThanOrEqual(2);
  });

  test("a click resets the column that j and k keep", async ({ page, start }) => {
    const fixture = await start("design-working");
    const { editable } = await openNormal(page, fixture, "aaaaaaaaaa\nbbbbbbbbbb\ncccccccccc\ndddddddddd");
    await page.keyboard.type("5lj");
    await expect.poll(() => head(editable)).toBe(16);
    await editable.locator(".cm-line").nth(2).click({ position: { x: 1, y: 5 } });
    await expect.poll(() => head(editable)).toBe(22);
    await page.keyboard.press("j");
    await expect.poll(() => head(editable)).toBe(33);
  });

  test("x dw yy p u Ctrl-R iXY . and v d edit the text", async ({ page, start }) => {
    const fixture = await start("design-working");
    const { editable } = await openNormal(page, fixture, "one two three\nfour five");
    const steps: [string, string][] = [
      ["x", "ne two three\nfour five"],
      ["dw", "two three\nfour five"],
      ["yyp", "two three\ntwo three\nfour five"],
      ["u", "two three\nfour five"],
      ["Control+r", "two three\ntwo three\nfour five"],
      ["ggj0iXY", "two three\nXYtwo three\nfour five"],
      ["Escape", "two three\nXYtwo three\nfour five"],
      [".", "two three\nXXYYtwo three\nfour five"],
      ["0vld", "two three\nYYtwo three\nfour five"],
    ];
    for (const [keys, text] of steps) {
      if (keys.includes("+") || keys === "Escape") await page.keyboard.press(keys);
      else await page.keyboard.type(keys);
      await expect.poll(() => editorText(editable), { message: keys }).toBe(text);
    }
  });

  test("i a o and . change the text and the mode as in vim", async ({ page, start }) => {
    const fixture = await start("design-working");
    const { editable, mode } = await openNormal(page, fixture, "abc");
    await page.keyboard.type("iX");
    await expect(mode).toHaveText("INSERT");
    await page.keyboard.press("Escape");
    await expect(mode).toHaveText("NORMAL");
    await expect.poll(() => editorText(editable)).toBe("Xabc");
    await page.keyboard.type("aY");
    await expect(mode).toHaveText("INSERT");
    await page.keyboard.press("Escape");
    await expect.poll(() => editorText(editable)).toBe("XYabc");
    await page.keyboard.type("oZ");
    await expect(mode).toHaveText("INSERT");
    await page.keyboard.press("Escape");
    await expect(mode).toHaveText("NORMAL");
    await expect.poll(() => editorText(editable)).toBe("XYabc\nZ");
    await page.keyboard.type(".");
    await expect(mode).toHaveText("NORMAL");
    await expect.poll(() => editorText(editable)).toBe("XYabc\nZ\nZ");
  });

  test.describe("touch", () => {
    test.use(touch);

    test("390px: gs and ge move to the line start and end in normal and visual mode", async ({ page, start }) => {
      const fixture = await start("design-working");
      await page.setViewportSize({ width: 390, height: 800 });
      const { editable, mode } = await openNormal(page, fixture, "hello world\nsecond line");
      await page.keyboard.type("lll");
      await expect.poll(() => head(editable)).toBe(3);
      await page.keyboard.type("ge");
      await expect.poll(() => head(editable)).toBe(10);
      await page.keyboard.type("gs");
      await expect.poll(() => head(editable)).toBe(0);
      await page.keyboard.type("lllv");
      await expect(mode).toHaveText("VISUAL");
      await page.keyboard.type("ge");
      await expect.poll(() => head(editable)).toBeGreaterThanOrEqual(10);
      await page.keyboard.type("gs");
      await expect.poll(() => head(editable)).toBeLessThanOrEqual(3);
    });
  });
});

type Locator = ReturnType<Page["getByRole"]>;

// Presses `key` `times` times; after each press focus must sit inside `layer`.
const expectFocusStays = async (page: Page, layer: Locator, key: string, times: number) => {
  for (let i = 0; i < times; i++) {
    await page.keyboard.press(key);
    await expect.poll(() => layer.evaluate((el) => el.contains(document.activeElement))).toBe(true);
  }
};

const PAUSED_TICKET = "#/t/demo:paused-epic/08";
const START_OVER = "Start over reason";

// Opens the start-over dialog on the paused ticket; returns it.
const openStartOver = async (page: Page) => {
  await page.evaluate((hash) => (location.hash = hash), PAUSED_TICKET);
  await page.getByRole("form", { name: "Recovery actions" }).getByRole("button", nav("Start over")).click();
  const dialog = page.getByRole("dialog", nav("Start over"));
  await expect(dialog).toBeVisible();
  return dialog;
};

// Opens the large editor over the Reason field of the start-over dialog.
const openReasonEditor = async (page: Page, dialog: Locator) => {
  await dialog.getByRole("button", nav("Open the note in a large editor")).click();
  const editor = page.getByRole("dialog", nav(START_OVER));
  await expect(editor.getByRole("textbox", nav(`${START_OVER} text`))).toBeFocused();
  return editor;
};

test.describe("126. Vi editor: layers and focus", () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test("Escape in the editor over the question sheet closes neither", async ({ page, start }) => {
    const fixture = await start("design", { autoAdvanceDelay: "1h" });
    await setViMode(fixture, page, true);
    await expect(card(page, "design-epic")).toBeVisible();
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
    await page.getByRole("button", nav("Start design session")).click();
    const questions = page.getByRole("group", nav("Questions"));
    // Answering Q8 moves the sheet on to Q8b.
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await questions.getByRole("button", nav("Note on question 8b")).click();
    await questions.getByRole("button", nav("Open the note in a large editor")).click();
    const editor = page.getByRole("dialog", nav("Note on Q8b - Migrate at startup?"));
    await expect(editor.getByRole("textbox", nav("Note on Q8b - Migrate at startup? text"))).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(editor).toBeVisible();
    await expect(questions).toBeVisible();
    // Escape in the Questions view would show the chat; it did not.
    await expect(page.getByRole("group", nav("View")).getByRole("button", { name: /^Questions/ })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  test("a backdrop press closes only the editor over the start-over dialog", async ({ page, start }) => {
    const fixture = await start("paused");
    await setViMode(fixture, page, true);
    const dialog = await openStartOver(page);
    const editor = await openReasonEditor(page, dialog);
    await page.mouse.click(5, 5);
    await expect(editor).toHaveCount(0);
    await expect(dialog).toBeVisible();
  });

  test("Cmd-K does not open the Jump menu over the editor; it does once the editor is closed", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const dialog = await openEditor(page, 1280, 900);
    const jump = page.getByRole("dialog", nav("Jump to epic"));
    await page.keyboard.press("ControlOrMeta+k");
    await expect(jump).toHaveCount(0);
    await expect(dialog).toBeVisible();
    await expectDoneCloses(page, dialog);
    await page.keyboard.press("ControlOrMeta+k");
    await expect(jump).toBeVisible();
  });

  test("Tab and Shift-Tab never leave the editor; Tab leaves no tab character", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const dialog = await openEditor(page, 1280, 900);
    const editable = dialog.getByRole("textbox", nav("Message to the agent text"));
    await expect(editable).toBeFocused();
    await page.keyboard.type("ab");
    await expectFocusStays(page, dialog, "Tab", 6);
    await expectFocusStays(page, dialog, "Shift+Tab", 6);
    await editable.focus();
    await page.keyboard.press("Tab");
    expect(await editable.evaluate((el) => el.textContent)).not.toContain("\t");
    await expect(editable).toHaveText("ab");
  });

  test.describe("with touch", () => {
    test.use(touch);

    test("Tab cycles through the editor, Esc and Done", async ({ page, start }) => {
      const fixture = await start("design-working");
      await setViMode(fixture, page, true);
      const dialog = await openEditor(page, 1280, 900);
      const seen = new Set<string>();
      for (let i = 0; i < 6; i++) {
        await page.keyboard.press("Tab");
        await expect.poll(() => dialog.evaluate((el) => el.contains(document.activeElement))).toBe(true);
        seen.add(await page.evaluate(() => document.activeElement?.textContent || document.activeElement?.getAttribute("aria-label") || "editor"));
      }
      expect([...seen]).toEqual(expect.arrayContaining(["Done", "Esc"]));
      await expectFocusStays(page, dialog, "Shift+Tab", 6);
    });

    test("Shift-Tab from the editor wraps to Done; Tab from Done returns to the editor", async ({ page, start }) => {
      const fixture = await start("design-working");
      await setViMode(fixture, page, true);
      const dialog = await openEditor(page, 1280, 900);
      const editable = dialog.getByRole("textbox", nav("Message to the agent text"));
      await editable.focus();
      await page.keyboard.press("Shift+Tab");
      await expect(dialog.getByRole("button", { name: "Done" })).toBeFocused();
      await page.keyboard.press("Tab");
      await expect(editable).toBeFocused();
    });
  });

  test("the editor is a named dialog with a named editable area", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const dialog = await openEditor(page, 1280, 900);
    await expect(page.getByRole("dialog", { name: MESSAGE })).toHaveCount(1);
    await expect(dialog.getByRole("textbox", { name: `${MESSAGE} text` })).toBeVisible();
  });

  test("Tab never leaves a plain modal", async ({ page, start }) => {
    await start("united");
    const ready = await section(page, "Ready to design");
    const first = ready.locator("[data-kind]").first();
    await first.getByRole("button", nav("Triage")).click();
    const dialog = page.getByRole("dialog", { name: "Triage" });
    await expect(dialog).toBeVisible();
    await expectFocusStays(page, dialog, "Tab", 8);
    await expectFocusStays(page, dialog, "Shift+Tab", 8);
  });

  test("Escape closes the start-over dialog, Tab stays in it, and the editor over it keeps both open", async ({
    page,
    start,
  }) => {
    const fixture = await start("paused");
    await setViMode(fixture, page, true);
    const dialog = await openStartOver(page);
    await expectFocusStays(page, dialog, "Tab", 8);
    await expectFocusStays(page, dialog, "Shift+Tab", 8);
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);

    const again = await openStartOver(page);
    const editor = await openReasonEditor(page, again);
    await page.keyboard.press("Escape");
    await expect(editor).toBeVisible();
    await expect(again).toBeVisible();
  });
});

const menuSwitch = (page: Page) => page.getByRole("menu", nav("Menu")).getByRole("menuitemcheckbox", nav("Vi mode"));

test.describe("127. Vi editor: setting", () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test("the menu has a Vi mode switch that turns on", async ({ page, start }) => {
    await start("design-working");
    await expect(card(page, "design-epic")).toBeVisible();
    await page.getByRole("button", nav("Menu")).click();
    const item = menuSwitch(page);
    await expect(item).toBeVisible();
    await expect(item).toHaveAttribute("aria-checked", "false");
    await item.click();
    await expect(item).toHaveAttribute("aria-checked", "true");
  });

  test("a second page of the same login shows the switch on", async ({ page, start }) => {
    await start("design-working");
    await expect(card(page, "design-epic")).toBeVisible();
    await page.getByRole("button", nav("Menu")).click();
    await menuSwitch(page).click();
    await expect(menuSwitch(page)).toHaveAttribute("aria-checked", "true");

    const second = await page.context().newPage();
    await second.goto(page.url());
    await expect(card(second, "design-epic")).toBeVisible();
    await second.getByRole("button", nav("Menu")).click();
    await expect(menuSwitch(second)).toHaveAttribute("aria-checked", "true");
    await second.close();
  });

  test("flipping the setting with the editor open shows and hides vi at once", async ({ page, start }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const { dialog } = await openWith(page, "hello", 2);
    const editable = dialog.getByRole("textbox", nav(`${MESSAGE} text`));
    const mode = dialog.getByTestId("vi-mode-text");
    const same = async () => {
      expect(await editorText(editable)).toBe("hello");
      expect(await editorSelection(editable)).toEqual([2, 2]);
    };
    await expect(mode).toHaveText("INSERT");
    await same();

    await fixture.cli("vimode", "off");
    await expect(mode).toHaveCount(0);
    await same();
    // Without vi, Escape does nothing and `x` types a letter.
    await page.keyboard.press("Escape");
    await page.keyboard.press("x");
    await expect(dialog).toBeVisible();
    expect(await editorText(editable)).toBe("hexllo");
    await page.keyboard.press("Backspace");
    await same();

    await fixture.cli("vimode", "on");
    await expect(mode).toHaveText("INSERT");
    await same();
    await page.keyboard.press("Escape");
    await expect(mode).toHaveText("NORMAL");
    await page.keyboard.press("x");
    // Vi `x` deletes the character under the caret instead of typing.
    expect(await editorText(editable)).toBe("hllo");
  });
});
