import { Box, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import {
  CardKindDesign,
  HoldQueuedText,
  SectionOnHold,
  type EpicListRow,
  ReasonNeedsYes,
  TargetRefused,
  type HoldRequest,
  type SessionInfo,
  type TargetsResult,
  type Workspace,
  type WorkspaceRequest,
} from "../generated/server";
import type { StopSession } from "../design/open";
import { useSessionManage } from "../agents/useSessionManage";
import type { RemoveSession } from "../agents/sessionWrites";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu";
import { MoreActionsButton } from "../ui/menu/MoreActionsButton";
import { useWrite, WriteError, type Refusable } from "../useWrite";
import { bulkArchive } from "../select/bulkWrites";
import { showToast, toaster } from "../ui/toaster";
import { postWorkspace } from "./postWorkspace";

/** What the menu needs of a Design or epic card, a chat or a jump row. One-off, Follow-ups and Other get no menu. */
export interface ItemRef {
  /** The Design or epic address, or the chat's session id. */
  key: string;
  name: string;
  kind: "design" | "epic" | "chat";
  /** The workspace the item sits in. */
  workspace: string;
  /** The epic (or the Design's -impl) holds a queue item. */
  queued?: boolean;
  held?: boolean;
  /** A chat's session, for Stop and Remove. */
  session?: SessionInfo;
}

/** The menu target of a Design or epic card; none for a card outside every workspace (One-off, Follow-ups, Other). */
export function itemOfCard(card: EpicListRow | undefined): ItemRef | undefined {
  if (!card?.workspace) return undefined;
  return {
    key: card.address,
    name: card.name,
    kind: card.card_kind === CardKindDesign ? "design" : "epic",
    workspace: card.workspace,
    queued: card.queued,
    held: card.section === SectionOnHold,
  };
}

/** The page-header form of {@link useItemActions}: no sections and no dialogs while the card is unknown. */
export function useCardActions(card: EpicListRow | undefined, workspaces: Workspace[]) {
  return useItemActions(itemOfCard(card), workspaces);
}

/** The one result of a one-target write, flat as `useWrite` wants it. */
export interface ItemWriteResult extends Refusable {
  sessions?: string[];
  /** An archive dry run's answer: whether gx sees the epic on trunk. */
  on_trunk?: boolean;
  /** The name an archive took when it clashed with one already archived. */
  new_address?: string;
}

export interface ItemWriters {
  move: (key: string, to: string) => Promise<ItemWriteResult>;
  hold: (epic: string, yes?: boolean) => Promise<ItemWriteResult>;
  unhold: (epic: string) => Promise<ItemWriteResult>;
  /** One epic's archive; `dryRun` only asks what it would stop. */
  archive?: (epic: string, opts: { dryRun?: boolean; yes?: boolean }) => Promise<ItemWriteResult>;
  /** A chat's Stop and Remove; the agent page's defaults when absent. */
  stop?: StopSession;
  remove?: RemoveSession;
}

/** Posts one target and unwraps its result: a refusal is `refused`, as `useWrite` reads it. */
async function postOne(route: string, body: WorkspaceRequest | HoldRequest): Promise<ItemWriteResult> {
  const { results } = await postWorkspace<TargetsResult>(route, body);
  const r = results[0];
  if (!r) throw new WriteError("The server answered with no result.");
  return { refused: r.status === TargetRefused, reason: r.reason, message: r.message, sessions: r.sessions };
}

export const postMove = (key: string, to: string) => postOne("move", { name: to, targets: [key] });
export const postHold = (epic: string, yes?: boolean) => postOne("hold", { targets: [epic], ...(yes ? { yes } : {}) });
export const postUnhold = (epic: string) => postOne("unhold", { targets: [epic] });

export async function postArchive(epic: string, opts: { dryRun?: boolean; yes?: boolean }): Promise<ItemWriteResult> {
  const r = (await bulkArchive([epic], opts))[0];
  if (!r) throw new WriteError("The server answered with no result.");
  return { refused: r.status === TargetRefused, reason: r.reason, message: r.message, sessions: r.sessions, on_trunk: r.on_trunk, new_address: r.new_address };
}

export const defaultWriters: ItemWriters = { move: postMove, hold: postHold, unhold: postUnhold, archive: postArchive };

const ARCHIVE_BUSY_TOAST = "item-archive-busy";

type Dialog = { kind: "hold"; sessions: string[] } | { kind: "archive"; sessions: string[]; onTrunk?: boolean };

interface DialogProps {
  item: ItemRef;
  writers: ItemWriters;
  run: ReturnType<typeof useWrite>["run"];
  close: () => void;
}

function HoldDialog({ item, writers, run, close, sessions }: DialogProps & { sessions: string[] }) {
  return (
    <Modal label={`Hold ${item.name}`} onClose={close} maxW="sm">
      <Text>Holding {item.name} ends its live sessions: {sessions.join(", ")}.</Text>
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="secondary" onClick={close} autoFocus>
          Cancel
        </Button>
        <Button
          tone="paused"
          onClick={() => {
            close();
            void run(() => writers.hold(item.key, true));
          }}
        >
          Hold and end sessions
        </Button>
      </Box>
    </Modal>
  );
}

function ArchiveDialog({ item, writers, run, close, sessions, onTrunk }: DialogProps & { sessions: string[]; onTrunk?: boolean }) {
  const archive = writers.archive ?? postArchive;
  return (
    <Modal label={`Archive ${item.name}`} onClose={close} maxW="md">
      <Text fontWeight="bold">Archive {item.name}? Live agents stop and the epic leaves the sidebar. Unarchive brings it back.</Text>
      {sessions.length > 0 && <Text>Live sessions will stop: {sessions.join(", ")}.</Text>}
      {onTrunk === false && (
        <Text color="tone.waiting.fg">⚠ {item.name} is not merged to trunk. Archiving keeps its branch, but the work is not on trunk.</Text>
      )}
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="secondary" onClick={close} autoFocus>
          Cancel
        </Button>
        <Button
          tone="running"
          onClick={() => {
            close();
            showToast({ type: "info", title: `Archiving ${item.name}…`, id: ARCHIVE_BUSY_TOAST, persistent: true });
            void run(() => archive(item.key, { yes: true }).finally(() => toaster.remove(ARCHIVE_BUSY_TOAST)), {
              success: (res) => ({ type: "success", title: `Archived ${res.new_address ? res.new_address.split(":").pop() : item.name}` }),
            });
          }}
        >
          Archive
        </Button>
      </Box>
    </Modal>
  );
}

/** The Move, Remove and Hold sections of an item's menu, plus the dialogs they open. No item, no sections and no dialogs. */
export function useItemActions(
  item: ItemRef | undefined,
  workspaces: Workspace[],
  writers: ItemWriters = defaultWriters,
  /** Runs after a chat's Remove lands. */
  onRemoved: () => void = () => {},
) {
  const { run, pending } = useWrite();
  const [dialog, setDialog] = useState<Dialog>();
  const manage = useSessionManage(item?.session, onRemoved, { stop: writers.stop, remove: writers.remove });
  if (!item) return { groups: [] as MenuEntry[][], dialogs: null as ReactNode };
  const close = () => setDialog(undefined);

  const groups: MenuEntry[][] = [];
  if (workspaces.length > 0) {
    groups.push([
      {
        kind: "submenu",
        value: "move",
        label: "Move to workspace",
        items: workspaces.map((w) => {
          const current = w.name === item.workspace;
          return {
            value: `move-${w.name}`,
            label: w.name,
            current,
            inlineHint: true,
            hint: current ? "✓" : undefined,
            disabled: pending,
            onSelect: () => void run(() => writers.move(item.key, w.name)),
          };
        }),
      },
    ]);
  }

  if (item.kind === "chat" && manage.items.length > 0) groups.push(manage.items);

  if (item.kind !== "chat") {
    const hold = () =>
      run(async () => {
        const res = await writers.hold(item.key);
        // A refusal that names live sessions is a question, not an error: ask, then re-post with yes.
        if (res.refused && res.reason === ReasonNeedsYes) {
          setDialog({ kind: "hold", sessions: res.sessions ?? [] });
          return { ...res, refused: false };
        }
        return res;
      });
    groups.push([
      item.held
        ? { value: "unhold", label: "Take off hold", disabled: pending, onSelect: () => void run(() => writers.unhold(item.key)) }
        : {
            value: "hold",
            label: "Hold",
            hint: item.queued ? HoldQueuedText : undefined,
            disabled: pending || item.queued,
            onSelect: () => void hold(),
          },
    ]);
    const archive = writers.archive ?? postArchive;
    // A dry run first: the dialog lists the live sessions it stops and warns when the epic is not on trunk.
    groups.push([
      {
        value: "archive",
        label: "Archive",
        disabled: pending,
        onSelect: () =>
          void run(async () => {
            const dry = await archive(item.key, { dryRun: true });
            if (dry.refused) return dry;
            setDialog({ kind: "archive", sessions: dry.sessions ?? [], onTrunk: dry.on_trunk });
            return dry;
          }),
      },
    ]);
  }

  const props = { item, writers, run, close };
  const dialogs: ReactNode = (
    <>
      {dialog?.kind === "hold" ? <HoldDialog {...props} sessions={dialog.sessions} /> : null}
      {dialog?.kind === "archive" ? <ArchiveDialog {...props} sessions={dialog.sessions} onTrunk={dialog.onTrunk} /> : null}
      {manage.dialog}
    </>
  );

  return { groups, dialogs };
}

/** The "..." menu of one item; renders nothing when it has no actions. */
export function ItemMenu({
  item,
  workspaces,
  writers,
  onRemoved,
  extra,
}: {
  item: ItemRef;
  workspaces: Workspace[];
  writers?: ItemWriters;
  onRemoved?: () => void;
  /** A card's own entries, as one more group after the item's. */
  extra?: MenuEntry[];
}) {
  const { groups, dialogs } = useItemActions(item, workspaces, writers, onRemoved);
  const all = extra?.length ? [...groups, extra] : groups;
  if (all.length === 0) return null;
  return (
    <>
      <GxMenu trigger={<MoreActionsButton />} items={menuGroups(all)} />
      {dialogs}
    </>
  );
}
