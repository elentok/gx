import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import type { SessionChatImage } from "../generated/server";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { sessionImageUrl } from "./sessionImages";

/** A dark full-size view of one message's images: Prev and Next stop at the ends, Close and Escape return to the chat. */
export function ImageViewer({
  images,
  sessionId,
  start,
  onClose,
}: {
  images: SessionChatImage[];
  sessionId: string;
  start: number;
  onClose: () => void;
}) {
  const [index, setIndex] = useState(start);
  const img = images[index];
  return (
    <Modal
      label="Image"
      onClose={onClose}
      data-mock-part="full-size"
      bg="black"
      borderColor="transparent"
      backdrop={{ p: "0" }}
      w="100%"
      maxW="720px"
      p="2.5"
      gap="2.5"
      alignItems="center"
    >
      {img?.id && !img.placeholder ? (
        <img
          src={sessionImageUrl(sessionId, img.id)}
          alt={`Image ${index + 1} of ${images.length}`}
          style={{ width: "100%", maxHeight: "380px", aspectRatio: "4/3", objectFit: "contain", display: "block", borderRadius: "6px" }}
        />
      ) : (
        <Box data-testid="viewer-image-placeholder" w="100%" maxH="380px" aspectRatio="4/3" bg="border" borderRadius="6px" />
      )}
      <Box display="flex" gap="2" alignItems="center" justifyContent="space-between" w="100%" color="text.muted" fontSize="sm">
        <Button emphasis="secondary" disabled={index === 0} onClick={() => setIndex(index - 1)}>
          ‹ Prev
        </Button>
        <Text as="span">{`${index + 1} of ${images.length}`}</Text>
        <Button emphasis="secondary" disabled={index >= images.length - 1} onClick={() => setIndex(index + 1)}>
          Next ›
        </Button>
        <Button emphasis="secondary" onClick={onClose}>
          Close ✕
        </Button>
      </Box>
    </Modal>
  );
}
