import { Box, chakra, Text, Textarea } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { useWide } from "../useWide";

const TOP_BAR = "48px";

const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface",
  borderColor: "border",
} as const;

/** A 5-row note that scrolls itself into view, with an expand button for a large editor. The change handler is the only save. */
export function NoteBox({
  name,
  title,
  value,
  placeholder,
  onChange,
  label,
  disabled,
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
}) {
  const [editing, setEditing] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const sheetMode = !useWide();
  useEffect(() => {
    box.current?.scrollIntoView?.({ block: "nearest" });
  }, []);
  return (
    <Box ref={box} position="relative">
      <Textarea
        {...field}
        borderRadius="12px"
        rows={5}
        lineHeight="1.5"
        py="2.5"
        ps="3"
        pe="46px"
        resize="none"
        aria-label={label ?? `Note text for ${name}`}
        placeholder={placeholder}
        disabled={disabled}
        value={value}
        onChange={(e) => onChange(e.target.value)}
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
        aria-label="Open the note in a large editor"
        title="Open the note in a large editor"
        onClick={() => setEditing(true)}
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M9.5 2.5h4v4M13.5 2.5L9 7M6.5 13.5h-4v-4M2.5 13.5L7 9" />
        </svg>
      </chakra.button>
      {editing && (
        <Modal
          label={title}
          onClose={() => setEditing(false)}
          backdrop={
            sheetMode
              ? { top: TOP_BAR, inset: undefined, left: "0", right: "0", bottom: "0", p: "2", alignItems: "stretch" }
              : undefined
          }
          bg="surface.raised"
          borderColor="border"
          borderRadius="12px"
          w={sheetMode ? "full" : "860px"}
          h={sheetMode ? "full" : "680px"}
          p={sheetMode ? "4" : "5"}
          gap="3"
        >
          <Box display="flex" alignItems="center" gap="2">
            <Text flex="1" fontSize={sheetMode ? "title" : "xl"} fontWeight="700">
              {title}
            </Text>
            <chakra.button
              type="button"
              aria-label="Close"
              w="40px"
              h="40px"
              fontSize="xl"
              lineHeight="1"
              borderRadius="8px"
              color="text"
              cursor="pointer"
              onClick={() => setEditing(false)}
            >
              ×
            </chakra.button>
          </Box>
          <Textarea
            autoFocus
            flex="1"
            minH="0"
            {...field}
            resize="none"
            lineHeight="1.55"
            p="3"
            borderRadius="10px"
            aria-label="Note text in the large editor"
            placeholder={placeholder}
            value={value}
            onChange={(e) => onChange(e.target.value)}
          />
          <Box display="flex" alignItems="center" gap="2">
            <Text flex="1" fontSize="xs" color="text.dim">
              Saved as you type
            </Text>
            <Button onClick={() => setEditing(false)}>Done</Button>
          </Box>
        </Modal>
      )}
    </Box>
  );
}
