import { Box, chakra, Textarea } from "@chakra-ui/react";
import { useRef, useState, type KeyboardEvent } from "react";
import { LargeEditor, type Selection } from "./LargeEditor";
import { fieldLook } from "./look";

/** Runs `submit` on Cmd/Ctrl+Enter; plain Enter stays a newline. */
export function submitOnCmdEnter(
  e: KeyboardEvent<HTMLTextAreaElement>,
  submit: () => void,
) {
  if (e.key !== "Enter" || !(e.metaKey || e.ctrlKey)) return;
  e.preventDefault();
  submit();
}

/** A textarea with an expand button that opens a large editor on the same text. The parent owns the text; there is no copy and no save step. */
export function ExpandableTextField({
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
}: {
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
}) {
  const area = useRef<HTMLTextAreaElement>(null);
  const [selection, setSelection] = useState<Selection | null>(null);
  const open = () =>
    setSelection({
      start: area.current?.selectionStart ?? 0,
      end: area.current?.selectionEnd ?? 0,
    });
  const close = (sel: Selection) => {
    area.current?.setSelectionRange(sel.start, sel.end);
    setSelection(null);
  };
  return (
    <Box position="relative">
      <Textarea
        ref={area}
        {...fieldLook}
        size={size}
        borderRadius={radius}
        autoFocus={autoFocus}
        rows={rows}
        lineHeight="1.5"
        py="2.5"
        ps="3"
        pe="46px"
        resize="none"
        aria-label={label}
        placeholder={placeholder}
        disabled={disabled}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onSubmit && ((e) => submitOnCmdEnter(e, onSubmit))}
      />
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
        />
      )}
    </Box>
  );
}
