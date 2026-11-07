import { Box, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { postStopSession, stateToken, type StopSession } from "../design/open";
import type { SessionInfo } from "../generated/server";
import { useWrite } from "../useWrite";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import type { MenuEntry } from "../ui/menu/Menu";
import { canManage, isExited, postRemoveSession, removeSession, type RemoveSession } from "./sessionWrites";

/** Stop (while live) and Remove menu items for a session a person manages, plus Remove's confirmation. Shared by the agent page and the sidebar's chat rows. */
export function useSessionManage(
  session: SessionInfo | undefined,
  onRemoved: () => void,
  { stop = postStopSession, remove = postRemoveSession }: { stop?: StopSession; remove?: RemoveSession } = {},
): { items: MenuEntry[]; dialog: ReactNode } {
  const { run, pending } = useWrite();
  const [confirming, setConfirming] = useState(false);
  if (!session || !canManage(session)) return { items: [], dialog: null };
  const live = !isExited(session);
  const name = session.name || session.id;
  const items: MenuEntry[] = [];
  if (live)
    items.push({ value: "stop", label: "Stop", danger: true, disabled: pending, onSelect: () => run(() => stop({ id: session.id, state: stateToken(session) })) });
  items.push({ value: "remove", label: "Remove", danger: true, disabled: pending, onSelect: () => setConfirming(true) });
  const dialog = confirming ? (
    <Modal label={`Remove ${name}`} onClose={() => setConfirming(false)} maxW="sm">
      <Text>{live ? `Stop and remove ${name}? Its session is deleted.` : `Remove ${name}? Its session is deleted.`}</Text>
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="secondary" onClick={() => setConfirming(false)} autoFocus>
          Keep
        </Button>
        <Button
          tone="paused"
          disabled={pending}
          onClick={() => {
            setConfirming(false);
            run(() => removeSession(session, { stop, remove }), { onDone: onRemoved });
          }}
        >
          Remove
        </Button>
      </Box>
    </Modal>
  ) : null;
  return { items, dialog };
}
