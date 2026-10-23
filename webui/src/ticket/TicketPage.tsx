import { useState } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { BackLink } from "../nav/BackLink";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import { ActionCard, needsActionCard } from "./ActionCard";
import type { Sent } from "./answer";
import { epicOfTicket, useTicketDetail, type FetchDetail } from "./detail";
import { TicketHeader } from "./TicketHeader";
import { hasSections, NothingHere, TicketSections } from "./TicketSections";

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
            <ActionCard
              detail={detail}
              sent={sent}
              resumed={resumed}
              onSent={setSent}
              onResumed={setResumed}
              refetch={refetch}
            />
            <TicketSections detail={detail} now={Date.now()} />
            {!hasSections(detail) && !needsActionCard(detail, sent, resumed) && <NothingHere />}
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
