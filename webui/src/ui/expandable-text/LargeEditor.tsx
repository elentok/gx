import { Box, Text } from "@chakra-ui/react";
import { lazy, Suspense, useRef } from "react";
import { useCoarsePointer, useWide } from "../../useWide";
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
  const wide = useWide();
  const coarse = useCoarsePointer();
  const current = useRef(selection);
  const close = () => onClose(current.current);
  // Layout follows the width, the bar follows the pointer. A phone's touch bar replaces the header and footer.
  const header = wide || !coarse;
  return (
    <BottomSheet
      label={title}
      onClose={close}
      belowTopBar
      fillViewport
      followViewport={wide && coarse}
      closeOnEscape={false}
      closeOnNarrowBackdrop={false}
      desktop={{ borderRadius: "12px", w: "860px", h: "680px", p: "5" }}
      bg="surface.raised"
      borderColor="border"
      p="4"
      gap="3"
    >
      {header && (
        <Text fontSize={wide ? "xl" : "title"} fontWeight="700">
          {title}
        </Text>
      )}
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
      {coarse ? (
        <Box data-testid="editor-bar" display="flex" alignItems="center" justifyContent="flex-end" gap="2">
          <Button h="44px" onClick={close}>
            Done
          </Button>
        </Box>
      ) : (
        <Box display="flex" alignItems="center" gap="2">
          <Text flex="1" fontSize="xs" color="text.dim">
            Saved as you type
          </Text>
          <Button onClick={close}>Done</Button>
        </Box>
      )}
    </BottomSheet>
  );
}
