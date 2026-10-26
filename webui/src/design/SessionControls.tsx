import { Box, Spinner, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useConnectionContext } from "../connection";
import type { DesignOpenRequest, SessionInfo } from "../generated/server";
import { Button } from "../ui/button/Button";
import { useWrite } from "../useWrite";
import {
  postDeleteDesign,
  postOpenDesign,
  postStopSession,
  stateToken,
  type DeleteDesign,
  type OpenDesign,
  type StopSession,
} from "./open";

/** The shared write hook, with a connection refresh after every accepted write. */
export function useDesignWrite() {
  const conn = useConnectionContext();
  const { run, pending } = useWrite();
  const runThenRefresh: typeof run = (call, opts) =>
    run(call, {
      ...opts,
      onDone: (res) => {
        conn.refresh();
        opts?.onDone?.(res);
      },
    });
  return { run: runThenRefresh, pending };
}

export const Failure = ({ children }: { children: string }) =>
  children ? (
    <Text role="alert" fontSize="sm" color="tone.deadlocked.fg">
      {children}
    </Text>
  ) : null;

/** Starts the Design's session. `onStarted` runs once it exists, e.g. to go to the Design page. */
export function StartDesignButton({
  address,
  open = postOpenDesign,
  onStarted,
}: {
  address: string;
  open?: OpenDesign;
  onStarted?: () => void;
}) {
  const { run, pending } = useDesignWrite();
  return (
    <Box display="flex" alignItems="center" gap="2">
      <Button tone="running" disabled={pending} onClick={() => run(() => open({ epic: address }), { onDone: onStarted })}>
        {pending && <Spinner size="sm" />}
        Start design session
      </Button>
    </Box>
  );
}

/** Deletes an uncharted Design after an inline confirm; success leaves for the Epics page. */
export function DeleteDesignButton({
  address,
  remove = postDeleteDesign,
  onDeleted,
}: {
  address: string;
  remove?: DeleteDesign;
  onDeleted: () => void;
}) {
  const { run, pending } = useDesignWrite();
  const [confirming, setConfirming] = useState(false);
  return (
    <Box display="flex" alignItems="center" gap="2">
      {confirming ? (
        <>
          <Text fontSize="sm">Delete this design?</Text>
          <Button size="sm" emphasis="secondary" disabled={pending} onClick={() => setConfirming(false)}>
            Keep
          </Button>
          <Button size="sm" tone="paused" disabled={pending} onClick={() => run(() => remove({ epic: address }), { onDone: onDeleted })}>
            {pending && <Spinner size="sm" />}
            Delete
          </Button>
        </>
      ) : (
        <Button size="sm" emphasis="ghost" tone="paused" onClick={() => setConfirming(true)}>
          Delete design
        </Button>
      )}
    </Box>
  );
}

/** Opens a new session for the Design; asks first, and sends the state token, when the shown one is live. */
export function useStartNew(address: string, shown: SessionInfo, open: OpenDesign = postOpenDesign) {
  const write = useDesignWrite();
  const isLive = shown.status.state !== "exited";
  const startNew = () => {
    if (isLive && !window.confirm("Stop the current session and start a new one?")) return;
    const body: DesignOpenRequest = { epic: address, new: true, ...(isLive ? { state: stateToken(shown) } : {}) };
    write.run(() => open(body));
  };
  return { startNew, pending: write.pending };
}

/** The chat header: the live-session menu, and New session. */
export function SessionHeader({
  address,
  shown,
  live,
  onSelect,
  open = postOpenDesign,
  stop = postStopSession,
}: {
  address: string;
  shown: SessionInfo;
  /** Live design sessions, newest first. */
  live: SessionInfo[];
  onSelect: (id: string) => void;
  open?: OpenDesign;
  stop?: StopSession;
}) {
  const { run, pending } = useDesignWrite();
  const [menu, setMenu] = useState(false);
  const startNew = useStartNew(address, shown, open);

  return (
    <Box display="flex" flexDirection="column" gap="1">
      <Box display="flex" alignItems="center" gap="2" justifyContent="flex-end">
        {live.length > 1 && (
          <Button size="sm" emphasis="secondary" aria-expanded={menu} onClick={() => setMenu((m) => !m)}>
            {live.length} live {menu ? "▴" : "▾"}
          </Button>
        )}
        <Button size="sm" emphasis="secondary" disabled={pending || startNew.pending} onClick={startNew.startNew}>
          {startNew.pending && <Spinner size="sm" />}
          New session
        </Button>
      </Box>
      {menu && (
        <Box role="menu" aria-label="Live sessions" display="flex" flexDirection="column" gap="1" fontSize="sm">
          {live.map((s) => (
            <Box key={s.id} display="flex" alignItems="center" gap="2" justifyContent="space-between">
              <Button
                role="menuitem"
                emphasis="ghost"
                aria-current={s.id === shown.id}
                onClick={() => {
                  onSelect(s.id);
                  setMenu(false);
                }}
              >
                {s.name || s.id}
                {s.id === shown.id && " ✓"}
              </Button>
              {s.id !== shown.id && (
                <Button size="sm" tone="deadlocked" disabled={pending} onClick={() => run(() => stop({ id: s.id, state: stateToken(s) }))}>
                  {pending && <Spinner size="sm" />}
                  Stop {s.name || s.id}
                </Button>
              )}
            </Box>
          ))}
        </Box>
      )}
    </Box>
  );
}
