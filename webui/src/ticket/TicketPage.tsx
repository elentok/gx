import { Box, Skeleton, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { BackLink } from "../nav/BackLink";
import { ConnectedTopBar } from "../nav/ConnectedTopBar";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { PageContainer } from "../ui/PageContainer";
import { Strip } from "../ui/strip/Strip";
import type { Sent } from "./answer";
import { AnswerForm } from "./AnswerForm";
import { AnswerResult } from "./AnswerResult";
import { useTicketDetail, type FetchDetail } from "./detail";
import { PermissionCard } from "./PermissionCard";
import { TicketReading } from "./TicketReading";
import { UnparkForm } from "./UnparkForm";

export function TicketPage({ address, fetchDetail }: { address: string; fetchDetail?: FetchDetail }) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  const { detail, notFound, refetch } = useTicketDetail(address, conn, fetchDetail);
  const [sent, setSent] = useState<Sent | null>(null);
  const [unparked, setUnparked] = useState("");
  // A ticket address is <project>:<epic>/<id>.
  const epicAddress = address.slice(0, address.lastIndexOf("/"));

  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <PageContainer>
        <ConnectedTopBar />
        <BackLink to={`/e/${epicAddress}`} label={`← ${epicAddress.slice(epicAddress.indexOf(":") + 1)}`} />
        {notFound ? (
          <EmptyState title="Ticket not found">{address} does not exist.</EmptyState>
        ) : detail ? (
          <>
            <TicketReading detail={detail} />
            <PermissionCard detail={detail} onDone={refetch} onStale={refetch} />
            <AnswerResult detail={detail} sent={sent} />
            <AnswerForm
              detail={detail}
              onSent={(s) => {
                setSent(s);
                refetch();
              }}
              onStale={refetch}
            />
            {unparked && (
              <Text role="status" aria-label="Unpark result" mt="4">
                Unparked · {unparked}
              </Text>
            )}
            <UnparkForm
              detail={detail}
              onDone={(words) => {
                setUnparked(words);
                refetch();
              }}
              onStale={refetch}
            />
          </>
        ) : (
          <Box mt="4" role="status" aria-label="Loading ticket" display="flex" flexDirection="column" gap="2">
            <Skeleton height="8" width="60%" />
            <Skeleton height="5" width="30%" />
          </Box>
        )}
      </PageContainer>
    </Box>
  );
}
