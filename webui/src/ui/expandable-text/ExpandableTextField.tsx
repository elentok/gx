import { Box, chakra, Text, Textarea } from "@chakra-ui/react";
import { useLayoutEffect, useRef, useState, type KeyboardEvent } from "react";
import { useCompletion, type CompletionConfig, type MenuKey } from "../../commands/useCompletion";
import type { ViMode } from "./CodeEditor";
import { LargeEditor, type EditorMenu, type Selection } from "./LargeEditor";
import { fieldLook } from "./look";
import { isMac } from "../../platform";
import { useViMode } from "../../vimode";

/** Runs `submit` on Cmd/Ctrl+Enter; plain Enter stays a newline. */
export function submitOnCmdEnter(
  e: KeyboardEvent<HTMLTextAreaElement>,
  submit: () => void,
) {
  if (e.key !== "Enter" || !(e.metaKey || e.ctrlKey)) return;
  e.preventDefault();
  submit();
}

interface FieldProps {
  value: string;
  onChange: (text: string) => void;
  rows?: number;
  /** The field's accessible name. */
  label: string;
  /** The large editor's title. Its accessible name is "<title> text". */
  title: string;
  placeholder?: string;
  disabled?: boolean;
  autoFocus?: boolean;
  /** Cmd/Ctrl+Enter in the field. */
  onSubmit?: () => void;
  size?: "sm" | "md";
  radius?: string;
  /** Overrides the field's background (default: the raised surface). */
  bg?: string;
  /** Opt in to the command completion menu in the inline textarea. */
  completion?: CompletionConfig;
  /** A status line inside the box, under the text. `null` keeps its room with no text; leave it out for no line. */
  note?: { text: string; red?: boolean } | null;
}

/** A textarea with an expand button that opens a large editor on the same text. The parent owns the text; there is no copy and no save step. */
export function ExpandableTextField(props: FieldProps) {
  const { completion, ...rest } = props;
  return completion ? <Completing {...rest} config={completion} /> : <Field {...rest} />;
}

interface Menu extends EditorMenu {
  onKeyDown: (e: MenuKey) => boolean;
}

function Completing({ config, ...rest }: Omit<FieldProps, "completion"> & { config: CompletionConfig }) {
  const [cursor, setCursor] = useState(0);
  // The large editor's vi mode; null while it is closed or vi is off. The menu stays shut outside INSERT.
  const [editorMode, setEditorMode] = useState<ViMode | null>(null);
  const pending = useRef<number | null>(null);
  const off = config.off || (editorMode !== null && editorMode !== "INSERT");
  const c = useCompletion({ ...config, off }, rest.value, cursor, rest.disabled, (text, at) => {
    pending.current = at;
    rest.onChange(text);
    setCursor(at);
  });
  return <Field {...rest} menu={{ node: c.menu, onKeyDown: c.onKeyDown, onCursor: setCursor, onMode: setEditorMode, pending }} />;
}

function Field({
  value,
  onChange,
  rows,
  label,
  title,
  placeholder,
  disabled,
  autoFocus,
  onSubmit,
  size,
  radius = "12px",
  bg,
  menu,
  note,
}: Omit<FieldProps, "completion"> & { menu?: Menu }) {
  const area = useRef<HTMLTextAreaElement>(null);
  const [selection, setSelection] = useState<Selection | null>(null);
  useLayoutEffect(() => {
    // While the editor is open it places the caret itself.
    if (selection) return;
    const at = menu?.pending.current;
    if (at == null) return;
    menu!.pending.current = null;
    area.current?.setSelectionRange(at, at);
  }, [value, menu, selection]);
  const open = () =>
    setSelection({
      start: area.current?.selectionStart ?? 0,
      end: area.current?.selectionEnd ?? 0,
    });
  const viMode = useViMode();
  // Focus and selection are set synchronously, inside the user's gesture and before the layer unmounts, so a phone keeps its keyboard.
  const close = (sel: Selection, refocus: boolean) => {
    if (refocus) {
      area.current?.focus();
      area.current?.setSelectionRange(sel.start, sel.end);
    }
    setSelection(null);
  };
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (viMode && !disabled && e.key.toLowerCase() === "g" && !e.altKey && !e.shiftKey && (isMac() ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey)) {
      e.preventDefault();
      open();
      return;
    }
    if (menu?.onKeyDown(e)) return;
    if (onSubmit) submitOnCmdEnter(e, onSubmit);
  };
  return (
    <Box position="relative">
      <Textarea
        ref={area}
        {...fieldLook}
        size={size}
        borderRadius={radius}
        {...(bg ? { bg } : {})}
        autoFocus={autoFocus}
        rows={rows}
        lineHeight="1.5"
        display="block"
        pt="2.5"
        pb={note === undefined ? "2.5" : "7"}
        ps="3"
        pe="46px"
        resize="none"
        aria-label={label}
        placeholder={placeholder}
        disabled={disabled}
        value={value}
        onChange={(e) => {
          menu?.onCursor(e.target.selectionStart);
          onChange(e.target.value);
        }}
        onSelect={menu ? (e) => menu.onCursor(e.currentTarget.selectionStart) : undefined}
        onKeyDown={onKeyDown}
      />
      {note !== undefined && (
        <Text
          position="absolute"
          bottom="1.5"
          left="3"
          right="46px"
          fontSize="xs"
          lineHeight="1.5"
          pointerEvents="none"
          truncate
          color={note?.red ? "tone.deadlocked.fg" : "text.dim"}
        >
          {note?.text}
        </Text>
      )}
      {!selection && menu?.node}
      <chakra.button
        type="button"
        position="absolute"
        top="1.5"
        right="1.5"
        w="36px"
        h="36px"
        borderRadius="8px"
        bg="surface.raised"
        color="text.muted"
        display="flex"
        alignItems="center"
        justifyContent="center"
        cursor="pointer"
        _disabled={{ opacity: 0.5, cursor: "not-allowed" }}
        disabled={disabled}
        aria-label="Open the note in a large editor"
        title="Open the note in a large editor"
        onClick={open}
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M9.5 2.5h4v4M13.5 2.5L9 7M6.5 13.5h-4v-4M2.5 13.5L7 9" />
        </svg>
      </chakra.button>
      {selection && (
        <LargeEditor
          title={title}
          value={value}
          onChange={onChange}
          placeholder={placeholder}
          selection={selection}
          onClose={close}
          onSubmit={onSubmit}
          menu={menu}
        />
      )}
    </Box>
  );
}
