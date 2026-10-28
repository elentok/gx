import { Box, chakra, Menu, Spinner, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { useConnectionContext } from "../connection";
import type { DesignOpenRequest, SessionInfo } from "../generated/server";
import { Button } from "../ui/button/Button";
import { GrabHandle, sheetPanel } from "../ui/modal/BottomSheet";
import { useWide } from "../useWide";
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

/** Opens a new session for the Design; asks first, and sends the state token, when the shown one is live. */
export function startNew(
  run: ReturnType<typeof useDesignWrite>["run"],
  address: string,
  shown: SessionInfo,
  open: OpenDesign = postOpenDesign,
) {
  const isLive = shown.status.state !== "exited";
  if (isLive && !window.confirm("Stop the current session and start a new one?")) return;
  const body: DesignOpenRequest = { epic: address, new: true, ...(isLive ? { state: stateToken(shown) } : {}) };
  run(() => open(body));
}

/** The popover's positioner, or on a phone a dimmed backdrop that holds the menu as a bottom sheet and closes it on a press. */
function MenuFrame({ sheet, children }: { sheet: boolean; children: ReactNode }) {
  if (!sheet) return <Menu.Positioner>{children}</Menu.Positioner>;
  return (
    <Menu.Context>
      {(menu) =>
        menu.open && (
          <Box
            position="fixed"
            inset="0"
            zIndex="modal"
            bg="overlay"
            display="flex"
            alignItems="flex-end"
            onClick={(e) => e.target === e.currentTarget && menu.setOpen(false)}
          >
            {children}
          </Box>
        )
      }
    </Menu.Context>
  );
}

const menuItemStyle = (sheetMode: boolean) =>
  ({
    cursor: "pointer",
    color: "text",
    ps: "3",
    _highlighted: { bg: "surface" },
    fontWeight: "600",
    fontSize: "17px",
    ...(sheetMode && { py: "3", minH: "44px" }),
  }) as const;
type MenuItemStyle = ReturnType<typeof menuItemStyle>;

/** The live sessions: pick one to show it, or stop one that is not shown. */
function LiveSessionRows({
  item,
  live,
  shown,
  pending,
  onSelect,
  onStop,
}: {
  item: MenuItemStyle;
  live: SessionInfo[];
  shown: SessionInfo;
  pending: boolean;
  onSelect: (id: string) => void;
  onStop: (s: SessionInfo) => void;
}) {
  return (
    <>
      <Text px="3" pt="2" mt="1" borderTopWidth="1px" borderColor="border" fontSize="xs" letterSpacing="wider" textTransform="uppercase" color="text.muted">
        Live sessions · {live.length}
      </Text>
      {live.map((s) => (
        <Box
          key={s.id}
          display="flex"
          alignItems="center"
          justifyContent="space-between"
          w="full"
          gap="2"
          pe="2"
          borderRadius="md"
          bg={s.id === shown.id ? "surface" : undefined}
        >
          <Menu.Item
            value={`session-${s.id}`}
            aria-current={s.id === shown.id}
            onClick={() => onSelect(s.id)}
            flex="1"
            w="auto"
            minW="0"
            justifyContent="space-between"
            {...item}
          >
            <Text as="span" truncate fontSize="13px" fontWeight="600">
              {s.name || s.id}
            </Text>
            {s.id === shown.id && <Text as="span" aria-hidden color="tone.running.fg">✓</Text>}
          </Menu.Item>
          {s.id !== shown.id && (
            <Menu.Item
              value={`stop-${s.id}`}
              closeOnSelect={false}
              aria-label={`Stop ${s.name || s.id}`}
              disabled={pending}
              onClick={() => onStop(s)}
              cursor="pointer"
              flex="none"
              w="auto"
              minH="32px"
              px="2.5"
              borderWidth="1px"
              borderColor="tone.deadlocked.fg"
              borderRadius="md"
              fontSize="13px"
              fontWeight="600"
              color="tone.deadlocked.fg"
            >
              {pending && <Spinner size="sm" />}
              Stop
            </Menu.Item>
          )}
        </Box>
      ))}
    </>
  );
}

/** "Delete design", which asks to confirm in place before it deletes. */
function DeleteRow({
  item,
  pending,
  confirming,
  setConfirming,
  onDelete,
}: {
  item: MenuItemStyle;
  pending: boolean;
  confirming: boolean;
  setConfirming: (confirming: boolean) => void;
  onDelete: () => void;
}) {
  if (!confirming)
    return (
      <Menu.Item value="delete-design" closeOnSelect={false} onClick={() => setConfirming(true)} {...item} color="tone.paused.fg">
        Delete design
      </Menu.Item>
    );
  return (
    <Box display="flex" alignItems="center" gap="2" px="3" py="1.5">
      <Text fontSize="sm">Delete this design?</Text>
      <Button size="sm" emphasis="secondary" disabled={pending} onClick={() => setConfirming(false)}>
        Keep
      </Button>
      <Button size="sm" tone="paused" disabled={pending} onClick={onDelete}>
        {pending && <Spinner size="sm" />}
        Delete
      </Button>
    </Box>
  );
}

/**
 * The header's "⋯" menu: New session, the live sessions (with 2 or more), and Delete design while it
 * can run. Renders nothing when none of them applies.
 */
export function DesignActionsMenu({
  address,
  shown,
  live,
  deletable,
  onSelect,
  onDeleted,
  open = postOpenDesign,
  stop = postStopSession,
  remove = postDeleteDesign,
}: {
  address: string;
  shown?: SessionInfo;
  /** Live design sessions, newest first. */
  live: SessionInfo[];
  /** The map has no Destination and no tickets yet, so it can still be deleted. */
  deletable?: boolean;
  onSelect: (id: string) => void;
  onDeleted: () => void;
  open?: OpenDesign;
  stop?: StopSession;
  remove?: DeleteDesign;
}) {
  const { run, pending } = useDesignWrite();
  const [confirming, setConfirming] = useState(false);
  const sheetMode = !useWide();
  const canDelete = deletable && live.length === 0;
  if (!shown && !canDelete) return null;

  const item = menuItemStyle(sheetMode);
  return (
    <Menu.Root
      lazyMount
      unmountOnExit
      positioning={{ placement: "bottom-end" }}
      onOpenChange={(e) => !e.open && setConfirming(false)}
    >
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
        <Menu.Content
          bg="surface.raised"
          borderWidth="1px"
          borderColor="border"
          minW={sheetMode ? "64" : "340px"}
          {...(sheetMode && { ...sheetPanel, pb: "6" })}
        >
          {sheetMode && <GrabHandle my="2" />}
          {shown && (
            <Menu.Item
              value="new-session"
              disabled={pending}
              onClick={() => startNew(run, address, shown, open)}
              flexDirection="column"
              alignItems="flex-start"
              {...item}
            >
              <Text as="span">New session</Text>
              <Text fontSize="xs" color="text.muted">
                {shown.status.state === "exited" ? "Start a fresh session" : "Stop this session and start a fresh one"}
              </Text>
            </Menu.Item>
          )}
          {shown && live.length > 1 && (
            <LiveSessionRows
              item={item}
              live={live}
              shown={shown}
              pending={pending}
              onSelect={onSelect}
              onStop={(s) => run(() => stop({ id: s.id, state: stateToken(s) }))}
            />
          )}
          {canDelete && (
            <DeleteRow
              item={item}
              pending={pending}
              confirming={confirming}
              setConfirming={setConfirming}
              onDelete={() => run(() => remove({ epic: address }), { onDone: onDeleted })}
            />
          )}
        </Menu.Content>
      </MenuFrame>
    </Menu.Root>
  );
}
