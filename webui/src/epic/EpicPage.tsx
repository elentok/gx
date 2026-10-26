import { Box, Link, Text } from "@chakra-ui/react";
import { useMemo } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import type { OpenDesign } from "../design/open";
import { StartDesignButton } from "../design/SessionControls";
import { useWide } from "../design/useWide";
import { hasDesignSession } from "../design/view";
import type { EpicListRow, SessionInfo } from "../generated/server";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";
import { pageRing } from "./ring";
import { MergeButton, isMergeable } from "../epics/MergeButton";
import { designHref, epicHref } from "../hrefs";
import { QueueButton } from "../epics/QueueButton";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { JumpMenu, waitingCount } from "../nav/JumpMenu";
import { Markdown } from "../markdown/Markdown";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { StatLine } from "../page/StatLine";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import { SegmentedBar } from "../ui/progress-bar/SegmentedBar";
import { TicketLine } from "../ui/ticket-line/TicketLine";
import { useEpicDetail, type EpicDetail, type FetchEpicDetail } from "./detail";
import { StatsSection } from "./StatsSection";
import {
  detailLine,
  foldedTail,
  iconOf,
  idOf,
  isCancelled,
  isFolded,
  isRunning,
  nestFolded,
  nestLive,
  pauseAction,
  rowContext,
  segmentsOf,
  statValues,
  toneOf,
} from "./rows";

function EpicBody({ detail, now }: { detail: EpicDetail; now: number }) {
  const folded = detail.rows.filter(isFolded);
  const cancelled = folded.filter(isCancelled).length;
  const ctx = rowContext(detail, now);
  // The server's wall clock is as of the fetch, so a running epic adds the time since then.
  const fetchedAt = useMemo(() => Date.now(), [detail]);
  const wallClock =
    detail.state === "running" && detail.wall_clock !== undefined
      ? detail.wall_clock + Math.max(0, (now - fetchedAt) / 1000)
      : detail.wall_clock;
  return (
    <>
      <Box mt="4">
        <SegmentedBar segments={segmentsOf(detail.rows)} value={detail.done} label="Epic progress" />
      </Box>
      <StatLine values={statValues({ ...detail, wall_clock: wallClock })} />
      {folded.length > 0 && (
        <Box as="details" mt="4">
          <Box as="summary" color="tone.done.fg">
            ✓ {folded.length - cancelled} done{cancelled > 0 && ` · ${cancelled} cancelled`}
          </Box>
          {nestFolded(detail.rows).map(({ row: r, indent }) => (
            <Box key={r.address} data-fold-indent={indent} ml={indent * 6}>
              <TicketLine
                id={idOf(r.address)}
                title={r.title}
                address={r.address}
                reason={foldedTail(r, ctx)}
                tone={isCancelled(r) ? "muted" : "done"}
                unclipped
                inlineReason
              />
            </Box>
          ))}
        </Box>
      )}
      <Box mt="4" display="flex" flexDirection="column" gap="3">
        {nestLive(detail.rows).map(({ row: r, indent }) => (
          <Box key={r.address} data-indent={indent} ml={indent * 6}>
            <TicketLine
              id={idOf(r.address)}
              title={r.title}
              address={r.address}
              reason={detailLine(r, ctx)}
              tone={toneOf(r)}
              unclipped
              inlineReason
              {...pauseAction(r)}
              live={isRunning(r)}
              icon={iconOf(r)}
              toneTitle
            />
          </Box>
        ))}
      </Box>
    </>
  );
}

// Start while the Design has no session, Open after; neither on a Done Design. Open makes no call.
function DesignSessionButton({ detail, open }: { detail: EpicDetail; open?: OpenDesign }) {
  const navigate = useNavigate();
  const sessions = useConnectionState(useConnectionContext()).snapshot?.sessions ?? [];
  if (detail.steps?.find((s) => s.step === "done")?.state === "done") return null;
  const goToDesign = () => navigate(designHref(detail.address).slice(1));
  return hasDesignSession(detail.address, sessions) ? (
    <Button tone="running" onClick={goToDesign}>
      Open design session
    </Button>
  ) : (
    <StartDesignButton address={detail.address} open={open} onStarted={goToDesign} />
  );
}

function EpicRing({
  detail,
  card,
  sessions,
  size,
}: {
  detail: EpicDetail;
  card?: EpicListRow;
  sessions: SessionInfo[];
  size: "md" | "sm";
}) {
  const r = pageRing(detail, card, sessions);
  return <ProgressRing progress={r.progress} tone={r.tone} kind={r.kind} label={r.label} size={size} live={r.live} />;
}

export function EpicPage({
  address,
  fetchDetail,
  openDesign,
  fetchList,
}: {
  address: string;
  fetchDetail?: FetchEpicDetail;
  openDesign?: OpenDesign;
  fetchList?: FetchEpicList;
}) {
  const conn = useConnectionContext();
  // Subscribing re-renders the page every tick, which keeps the running clocks counting.
  const sessions = useConnectionState(conn).snapshot?.sessions ?? [];
  const wide = useWide();
  const { detail, notFound, refetch } = useEpicDetail(address, conn, fetchDetail);
  const { list } = useEpicList(fetchList, conn);
  // The payload has no name field: the epic name is the address after the project.
  const name = address.slice(address.indexOf(":") + 1);
  // A joined card lists both of its epics; the other one is the Design or its -impl epic.
  const joinedCard = list?.epics?.find((e) => e.addresses?.includes(address));
  const partner = joinedCard?.addresses?.find((a) => a !== address);
  // The card speaks for this page only while its active (first) address is this epic.
  const card = joinedCard?.addresses?.[0] === address ? joinedCard : undefined;
  const summary = detail?.summary && <Markdown mt="2">{detail.summary}</Markdown>;

  return (
    <DetailPage>
      {notFound ? (
        <EmptyState title="Epic not found">{address} does not exist.</EmptyState>
      ) : detail ? (
        <>
          <PageHeader
            backTo="/epics"
            backLabel="← Epics"
            waiting={waitingCount(list?.epics ?? [], address)}
            title={name}
            lead={<EpicRing detail={detail} card={card} sessions={sessions} size={wide ? "md" : "sm"} />}
            heading={
              <>
                <JumpMenu name={name} current={address} epics={list?.epics ?? []} />
                {wide && summary}
              </>
            }
          >
            {!wide && summary}
            {partner && (
              <Link href={epicHref(partner)} display="block" mt="2" fontSize="sm" color="tone.running.fg">
                {address.endsWith("-impl") ? "Open design →" : "Open implementation epic →"}
              </Link>
            )}
            {detail.state === "paused" && detail.land_block_reason && (
              <Text data-reason mt="2" fontSize="sm" color="text.muted">
                {detail.land_block_reason}
              </Text>
            )}
            {detail.draft && address.endsWith("-impl") && (
              <Box mt="2" display="flex" alignItems="center" gap="2">
                <Button tone="running" disabled>
                  Queue
                </Button>
                <Text as="span" color="text.muted">
                  Still publishing tickets
                </Text>
              </Box>
            )}
            {detail.kind === "map" && (
              <Box mt="2">
                <DesignSessionButton detail={detail} open={openDesign} />
              </Box>
            )}
            {detail.queueable && (
              <Box mt="2" display="flex" alignItems="center" gap="2">
                <QueueButton address={address} state={detail.state ?? ""} onQueued={refetch} />
              </Box>
            )}
            {isMergeable(detail.state, !!detail.land_block_reason) && (
              <Box mt="2" display="flex" alignItems="center" gap="2">
                <MergeButton address={address} state={detail.state ?? ""} onMerged={refetch} />
              </Box>
            )}
          </PageHeader>
          {detail.total === 0 ? (
            <Text mt="4" color="text.muted">
              No tickets yet
            </Text>
          ) : (
            <EpicBody detail={detail} now={Date.now()} />
          )}
          <StatsSection detail={detail} />
        </>
      ) : (
        <Spinner label="Loading epic" />
      )}
    </DetailPage>
  );
}
