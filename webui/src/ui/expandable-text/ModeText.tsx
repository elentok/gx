import { Box } from "@chakra-ui/react";
import type { Tone } from "../tone";
import type { ViMode } from "./CodeEditor";

const MODE_TONE: Record<ViMode, Tone> = { INSERT: "done", NORMAL: "running", VISUAL: "waiting" };

// A button or pill on the touch bar must not take focus from the editor, or the on-screen keyboard closes.
export const keepFocus = (e: { preventDefault: () => void }) => e.preventDefault();

/** The vi mode as text. The one polite live region of the editor; its text changes only when the mode does. */
export function ModeText({ mode, pill }: { mode: ViMode; pill?: boolean }) {
  const tone = MODE_TONE[mode];
  return (
    <Box
      role="status"
      data-tone={tone}
      data-testid={pill ? "vi-mode-pill" : "vi-mode-text"}
      onMouseDown={keepFocus}
      flex="none"
      fontFamily="mono"
      fontSize="sm"
      fontWeight="600"
      color={`tone.${tone}.fg`}
      {...(pill && {
        display: "flex",
        alignItems: "center",
        h: "44px",
        px: "3",
        borderRadius: "12px",
        borderWidth: "1px",
        borderColor: "border",
        bg: "surface.raised",
      })}
    >
      {mode}
    </Box>
  );
}
