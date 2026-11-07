import { Box, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import {
  CardKindDesign,
  HoldQueuedText,
  SectionOnHold,
  type EpicListRow,
  ReasonHoldLiveSessions,
  type HoldResult,
  type Workspace,
  type WorkspaceResult,
} from "../generated/server";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { GxMenu, menuGroups, type MenuEntry } from "../ui/menu/Menu";
import { MoreActionsButton } from "../ui/menu/MoreActionsButton";
import { useWrite } from "../useWrite";
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

export interface ItemWriters {
  move: (key: string, to: string) => Promise<WorkspaceResult>;
  hold: (epic: string, yes?: boolean) => Promise<HoldResult>;
  unhold: (epic: string) => Promise<HoldResult>;
}

export const postMove = (key: string, to: string) => postWorkspace<WorkspaceResult>("move", { name: to, key });
export const postHold = (epic: string, yes?: boolean) =>
  postWorkspace<HoldResult>("hold", { epic, ...(yes ? { yes } : {}) });
export const postUnhold = (epic: string) => postWorkspace<HoldResult>("unhold", { epic });

export const defaultWriters: ItemWriters = { move: postMove, hold: postHold, unhold: postUnhold };

type Dialog = { kind: "hold"; sessions: string[] };

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

/** The Move, Remove and Hold sections of an item's menu, plus the dialogs they open. No item, no sections and no dialogs. */
export function useItemActions(item: ItemRef | undefined, workspaces: Workspace[], writers: ItemWriters = defaultWriters) {
  const { run, pending } = useWrite();
  const [dialog, setDialog] = useState<Dialog>();
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

  if (item.kind !== "chat") {
    const hold = () =>
      run(async () => {
        const res = await writers.hold(item.key);
        // A refusal that names live sessions is a question, not an error: ask, then re-post with yes.
        if (res.refused && res.reason === ReasonHoldLiveSessions) {
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
  }

  const props = { item, writers, run, close };
  const dialogs: ReactNode = dialog ? <HoldDialog {...props} sessions={dialog.sessions} /> : null;

  return { groups, dialogs };
}

/** The "..." menu of one item; renders nothing when it has no actions. */
export function ItemMenu({ item, workspaces, writers }: { item: ItemRef; workspaces: Workspace[]; writers?: ItemWriters }) {
  const { groups, dialogs } = useItemActions(item, workspaces, writers);
  if (groups.length === 0) return null;
  return (
    <>
      <GxMenu trigger={<MoreActionsButton />} items={menuGroups(groups)} />
      {dialogs}
    </>
  );
}
