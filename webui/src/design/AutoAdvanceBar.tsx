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
import { AutoMenu, type AutoMenuState } from "./AutoMenu";

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

// The Auto menu, and the countdown with its Cancel while one runs.
export function AutoAdvanceBar({
  session,
  disabled,
  run,
  setAutoAdvance,
  cancelAutoAdvance,
  menu,
}: {
  session: SessionInfo;
  disabled: boolean;
  run: RunCall;
  setAutoAdvance: SetAutoAdvance;
  cancelAutoAdvance: CancelAutoAdvance;
  menu: AutoMenuState;
}) {
  const secondsLeft = useAutoAdvanceCountdown(session);
  const state = stateToken(session);
  return (
    <Box display="flex" gap="3" alignItems="center">
      <AutoMenu
        state={menu}
        switches={[
          {
            label: "Auto-advance",
            description: "Go to the next step after a short countdown.",
            on: session.auto_advance,
            disabled,
            onFlip: () =>
              run(() =>
                setAutoAdvance({
                  id: session.id,
                  state,
                  off: session.auto_advance,
                }),
              ),
          },
          {
            label: "Auto-queue",
            description:
              "Queue the implementation epic when it's ready. Coming soon.",
            on: false,
            disabled: true,
          },
        ]}
      />
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
