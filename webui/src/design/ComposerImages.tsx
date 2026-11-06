import { Box, Spinner } from "@chakra-ui/react";
import { useRef, type ClipboardEvent, type DragEvent, type ReactNode } from "react";
import { Button } from "../ui/button/Button";
import { MAX_IMAGES, MSG_MAX_IMAGES } from "./imageLimits";
import type { ComposerImage, ComposerImages as State } from "./useComposerImages";

const imageFiles = (list: FileList | null | undefined): File[] =>
  Array.from(list ?? []).filter((f) => f.type.startsWith("image/") || f.type === "");

function Thumb({ image, onRetry, onRemove }: { image: ComposerImage; onRetry: () => void; onRemove: () => void }) {
  const failed = image.state === "failed";
  const retry = failed && image.canRetry;
  return (
    <Box position="relative" flex="none" w="72px" h="72px" data-state={image.state}>
      <Box
        w="full"
        h="full"
        borderRadius="6px"
        bg="surface.raised"
        borderWidth="1px"
        borderColor="border"
        bgImage={image.url ? `url(${image.url})` : undefined}
        bgSize="cover"
        bgPos="center"
      />
      {image.state === "uploading" && (
        <Box position="absolute" inset="0" borderRadius="6px" bg="blackAlpha.600" display="flex" alignItems="center" justifyContent="center">
          <Spinner size="sm" aria-label="Uploading" />
        </Box>
      )}
      {failed && (
        <Box
          as={retry ? "button" : "div"}
          position="absolute"
          inset="0"
          p="1"
          pt="16px"
          overflow="hidden"
          borderRadius="6px"
          borderWidth="1px"
          borderColor="tone.deadlocked.fg"
          bg="tone.deadlocked.bg"
          color="tone.deadlocked.fg"
          fontSize="10px"
          lineHeight="1.2"
          textAlign="center"
          display="flex"
          alignItems="center"
          justifyContent="center"
          cursor={retry ? "pointer" : "default"}
          onClick={retry ? onRetry : undefined}
        >
          {retry ? `${image.message}, tap to retry` : image.message}
        </Box>
      )}
      <Box
        as="button"
        position="absolute"
        top="-7px"
        right="-7px"
        w="26px"
        h="26px"
        borderRadius="full"
        bg="surface.raised"
        borderWidth="1px"
        borderColor="border"
        fontSize="sm"
        cursor="pointer"
        aria-label="Remove image"
        onClick={onRemove}
      >
        ✕
      </Box>
    </Box>
  );
}

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

/** Thumbnail row above `children` (the text box), Add and the status line beside `send`. Paste and drop anywhere inside add images. */
export function ComposerImages({ state, children, send }: { state: State; children: ReactNode; send: ReactNode }) {
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
        <Box display="flex" gap="10px" overflowX="auto" pt="2" pb="1" pl="0.5" pr="1" aria-label="Images">
          {state.images.map((image) => (
            <Thumb key={image.key} image={image} onRetry={() => state.retry(image.key)} onRemove={() => state.remove(image.key)} />
          ))}
        </Box>
      )}
      {children}
      <Box display="flex" gap="2" alignItems="center">
        <Button emphasis="secondary" aria-label="Add image" onClick={() => input.current?.click()}>
          📎 Add
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
        <Box flex="1" fontSize="sm" color={line?.red ? "tone.deadlocked.fg" : "text.dim"}>
          {line?.text}
        </Box>
        {send}
      </Box>
    </Box>
  );
}
