import { Box, Menu, Spinner, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { copyText } from "../clipboard";
import { useConnectionContext } from "../connection";
import type { DesignOpenRequest, SessionInfo } from "../generated/server";
import { Button } from "../ui/button/Button";
import { MoreActionsMenu, type MoreActionsItem, type MoreActionsSection } from "../ui/more-actions/MoreActionsMenu";
import { useWide } from "../useWide";
import { agentDetails } from "./agentDetails";
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
export function MenuFrame({ sheet, children }: { sheet: boolean; children: ReactNode }) {
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

export const menuItemStyle = (sheetMode: boolean) =>
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
      <Text px="3" pt="2" pb="0.5" fontFamily="mono" fontSize="11px" letterSpacing="0.07em" textTransform="uppercase" color="text.muted">
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

/** The in-place confirm "Delete design" turns into before it deletes. */
function ConfirmDelete({ pending, onKeep, onDelete }: { pending: boolean; onKeep: () => void; onDelete: () => void }) {
  return (
    <Box display="flex" alignItems="center" gap="2" px="3" py="1.5">
      <Text fontSize="sm">Delete this design?</Text>
      <Button size="sm" emphasis="secondary" disabled={pending} onClick={onKeep}>
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
 * The header's "⋯" menu: the Agent block, New session, the live sessions (with 2 or more), Copy design ID,
 * and Delete design while it can run. It always renders, since Copy design ID always applies.
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
  const wide = useWide();
  const item = menuItemStyle(!wide);
  const canDelete = deletable && live.length === 0;

  const agent = shown && agentDetails(shown);
  const agentRows = agent
    ? [
        agent.tier && ["Tier", agent.tier],
        !wide && agent.effort && ["Effort", agent.effort],
        agent.switched && ["", "switched by hand"],
      ].filter((row): row is string[] => !!row)
    : [];
  const sections: MoreActionsSection[] = [];
  if (agentRows.length > 0) sections.push({ content: <AgentBlock rows={agentRows} /> });
  const sessionItems: MoreActionsItem[] = [];
  if (shown)
    sessionItems.push({
      value: "new-session",
      disabled: pending,
      label: "New session",
      hint: shown.status.state === "exited" ? "Start a fresh session" : "Stop this session and start a fresh one",
      onSelect: () => startNew(run, address, shown, open),
    });
  if (sessionItems.length > 0) sections.push(sessionItems);
  if (shown && live.length > 1)
    sections.push({
      content: (
        <LiveSessionRows
          item={item}
          live={live}
          shown={shown}
          pending={pending}
          onSelect={onSelect}
          onStop={(s) => run(() => stop({ id: s.id, state: stateToken(s) }))}
        />
      ),
    });
  sections.push([{ value: "copy-design-id", label: "Copy design ID", hint: address, onSelect: () => void copyText(address, null) }]);
  if (canDelete && confirming)
    sections.push({
      content: (
        <ConfirmDelete
          pending={pending}
          onKeep={() => setConfirming(false)}
          onDelete={() => run(() => remove({ epic: address }), { onDone: onDeleted })}
        />
      ),
    });
  else if (canDelete)
    (sections[sections.length - 1] as MoreActionsItem[]).push({
      value: "delete-design",
      label: "Delete design",
      danger: true,
      closeOnSelect: false,
      onSelect: () => setConfirming(true),
    });
  return <MoreActionsMenu sections={sections} onClosed={() => setConfirming(false)} />;
}

/** The menu's key-value rows for what the header leaves out; a row with no key is a note. */
function AgentBlock({ rows }: { rows: string[][] }) {
  return (
    <Box>
      <Text fontFamily="mono" fontSize="11px" letterSpacing="0.07em" textTransform="uppercase" color="text.muted" px="3" pt="2" pb="0.5">
        Agent
      </Text>
      {rows.map(([key, value]) => (
        <Box key={key + value} display="flex" justifyContent="space-between" gap="3" px="3" py="5px" fontSize="13.5px" color="text.dim">
          {key && <span>{key}</span>}
          <Text as="b" fontFamily="mono" fontSize="13px" fontWeight="500" color="text">
            {value}
          </Text>
        </Box>
      ))}
    </Box>
  );
}
