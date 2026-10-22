import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { BackLink } from "../nav/BackLink";
import { ConnectedTopBar } from "../nav/ConnectedTopBar";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { PageContainer } from "../ui/PageContainer";
import { Spinner } from "../ui/spinner/Spinner";
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
              <Text role="status" aria-label="Resume result" mt="4">
                Resumed ·{unparked}
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
          <Spinner label="Loading ticket" />
        )}
      </PageContainer>
    </Box>
  );
}
