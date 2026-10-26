import { Box, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import type { SessionInfo } from "../generated/server";
import { Button } from "../ui/button/Button";
import type { Refusable } from "../useWrite";
import {
  stateToken,
  type CancelAutoAdvance,
  type SetAutoAdvance,
} from "./open";
import { useWide } from "./useWide";

export type RunCall = (call: () => Promise<Refusable>) => Promise<void>;

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
  const wide = useWide();
  const secondsLeft = useAutoAdvanceCountdown(session);
  const state = stateToken(session);
  return (
    <Box display="flex" gap="3" alignItems="center">
      <Button
        emphasis="ghost"
        tone="muted"
        role="switch"
        aria-checked={session.auto_advance}
        aria-label={`Auto-advance ${session.auto_advance ? "on" : "off"}`}
        disabled={disabled}
        onClick={() =>
          run(() =>
            setAutoAdvance({
              id: session.id,
              state,
              off: session.auto_advance,
            }),
          )
        }
      >
        <Box
          aria-hidden
          w="28px"
          h="16px"
          borderRadius="full"
          bg={session.auto_advance ? "tone.waiting.fg" : "text.dim"}
          position="relative"
          flexShrink={0}
        >
          <Box
            position="absolute"
            top="2px"
            left={session.auto_advance ? "14px" : "2px"}
            w="12px"
            h="12px"
            borderRadius="full"
            bg="surface"
          />
        </Box>
        {wide && <Text as="span">Auto-advance</Text>}
      </Button>
      {secondsLeft !== undefined && (
        <>
          <Text>Continuing in {secondsLeft}s</Text>
          <Button
            emphasis="secondary"
            disabled={disabled}
            onClick={() =>
              run(() => cancelAutoAdvance({ id: session.id, state }))
            }
          >
            Cancel
          </Button>
        </>
      )}
    </Box>
  );
}
