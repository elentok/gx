import { Box, Heading, Skeleton, Text } from "@chakra-ui/react";
import { Link } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { PageContainer } from "../ui/PageContainer";
import { Strip } from "../ui/strip/Strip";
import { SegmentedBar } from "../ui/progress-bar/SegmentedBar";
import { TicketLine } from "../ui/ticket-line/TicketLine";
import { useEpicDetail, type EpicDetail, type FetchEpicDetail } from "./detail";
import { detailLine, foldedTail, isCancelled, isFolded, segmentsOf, statSentence, toneOf } from "./rows";

function EpicBody({ detail }: { detail: EpicDetail }) {
  const folded = detail.rows.filter(isFolded);
  const live = detail.rows.filter((r) => !isFolded(r));
  const cancelled = folded.filter(isCancelled).length;
  const idOf = (address: string) => address.slice(address.lastIndexOf("/") + 1);
  return (
    <>
      <Box mt="4">
        <SegmentedBar segments={segmentsOf(detail.rows)} value={detail.done} label="Epic progress" />
      </Box>
      <Text mt="2" fontSize="sm" color="text.muted">
        {statSentence(detail)}
      </Text>
      {folded.length > 0 && (
        <Box as="details" mt="4">
          <summary>
            ✓ {folded.length - cancelled} done{cancelled > 0 && ` · ${cancelled} cancelled`}
          </summary>
          {folded.map((r) => (
            <TicketLine
              key={r.address}
              id={idOf(r.address)}
              title={r.title}
              reason={foldedTail(r, detail.kind)}
              tone="muted"
              unclipped
            />
          ))}
        </Box>
      )}
      <Box mt="4" display="flex" flexDirection="column" gap="2">
        {live.map((r) => (
          <Box key={r.address} p="3" borderRadius="md" bg={`tone.${toneOf(r)}.bg`}>
            <TicketLine id={idOf(r.address)} title={r.title} reason={detailLine(r, detail.kind)} tone={toneOf(r)} unclipped />
          </Box>
        ))}
      </Box>
    </>
  );
}

export function EpicPage({ address, fetchDetail }: { address: string; fetchDetail?: FetchEpicDetail }) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  const { detail, notFound } = useEpicDetail(address, conn, fetchDetail);

  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <PageContainer>
        <Link to="/">← Queue</Link>
        {notFound ? (
          <EmptyState title="Epic not found">{address} does not exist.</EmptyState>
        ) : detail ? (
          <Box mt="4">
            {/* The payload has no name field: the epic name is the address after the project. */}
            <Heading as="h1" fontFamily="condensed" size="lg">
              {address.slice(address.indexOf(":") + 1)}
            </Heading>
            {detail.summary && <Text mt="2">{detail.summary}</Text>}
            {detail.total === 0 ? (
              <Text mt="4" color="text.muted">
                No tickets yet
              </Text>
            ) : (
              <EpicBody detail={detail} />
            )}
          </Box>
        ) : (
          <Box mt="4" role="status" aria-label="Loading epic" display="flex" flexDirection="column" gap="2">
            <Skeleton height="8" width="60%" />
            <Skeleton height="5" width="30%" />
          </Box>
        )}
      </PageContainer>
    </Box>
  );
}
