import { Text } from "@chakra-ui/react";
import { useState } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { BackLink } from "../nav/BackLink";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import type { Sent } from "./answer";
import { AnswerForm } from "./AnswerForm";
import { AnswerResult } from "./AnswerResult";
import { epicOfTicket, useTicketDetail, type FetchDetail } from "./detail";
import { PermissionCard } from "./PermissionCard";
import { TicketHeader } from "./TicketHeader";
import { TicketReading } from "./TicketReading";
import { ResumeForm } from "./ResumeForm";

export function TicketPage({ address, fetchDetail }: { address: string; fetchDetail?: FetchDetail }) {
  const conn = useConnectionContext();
  // Subscribing re-renders the page every tick, which keeps a running ticket's clock counting.
  useConnectionState(conn);
  const { detail, notFound, refetch } = useTicketDetail(address, conn, fetchDetail);
  const [sent, setSent] = useState<Sent | null>(null);
  const [resumed, setResumed] = useState("");
  const epicAddress = epicOfTicket(address);
  const backTo = `/e/${epicAddress}`;
  const backLabel = `← ${epicAddress.slice(epicAddress.indexOf(":") + 1)}`;

  return (
    <DetailPage>
        {notFound ? (
          <>
            <BackLink to={backTo} label={backLabel} />
            <EmptyState title="Ticket not found">{address} does not exist.</EmptyState>
          </>
        ) : detail ? (
          <>
            <PageHeader backTo={backTo} backLabel={backLabel} title={detail.title}>
              <TicketHeader detail={detail} now={Date.now()} />
            </PageHeader>
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
            {resumed && (
              <Text role="status" aria-label="Resume result" mt="4">
                Resumed ·{resumed}
              </Text>
            )}
            <ResumeForm
              detail={detail}
              onDone={(words) => {
                setResumed(words);
                refetch();
              }}
              onStale={refetch}
            />
          </>
        ) : (
          <>
            <BackLink to={backTo} label={backLabel} />
            <Spinner label="Loading ticket" />
          </>
        )}
    </DetailPage>
  );
}
