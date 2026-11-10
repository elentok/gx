import { useEffect, useRef } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { postDevRestart as defaultPostDevRestart } from "../dev";
import type { DevState } from "../generated/server";
import { showToast } from "../ui/toaster";

// Whichever pill is rendered (sidebar row or footer line) is the popover's trigger.
function openDevPopover() {
  document.querySelector<HTMLElement>('[data-part="dev-pill"]')?.click();
}

/**
 * Toasts once each time a refetched snapshot moves Dev mode to down. The first snapshot only sets
 * the baseline, so a page that loads already down stays quiet.
 */
export function useDevToast({
  postDevRestart = defaultPostDevRestart,
  openPopover = openDevPopover,
}: { postDevRestart?: () => void; openPopover?: () => void } = {}) {
  const state: DevState | undefined = useConnectionState(useConnectionContext()).snapshot?.dev?.state;
  const prev = useRef<DevState | undefined>(undefined);
  useEffect(() => {
    if (!state) return;
    if (prev.current && prev.current !== "down" && state === "down") {
      showToast({
        type: "error",
        title: "Vite is down. gx serves the built files.",
        actions: [
          { label: "Details", onClick: openPopover },
          { label: "Restart Vite", onClick: postDevRestart },
        ],
      });
    }
    prev.current = state;
  }, [state, openPopover, postDevRestart]);
}
