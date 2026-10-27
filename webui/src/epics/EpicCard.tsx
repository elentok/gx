import { Box, Link, Spinner, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import {
  CardActionAnswer,
  CardActionContinue,
  CardActionDesign,
  CardActionMerge,
  CardActionOpen,
  CardActionOpenEpic,
  CardActionOpenTicket,
  CardActionQueue,
  CardActionStashMerge,
  CardActionTriage,
  CardKindDesign,
  CardKindImplementation,
  SectionNeedsYou,
  type CardAction,
  type EpicListRow,
} from "../generated/server";
import { designHref, epicHref, ticketHref } from "../hrefs";
import { postOpenDesign, type OpenDesign } from "../design/open";
import { Button } from "../ui/button/Button";
import { useWrite } from "../useWrite";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";
import { EpicActionButton } from "./EpicActionButton";
import { titleHref } from "./cardLinks";
import { timeLabel } from "./cardTime";
import { mergeToast } from "./MergeButton";
import { postEpicMerge, postEpicStashMerge, postQueueAdd, type PostEpicWrite } from "./queue";
import { RING_KIND, isCreatingTickets, isLive, needsYouTone, ringProgress, ringTone } from "./ring";
import { TriageDialog } from "./TriageDialog";

function ringLabel(row: EpicListRow): string {
  if (isCreatingTickets(row)) return "…";
  if (row.card_kind === CardKindDesign) return `${row.ring_progress ?? 0}%`;
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
  openDesign?: OpenDesign;
  /** Goes to a hash route. */
  navigate?: (href: string) => void;
}

const goTo = (href: string) => {
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
  const { queue = postQueueAdd, merge = postEpicMerge, stashMerge = postEpicStashMerge, openDesign = postOpenDesign, navigate = goTo } = writes;
  const [triage, setTriage] = useState(false);
  const { run, pending } = useWrite();

  const startDesign = (epic: string) => run(() => openDesign({ epic }), { onDone: () => navigate(designHref(epic)) });

  function button(a: CardAction, i: number) {
    const target = a.target ?? row.address;
    const emphasis = i === 0 ? "main" : "secondary";
    if (a.kind === CardActionQueue || a.kind === CardActionMerge || a.kind === CardActionStashMerge) {
      const isQueue = a.kind === CardActionQueue;
      const isStash = a.kind === CardActionStashMerge;
      return (
        <EpicActionButton
          key={i}
          address={target}
          state={row.state}
          post={isQueue ? queue : isStash ? stashMerge : merge}
          label={a.label}
          pendingLabel={isQueue ? "Queueing…" : "Merging…"}
          success={isQueue || isStash ? undefined : (res) => mergeToast(target, res)}
          onDone={onChanged}
        />
      );
    }
    const onClick = {
      [CardActionDesign]: () => startDesign(target),
      [CardActionContinue]: () => startDesign(target),
      [CardActionOpen]: () => navigate(designHref(target)),
      [CardActionAnswer]: () => navigate(a.link ?? ticketHref(target)),
      [CardActionOpenTicket]: () => navigate(ticketHref(target)),
      [CardActionOpenEpic]: () => navigate(epicHref(target)),
      [CardActionTriage]: () => setTriage(true),
    }[a.kind];
    return (
      <Button key={i} tone="running" emphasis={emphasis} disabled={pending} onClick={onClick}>
        {pending && (a.kind === CardActionDesign || a.kind === CardActionContinue) && <Spinner size="sm" />}
        {a.label}
      </Button>
    );
  }

  return (
    <>
      {row.actions?.map(button)}
      {triage && <TriageDialog project={row.project} onClose={() => setTriage(false)} />}
    </>
  );
}

/** One /v1/epics card row. */
export function EpicCard({
  row,
  now = Date.now(),
  writes = {},
  onChanged = () => {},
}: {
  row: EpicListRow;
  /** The page clock, in epoch milliseconds. */
  now?: number;
  writes?: CardWrites;
  onChanged?: () => void;
}) {
  const time = timeLabel(row.time_kind, row.time_start, now);
  const kind = RING_KIND[row.card_kind ?? CardKindImplementation];
  const needsYou = row.section === SectionNeedsYou;
  const seconds = useCountdown(row.countdown_ends_at);
  const reason = seconds !== undefined ? `Next run starts in ${seconds}s` : row.reason;
  const hasButtons = (row.actions?.length ?? 0) > 0;
  const tone = needsYou ? needsYouTone(row) : undefined;
  return (
    <Box
      data-kind={kind}
      data-section={row.section}
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
            {" · "}
            {time && (
              <>
                <Text as="span" data-testid="time" color="text.dim" whiteSpace="nowrap" title={new Date(row.time_start ?? "").toLocaleString()}>
                  {time}
                </Text>
                {" · "}
              </>
            )}
            <Text as="span" data-testid="status" color={`kind.${kind}.fg`}>
              {row.status_words}
            </Text>
          </Text>
        </Box>
      </Box>
      {(needsYou || reason) && (
        <Box display="flex" flexDirection="column" alignItems="flex-start" gap="2">
          {needsYou && tone && (
            <Box
              data-testid="pill"
              display="inline-flex"
              alignItems="center"
              gap="2"
              h="26px"
              px="3"
              borderRadius="full"
              fontSize="14px"
              color={`tone.${tone}.fg`}
              bg={`tone.${tone}.bg`}
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
