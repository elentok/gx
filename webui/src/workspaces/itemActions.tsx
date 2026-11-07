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
import { MoreActionsMenu, type MoreActionsItem, type MoreActionsSection } from "../ui/more-actions/MoreActionsMenu";
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

const NO_ITEM: ItemRef = { key: "", name: "", kind: "chat", workspace: "" };

/** The page-header form of {@link useItemActions}: no sections and no dialogs while the card is unknown. */
export function useCardActions(card: EpicListRow | undefined, workspaces: Workspace[]) {
  const item = itemOfCard(card);
  const actions = useItemActions(item ?? NO_ITEM, workspaces);
  return item ? actions : { sections: [] as MoreActionsSection[], dialogs: null as ReactNode };
}

export interface ItemWriters {
  move: (key: string, to: string) => Promise<WorkspaceResult>;
  remove: (key: string) => Promise<WorkspaceResult>;
  hold: (epic: string, yes?: boolean) => Promise<HoldResult>;
  unhold: (epic: string) => Promise<HoldResult>;
}

export const postMove = (key: string, to: string) => postWorkspace<WorkspaceResult>("move", { name: to, key });
export const postRemove = (key: string) => postWorkspace<WorkspaceResult>("remove", { name: "", key });
export const postHold = (epic: string, yes?: boolean) =>
  postWorkspace<HoldResult>("hold", { epic, ...(yes ? { yes } : {}) });
export const postUnhold = (epic: string) => postWorkspace<HoldResult>("unhold", { epic });

export const defaultWriters: ItemWriters = { move: postMove, remove: postRemove, hold: postHold, unhold: postUnhold };

type Dialog = { kind: "move" } | { kind: "remove" } | { kind: "hold"; sessions: string[] };

/** The Move, Remove and Hold sections of an item's menu, plus the dialogs they open. */
export function useItemActions(item: ItemRef, workspaces: Workspace[], writers: ItemWriters = defaultWriters) {
  const { run, pending } = useWrite();
  const [dialog, setDialog] = useState<Dialog>();
  const close = () => setDialog(undefined);
  const others = workspaces.filter((w) => w.name !== item.workspace);
  const isDefault = !!workspaces.find((w) => w.name === item.workspace)?.default;

  const sections: MoreActionsSection[] = [];
  const filing: MoreActionsItem[] = [];
  if (others.length > 0) filing.push({ value: "move", label: "Move to workspace", disabled: pending, onSelect: () => setDialog({ kind: "move" }) });
  if (!isDefault) filing.push({ value: "remove", label: "Remove from workspace", danger: true, disabled: pending, onSelect: () => setDialog({ kind: "remove" }) });
  if (filing.length > 0) sections.push(filing);

  if (item.kind !== "chat") {
    const hold = () =>
      item.queued ||
      run(async () => {
        const res = await writers.hold(item.key);
        // A refusal that names live sessions is a question, not an error: ask, then re-post with yes.
        if (res.refused && res.reason === ReasonHoldLiveSessions) {
          setDialog({ kind: "hold", sessions: res.sessions ?? [] });
          return { ...res, refused: false };
        }
        return res;
      });
    sections.push([
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

  const confirmHold = () => {
    close();
    void run(() => writers.hold(item.key, true));
  };

  let dialogs: ReactNode = null;
  if (dialog?.kind === "move")
    dialogs = (
      <Modal label={`Move ${item.name}`} onClose={close} maxW="sm">
        <Text>Move {item.name} to:</Text>
        <Box display="flex" flexDirection="column" gap="2">
          {others.map((w) => (
            <Button
              key={w.name}
              emphasis="secondary"
              onClick={() => {
                close();
                void run(() => writers.move(item.key, w.name));
              }}
            >
              {w.name}
            </Button>
          ))}
        </Box>
      </Modal>
    );
  else if (dialog?.kind === "remove")
    dialogs = (
      <Modal label={`Remove ${item.name}`} onClose={close} maxW="sm">
        <Text>Remove {item.name} from {item.workspace}? It moves back to Default.</Text>
        <Box display="flex" gap="2" justifyContent="flex-end">
          <Button emphasis="secondary" onClick={close} autoFocus>
            Keep
          </Button>
          <Button
            tone="paused"
            onClick={() => {
              close();
              void run(() => writers.remove(item.key));
            }}
          >
            Remove
          </Button>
        </Box>
      </Modal>
    );
  else if (dialog?.kind === "hold")
    dialogs = (
      <Modal label={`Hold ${item.name}`} onClose={close} maxW="sm">
        <Text>Holding {item.name} ends its live sessions: {dialog.sessions.join(", ")}.</Text>
        <Box display="flex" gap="2" justifyContent="flex-end">
          <Button emphasis="secondary" onClick={close} autoFocus>
            Cancel
          </Button>
          <Button tone="paused" onClick={confirmHold}>
            Hold and end sessions
          </Button>
        </Box>
      </Modal>
    );

  return { sections, dialogs };
}

/** The "..." menu of one item; renders nothing when it has no actions. */
export function ItemMenu({ item, workspaces, writers }: { item: ItemRef; workspaces: Workspace[]; writers?: ItemWriters }) {
  const { sections, dialogs } = useItemActions(item, workspaces, writers);
  if (sections.length === 0) return null;
  return (
    <>
      <MoreActionsMenu sections={sections} />
      {dialogs}
    </>
  );
}
