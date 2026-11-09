import { Box } from "@chakra-ui/react";
import { useState } from "react";
import { useAttachIcon } from "./ComposerImages";
import { MAX_IMAGES, MSG_MAX_IMAGES } from "./imageLimits";
import { ImageStrip } from "./ImageStrip";
import { ImageViewer } from "./ImageViewer";
import type { DraftImages as State } from "./useDraftImages";

export const MSG_LIMITS = "Paste or add images (PNG, JPEG, GIF, WebP, up to 20 MB, 10 per draft)";

function lineText(s: State, desktop: boolean): { text: string; red: boolean } | undefined {
  if (s.error === MSG_MAX_IMAGES) return { text: MSG_MAX_IMAGES, red: true };
  if (s.failed) return { text: s.failed === 1 ? "1 image failed. Remove it or retry before starting." : `${s.failed} images failed. Remove them or retry before starting.`, red: true };
  if (s.uploading) return { text: `${s.uploading} uploading`, red: false };
  if (s.error) return { text: s.error, red: true };
  if (s.images.length) return { text: `${s.count} of ${MAX_IMAGES} images`, red: false };
  return desktop ? { text: MSG_LIMITS, red: false } : undefined;
}

/**
 * The thumbnail row and the status line under it. Desktop adds the Add image tile and the limits line; phone and tablet
 * have the footer paperclip instead, so the row shows only with images and the line only with something to say.
 * Paste and the file input are wired by the form, not here.
 */
export function DraftImages({ state, pick }: { state: State; pick: () => void }) {
  const tile = !useAttachIcon();
  const [viewing, setViewing] = useState<number>();
  const line = lineText(state, tile);
  // The viewer lists ready images only, so a thumbnail's index shifts past the others.
  const open = (i: number) => setViewing(state.images.slice(0, i).filter((x) => x.state === "ready").length);
  return (
    <>
      {(tile || state.images.length > 0) && (
        <ImageStrip
          images={state.images}
          onOpen={open}
          onRetry={state.retry}
          onRemove={state.remove}
          trailing={
            tile && (
              <Box
                as="button"
                {...{ type: "button" }}
                flex="none"
                w="72px"
                h="72px"
                borderRadius="6px"
                borderWidth="1px"
                borderStyle="dashed"
                borderColor="border"
                color="text"
                fontSize="14px"
                fontWeight="400"
                lineHeight="1.2"
                px="3"
                textAlign="center"
                cursor="pointer"
                display="flex"
                flexDirection="column"
                alignItems="center"
                justifyContent="center"
                gap="2px"
                aria-label="Add image"
                data-mock-part="add-image-button"
                onClick={pick}
              >
                <Box as="b" fontSize="20px" fontWeight="400" lineHeight="1">
                  ＋
                </Box>
                Add image
              </Box>
            )
          }
        />
      )}
      {line && (
        <Box pl="0.5" pb="2" fontSize="12px" color={line.red ? "tone.deadlocked.fg" : "text.dim"} flexShrink="0" data-mock-part="image-line">
          {line.text}
        </Box>
      )}
      {viewing !== undefined && (
        <ImageViewer
          images={state.readyIds.map((id) => ({ id }))}
          source={(img) => state.urlOf(img.id ?? "")}
          start={viewing}
          onClose={() => setViewing(undefined)}
        />
      )}
    </>
  );
}
