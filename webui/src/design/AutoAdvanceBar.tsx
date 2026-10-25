import { Box, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import type { SessionInfo } from "../generated/server";
import { Button } from "../ui/button/Button";
import {
  stateToken,
  type CancelAutoAdvance,
  type SetAutoAdvance,
} from "./open";

export type RunCall = (
  call: () => Promise<{ refused?: boolean; message?: string }>,
  failure: string,
) => Promise<void>;

// Counts down to the server's deadline; undefined with no countdown running.
export function useAutoAdvanceCountdown(session: SessionInfo) {
  const deadline = session.auto_advance_at
    ? Date.parse(session.auto_advance_at)
    : undefined;
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (deadline === undefined) return;
    setNow(Date.now());
    const t = setInterval(() => setNow(Date.now()), 250);
    return () => clearInterval(t);
  }, [deadline]);
  return deadline === undefined
    ? undefined
    : Math.max(0, Math.ceil((deadline - now) / 1000));
}

// The switch, and the countdown with its Cancel while one runs.
export function AutoAdvanceBar({
  session,
  disabled,
  run,
  setAutoAdvance,
  cancelAutoAdvance,
}: {
  session: SessionInfo;
  disabled: boolean;
  run: RunCall;
  setAutoAdvance: SetAutoAdvance;
  cancelAutoAdvance: CancelAutoAdvance;
}) {
  const secondsLeft = useAutoAdvanceCountdown(session);
  const state = stateToken(session);
  return (
    <Box display="flex" gap="3" alignItems="center">
      <Button
        emphasis="ghost"
        tone="muted"
        role="switch"
        aria-checked={session.auto_advance}
        disabled={disabled}
        onClick={() =>
          run(
            () =>
              setAutoAdvance({
                id: session.id,
                state,
                off: session.auto_advance,
              }),
            "Could not switch auto-advance. Try again.",
          )
        }
      >
        Auto-advance {session.auto_advance ? "on" : "off"}
      </Button>
      {secondsLeft !== undefined && (
        <>
          <Text>Continuing in {secondsLeft}s</Text>
          <Button
            emphasis="secondary"
            disabled={disabled}
            onClick={() =>
              run(
                () => cancelAutoAdvance({ id: session.id, state }),
                "Could not cancel. Try again.",
              )
            }
          >
            Cancel
          </Button>
        </>
      )}
    </Box>
  );
}
