import { Box } from "@chakra-ui/react";
import { useEffect, useRef } from "react";
import { ExpandableTextField } from "../ui/expandable-text/ExpandableTextField";

/** A 5-row note that scrolls itself into view when it mounts. */
export function NoteBox({
  name,
  title,
  value,
  placeholder,
  onChange,
  label,
  disabled,
  autoFocus = false,
  onSubmit,
}: {
  /** What the note is on, for its labels: "question 2". */
  name: string;
  /** The large editor's title. */
  title: string;
  value: string;
  placeholder: string;
  onChange: (text: string) => void;
  /** The box's accessible name, when it is not "Note text for <name>". */
  label?: string;
  disabled?: boolean;
  /** Focus the box when it mounts. */
  autoFocus?: boolean;
  /** Cmd/Ctrl+Enter in the box. */
  onSubmit?: () => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    box.current?.scrollIntoView?.({ block: "nearest" });
  }, []);
  return (
    <Box ref={box}>
      <ExpandableTextField
        rows={5}
        label={label ?? `Note text for ${name}`}
        title={title}
        value={value}
        placeholder={placeholder}
        onChange={onChange}
        disabled={disabled}
        autoFocus={autoFocus}
        onSubmit={onSubmit}
      />
    </Box>
  );
}
