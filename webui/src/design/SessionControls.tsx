import { Box, Spinner, Text } from "@chakra-ui/react";
import { useState } from "react";
import { copyText } from "../clipboard";
import { useConnectionContext } from "../connection";
import type { DesignOpenRequest, EpicListRow, SessionInfo } from "../generated/server";
import { useCardActions } from "../workspaces/itemActions";
import { useWorkspaces } from "../workspaces/useActiveWorkspace";
import { Button } from "../ui/button/Button";
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu";
import { MoreActionsButton } from "../ui/menu/MoreActionsButton";
import { useWide } from "../useWide";
import { agentDetails } from "./agentDetails";
import { useWrite } from "../useWrite";
import {
  postDeleteDesign,
  postOpenDesign,
  postStopSession,
  stateToken,
  type DeleteDesign,
  type OpenFlow,
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

/** Starts the flow's session. `onStarted` runs once it exists, e.g. to go to the Design page. */
export function StartDesignButton({
  address,
  open = postOpenDesign,
  onStarted,
  label = "Start design session",
}: {
  address: string;
  open?: OpenFlow;
  onStarted?: () => void;
  label?: string;
}) {
  const { run, pending } = useDesignWrite();
  return (
    <Box display="flex" alignItems="center" gap="2">
      <Button tone="running" disabled={pending} onClick={() => run(() => open({ epic: address }), { onDone: onStarted })}>
        {pending && <Spinner size="sm" />}
        {label}
      </Button>
    </Box>
  );
}

/** Opens a new session for the Design; asks first, and sends the state token, when the shown one is live. */
export function startNew(
  run: ReturnType<typeof useDesignWrite>["run"],
  address: string,
  shown: SessionInfo,
  open: OpenFlow = postOpenDesign,
) {
  const isLive = shown.status.state !== "exited";
  if (isLive && !window.confirm("Stop the current session and start a new one?")) return;
  const body: DesignOpenRequest = { epic: address, new: true, ...(isLive ? { state: stateToken(shown) } : {}) };
  run(() => open(body));
}

/** The live sessions: pick one to show it, or stop one that is not shown. */
function liveSessionEntries(
  live: SessionInfo[],
  shown: SessionInfo,
  pending: boolean,
  onSelect: (id: string) => void,
  onStop: (s: SessionInfo) => void,
): MenuEntry[] {
  const heading: MenuEntry = {
    kind: "content",
    value: "live-sessions",
    content: (
      <Text px="3" pt="2" pb="0.5" fontFamily="mono" fontSize="11px" letterSpacing="0.07em" textTransform="uppercase" color="text.muted">
        Live sessions · {live.length}
      </Text>
    ),
  };
  return [
    heading,
    ...live.flatMap((s): MenuEntry[] => {
      const name = s.name || s.id;
      const isShown = s.id === shown.id;
      return [
        { value: `session-${s.id}`, label: name, hint: isShown ? "✓" : undefined, inlineHint: true, current: isShown, onSelect: () => onSelect(s.id) },
        ...(isShown
          ? []
          : [
              {
                value: `stop-${s.id}`,
                label: `Stop ${name}`,
                danger: true,
                closeOnSelect: false,
                disabled: pending,
                onSelect: () => onStop(s),
              },
            ]),
      ];
    }),
  ];
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
  card,
  designItems = true,
  extraItems = [],
}: {
  address: string;
  /** Items another flow adds beside Copy ID, such as the Triage's Abandon. */
  extraItems?: MenuEntry[];
  /** Delete, Hold and workspace Move belong to a Design only; another flow's menu leaves them out. */
  designItems?: boolean;
  /** The Design's card; its workspace and hold state drive the Move, Remove and Hold items. */
  card?: EpicListRow;
  shown?: SessionInfo;
  /** Live design sessions, newest first. */
  live: SessionInfo[];
  /** The map has no Destination and no tickets yet, so it can still be deleted. */
  deletable?: boolean;
  onSelect: (id: string) => void;
  onDeleted: () => void;
  open?: OpenFlow;
  stop?: StopSession;
  remove?: DeleteDesign;
}) {
  const { run, pending } = useDesignWrite();
  const [confirming, setConfirming] = useState(false);
  const wide = useWide();
  const { workspaces } = useWorkspaces(useConnectionContext());
  const itemActions = useCardActions(card, workspaces);
  const canDelete = designItems && deletable && live.length === 0;

  const groups: MenuEntry[][] = [];
  const agent = shown && agentSection(shown, wide);
  if (agent) groups.push(agent);
  const sessionItems: MenuEntry[] = [];
  if (shown)
    sessionItems.push({
      value: "new-session",
      disabled: pending,
      label: "New session",
      hint: shown.status.state === "exited" ? "Start a fresh session" : "Stop this session and start a fresh one",
      onSelect: () => startNew(run, address, shown, open),
    });
  if (shown && live.length > 1)
    sessionItems.push(...liveSessionEntries(live, shown, pending, onSelect, (s) => run(() => stop({ id: s.id, state: stateToken(s) }))));
  if (sessionItems.length > 0) groups.push(sessionItems);
  const copySection: MenuEntry[] = [
    { value: "copy-design-id", label: designItems ? "Copy design ID" : "Copy triage ID", hint: address, inlineHint: true, onSelect: () => void copyText(address, null) },
  ];
  copySection.push(...extraItems);
  if (canDelete && !confirming)
    copySection.push({
      value: "delete-design",
      label: "Delete design",
      danger: true,
      closeOnSelect: false,
      onSelect: () => setConfirming(true),
    });
  groups.push(copySection);
  if (designItems) groups.push(...itemActions.groups);
  if (canDelete && confirming)
    groups.push([
      {
        kind: "content",
        value: "confirm-delete",
        content: (
          <ConfirmDelete
            pending={pending}
            onKeep={() => setConfirming(false)}
            onDelete={() => run(() => remove({ epic: address }), { onDone: onDeleted })}
          />
        ),
      },
    ]);
  return (
    <>
      <GxMenu trigger={<MoreActionsButton />} items={menuGroups(groups)} onClosed={() => setConfirming(false)} />
      {designItems && itemActions.dialogs}
    </>
  );
}

/** The menu's Agent block for what the header leaves out (Tier, Effort on a phone, the switched note); none when it has no rows. */
export function agentSection(session: SessionInfo, wide: boolean): MenuEntry[] | undefined {
  const agent = agentDetails(session);
  const rows: AgentRow[] = [];
  if (agent.tier) rows.push({ key: "Tier", value: agent.tier });
  if (!wide && agent.effort) rows.push({ key: "Effort", value: agent.effort });
  if (agent.switched) rows.push({ note: "switched by hand" });
  return rows.length > 0 ? [{ kind: "content", value: "agent", content: <AgentBlock rows={rows} /> }] : undefined;
}

type AgentRow = { key: string; value: string } | { note: string };

function AgentBlock({ rows }: { rows: AgentRow[] }) {
  return (
    <Box>
      <Text fontFamily="mono" fontSize="11px" letterSpacing="0.07em" textTransform="uppercase" color="text.muted" px="3" pt="2" pb="0.5">
        Agent
      </Text>
      {rows.map((row) => (
        <Box key={"note" in row ? row.note : row.key} display="flex" justifyContent="space-between" gap="3" px="3" py="5px" fontSize="13.5px" color="text.dim">
          {"key" in row && <span>{row.key}</span>}
          <Text as="b" fontFamily="mono" fontSize="13px" fontWeight="500" color="text">
            {"note" in row ? row.note : row.value}
          </Text>
        </Box>
      ))}
    </Box>
  );
}
