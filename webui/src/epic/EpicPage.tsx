import { Box, Heading, Text } from "@chakra-ui/react";
import { useMemo } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { QueueButton } from "../epics/QueueButton";
import { BackLink } from "../nav/BackLink";
import { ConnectedTopBar } from "../nav/ConnectedTopBar";
import { Markdown } from "../markdown/Markdown";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { PageContainer } from "../ui/PageContainer";
import { Spinner } from "../ui/spinner/Spinner";
import { Strip } from "../ui/strip/Strip";
import { SegmentedBar } from "../ui/progress-bar/SegmentedBar";
import { TicketLine } from "../ui/ticket-line/TicketLine";
import { useEpicDetail, type EpicDetail, type FetchEpicDetail } from "./detail";
import {
  detailLine,
  foldedTail,
  idOf,
  isCancelled,
  isFolded,
  nestFolded,
  nestLive,
  rowContext,
  segmentsOf,
  statSentence,
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
      <Text mt="2" fontSize="sm" color="text.muted">
        {statSentence({ ...detail, wall_clock: wallClock })}
      </Text>
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
                reason={foldedTail(r, ctx)}
                tone={isCancelled(r) ? "muted" : "done"}
                unclipped
                inlineReason
              />
            </Box>
          ))}
        </Box>
      )}
      <Box mt="4" display="flex" flexDirection="column" gap="2">
        {nestLive(detail.rows).map(({ row: r, indent }) => (
          <Box key={r.address} data-indent={indent} ml={indent * 6} p="3" borderRadius="md" bg={`tone.${toneOf(r)}.bg`}>
            <TicketLine
              id={idOf(r.address)}
              title={r.title}
              reason={detailLine(r, ctx)}
              tone={toneOf(r)}
              unclipped
              inlineReason
              hint={r.needs_answer ? r.question : undefined}
              href={r.needs_answer ? `#/t/${r.address}` : undefined}
            />
          </Box>
        ))}
      </Box>
    </>
  );
}

export function EpicPage({ address, fetchDetail }: { address: string; fetchDetail?: FetchEpicDetail }) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  const { detail, notFound, refetch } = useEpicDetail(address, conn, fetchDetail);

  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <PageContainer>
        <ConnectedTopBar />
        <BackLink to="/q" label="← Queue" />
        {notFound ? (
          <EmptyState title="Epic not found">{address} does not exist.</EmptyState>
        ) : detail ? (
          <Box mt="4">
            {/* The payload has no name field: the epic name is the address after the project. */}
            <Heading as="h1" fontFamily="condensed" fontSize="2xl">
              {address.slice(address.indexOf(":") + 1)}
            </Heading>
            {detail.queueable && (
              <Box mt="2" display="flex" alignItems="center" gap="2">
                <QueueButton address={address} state={detail.state ?? ""} onQueued={refetch} />
              </Box>
            )}
            {detail.summary && <Markdown mt="2">{detail.summary}</Markdown>}
            {detail.total === 0 ? (
              <Text mt="4" color="text.muted">
                No tickets yet
              </Text>
            ) : (
              <EpicBody detail={detail} now={Date.now()} />
            )}
          </Box>
        ) : (
          <Spinner label="Loading epic" />
        )}
      </PageContainer>
    </Box>
  );
}
