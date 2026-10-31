import { Box, chakra, Text, Textarea } from "@chakra-ui/react";
import { useEffect, useRef } from "react";
import { useWide } from "../../useWide";
import { Button } from "../button/Button";
import { BottomSheet } from "../modal/BottomSheet";
import { fieldLook } from "./look";

export type Selection = { start: number; end: number };

/** The large editor of an ExpandableTextField. It reads and writes the parent's text directly; Done only closes it. */
export function LargeEditor({
  title,
  value,
  onChange,
  placeholder,
  selection,
  onClose,
}: {
  title: string;
  value: string;
  onChange: (text: string) => void;
  placeholder?: string;
  /** The selection the field had when the editor opened. */
  selection: Selection;
  /** Called with the editor's selection when it closes. */
  onClose: (selection: Selection) => void;
}) {
  const sheetMode = !useWide();
  const area = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    area.current?.setSelectionRange(selection.start, selection.end);
    // Only the selection at open time matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  const close = () =>
    onClose({
      start: area.current?.selectionStart ?? 0,
      end: area.current?.selectionEnd ?? 0,
    });
  return (
    <BottomSheet
      label={title}
      onClose={close}
      belowTopBar
      fillViewport
      desktop={{ borderRadius: "12px", w: "860px", h: "680px", p: "5" }}
      bg="surface.raised"
      borderColor="border"
      p="4"
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
          onClick={close}
        >
          ×
        </chakra.button>
      </Box>
      <Textarea
        ref={area}
        autoFocus
        flex="1"
        minH="0"
        {...fieldLook}
        resize="none"
        lineHeight="1.55"
        p="3"
        borderRadius="10px"
        aria-label={`${title} text`}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <Box display="flex" alignItems="center" gap="2">
        <Text flex="1" fontSize="xs" color="text.dim">
          Saved as you type
        </Text>
        <Button onClick={close}>Done</Button>
      </Box>
    </BottomSheet>
  );
}
