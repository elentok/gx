import { getCM, Vim } from "@replit/codemirror-vim";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../../connection";
import { WideContext } from "../../useWide";
import { renderThemed } from "../render";
import { ExpandableTextField } from "./ExpandableTextField";
import { findLargeEditor, largeEditorText, typeInLargeEditor, viewOf } from "./testing";

afterEach(cleanup);

const TITLE = "Note on Q2 - Pick";
const expand = () => screen.getByLabelText("Open the note in a large editor");
const field = () => screen.getByLabelText("Note") as HTMLTextAreaElement;

function Harness({
  onChange = () => {},
  disabled,
}: {
  onChange?: (t: string) => void;
  disabled?: boolean;
}) {
  const [text, setText] = useState("");
  return (
    <ExpandableTextField
      label="Note"
      title={TITLE}
      rows={3}
      placeholder="Type"
      disabled={disabled}
      value={text}
      onChange={(t) => (setText(t), onChange(t))}
    />
  );
}

test("the textarea gets value, rows, label, placeholder and disabled", () => {
  renderThemed(
    <ExpandableTextField
      label="Note"
      title={TITLE}
      rows={3}
      placeholder="Type"
      disabled
      value="hi"
      onChange={() => {}}
    />,
  );
  const area = field();
  expect(area.value).toBe("hi");
  expect(area.rows).toBe(3);
  expect(area.placeholder).toBe("Type");
  expect(area.disabled).toBe(true);
});

test("typing calls onChange", () => {
  const onChange = vi.fn();
  renderThemed(<Harness onChange={onChange} />);
  fireEvent.change(field(), { target: { value: "abc" } });
  expect(onChange).toHaveBeenCalledWith("abc");
});

test("the expand button opens a dialog named by the title; the editor is a multiline textbox named '<title> text'", async () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe(TITLE);
  const editor = await findLargeEditor(TITLE);
  expect(editor.getAttribute("aria-label")).toBe(`${TITLE} text`);
  expect(editor.getAttribute("aria-multiline")).toBe("true");
});

test("the editor edits the same text and Done closes it", async () => {
  const onChange = vi.fn();
  renderThemed(<Harness onChange={onChange} />);
  fireEvent.click(expand());
  await typeInLargeEditor(TITLE, "typed");
  expect(onChange).toHaveBeenCalledWith("typed");
  fireEvent.click(screen.getByText("Done"));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(field().value).toBe("typed");
});

test("the editor shows a new value prop and keeps the caret: no copy", async () => {
  const onChange = vi.fn();
  function Parent() {
    const [text, setText] = useState("hello world");
    return (
      <>
        <button onClick={() => setText("hello world!")}>Set new</button>
        <ExpandableTextField
          label="Note"
          title={TITLE}
          value={text}
          onChange={onChange}
        />
      </>
    );
  }
  renderThemed(<Parent />);
  field().setSelectionRange(2, 2);
  fireEvent.click(expand());
  await findLargeEditor(TITLE);
  expect(largeEditorText(TITLE)).toBe("hello world");
  fireEvent.click(screen.getByText("Set new"));
  expect(largeEditorText(TITLE)).toBe("hello world!");
  expect(viewOf(TITLE).state.selection.main.head).toBe(2);
  expect(onChange).not.toHaveBeenCalled();
});

test("Tab does not insert a tab character", async () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  const editor = await findLargeEditor(TITLE);
  await typeInLargeEditor(TITLE, "abc");
  fireEvent.keyDown(editor, { key: "Tab" });
  fireEvent.keyDown(editor, { key: "Tab", shiftKey: true });
  expect(largeEditorText(TITLE)).toBe("abc");
});

test("the placeholder shows while the editor is empty", async () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  await findLargeEditor(TITLE);
  expect(screen.getByRole("dialog").querySelector(".cm-placeholder")?.textContent).toBe("Type");
});

test("a disabled field has a disabled expand button", () => {
  renderThemed(<Harness disabled />);
  expect((expand() as HTMLButtonElement).disabled).toBe(true);
});

test("the selection goes to the editor on open and back to the field on close", async () => {
  renderThemed(
    <ExpandableTextField
      label="Note"
      title={TITLE}
      value="hello world"
      onChange={() => {}}
    />,
  );
  field().setSelectionRange(2, 7);
  fireEvent.click(expand());
  await findLargeEditor(TITLE);
  const sel = () => {
    const { from, to } = viewOf(TITLE).state.selection.main;
    return [from, to];
  };
  expect(sel()).toEqual([2, 7]);
  viewOf(TITLE).dispatch({ selection: { anchor: 1, head: 4 } });
  fireEvent.click(screen.getByText("Done"));
  expect([field().selectionStart, field().selectionEnd]).toEqual([1, 4]);
});

test("Escape does not close the editor", () => {
  renderThemed(<Harness />);
  fireEvent.click(expand());
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.getByRole("dialog")).toBeTruthy();
});

function withVi(on: boolean, ui: ReactNode) {
  const state = { snapshot: { sessions: [], vi_mode: on }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  return renderThemed(
    <WideContext.Provider value>
      <ConnectionContext.Provider value={conn}>{ui}</ConnectionContext.Provider>
    </WideContext.Provider>,
  );
}

function Keys({
  disabled,
  onSubmit,
  onKeyDown,
}: {
  disabled?: boolean;
  onSubmit?: () => void;
  onKeyDown?: () => void;
}) {
  const [text, setText] = useState("hello world");
  return (
    <form onKeyDown={onKeyDown}>
      <ExpandableTextField label="Note" title={TITLE} value={text} onChange={setText} disabled={disabled} onSubmit={onSubmit} />
    </form>
  );
}

function stubPlatform(platform: string) {
  vi.spyOn(navigator, "platform", "get").mockReturnValue(platform);
}

afterEach(() => vi.restoreAllMocks());

test("Cmd-G on a Mac and Ctrl-G elsewhere open the editor and cancel the browser default", async () => {
  for (const [platform, mod, other] of [
    ["MacIntel", "metaKey", "ctrlKey"],
    ["Win32", "ctrlKey", "metaKey"],
  ] as const) {
    stubPlatform(platform);
    withVi(true, <Keys />);
    field().focus();
    expect(fireEvent.keyDown(field(), { key: "g", [other]: true })).toBe(true);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(fireEvent.keyDown(field(), { key: "g", [mod]: true })).toBe(false);
    await findLargeEditor(TITLE);
    cleanup();
  }
});

test("with vi mode off the key is not handled", () => {
  stubPlatform("MacIntel");
  withVi(false, <Keys />);
  expect(fireEvent.keyDown(field(), { key: "g", metaKey: true })).toBe(true);
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("with vi mode on, the key with focus on the page body opens nothing", () => {
  stubPlatform("MacIntel");
  withVi(true, <Keys />);
  document.body.focus();
  fireEvent.keyDown(document.body, { key: "g", metaKey: true });
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("a disabled field opens nothing", () => {
  stubPlatform("MacIntel");
  withVi(true, <Keys disabled />);
  fireEvent.keyDown(field(), { key: "g", metaKey: true });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect((expand() as HTMLButtonElement).disabled).toBe(true);
});

test("the open key inside the editor closes it and cancels the default", async () => {
  withVi(true, <Keys />);
  fireEvent.click(expand());
  const editor = await findLargeEditor(TITLE);
  // CodeMirror reads Mod as Ctrl in jsdom.
  expect(fireEvent.keyDown(editor, { key: "g", ctrlKey: true })).toBe(false);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(field());
});

test("Cmd-Enter in the editor closes first, then submits once, without refocus or the field's handler", async () => {
  const onKeyDown = vi.fn();
  const onSubmit = vi.fn();
  withVi(true, <Keys onSubmit={onSubmit} onKeyDown={onKeyDown} />);
  const focus = vi.spyOn(field(), "focus");
  fireEvent.click(expand());
  const editor = await findLargeEditor(TITLE);
  fireEvent.keyDown(editor, { key: "Enter", ctrlKey: true });
  expect(onSubmit).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(focus).not.toHaveBeenCalled();
  expect(onKeyDown).not.toHaveBeenCalled();
});

test("Cmd-Enter without a submit action leaves the editor open", async () => {
  const onKeyDown = vi.fn();
  withVi(true, <Keys onKeyDown={onKeyDown} />);
  fireEvent.click(expand());
  const editor = await findLargeEditor(TITLE);
  fireEvent.keyDown(editor, { key: "Enter", ctrlKey: true });
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(onKeyDown).not.toHaveBeenCalled();
});

test("Done, the open key, a backdrop press and ZZ each refocus the field with the editor's selection, before the dialog goes", async () => {
  const closers: [string, () => void][] = [
    ["Done", () => fireEvent.click(screen.getByText("Done"))],
    // CodeMirror reads Mod as Ctrl in jsdom.
    ["Cmd-G", () => void fireEvent.keyDown(viewOf(TITLE).contentDOM, { key: "g", ctrlKey: true })],
    ["backdrop", () => {
      const backdrop = screen.getByRole("dialog").parentElement!;
      fireEvent.mouseDown(backdrop);
      fireEvent.click(backdrop);
    }],
    ["ZZ", () => {
      const cm = getCM(viewOf(TITLE))!;
      act(() => void Vim.handleKey(cm, "ZZ", "user"));
    }],
  ];
  for (const [name, close] of closers) {
    withVi(true, <Keys />);
    field().setSelectionRange(1, 3);
    fireEvent.click(expand());
    await findLargeEditor(TITLE);
    // Vim's Escape moves the caret, so the expectation is read after it.
    viewOf(TITLE).dispatch({ selection: { anchor: 2, head: 5 } });
    act(() => void Vim.handleKey(getCM(viewOf(TITLE))!, "<Esc>", "user"));
    const { from, to } = viewOf(TITLE).state.selection.main;
    let dialogAtFocus: boolean | undefined;
    vi.spyOn(field(), "focus").mockImplementation(() => {
      dialogAtFocus = !!screen.queryByRole("dialog");
    });
    close();
    await waitFor(() => expect(screen.queryByRole("dialog"), name).toBeNull());
    expect(dialogAtFocus, name).toBe(true);
    expect([field().selectionStart, field().selectionEnd], name).toEqual([from, to]);
    cleanup();
    vi.restoreAllMocks();
  }
});

const entries = ["compact", "context", "help"].map((name) => ({ name, aliases: [], arg_hint: "", description: name, source: "built-in" }));
const fetcher = async () => ({ degraded: false, entries }) as never;

function Completing({ harness }: { harness: string }) {
  const [text, setText] = useState("");
  return (
    <ExpandableTextField
      label="Note"
      title={TITLE}
      value={text}
      onChange={setText}
      completion={{ target: { sessionId: `s-${harness}` }, harness, fetcher }}
    />
  );
}

const options = () => screen.queryAllByRole("option").map((o) => o.textContent);

async function openEditor(harness = "claude") {
  withVi(true, <Completing harness={harness} />);
  fireEvent.click(expand());
  return findLargeEditor(TITLE);
}

test("the menu opens in the large editor on /, Down then Enter fills it", async () => {
  const editor = await openEditor();
  await typeInLargeEditor(TITLE, "/co");
  await waitFor(() => expect(options()).toHaveLength(2));
  fireEvent.keyDown(editor, { key: "ArrowDown" });
  fireEvent.keyDown(editor, { key: "Enter" });
  expect(largeEditorText(TITLE)).toBe("/context ");
  expect(viewOf(TITLE).state.selection.main.head).toBe("/context ".length);
  expect(screen.queryByRole("listbox")).toBeNull();
});

test("Escape closes the menu first and a second Escape leaves INSERT", async () => {
  const editor = await openEditor();
  await typeInLargeEditor(TITLE, "/co");
  await waitFor(() => expect(options()).toHaveLength(2));
  fireEvent.keyDown(editor, { key: "Escape" });
  expect(screen.queryByRole("listbox")).toBeNull();
  expect(screen.getByText("INSERT")).toBeTruthy();
  act(() => void Vim.handleKey(getCM(viewOf(TITLE))!, "<Esc>", "user"));
  expect(screen.getByText("NORMAL")).toBeTruthy();
});

test("the menu stays shut in NORMAL and returns in INSERT", async () => {
  await openEditor();
  await typeInLargeEditor(TITLE, "/co");
  await waitFor(() => expect(options()).toHaveLength(2));
  const cm = getCM(viewOf(TITLE))!;
  act(() => void Vim.handleKey(cm, "<Esc>", "user"));
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  act(() => void Vim.handleKey(cm, "j", "user"));
  expect(screen.queryByRole("listbox")).toBeNull();
  act(() => void Vim.handleKey(cm, "i", "user"));
  await waitFor(() => expect(options()).toHaveLength(2));
});

test("$ opens the menu in the editor for Codex only", async () => {
  await openEditor("codex");
  await typeInLargeEditor(TITLE, "$co");
  await waitFor(() => expect(options()).toHaveLength(2));
  cleanup();
  await openEditor("claude");
  await typeInLargeEditor(TITLE, "$co");
  expect(screen.queryByRole("listbox")).toBeNull();
});
