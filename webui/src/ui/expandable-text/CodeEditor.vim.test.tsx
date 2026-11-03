import { EditorView } from "@codemirror/view";
import { getCM, Vim } from "@replit/codemirror-vim";
import { act, cleanup, screen } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, describe, expect, test, vi } from "vitest";
import { ChakraProvider } from "@chakra-ui/react";
import { renderThemed } from "../render";
import { system } from "../theme";
import { AppToaster } from "../toaster";
import CodeEditor, { type CodeEditorHandle, type ViMode } from "./CodeEditor";

afterEach(cleanup);

const DOC = "one two\nthree four\nfive";

function mount(viMode: boolean, selection = { start: 0, end: 0 }, doc = DOC) {
  const onModeChange = vi.fn<(m: ViMode) => void>();
  const onZZ = vi.fn();
  const handle = createRef<CodeEditorHandle>();
  const props = (vi: boolean) => ({
    label: "Note text",
    value: doc,
    onChange: () => {},
    selection,
    onSelectionChange: () => {},
    viMode: vi,
    onModeChange,
    onZZ,
    handle,
  });
  const r = renderThemed(<CodeEditor {...props(viMode)} />);
  const view = EditorView.findFromDOM(screen.getByRole("textbox"))!;
  const keys = (k: string) => act(() => void Vim.handleKey(getCM(view)!, k, "user"));
  const text = () => view.state.doc.toString();
  const caret = () => view.state.selection.main.head;
  return { view, keys, text, caret, onModeChange, onZZ, handle, setVi: (v: boolean) =>
      r.rerender(
        <ChakraProvider value={system}>
          <CodeEditor {...props(v)} />
          <AppToaster />
        </ChakraProvider>,
      ),
  };
}

describe("vi on", () => {
  test("opens in INSERT, then Esc, v, Esc i report the modes", () => {
    const { keys, onModeChange } = mount(true);
    expect(onModeChange).toHaveBeenLastCalledWith("INSERT");
    keys("<Esc>");
    expect(onModeChange).toHaveBeenLastCalledWith("NORMAL");
    keys("v");
    expect(onModeChange).toHaveBeenLastCalledWith("VISUAL");
    keys("<Esc>");
    keys("i");
    expect(onModeChange).toHaveBeenLastCalledWith("INSERT");
  });

  test("opens with the field's caret", () => {
    const { caret } = mount(true, { start: 4, end: 4 });
    expect(caret()).toBe(4);
  });

  test("a letter in NORMAL does not insert text", () => {
    const { keys, text } = mount(true);
    keys("<Esc>");
    keys("q");
    expect(text()).toBe(DOC);
  });

  test.each(["", "v"])("gs and ge go to line start and end (prefix %j)", (prefix) => {
    const { keys, caret, view } = mount(true, { start: 11, end: 11 });
    keys("<Esc>");
    if (prefix) keys(prefix);
    keys("gs");
    expect(view.state.doc.lineAt(caret()).from).toBe(8);
    expect(caret()).toBe(8);
    keys("ge");
    expect(caret()).toBeGreaterThanOrEqual(view.state.doc.lineAt(caret()).to - 1);
    expect(view.state.doc.lineAt(caret()).number).toBe(2);
  });

  test("x, dw, yy p, u and Ctrl-R edit as in vim", () => {
    const { keys, text } = mount(true);
    keys("<Esc>");
    keys("x");
    expect(text()).toBe("ne two\nthree four\nfive");
    keys("d");
    keys("w");
    expect(text()).toBe("two\nthree four\nfive");
    keys("y");
    keys("y");
    keys("p");
    expect(text()).toBe("two\ntwo\nthree four\nfive");
    keys("u");
    expect(text()).toBe("two\nthree four\nfive");
    keys("<C-r>");
    expect(text()).toBe("two\ntwo\nthree four\nfive");
  });

  test("h l w e 0 f gg G move the caret as vim does", () => {
    const { keys, caret } = mount(true, { start: 0, end: 0 }, "alpha beta gamma\ndelta epsilon\nzeta");
    keys("<Esc>");
    const steps: [string, number][] = [
      ["l", 1],
      ["h", 0],
      ["w", 6],
      ["e", 9],
      ["0", 0],
      ["fg", 11],
      ["G", 31],
      ["gg", 0],
    ];
    for (const [k, at] of steps) {
      for (const key of k === "fg" || k === "gg" ? [k] : [...k]) keys(key);
      expect(caret(), k).toBe(at);
    }
  });

  test("i, a, o and . change the text and the mode", () => {
    const { keys, text, view, onModeChange } = mount(true, { start: 0, end: 0 }, "abc");
    const type = (s: string) => act(() => view.dispatch(view.state.replaceSelection(s)));
    keys("<Esc>");
    for (const [key, typed, after] of [
      ["i", "X", "Xabc"],
      ["a", "Y", "XYabc"],
      ["o", "Z", "XYabc\nZ"],
    ]) {
      keys(key);
      expect(onModeChange, key).toHaveBeenLastCalledWith("INSERT");
      type(typed);
      keys("<Esc>");
      expect(onModeChange, key).toHaveBeenLastCalledWith("NORMAL");
      expect(text(), key).toBe(after);
    }
    keys("gg");
    keys("x");
    keys(".");
    expect(text()).toBe("abc\nZ");
  });

  test("v d deletes the selection", () => {
    const { keys, text } = mount(true, { start: 0, end: 0 }, "two three");
    keys("<Esc>");
    keys("v");
    keys("l");
    keys("d");
    expect(text()).toBe("o three");
  });

  test("ZZ calls the close callback once", () => {
    const { keys, onZZ } = mount(true);
    keys("<Esc>");
    keys("ZZ");
    expect(onZZ).toHaveBeenCalledTimes(1);
  });

  test("Esc in NORMAL is not stopped by the engine", () => {
    const { view, keys } = mount(true);
    keys("<Esc>");
    const seen = vi.fn();
    document.body.addEventListener("keydown", seen);
    view.contentDOM.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
    document.body.removeEventListener("keydown", seen);
    expect(seen).toHaveBeenCalled();
  });
});

describe("vi off", () => {
  test("typing inserts letters and no mode is reported", () => {
    const { view, text, onModeChange } = mount(false);
    act(() => view.dispatch(view.state.replaceSelection("q")));
    expect(text()).toBe("q" + DOC);
    expect(onModeChange).not.toHaveBeenCalled();
  });
});

describe("toggling the setting on an open editor", () => {
  test("keeps text and selection, and vi keys stop and start", () => {
    const { view, text, setVi, onModeChange, keys } = mount(false, { start: 2, end: 5 });
    const sel = () => [view.state.selection.main.from, view.state.selection.main.to];
    setVi(true);
    expect(text()).toBe(DOC);
    expect(sel()).toEqual([2, 5]);
    expect(onModeChange).toHaveBeenLastCalledWith("INSERT");
    keys("<Esc>");
    expect(onModeChange).toHaveBeenLastCalledWith("NORMAL");
    setVi(false);
    expect(text()).toBe(DOC);
    expect(getCM(view)).toBeNull();
  });
});

describe("escape() handle", () => {
  test("leaves insert mode and keeps focus on the editor", () => {
    const { handle, onModeChange, view } = mount(true);
    act(() => handle.current!.escape());
    expect(onModeChange).toHaveBeenLastCalledWith("NORMAL");
    expect(view.hasFocus).toBe(true);
  });
});
