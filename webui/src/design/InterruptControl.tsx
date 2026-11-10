import { Spinner } from "@chakra-ui/react";
import { useEffect, useRef, useState, type ComponentProps } from "react";
import { Button } from "../ui/button/Button";
import { hasLayer } from "../ui/modal/layers";
import { useWide } from "../useWide";
import { InterruptPopover } from "./InterruptPopover";

// Anything open that takes Esc first. The completion menu's own handler has not run when we judge, so it is
// found by its DOM; the Questions view leaves on this same Escape.
const ON_TOP = '[role="dialog"],[role="alertdialog"],[role="menu"],[role="listbox"],[data-questions-view]';

// The Interrupt button, its confirm and the confirm's open state. Rendered only while the composer is Working.
export function InterruptControl({
  connected,
  pending,
  onInterrupt,
  buttonProps,
}: {
  connected: boolean;
  /** A write is in flight. */
  pending: boolean;
  /** Runs once the confirm is accepted; only called while connected. */
  onInterrupt: () => Promise<unknown>;
  /** Layout props for the button (the Design composer's slot). */
  buttonProps?: Partial<Omit<ComponentProps<typeof Button>, "ref">>;
}) {
  const [open, setOpen] = useState(false);
  const [viaKey, setViaKey] = useState(false);
  const ref = useRef<HTMLButtonElement>(null);
  const wide = useWide();
  const disabled = !connected || pending;
  useEffect(() => {
    if (!wide || disabled) return;
    // Capture phase: judge what is on top as it was before this same Escape closes it.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || open || hasLayer() || document.querySelector(ON_TOP)) return;
      setViaKey(true);
      setOpen(true);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [wide, disabled, open]);
  return (
    <>
      <Button
        ref={ref}
        emphasis="secondary"
        {...buttonProps}
        disabled={disabled}
        title={wide ? "Interrupt (Esc)" : undefined}
        aria-expanded={open}
        onClick={() => {
          setViaKey(false);
          setOpen(!open);
        }}
      >
        {pending && <Spinner size="sm" />}
        Interrupt
      </Button>
      <InterruptPopover
        open={open}
        initialFocus={viaKey ? "confirm" : "cancel"}
        disabled={disabled}
        anchorRef={ref}
        onConfirm={() => {
          setOpen(false);
          return connected ? onInterrupt() : Promise.resolve();
        }}
        onClose={() => setOpen(false)}
      />
    </>
  );
}
