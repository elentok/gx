import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { Compartment, EditorState, Prec } from "@codemirror/state";
import { EditorView, keymap, placeholder as placeholderExt } from "@codemirror/view";
import { getCM, vim, Vim } from "@replit/codemirror-vim";
import { Box } from "@chakra-ui/react";
import { useEffect, useImperativeHandle, useRef, type Ref } from "react";
import type { Selection } from "./LargeEditor";
import { fieldLook } from "./look";

export type ViMode = "INSERT" | "NORMAL" | "VISUAL";

/** What the host can ask of the editor. */
export type CodeEditorHandle = {
  /** Sends vi Escape and keeps focus in the editor. */
  escape: () => void;
};

// The vim registry is module-level, so ZZ finds its editor's callback through the view it fires in.
const closers = new WeakMap<object, () => void>();
let mapped = false;

function mapOnce() {
  if (mapped) return;
  mapped = true;
  for (const ctx of ["normal", "visual"]) {
    // Screen lines, not paragraphs.
    Vim.map("j", "gj", ctx);
    Vim.map("k", "gk", ctx);
    Vim.map("gs", "0", ctx);
    // Replaces vim's "end of previous word".
    Vim.map("ge", "$", ctx);
  }
  Vim.defineAction("closeEditor", (cm: object) => closers.get(cm)?.());
  Vim.mapCommand("ZZ", "action", "closeEditor", {}, { context: "normal" });
}

function modeOf(mode: string): ViMode {
  if (mode === "visual") return "VISUAL";
  return mode === "insert" || mode === "replace" ? "INSERT" : "NORMAL";
}

const theme = EditorView.theme({
  "&": { height: "100%", fontSize: fieldLook.fontSize, color: "inherit", backgroundColor: "transparent" },
  "&.cm-focused": { outline: "none" },
  ".cm-scroller": { fontFamily: "inherit", lineHeight: "1.55", overflow: "auto" },
  ".cm-content": { padding: "12px" },
  ".cm-line": { padding: "0" },
  ".cm-placeholder": { color: "var(--chakra-colors-text-dim)" },
});

/** The first and last differing positions, so an outside edit keeps the caret where it was. */
function diff(a: string, b: string) {
  let start = 0;
  const max = Math.min(a.length, b.length);
  while (start < max && a[start] === b[start]) start++;
  let endA = a.length;
  let endB = b.length;
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--;
    endB--;
  }
  return { from: start, to: endA, insert: b.slice(start, endB) };
}

/** A CodeMirror 6 editor over the parent's text. Loaded lazily by LargeEditor. */
export default function CodeEditor({
  label,
  value,
  onChange,
  placeholder,
  selection,
  onSelectionChange,
  viMode,
  onModeChange,
  onZZ,
  onSubmit,
  onToggle,
  handle,
}: {
  label: string;
  value: string;
  onChange: (text: string) => void;
  placeholder?: string;
  /** The selection to open with. Read once. */
  selection: Selection;
  onSelectionChange: (selection: Selection) => void;
  /** Vi keys on or off. Applies live. */
  viMode: boolean;
  /** The vi mode, on every change. Not called with vi mode off. */
  onModeChange?: (mode: ViMode) => void;
  /** ZZ in normal mode. */
  onZZ?: () => void;
  /** Mod-Enter. The key is handled either way, so it never inserts a line or reaches a form. */
  onSubmit?: () => void;
  /** Mod-g. Left unhandled when absent. */
  onToggle?: () => void;
  handle?: Ref<CodeEditorHandle>;
}) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const callbacks = useRef({ onChange, onSelectionChange, onModeChange, onZZ, onSubmit, onToggle });
  callbacks.current = { onChange, onSelectionChange, onModeChange, onZZ, onSubmit, onToggle };
  const external = useRef(false);
  const viSlot = useRef(new Compartment());
  const stopVi = useRef<(() => void) | null>(null);

  // Turns the engine on: it starts in insert mode and reports every mode change.
  const startVi = (v: EditorView) => {
    mapOnce();
    const cm = getCM(v);
    if (!cm) return;
    const onMode = (e: { mode: string }) => callbacks.current.onModeChange?.(modeOf(e.mode));
    cm.on("vim-mode-change", onMode);
    closers.set(cm, () => callbacks.current.onZZ?.());
    // Entering insert mode with i collapses the selection; put it back.
    const { anchor, head } = v.state.selection.main;
    Vim.handleKey(cm, "i", "mapping");
    v.dispatch({ selection: { anchor, head } });
    callbacks.current.onModeChange?.("INSERT");
    stopVi.current = () => {
      cm.off("vim-mode-change", onMode);
      closers.delete(cm);
    };
  };

  useImperativeHandle(
    handle,
    () => ({
      escape() {
        const v = view.current;
        const cm = v && getCM(v);
        if (cm) Vim.handleKey(cm, "<Esc>", "user");
        v?.focus();
      },
    }),
    [],
  );

  useEffect(() => {
    const clamp = (n: number) => Math.min(n, value.length);
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: value,
        selection: { anchor: clamp(selection.start), head: clamp(selection.end) },
        extensions: [
          // Highest, or the vim keymap consumes these in normal mode.
          Prec.highest(
            keymap.of([
              {
                key: "Mod-Enter",
                preventDefault: true,
                stopPropagation: true,
                run: () => (callbacks.current.onSubmit?.(), true),
              },
              {
                key: "Mod-g",
                preventDefault: true,
                stopPropagation: true,
                run: () => {
                  if (!callbacks.current.onToggle) return false;
                  callbacks.current.onToggle();
                  return true;
                },
              },
            ]),
          ),
          viSlot.current.of(viMode ? vim() : []),
          history(),
          // No indentWithTab: Tab and Shift-Tab keep moving focus.
          keymap.of([...defaultKeymap, ...historyKeymap]),
          EditorView.lineWrapping,
          placeholderExt(placeholder ?? ""),
          EditorView.contentAttributes.of({ "aria-label": label, "aria-multiline": "true" }),
          theme,
          // A click or tap forgets the stored column, so the next j starts from the clicked spot.
          EditorView.domEventHandlers({
            pointerdown: (_e, v) => {
              const vimState = getCM(v)?.state.vim;
              if (vimState) vimState.lastMotion = null;
            },
          }),
          EditorView.updateListener.of((u) => {
            if (u.docChanged && !external.current) callbacks.current.onChange(u.state.doc.toString());
            if (u.selectionSet || u.docChanged) {
              const { from, to } = u.state.selection.main;
              callbacks.current.onSelectionChange({ start: from, end: to });
            }
          }),
        ],
      }),
    });
    view.current = v;
    if (viMode) startVi(v);
    v.focus();
    // The editor is created once; later props are applied by the effects below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    return () => {
      stopVi.current?.();
      stopVi.current = null;
      v.destroy();
    };
  }, []);

  const firstVi = useRef(true);
  useEffect(() => {
    // The first run is the creation above.
    if (firstVi.current) {
      firstVi.current = false;
      return;
    }
    const v = view.current;
    if (!v) return;
    stopVi.current?.();
    stopVi.current = null;
    v.dispatch({ effects: viSlot.current.reconfigure(viMode ? vim() : []) });
    if (viMode) startVi(v);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [viMode]);

  useEffect(() => {
    const v = view.current;
    if (!v) return;
    const current = v.state.doc.toString();
    if (current === value) return;
    external.current = true;
    try {
      v.dispatch({ changes: diff(current, value) });
    } finally {
      external.current = false;
    }
  }, [value]);

  return <Box ref={host} flex="1" minH="0" overflow="hidden" borderWidth="1px" borderRadius="10px" {...fieldLook} />;
}
