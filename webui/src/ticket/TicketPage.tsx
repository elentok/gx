import { Box } from "@chakra-ui/react";
import { useState } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { fetchTicketScreen, type FetchScreen } from "../design/screen";
import { ScreenModal } from "../design/ScreenModal";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import { ActionCard, needsActionCard } from "./ActionCard";
import type { Sent } from "./answer";
import { useTicketDetail, type FetchDetail } from "./detail";
import { TicketHeader } from "./TicketHeader";
import { hasSections, NothingHere, TicketSections } from "./TicketSections";

export function TicketPage({
  address,
  fetchDetail,
  fetchScreen = fetchTicketScreen,
}: {
  address: string;
  fetchDetail?: FetchDetail;
  fetchScreen?: FetchScreen;
}) {
  const conn = useConnectionContext();
  // Subscribing re-renders the page every tick, which keeps a running ticket's clock counting.
  useConnectionState(conn);
  const { detail, notFound, refetch } = useTicketDetail(address, conn, fetchDetail);
  const [sent, setSent] = useState<Sent | null>(null);
  const [showScreen, setShowScreen] = useState(false);

  return (
    <DetailPage mode="narrow">
        {notFound ? (
          <EmptyState title="Ticket not found">{address} does not exist.</EmptyState>
        ) : detail ? (
          <>
            <PageHeader title={detail.title}>
              <TicketHeader detail={detail} now={Date.now()} />
            </PageHeader>
            {detail.has_screen && (
              <Box mt="2">
                <Button emphasis="secondary" onClick={() => setShowScreen(true)}>
                  Screen
                </Button>
              </Box>
            )}
            {showScreen && <ScreenModal target={address} fetcher={fetchScreen} onClose={() => setShowScreen(false)} />}
            <ActionCard
              detail={detail}
              sent={sent}
              onSent={setSent}
              refetch={refetch}
            />
            {!detail.parse_error && (
              <>
                <TicketSections detail={detail} now={Date.now()} />
                {!hasSections(detail) && !needsActionCard(detail, sent) && <NothingHere />}
              </>
            )}
          </>
        ) : (
          <Spinner label="Loading ticket" />
        )}
    </DetailPage>
  );
}
