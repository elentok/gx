import { Box } from "@chakra-ui/react";
import { useRef, type ClipboardEvent, type DragEvent, type ReactNode } from "react";
import { LuPaperclip } from "react-icons/lu";
import { Button } from "../ui/button/Button";
import { MAX_IMAGES, MSG_MAX_IMAGES } from "./imageLimits";
import { ImageStrip } from "./ImageStrip";
import type { ComposerImages as State } from "./useComposerImages";

const imageFiles = (list: FileList | null | undefined): File[] =>
  Array.from(list ?? []).filter((f) => f.type.startsWith("image/") || f.type === "");

function lineText(state: State): { text: string; red: boolean } | undefined {
  if (state.error === MSG_MAX_IMAGES) return { text: MSG_MAX_IMAGES, red: true };
  // Counts every failed tile, refused ones included; state.failed only counts what blocks a send.
  const failed = state.images.filter((i) => i.state === "failed").length;
  if (failed || state.uploading) {
    return { text: `${failed} failed · ${state.uploading} uploading`, red: failed > 0 };
  }
  if (state.error) return { text: state.error, red: true };
  if (state.images.length) return { text: `${state.images.length} of ${MAX_IMAGES} images`, red: false };
  return undefined;
}

/**
 * Thumbnail row above `children` (the text box), Add and the status line beside `send`. Paste and drop anywhere inside add images.
 * `phone` makes Add an icon and moves the status line above the row.
 */
export function ComposerImages({
  state,
  children,
  send,
  afterAdd,
  phone = false,
}: {
  state: State;
  children: ReactNode;
  send: ReactNode;
  /** Sits right next to Add, before the status line. */
  afterAdd?: ReactNode;
  phone?: boolean;
}) {
  const input = useRef<HTMLInputElement>(null);
  const onPaste = (e: ClipboardEvent) => {
    const files = imageFiles(e.clipboardData?.files);
    if (files.length === 0) return;
    e.preventDefault();
    void state.addFiles(files);
  };
  const onDragOver = (e: DragEvent) => {
    if (Array.from(e.dataTransfer?.types ?? []).includes("Files")) e.preventDefault();
  };
  const onDrop = (e: DragEvent) => {
    const files = imageFiles(e.dataTransfer?.files);
    if (files.length === 0) return;
    e.preventDefault();
    void state.addFiles(files);
  };
  const line = lineText(state);
  return (
    <Box display="flex" flexDirection="column" gap="2" onPaste={onPaste} onDragOver={onDragOver} onDrop={onDrop}>
      {state.images.length > 0 && (
        <ImageStrip images={state.images} onRetry={state.retry} onRemove={state.remove} />
      )}
      {children}
      {phone && line && (
        <Box fontSize="sm" color={line.red ? "tone.deadlocked.fg" : "text.dim"}>
          {line.text}
        </Box>
      )}
      <Box display="flex" gap="2" alignItems="center">
        <Button
          emphasis="secondary"
          aria-label="Add image"
          title={phone ? "Add image" : undefined}
          {...(phone ? { w: "44px", h: "44px", px: "0", justifyContent: "center" } : {})}
          flex="none"
          onClick={() => input.current?.click()}
        >
          <LuPaperclip size={phone ? 20 : undefined} />
          {!phone && " Add"}
        </Button>
        <input
          ref={input}
          type="file"
          accept="image/*"
          multiple
          hidden
          data-testid="image-input"
          onChange={(e) => {
            void state.addFiles(imageFiles(e.target.files));
            e.target.value = "";
          }}
        />
        {afterAdd}
        {!phone && (
          <Box flex="1" fontSize="sm" color={line?.red ? "tone.deadlocked.fg" : "text.dim"}>
            {line?.text}
          </Box>
        )}
        {send}
      </Box>
    </Box>
  );
}
