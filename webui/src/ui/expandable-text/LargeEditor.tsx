import { Box, Text } from "@chakra-ui/react";
import { lazy, Suspense, useRef, useState } from "react";
import type { CodeEditorHandle, ViMode } from "./CodeEditor";
import { keepFocus, ModeText } from "./ModeText";
import { useViMode } from "../../vimode";
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
  onSubmit,
}: {
  title: string;
  value: string;
  onChange: (text: string) => void;
  placeholder?: string;
  /** The selection the field had when the editor opened. */
  selection: Selection;
  /** Called with the editor's selection when it closes. `refocus` is false after a submit. */
  onClose: (selection: Selection, refocus: boolean) => void;
  /** Mod-Enter: closes without refocus, then runs it. Absent: the key does nothing. */
  onSubmit?: () => void;
}) {
  const viMode = useViMode();
  const wide = useWide();
  const coarse = useCoarsePointer();
  const [mode, setMode] = useState<ViMode>("INSERT");
  const editor = useRef<CodeEditorHandle>(null);
  const current = useRef(selection);
  const close = () => onClose(current.current, true);
  const submit =
    onSubmit &&
    (() => {
      onClose(current.current, false);
      onSubmit();
    });
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
          viMode={viMode}
          onModeChange={setMode}
          handle={editor}
          onZZ={close}
          onSubmit={submit}
          onToggle={viMode ? close : undefined}
        />
      </Suspense>
      {coarse ? (
        <Box data-testid="editor-bar" display="flex" alignItems="center" gap="2">
          {viMode && (
            <>
              <ModeText mode={mode} pill />
              <Button emphasis="main" tone="running" h="44px" onMouseDown={keepFocus} onClick={() => editor.current?.escape()}>
                Esc
              </Button>
            </>
          )}
          <Box flex="1" />
          <Button h="44px" onClick={close}>
            Done
          </Button>
        </Box>
      ) : (
        <Box display="flex" alignItems="center" gap="2">
          {viMode && <ModeText mode={mode} />}
          <Text flex="1" fontSize="xs" color="text.dim">
            Saved as you type
          </Text>
          <Button onClick={close}>Done</Button>
        </Box>
      )}
    </BottomSheet>
  );
}
