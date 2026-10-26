import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useConnectionContext } from "../connection";
import type { DesignOpenRequest, SessionInfo } from "../generated/server";
import { Button } from "../ui/button/Button";
import { postOpenDesign, postStopSession, stateToken, type OpenDesign, type StopSession } from "./open";

/** Runs one write at a time: a second click while one is in flight does nothing. */
export function useWrite() {
  const conn = useConnectionContext();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const run = async (call: () => Promise<{ refused?: boolean; message?: string }>, onDone?: () => void) => {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      const res = await call();
      if (res.refused) setError(res.message ?? "The request was refused.");
      else {
        conn.refresh();
        onDone?.();
      }
    } catch {
      setError("Could not reach gx. Try again.");
    } finally {
      setPending(false);
    }
  };
  return { run, pending, error };
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
  const { run, pending, error } = useWrite();
  return (
    <Box display="flex" alignItems="center" gap="2">
      <Button tone="running" disabled={pending} onClick={() => run(() => open({ epic: address }), onStarted)}>
        Start design session
      </Button>
      <Failure>{error}</Failure>
    </Box>
  );
}

/** Opens a new session for the Design; asks first, and sends the state token, when the shown one is live. */
export function useStartNew(address: string, shown: SessionInfo, open: OpenDesign = postOpenDesign) {
  const write = useWrite();
  const isLive = shown.status.state !== "exited";
  const startNew = () => {
    if (isLive && !window.confirm("Stop the current session and start a new one?")) return;
    const body: DesignOpenRequest = { epic: address, new: true, ...(isLive ? { state: stateToken(shown) } : {}) };
    write.run(() => open(body));
  };
  return { startNew, pending: write.pending, error: write.error };
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
  const { run, pending, error } = useWrite();
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
                  Stop {s.name || s.id}
                </Button>
              )}
            </Box>
          ))}
        </Box>
      )}
      <Failure>{error || startNew.error}</Failure>
    </Box>
  );
}
