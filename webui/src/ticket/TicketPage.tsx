import { Text } from "@chakra-ui/react";
import { useState } from "react";
import { useConnectionContext } from "../connection";
import { BackLink } from "../nav/BackLink";
import { DetailPage } from "../page/DetailPage";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import type { Sent } from "./answer";
import { AnswerForm } from "./AnswerForm";
import { AnswerResult } from "./AnswerResult";
import { useTicketDetail, type FetchDetail } from "./detail";
import { PermissionCard } from "./PermissionCard";
import { TicketReading } from "./TicketReading";
import { ResumeForm } from "./ResumeForm";

export function TicketPage({ address, fetchDetail }: { address: string; fetchDetail?: FetchDetail }) {
  const conn = useConnectionContext();
  const { detail, notFound, refetch } = useTicketDetail(address, conn, fetchDetail);
  const [sent, setSent] = useState<Sent | null>(null);
  const [resumed, setResumed] = useState("");
  // A ticket address is <project>:<epic>/<id>.
  const epicAddress = address.slice(0, address.lastIndexOf("/"));

  return (
    <DetailPage>
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
          <Spinner label="Loading ticket" />
        )}
    </DetailPage>
  );
}
