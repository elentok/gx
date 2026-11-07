import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import type { PostPrompt } from "../design/prompt";
import { usePromptSend } from "../design/usePromptSend";
import { postStopSession, stateToken, type StopSession } from "../design/open";
import { agentSection } from "../design/SessionControls";
import type { SessionInfo } from "../generated/server";
import { useWrite } from "../useWrite";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu";
import { MoreActionsButton } from "../ui/menu/MoreActionsButton";
import { useWide } from "../useWide";
import { canManage, isExited, postRemoveSession, removeSession, type RemoveSession } from "./sessionWrites";

/** The "⋯" menu: the Agent block, plus Stop (while live) and Remove for a session a person manages. */
export function SessionActions({
  session,
  onRemoved,
  stop = postStopSession,
  remove = postRemoveSession,
  promote,
}: {
  session: SessionInfo;
  onRemoved: () => void;
  stop?: StopSession;
  remove?: RemoveSession;
  /** Adds "Promote to Design"; absent for a gx-driven agent. */
  promote?: {
    lastMessage?: string;
    hasMessages: boolean;
    connected: boolean;
    /** Why the composer cannot send; the caller decides the text. */
    reason?: string;
    post?: PostPrompt;
    onStale?: () => void;
  };
}) {
  const { run, pending } = useWrite();
  const promoteSend = usePromptSend({
    session,
    lastMessage: promote?.lastMessage,
    connected: promote?.connected ?? false,
    post: promote?.post,
    onStale: promote?.onStale,
    toastRefusal: true,
  });
  const [confirming, setConfirming] = useState(false);
  const wide = useWide();
  const live = !isExited(session);
  const name = session.name || session.id;
  const groups: MenuEntry[][] = [];
  const agent = agentSection(session, wide);
  if (agent) groups.push(agent);
  if (promote) {
    const off = promote.reason ?? (promote.hasMessages ? undefined : "No messages yet. Promote turns on after the first message.");
    groups.push([
      {
        value: "promote",
        label: "Promote to Design",
        hint: off,
        disabled: !promoteSend.enabled || off !== undefined,
        onSelect: () => promoteSend.send({ promote: true }),
      },
    ]);
  }
  if (canManage(session)) {
    const items: MenuEntry[] = [];
    if (live)
      items.push({ value: "stop", label: "Stop", danger: true, disabled: pending, onSelect: () => run(() => stop({ id: session.id, state: stateToken(session) })) });
    items.push({ value: "remove", label: "Remove", danger: true, disabled: pending, onSelect: () => setConfirming(true) });
    groups.push(items);
  }
  // A queue session with no tier, model or effort has nothing to put in the menu.
  if (groups.length === 0) return null;
  return (
    <Box>
      <GxMenu trigger={<MoreActionsButton />} items={menuGroups(groups)} />
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
