import { Box } from "@chakra-ui/react";
import { useRef, type ClipboardEvent, type DragEvent, type ReactNode } from "react";
import { LuPaperclip } from "react-icons/lu";
import { Button } from "../ui/button/Button";
import { useDevice } from "../useWide";
import { MAX_IMAGES, MSG_MAX_IMAGES } from "./imageLimits";
import { ImageStrip } from "./ImageStrip";
import type { ComposerImages as State } from "./useComposerImages";

export const imageFiles = (list: FileList | null | undefined): File[] =>
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

/** The one attach rule: phone and tablet get an icon-only paperclip, desktop the labelled control. */
export const useAttachIcon = (): boolean => useDevice() !== "desktop";

/** The attach button for `useAttachIcon`; the New Design sheet reuses it. */
export function AttachButton({ onClick }: { onClick: () => void }) {
  const icon = useAttachIcon();
  return (
    <Button
      type="button"
      emphasis="secondary"
      aria-label="Add image"
      title={icon ? "Add image" : undefined}
      {...(icon ? { w: "44px", h: "44px", px: "0", justifyContent: "center" } : {})}
      flex="none"
      onClick={onClick}
    >
      <LuPaperclip size={icon ? 20 : undefined} />
      {!icon && " Add"}
    </Button>
  );
}

/**
 * Thumbnail row above `children` (the text box), Add and the status line beside `send`. Paste and drop anywhere inside add images.
 * Add follows `useAttachIcon`; `phone` moves the status line above the row.
 */
export function ComposerImages({
  state,
  children,
  send,
  afterAdd,
  phone = false,
  footer,
  gap = "2",
}: {
  /** A dim line between the text box and the button row; leave it out when the box carries it. */
  footer?: ReactNode;
  state: State;
  children: ReactNode;
  send: ReactNode;
  /** Sits right next to Add, before the status line. */
  afterAdd?: ReactNode;
  phone?: boolean;
  /** Space between the stacked parts: thumbnails, text box, button row. */
  gap?: string;
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
    <Box display="flex" flexDirection="column" gap={gap} onPaste={onPaste} onDragOver={onDragOver} onDrop={onDrop}>
      {state.images.length > 0 && (
        <ImageStrip images={state.images} onRetry={state.retry} onRemove={state.remove} />
      )}
      {children}
      {footer}
      {phone && line && (
        <Box fontSize="sm" color={line.red ? "tone.deadlocked.fg" : "text.dim"}>
          {line.text}
        </Box>
      )}
      <Box display="flex" gap="2" alignItems="center">
        <AttachButton onClick={() => input.current?.click()} />
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
