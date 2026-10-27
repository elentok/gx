import { Box, type BoxProps } from "@chakra-ui/react";
import { useEffect, useRef, type ReactNode, type Ref } from "react";

interface ModalProps extends Omit<BoxProps, "children"> {
  label: string;
  onClose: () => void;
  /** Overrides for the backdrop, such as a bottom sheet's placement. */
  backdrop?: BoxProps;
  ref?: Ref<HTMLDivElement>;
  children: ReactNode;
}

/** A dialog over a dimmed backdrop. Escape and a press on the backdrop close it; the rest of the props style the panel. */
export function Modal({ label, onClose, backdrop, children, ...panel }: ModalProps) {
  // A drag that starts in the panel and ends on the backdrop fires a click there; only a press on the backdrop closes.
  const pressedBackdrop = useRef(false);
  const close = useRef(onClose);
  close.current = onClose;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close.current();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  return (
    <Box
      position="fixed"
      inset="0"
      zIndex="modal"
      bg="overlay"
      display="flex"
      alignItems="center"
      justifyContent="center"
      p="4"
      {...backdrop}
      onMouseDown={(e) => (pressedBackdrop.current = e.target === e.currentTarget)}
      onClick={(e) => pressedBackdrop.current && e.target === e.currentTarget && onClose()}
    >
      <Box
        role="dialog"
        aria-modal="true"
        aria-label={label}
        bg="surface"
        borderWidth="1px"
        borderColor="border"
        borderRadius="md"
        p="3"
        maxW="full"
        maxH="full"
        display="flex"
        flexDirection="column"
        gap="2"
        {...panel}
      >
        {children}
      </Box>
    </Box>
  );
}
