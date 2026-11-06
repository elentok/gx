import { Box } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { fetchTicketScreen, type FetchScreen } from "../design/screen";
import { ScreenModal } from "../design/ScreenModal";
import { DetailPage } from "../page/DetailPage";
import { VerdictLine, verdictText } from "../verdict";
import { isExited } from "../agents/sessionWrites";
import { PageHeader } from "../page/PageHeader";
import { Button } from "../ui/button/Button";
import { EmptyState } from "../ui/empty-state/EmptyState";
import { Spinner } from "../ui/spinner/Spinner";
import { ActionCard, needsActionCard } from "./ActionCard";
import type { Sent } from "./answer";
import { useTicketDetail, type FetchDetail } from "./detail";
import { TicketHeader } from "./TicketHeader";
import { hasSections, NothingHere, TicketSections } from "./TicketSections";

const WAITING_REFETCH_MS = 2000;

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
  const snapshot = useConnectionState(conn).snapshot;
  const navigate = useNavigate();
  const agent = snapshot?.sessions?.find((s) => s.ticket === address && !isExited(s));
  const { detail, notFound, refetch } = useTicketDetail(address, conn, fetchDetail);
  const verdict = verdictText(snapshot?.pending, address, detail?.stats?.launch_step);
  // The lock being taken emits no ticket event, so a page that shows the wait re-reads until it clears.
  const waiting = !!detail?.waiting_label;
  useEffect(() => {
    if (!waiting) return;
    const id = setInterval(refetch, WAITING_REFETCH_MS);
    return () => clearInterval(id);
  }, [waiting, refetch]);
  const [sent, setSent] = useState<Sent | null>(null);
  const [showScreen, setShowScreen] = useState(false);

  return (
    <DetailPage mode="narrow" title={{ kind: "Ticket", name: detail?.title.trim() || address }}>
        {notFound ? (
          <EmptyState title="Ticket not found">{address} does not exist.</EmptyState>
        ) : detail ? (
          <>
            <PageHeader title={detail.title}>
              <TicketHeader detail={detail} now={Date.now()} />
              {verdict && (
                <Box mt="2">
                  <VerdictLine text={verdict} />
                </Box>
              )}
            </PageHeader>
            {(detail.has_screen || agent) && (
              <Box mt="2" display="flex" gap="2">
                {detail.has_screen && (
                  <Button emphasis="secondary" onClick={() => setShowScreen(true)}>
                    Screen
                  </Button>
                )}
                {agent && (
                  <Button emphasis="secondary" onClick={() => navigate(`/agents/${agent.id}`)}>
                    Goto agent
                  </Button>
                )}
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
