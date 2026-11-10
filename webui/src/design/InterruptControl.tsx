import { Spinner } from "@chakra-ui/react";
import { useRef, useState, type ComponentProps } from "react";
import { Button } from "../ui/button/Button";
import { InterruptPopover } from "./InterruptPopover";

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
  const ref = useRef<HTMLButtonElement>(null);
  const disabled = !connected || pending;
  return (
    <>
      <Button
        ref={ref}
        emphasis="secondary"
        {...buttonProps}
        disabled={disabled}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {pending && <Spinner size="sm" />}
        Interrupt
      </Button>
      <InterruptPopover
        open={open}
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
