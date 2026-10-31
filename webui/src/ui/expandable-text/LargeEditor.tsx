import { Box, chakra, Text } from "@chakra-ui/react";
import { lazy, Suspense, useRef } from "react";
import { useWide } from "../../useWide";
import { Button } from "../button/Button";
import { BottomSheet } from "../modal/BottomSheet";
import { Spinner } from "../spinner/Spinner";

// Its own chunk: only this import pulls @codemirror/*.
const CodeEditor = lazy(() => import("./CodeEditor"));

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
  const current = useRef(selection);
  const close = () => onClose(current.current);
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
      <Suspense
        fallback={
          <Box flex="1" display="flex" alignItems="center" justifyContent="center">
            <Spinner label="Loading editor" />
          </Box>
        }
      >
        <CodeEditor
          label={`${title} text`}
          value={value}
          onChange={onChange}
          placeholder={placeholder}
          selection={selection}
          onSelectionChange={(s) => {
            current.current = s;
          }}
        />
      </Suspense>
      <Box display="flex" alignItems="center" gap="2">
        <Text flex="1" fontSize="xs" color="text.dim">
          Saved as you type
        </Text>
        <Button onClick={close}>Done</Button>
      </Box>
    </BottomSheet>
  );
}
