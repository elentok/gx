import { Box, Heading, Skeleton } from "@chakra-ui/react";
import { Link } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { Chip } from "../ui/chip/Chip";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Strip } from "../ui/strip/Strip";
import { useTicketDetail, type FetchDetail } from "./detail";

export function TicketPage({ address, fetchDetail }: { address: string; fetchDetail?: FetchDetail }) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  const { detail, notFound } = useTicketDetail(address, conn, fetchDetail);

  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <Box p="4" maxW="1000px" mx="auto">
        <Link to="/">← Queue</Link>
        {notFound ? (
          <EmptyState title="Ticket not found">{address} does not exist.</EmptyState>
        ) : detail ? (
          <Box mt="4">
            <Heading as="h1" fontFamily="condensed" size="lg">
              {detail.title}
            </Heading>
            <Box display="flex" gap="2" mt="2">
              <Chip tone="muted">{detail.address}</Chip>
              <Chip tone="muted">{detail.status}</Chip>
            </Box>
          </Box>
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
