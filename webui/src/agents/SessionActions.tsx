import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import { postStopSession, stateToken, type StopSession } from "../design/open";
import { agentSection } from "../design/SessionControls";
import type { SessionInfo } from "../generated/server";
import { useWrite } from "../useWrite";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { MoreActionsMenu, type MoreActionsItem, type MoreActionsSection } from "../ui/more-actions/MoreActionsMenu";
import { useWide } from "../useWide";
import { canManage, isExited, postRemoveSession, removeSession, type RemoveSession } from "./sessionWrites";

/** The "⋯" menu: the Agent block, plus Stop (while live) and Remove for a session a person manages. */
export function SessionActions({
  session,
  onRemoved,
  stop = postStopSession,
  remove = postRemoveSession,
}: {
  session: SessionInfo;
  onRemoved: () => void;
  stop?: StopSession;
  remove?: RemoveSession;
}) {
  const { run, pending } = useWrite();
  const [confirming, setConfirming] = useState(false);
  const wide = useWide();
  const live = !isExited(session);
  const name = session.name || session.id;
  const sections: MoreActionsSection[] = [];
  const agent = agentSection(session, wide);
  if (agent) sections.push(agent);
  if (canManage(session)) {
    const items: MoreActionsItem[] = [];
    if (live)
      items.push({ value: "stop", label: "Stop", danger: true, disabled: pending, onSelect: () => run(() => stop({ id: session.id, state: stateToken(session) })) });
    items.push({ value: "remove", label: "Remove", danger: true, disabled: pending, onSelect: () => setConfirming(true) });
    sections.push(items);
  }
  // A queue session with no tier, model or effort has nothing to put in the menu.
  if (sections.length === 0) return null;
  return (
    <Box>
      <MoreActionsMenu sections={sections} />
      {confirming && (
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
      )}
    </Box>
  );
}
