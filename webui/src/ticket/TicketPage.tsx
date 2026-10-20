import { Box, Skeleton } from "@chakra-ui/react";
import { Link } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Strip } from "../ui/strip/Strip";
import { AnswerForm } from "./AnswerForm";
import { useTicketDetail, type FetchDetail } from "./detail";
import { TicketReading } from "./TicketReading";

export function TicketPage({ address, fetchDetail }: { address: string; fetchDetail?: FetchDetail }) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  const { detail, notFound, refetch } = useTicketDetail(address, conn, fetchDetail);

  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <Box p="4" maxW="1000px" mx="auto">
        <Link to="/">← Queue</Link>
        {notFound ? (
          <EmptyState title="Ticket not found">{address} does not exist.</EmptyState>
        ) : detail ? (
          <>
            <TicketReading detail={detail} />
            <AnswerForm key={detail.state} detail={detail} onSent={refetch} />
          </>
        ) : (
          <Box mt="4" role="status" aria-label="Loading ticket" display="flex" flexDirection="column" gap="2">
            <Skeleton height="8" width="60%" />
            <Skeleton height="5" width="30%" />
          </Box>
        )}
      </Box>
    </Box>
  );
}
