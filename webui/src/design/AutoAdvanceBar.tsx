import { Box, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import type { SessionInfo } from "../generated/server";
import { slugOf } from "../slug";
import { Button } from "../ui/button/Button";
import type { Refusable } from "../useWrite";
import {
  stateToken,
  type CancelAutoAdvance,
  type SetAutoAdvance,
  type SetAutoQueue,
} from "./open";
import { AutoMenu, type AutoMenuState } from "./AutoMenu";

export type RunCall = (call: () => Promise<Refusable>) => Promise<void>;

// Counts down to the server's deadline; undefined with no countdown running.
function useCountdown(at: string | undefined) {
  const deadline = at ? Date.parse(at) : undefined;
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

// One countdown line with its Cancel; nothing with no countdown running. `strip` is the phone's own row above the footer.
function CountdownRow({
  at,
  text,
  cancelName,
  disabled,
  onCancel,
  strip,
}: {
  at: string | undefined;
  text: (secondsLeft: number) => string;
  cancelName?: string;
  disabled: boolean;
  onCancel: () => void;
  strip: boolean;
}) {
  const secondsLeft = useCountdown(at);
  if (secondsLeft === undefined) return null;
  return (
    <Box
      display="flex"
      gap="3"
      alignItems="center"
      {...(strip ? { justifyContent: "space-between", minH: "44px", px: "1", "data-part": "countdown-strip" } : {})}
    >
      <Text color="tone.waiting.fg" whiteSpace="nowrap">
        {text(secondsLeft)}
      </Text>
      <Button
        emphasis="secondary"
        aria-label={cancelName}
        disabled={disabled}
        {...(strip ? { h: "40px" } : {})}
        onClick={onCancel}
      >
        Cancel
      </Button>
    </Box>
  );
}

// The session's ticket is `project:epic/NN`; the countdown names the epic.
function epicName(session: SessionInfo): string {
  const address = session.ticket ?? "";
  return slugOf(address.split("/")[0]);
}

// "Continuing in Ns" and "Queuing <epic> in Ns", each with its Cancel; they stack when both run. Only the Auto-queue Cancel carries a name, so "Cancel" stays the auto-advance one.
export function AutoCountdown({
  session,
  disabled,
  run,
  cancelAutoAdvance,
  cancelAutoQueue,
  strip = false,
}: {
  session: SessionInfo;
  disabled: boolean;
  run: RunCall;
  cancelAutoAdvance: CancelAutoAdvance;
  cancelAutoQueue: CancelAutoAdvance;
  strip?: boolean;
}) {
  const state = stateToken(session);
  return (
    // Hidden while empty so it adds no gap to the footer column it sits in.
    <Box display="flex" flexDirection="column" gap="1" _empty={{ display: "none" }}>
      <CountdownRow
        at={session.auto_advance_at}
        text={(s) => `Continuing in ${s}s`}
        disabled={disabled}
        strip={strip}
        onCancel={() => run(() => cancelAutoAdvance({ id: session.id, state }))}
      />
      <CountdownRow
        at={session.auto_queue_at}
        text={(s) => `Queuing ${epicName(session)} in ${s}s`}
        cancelName="Cancel auto-queue"
        disabled={disabled}
        strip={strip}
        onCancel={() => run(() => cancelAutoQueue({ id: session.id, state }))}
      />
    </Box>
  );
}

// The Auto menu, and on a wide screen the countdown with its Cancel while one runs.
export function AutoAdvanceBar({
  session,
  disabled,
  run,
  setAutoAdvance,
  setAutoQueue,
  cancelAutoAdvance,
  cancelAutoQueue,
  menu,
  phone = false,
}: {
  /** The phone footer has no room for the long switch names. */
  phone?: boolean;
  session: SessionInfo;
  disabled: boolean;
  run: RunCall;
  setAutoAdvance: SetAutoAdvance;
  setAutoQueue: SetAutoQueue;
  cancelAutoAdvance: CancelAutoAdvance;
  cancelAutoQueue: CancelAutoAdvance;
  menu: AutoMenuState;
}) {
  const state = stateToken(session);
  return (
    <Box display="flex" gap="3" alignItems="center">
      <AutoMenu
        state={menu}
        tall={phone}
        switches={[
          {
            label: "Auto-advance",
            short: phone ? "Advance" : undefined,
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
            short: phone ? "Queue" : undefined,
            description: "Queue the implementation epic when it's ready.",
            on: session.auto_queue,
            disabled,
            onFlip: () =>
              run(() =>
                setAutoQueue({
                  id: session.id,
                  state,
                  on: !session.auto_queue,
                }),
              ),
          },
        ]}
      />
      {!phone && (
        <AutoCountdown
          session={session}
          disabled={disabled}
          run={run}
          cancelAutoAdvance={cancelAutoAdvance}
          cancelAutoQueue={cancelAutoQueue}
        />
      )}
    </Box>
  );
}
