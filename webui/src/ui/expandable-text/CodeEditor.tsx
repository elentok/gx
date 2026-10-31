import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { EditorState } from "@codemirror/state";
import { EditorView, keymap, placeholder as placeholderExt } from "@codemirror/view";
import { Box } from "@chakra-ui/react";
import { useEffect, useRef } from "react";
import type { Selection } from "./LargeEditor";
import { fieldLook } from "./look";

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
}: {
  label: string;
  value: string;
  onChange: (text: string) => void;
  placeholder?: string;
  /** The selection to open with. Read once. */
  selection: Selection;
  onSelectionChange: (selection: Selection) => void;
}) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const callbacks = useRef({ onChange, onSelectionChange });
  callbacks.current = { onChange, onSelectionChange };
  const external = useRef(false);

  useEffect(() => {
    const clamp = (n: number) => Math.min(n, value.length);
    const v = new EditorView({
      parent: host.current!,
      state: EditorState.create({
        doc: value,
        selection: { anchor: clamp(selection.start), head: clamp(selection.end) },
        extensions: [
          history(),
          // No indentWithTab: Tab and Shift-Tab keep moving focus.
          keymap.of([...defaultKeymap, ...historyKeymap]),
          EditorView.lineWrapping,
          placeholderExt(placeholder ?? ""),
          EditorView.contentAttributes.of({ "aria-label": label, "aria-multiline": "true" }),
          theme,
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
    v.focus();
    // The editor is created once; later props are applied by the effect below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    return () => v.destroy();
  }, []);

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
