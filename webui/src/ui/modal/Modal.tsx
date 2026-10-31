import { Box, type BoxProps } from "@chakra-ui/react";
import { useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent, type ReactNode, type Ref } from "react";
import { useIsTopLayer } from "./layers";

interface ModalProps extends Omit<BoxProps, "children"> {
  label: string;
  onClose: () => void;
  /** Overrides for the backdrop, such as a bottom sheet's placement. */
  backdrop?: BoxProps;
  ref?: Ref<HTMLDivElement>;
  /** Escape closes the topmost layer; false for a dialog that handles Escape itself. */
  closeOnEscape?: boolean;
  /** A press on the backdrop closes; false to ignore it. */
  closeOnBackdrop?: boolean;
  children: ReactNode;
}

const FOCUSABLE = 'a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])';

function focusables(panel: HTMLElement) {
  return [...panel.querySelectorAll<HTMLElement>(FOCUSABLE)].filter(
    (el) => !el.hasAttribute("disabled") && el.getAttribute("type") !== "hidden",
  );
}

/** Wraps Tab and Shift-Tab at the panel's ends; inner moves are left to the browser. */
function trapTab(e: ReactKeyboardEvent<HTMLElement>) {
  if (e.key !== "Tab") return;
  const panel = e.currentTarget;
  const items = focusables(panel);
  const first = items[0];
  const last = items[items.length - 1];
  const active = document.activeElement;
  if (!first || !last) return e.preventDefault();
  const wrapTo = !panel.contains(active) ? first : e.shiftKey && (active === first || active === panel) ? last : !e.shiftKey && active === last ? first : null;
  if (!wrapTo) return;
  e.preventDefault();
  wrapTo.focus();
}

/** A dialog over a dimmed backdrop. Escape (when topmost) and a press on the backdrop close it; the rest of the props style the panel. */
export function Modal({ label, onClose, backdrop, closeOnEscape = true, closeOnBackdrop = true, children, ...panel }: ModalProps) {
  // A drag that starts in the panel and ends on the backdrop fires a click there; only a press on the backdrop closes.
  const pressedBackdrop = useRef(false);
  const close = useRef(onClose);
  close.current = onClose;
  const isTop = useIsTopLayer();
  const top = useRef(isTop);
  top.current = isTop;
  useEffect(() => {
    if (!closeOnEscape) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && top.current() && close.current();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [closeOnEscape]);
  const backdropRef = useRef<HTMLDivElement>(null);
  // Keep focus in the layer from the start, so the first Tab does not land on the page behind.
  useEffect(() => {
    const panelEl = backdropRef.current?.firstElementChild as HTMLElement | null;
    if (!panelEl || panelEl.contains(document.activeElement)) return;
    (focusables(panelEl)[0] ?? panelEl).focus();
  }, []);
  return (
    <Box
      ref={backdropRef}
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
      onClick={(e) => closeOnBackdrop && pressedBackdrop.current && e.target === e.currentTarget && onClose()}
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
        tabIndex={-1}
        outline="none"
        {...panel}
        onKeyDown={trapTab}
      >
        {children}
      </Box>
    </Box>
  );
}
