import { Box, Link, Spinner, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import {
  CardActionAnswer,
  CardActionContinue,
  CardActionCreatePR,
  CardActionDesign,
  CardActionMarkMerged,
  CardActionMerge,
  CardActionOpen,
  CardActionOpenEpic,
  CardActionOpenPR,
  CardActionOpenTicket,
  CardActionQueue,
  CardActionStashMerge,
  CardActionTriage,
  CardActionTryAgain,
  CardActionUnhold,
  CardKindDesign,
  CardKindFollowUps,
  CardKindImplementation,
  SectionNeedsYou,
  SectionOnHold,
  type CardAction,
  type EpicListRow,
  type Workspace,
} from "../generated/server";
import { SelectBox } from "../shell/SidebarSelect";
import { followUpsFocus } from "../workspaces/followUpsBadge";
import { ItemMenu, itemOfCard, postUnhold, type ItemWriters } from "../workspaces/itemActions";
import { designHref, epicHref, ticketHref, triageHref } from "../hrefs";
import { postOpenDesign, type OpenDesign, type OpenFlow } from "../design/open";
import { postOpenTriage } from "../triage/api";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import type { MenuEntry } from "../ui/menu/Menu";
import { useWrite } from "../useWrite";
import { launchOrVerdictText } from "../verdict";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";
import { EpicActionButton } from "./EpicActionButton";
import { titleHref } from "./cardLinks";
import { timeLabel } from "./cardTime";
import { mergeToast } from "./MergeButton";
import { postEpicCreatePR, postEpicMarkMerged, postEpicMerge, postEpicStashMerge, postEpicTryAgain, postQueueAdd, type PostEpicWrite } from "./queue";
import { PrLine } from "./PrLine";
import { READY_FOR_REVIEW_REASON, RING_KIND, isCreatingTickets, isLive, isReadyForReview, needsYouTone, ringProgress, ringTone } from "./ring";

function ringLabel(row: EpicListRow): string {
  if (isCreatingTickets(row)) return "…";
  if (row.card_kind === CardKindDesign || row.card_kind === CardKindFollowUps) return `${row.ring_progress ?? 0}%`;
  if (row.card_kind === CardKindImplementation) return `${row.done}/${row.total}`;
  return "";
}

function useCountdown(endsAt: string | undefined): number | undefined {
  const left = () => (endsAt ? Math.max(0, Math.ceil((Date.parse(endsAt) - Date.now()) / 1000)) : undefined);
  const [seconds, setSeconds] = useState(left);
  useEffect(() => {
    setSeconds(left());
    if (!endsAt) return;
    const id = setInterval(() => setSeconds(left()), 1000);
    return () => clearInterval(id);
    // left() only reads endsAt.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [endsAt]);
  return seconds;
}

export interface CardWrites {
  queue?: PostEpicWrite;
  merge?: PostEpicWrite;
  stashMerge?: PostEpicWrite;
  tryAgain?: PostEpicWrite;
  createPR?: PostEpicWrite;
  markMerged?: PostEpicWrite;
  openDesign?: OpenDesign;
  openTriage?: OpenFlow;
  /** Goes to a hash route. */
  navigate?: (href: string) => void;
  unhold?: ItemWriters["unhold"];
}

/**
 * Open on GitHub is the PR line's link, so the footer leaves it out. A failing PR is the exception: the mock's
 * footer is just Open PR beside Try again, and Mark as merged has no card button.
 */
const footerActions = (row: EpicListRow): CardAction[] => {
  const all = row.actions ?? [];
  const create = all.find((a) => a.kind === CardActionCreatePR);
  // Create PR stands alone in a pr project and sits left of Merge, the main button, in a merge project.
  if (create) {
    const merges = all.filter((a) => a.kind === CardActionMerge || a.kind === CardActionStashMerge);
    return [create, ...merges];
  }
  const tryAgain = all.find((a) => a.kind === CardActionTryAgain);
  if (tryAgain) return [...all.filter((a) => a.kind === CardActionOpenPR), tryAgain];
  return all.filter((a) => a.kind !== CardActionOpenPR && a.kind !== CardActionMarkMerged);
};

const goTo =(href: string) => {
  window.location.hash = href;
};

function ActionButtons({
  row,
  writes,
  onChanged,
}: {
  row: EpicListRow;
  writes: CardWrites;
  onChanged: () => void;
}) {
  const { queue = postQueueAdd, merge = postEpicMerge, stashMerge = postEpicStashMerge, tryAgain = postEpicTryAgain, createPR = postEpicCreatePR, openDesign = postOpenDesign, openTriage = postOpenTriage, navigate = goTo, unhold = postUnhold } = writes;
  const { run, pending } = useWrite();

  const startDesign = (epic: string) => run(() => openDesign({ epic }), { onDone: () => navigate(designHref(epic)) });
  const startTriage = (project: string) => run(() => openTriage({ epic: project }), { onDone: () => navigate(triageHref(project)) });

  function button(a: CardAction, i: number) {
    const target = a.target ?? row.address;
    const emphasis = i === 0 ? "main" : "secondary";
    if (a.kind === CardActionOpenPR) {
      return (
        <Button key={i} tone="running" emphasis="secondary" onClick={() => window.open(a.link, "_blank", "noopener,noreferrer")}>
          Open PR
        </Button>
      );
    }
    if (a.kind === CardActionCreatePR) {
      return (
        <EpicActionButton
          key={i}
          address={target}
          state={row.state}
          post={createPR}
          label={a.label}
          emphasis={footerActions(row).length > 1 ? "secondary" : undefined}
          pendingLabel="Creating PR…"
          spinner={false}
          onDone={onChanged}
        />
      );
    }
    if (a.kind === CardActionQueue || a.kind === CardActionMerge || a.kind === CardActionStashMerge || a.kind === CardActionTryAgain) {
      const isQueue = a.kind === CardActionQueue;
      const isStash = a.kind === CardActionStashMerge;
      const isTryAgain = a.kind === CardActionTryAgain;
      return (
        <EpicActionButton
          key={i}
          address={target}
          state={row.state}
          post={isQueue ? queue : isStash ? stashMerge : isTryAgain ? tryAgain : merge}
          label={a.label}
          emphasis={isStash ? "secondary" : undefined}
          pendingLabel={isQueue ? "Queueing…" : isTryAgain ? "Trying again…" : "Merging…"}
          success={isQueue || isStash || isTryAgain ? undefined : (res) => mergeToast(target, res)}
          onDone={onChanged}
        />
      );
    }
    const followUps = row.card_kind === CardKindFollowUps;
    const onClick = {
      [CardActionUnhold]: () => void run(() => unhold(target)),
      [CardActionDesign]: () => startDesign(target),
      [CardActionContinue]: () => (followUps ? startTriage(target) : startDesign(target)),
      [CardActionOpen]: () => navigate(a.link ?? designHref(target)),
      [CardActionAnswer]: () => navigate(a.link ?? ticketHref(target)),
      [CardActionOpenTicket]: () => navigate(ticketHref(target)),
      [CardActionOpenEpic]: () => navigate(epicHref(target)),
      [CardActionTriage]: () => startTriage(target),
    }[a.kind];
    return (
      <Button key={i} tone="running" emphasis={emphasis} disabled={pending} onClick={onClick}>
        {pending && (a.kind === CardActionDesign || a.kind === CardActionContinue || a.kind === CardActionTriage) && <Spinner size="sm" />}
        {a.label}
      </Button>
    );
  }

  return <>{footerActions(row).map(button)}</>;
}

/** One /v1/epics card row. */
export function EpicCard({
  row,
  now = Date.now(),
  writes = {},
  workspaces = [],
  onChanged = () => {},
  select,
}: {
  row: EpicListRow;
  /** Set while the page is in select mode: the card's corner checkbox. */
  select?: { checked: boolean; onToggle: () => void };
  /** The page clock, in epoch milliseconds. */
  now?: number;
  writes?: CardWrites;
  /** The ordered workspaces the "..." menu moves between. */
  workspaces?: Workspace[];
  onChanged?: () => void;
}) {
  const time = timeLabel(row.time_kind, row.time_start, now);
  const kind = RING_KIND[row.card_kind ?? CardKindImplementation];
  const needsYou = row.section === SectionNeedsYou;
  const seconds = useCountdown(row.countdown_ends_at);
  // The countdown wins, then the launch step or cap wait (one line, never both), then the server's reason.
  const reason =
    seconds !== undefined
      ? `Next run starts in ${seconds}s`
      : (launchOrVerdictText(row.launch_step, row.verdict, row.reason) ?? row.reason ?? (isReadyForReview(row) ? READY_FOR_REVIEW_REASON : undefined));
  const hasButtons = footerActions(row).length > 0;
  const tone = needsYou ? needsYouTone(row) : undefined;
  const pillTone = tone ?? (row.card_kind === CardKindFollowUps && row.state_pill ? needsYouTone(row) : undefined);
  const held = row.section === SectionOnHold;
  const { run } = useWrite();
  const [confirmMerged, setConfirmMerged] = useState(false);
  const markMerged = row.actions?.find((a) => a.kind === CardActionMarkMerged);
  const markMergedAddress = markMerged?.target ?? row.address;
  const extra: MenuEntry[] | undefined = markMerged
    ? [{ value: "mark-merged", label: markMerged.label, onSelect: () => setConfirmMerged(true) }]
    : undefined;
  return (
    <Box
      data-kind={kind}
      data-section={row.section}
      data-focus={row.card_kind === CardKindFollowUps ? followUpsFocus(row.project) : undefined}
      opacity={held ? 0.6 : undefined}
      position="relative"
      bg="surface.raised"
      borderWidth="1px"
      borderColor="border"
      borderRadius="16px"
      p="20px"
      display="flex"
      flexDirection="column"
      gap="3"
      style={
        tone
          ? { borderColor: `color-mix(in srgb, var(--chakra-colors-tone-${tone}-fg) 40%, var(--chakra-colors-border))` }
          : undefined
      }
    >
      {select && (
        <Box position="absolute" top="4px" left="4px" display="flex">
          <SelectBox checked={select.checked} label={`Select ${row.name}`} onToggle={select.onToggle} />
        </Box>
      )}
      <Box display="flex" alignItems="center" gap="4">
        <ProgressRing
          progress={ringProgress(row)}
          tone={ringTone(row)}
          kind={kind}
          label={ringLabel(row)}
          live={isLive(row)}
        />
        <Box minW="0">
          <Link href={titleHref(row)} color="inherit" fontFamily="condensed" fontSize="24px" fontWeight="600" lineHeight="1.15">
            {row.name}
          </Link>
          <Text fontSize="15px">
            <Text as="span" color="text.dim">
              {row.project}
            </Text>
            {row.workspace && (
              <>
                {" · "}
                <Text as="span" data-testid="workspace" color="text.dim">
                  {row.workspace}
                </Text>
              </>
            )}
            {" · "}
            {time && (
              <>
                <Text as="span" data-testid="time" color="text.dim" whiteSpace="nowrap" title={new Date(row.time_start ?? "").toLocaleString()}>
                  {time}
                </Text>
                {" · "}
              </>
            )}
            <Text as="span" data-testid="status" color={isReadyForReview(row) ? "tone.done.fg" : row.pr && !tone ? "kind.design.fg" : `kind.${kind}.fg`}>
              {row.status_words}
            </Text>
          </Text>
        </Box>
        {row.workspace && (
          <Box ml="auto" flex="none">
            <ItemMenu item={itemOfCard(row)!} workspaces={workspaces} extra={extra} />
          </Box>
        )}
        {confirmMerged && (
          <Modal label={`Mark ${row.name} as merged`} onClose={() => setConfirmMerged(false)} maxW="sm">
            <Text>Mark {row.name} as merged? Do this only when its PR was merged outside gx.</Text>
            <Box display="flex" gap="2" justifyContent="flex-end">
              <Button emphasis="secondary" onClick={() => setConfirmMerged(false)} autoFocus>
                Cancel
              </Button>
              <Button
                tone="done"
                onClick={() => {
                  setConfirmMerged(false);
                  void run(() => (writes.markMerged ?? postEpicMarkMerged)({ address: markMergedAddress, state: row.state }), { onDone: onChanged });
                }}
              >
                Mark as merged
              </Button>
            </Box>
          </Modal>
        )}
      </Box>
      <PrLine row={row} />
      {(needsYou || pillTone || reason) && (
        <Box display="flex" flexDirection="column" alignItems="flex-start" gap="2">
          {pillTone && (
            <Box
              data-testid="pill"
              display="inline-flex"
              alignItems="center"
              gap="2"
              h="26px"
              px="3"
              borderRadius="full"
              fontSize="14px"
              color={`tone.${pillTone}.fg`}
              bg={`tone.${pillTone}.bg`}
            >
              <Box w="7px" h="7px" borderRadius="full" bg="currentColor" />
              {row.state_pill}
            </Box>
          )}
          {reason && (
            <Text data-testid="reason" fontSize="14px" color="text.dim" lineClamp="2">
              {reason}
            </Text>
          )}
        </Box>
      )}
      {hasButtons && (
        <Box data-testid="footer" mt="auto" display="flex" flexWrap="wrap" justifyContent="flex-end" alignItems="center" gap="2">
          <ActionButtons row={row} writes={writes} onChanged={onChanged} />
        </Box>
      )}
    </Box>
  );
}
