import { chakra, Portal, Tooltip } from "@chakra-ui/react";
import { useEffect, useRef, useState, type ReactNode } from "react";

interface Props {
  /** The accessible name of the trigger. */
  label: string;
  /** The tooltip text. */
  content: ReactNode;
  /** The trigger face. It defaults to the "i" marker. */
  children?: ReactNode;
  /** "marker" is the 14px circle; "chip" is a text pill that takes its colour from `chipColor`. */
  variant?: "marker" | "chip";
  /** The chip's border and text colour (a Chakra token). */
  chipColor?: string;
}

const markerStyle = {
  w: "14px",
  h: "14px",
  borderRadius: "full",
  borderColor: "text.muted",
  color: "text.muted",
  fontSize: "10px",
  fontStyle: "italic",
} as const;

// Controlled, because the installed Chakra Tooltip has no open-on-click prop. A tap toggles it.
export function InfoTip({ label, content, children = "i", variant = "marker", chipColor = "text.muted" }: Props) {
  const [open, setOpen] = useState(false);
  const marker = useRef<HTMLButtonElement>(null);
  // Native listeners, because the menu listens natively and React's synthetic stopPropagation
  // runs after it. The menu handles arrows, typeahead, Enter and Space; keep them off this marker.
  useEffect(() => {
    const el = marker.current;
    if (!el) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Tab" && e.key !== "Escape") e.stopPropagation();
    };
    el.addEventListener("keydown", onKeyDown);
    return () => el.removeEventListener("keydown", onKeyDown);
  }, []);
  // The menu dismisses on Escape in the capture phase, so only a capture listener on window runs
  // first. While the tooltip is open, Escape closes it and goes no further.
  useEffect(() => {
    if (!open) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopImmediatePropagation();
      setOpen(false);
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, [open]);
  return (
    <Tooltip.Root
      open={open}
      onOpenChange={(e) => setOpen(e.open)}
      closeOnClick={false}
      closeOnPointerDown={false}
      openDelay={0}
      closeDelay={0}
      interactive
    >
      <Tooltip.Trigger asChild>
        <chakra.button
          type="button"
          aria-label={label}
          ref={marker}
          onClick={() => setOpen((o) => !o)}
          onFocus={() => setOpen(true)}
          onBlur={() => setOpen(false)}
          display="inline-flex"
          alignItems="center"
          justifyContent="center"
          flexShrink={0}
          borderWidth="1px"
          lineHeight="1"
          cursor="pointer"
          {...(variant === "chip"
            ? { px: "7px", py: "1px", borderRadius: "full", borderColor: chipColor, color: chipColor, fontSize: "12px", style: { fontVariantNumeric: "tabular-nums" } }
            : markerStyle)}
        >
          {children}
        </chakra.button>
      </Tooltip.Trigger>
      <Portal>
        <Tooltip.Positioner>
          <Tooltip.Content bg="surface.raised" color="text" borderWidth="1px" borderColor="border" borderRadius="md" px="2" py="1" fontSize="xs" maxW="64">
            {content}
          </Tooltip.Content>
        </Tooltip.Positioner>
      </Portal>
    </Tooltip.Root>
  );
}
