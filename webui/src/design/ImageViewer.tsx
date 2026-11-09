import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import type { SessionChatImage } from "../generated/server";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";

/** A dark full-size view of a list of images: Prev and Next stop at the ends, Close and Escape return to the page. */
export function ImageViewer({
  images,
  source,
  start,
  onClose,
}: {
  images: SessionChatImage[];
  /** The URL of one image; chat images use the session source, other callers bring their own. */
  source: (image: SessionChatImage) => string;
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
          src={source(img)}
          alt={`Image ${index + 1} of ${images.length}`}
          style={{ width: "100%", maxHeight: "380px", aspectRatio: "4/3", objectFit: "contain", display: "block", borderRadius: "6px" }}
        />
      ) : (
        <Box data-testid="viewer-image-placeholder" w="100%" maxH="380px" aspectRatio="4/3" bg="border" borderRadius="6px" />
      )}
      <Box display="flex" gap="2" alignItems="center" justifyContent="space-between" w="100%" color="text.muted" fontSize="sm">
        <Button type="button" emphasis="secondary" disabled={index === 0} onClick={() => setIndex(index - 1)}>
          ‹ Prev
        </Button>
        <Text as="span">{`${index + 1} of ${images.length}`}</Text>
        <Button type="button" emphasis="secondary" disabled={index >= images.length - 1} onClick={() => setIndex(index + 1)}>
          Next ›
        </Button>
        <Button type="button" emphasis="secondary" onClick={onClose}>
          Close ✕
        </Button>
      </Box>
    </Modal>
  );
}
