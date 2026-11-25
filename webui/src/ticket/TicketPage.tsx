import { Box } from "@chakra-ui/react"
import { useEffect, useRef, useState } from "react"
import { useNavigate } from "react-router"
import { useActions } from "../actions/useActions"
import { ActionStartOver, type TicketDetail } from "../generated/server"
import { GxMenu, menuGroups } from "../ui/menu/Menu"
import { MoreActionsButton } from "../ui/menu/MoreActionsButton"
import { groupsOf } from "../workspaces/itemActions"
import { ticketEntity } from "./recover"
import { useConnectionContext, useConnectionState } from "../connection"
import { FileLinkProvider } from "../files/FileLinkContext"
import { fetchTicketScreen, type FetchScreen } from "../design/screen"
import { ScreenModal } from "../design/ScreenModal"
import { DetailPage } from "../page/DetailPage"
import { VerdictLine, verdictText } from "../verdict"
import { isExited } from "../agents/sessionWrites"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { StateHeader } from "../page/StateHeader"
import { Button } from "../ui/button/Button"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { ActionCard, needsActionCard } from "./ActionCard"
import { recordSent, useSent } from "./sent"
import { useDetailReady } from "../useReady"
import { useTicketDetail, type FetchDetail } from "./detail"
import { TicketHeader } from "./TicketHeader"
import { hasSections, NothingHere, TicketSections } from "./TicketSections"

const WAITING_REFETCH_MS = 2000

// The ⋯ of the page: Copy ID, then Start over and Cancel run. The confirm anchors on the ⋯ once the menu closed.
function TicketMenu({ detail, onRecovered }: { detail: TicketDetail; onRecovered: () => void }) {
  const wrap = useRef<HTMLSpanElement>(null)
  const { actions } = useActions(ticketEntity(detail), {
    surface: () => {
      const anchor = wrap.current?.querySelector("button")
      return anchor ? { mode: "anchor", anchor } : { mode: "modal" }
    },
    state: detail.state ?? "",
    tooltip: detail.action_info?.[ActionStartOver]?.tooltip,
    onRecovered,
  })
  return (
    <span ref={wrap} style={{ display: "contents" }}>
      <GxMenu trigger={<MoreActionsButton />} items={menuGroups(groupsOf(actions))} />
    </span>
  )
}

export function TicketPage({
  address,
  fetchDetail,
  fetchScreen = fetchTicketScreen,
}: {
  address: string
  fetchDetail?: FetchDetail
  fetchScreen?: FetchScreen
}) {
  const conn = useConnectionContext()
  // Subscribing re-renders the page every tick, which keeps a running ticket's clock counting.
  const snapshot = useConnectionState(conn).snapshot
  const navigate = useNavigate()
  const agent = snapshot?.sessions?.find((s) => s.ticket === address && !isExited(s))
  const { detail, notFound, source, refetch } = useTicketDetail(address, conn, fetchDetail)
  useDetailReady({ detail, notFound, source })
  const verdict = verdictText(snapshot?.pending, address, detail?.stats?.launch_step)
  // The lock being taken emits no ticket event, so a page that shows the wait re-reads until it clears.
  const waiting = !!detail?.waiting_label
  useEffect(() => {
    if (!waiting) return
    const id = setInterval(refetch, WAITING_REFETCH_MS)
    return () => clearInterval(id)
  }, [waiting, refetch])
  // A live task ending emits no ticket event, and the gate card's buttons depend on it.
  const taskLive = !!detail?.task
  useEffect(() => {
    if (!taskLive) return
    const id = setInterval(refetch, WAITING_REFETCH_MS)
    return () => clearInterval(id)
  }, [taskLive, refetch])
  const sent = useSent(address)
  const [showScreen, setShowScreen] = useState(false)

  return (
    <DetailPage title={{ kind: "Ticket", name: detail?.title.trim() || address }}>
      <FileLinkProvider base={{ project: address.split(":")[0] }}>
        {notFound ? (
          <>
            <StateHeader title={address} />
            <EmptyState title="Ticket not found">{address} does not exist.</EmptyState>
          </>
        ) : detail ? (
          <>
            <PageHeader
              title={detail.title}
              icon={<PageIcon kind="ticket" />}
              subtitle={address}
              actions={<TicketMenu detail={detail} onRecovered={refetch} />}
            >
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
            {showScreen && (
              <ScreenModal
                target={address}
                fetcher={fetchScreen}
                onClose={() => setShowScreen(false)}
              />
            )}
            <ActionCard
              detail={detail}
              sent={sent}
              onSent={(s) => recordSent(address, s)}
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
          <>
            <StateHeader title={address} />
            <FullScreenLoader label="Loading ticket" />
          </>
        )}
      </FileLinkProvider>
    </DetailPage>
  )
}
