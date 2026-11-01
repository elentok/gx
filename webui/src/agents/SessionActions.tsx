import { Box, chakra, Menu, Text } from "@chakra-ui/react";
import { useState } from "react";
import { postStopSession, stateToken, type StopSession } from "../design/open";
import { MenuFrame, menuItemStyle } from "../design/SessionControls";
import type { SessionInfo } from "../generated/server";
import { useWrite } from "../useWrite";
import { Button } from "../ui/button/Button";
import { GrabHandle, sheetPanel } from "../ui/modal/BottomSheet";
import { Modal } from "../ui/modal/Modal";
import { useWide } from "../useWide";
import { canManage, isExited, postRemoveSession, removeSession, type RemoveSession } from "./sessionWrites";

/** The "⋯" menu with Stop (while live) and Remove for the selected agent. Nothing for a Queue agent. */
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
  const sheetMode = !useWide();
  if (!canManage(session)) return null;
  const live = !isExited(session);
  const name = session.name || session.id;
  const item = menuItemStyle(sheetMode);
  return (
    <Box>
      <Menu.Root lazyMount unmountOnExit positioning={{ placement: "bottom-end" }}>
        <Menu.Trigger asChild>
          <chakra.button
            type="button"
            aria-label="More actions"
            cursor="pointer"
            color="text.muted"
            fontSize="xl"
            lineHeight="1"
            w="44px"
            h="44px"
            borderWidth="1px"
            borderColor="border"
            borderRadius="md"
            display="inline-flex"
            alignItems="center"
            justifyContent="center"
          >
            ⋯
          </chakra.button>
        </Menu.Trigger>
        <MenuFrame sheet={sheetMode}>
          <Menu.Content bg="surface.raised" borderWidth="1px" borderColor="border" minW={sheetMode ? "64" : "200px"} {...(sheetMode && { ...sheetPanel, pb: "6" })}>
            {sheetMode && <GrabHandle my="2" />}
            {live && (
              <Menu.Item value="stop" disabled={pending} onClick={() => run(() => stop({ id: session.id, state: stateToken(session) }))} {...item} color="tone.deadlocked.fg">
                Stop
              </Menu.Item>
            )}
            <Menu.Item value="remove" disabled={pending} onClick={() => setConfirming(true)} {...item} color="tone.paused.fg">
              Remove
            </Menu.Item>
          </Menu.Content>
        </MenuFrame>
      </Menu.Root>
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
