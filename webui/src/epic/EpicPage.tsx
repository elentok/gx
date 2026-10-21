import { Box, Heading, Skeleton, Text } from "@chakra-ui/react";
import { Link } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { PageContainer } from "../ui/PageContainer";
import { Strip } from "../ui/strip/Strip";
import { useEpicDetail, type FetchEpicDetail } from "./detail";

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
            {detail.total === 0 && (
              <Text mt="4" color="text.muted">
                No tickets yet
              </Text>
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
