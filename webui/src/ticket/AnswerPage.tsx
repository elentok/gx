import { Box, chakra, Heading } from "@chakra-ui/react"
import { useRef, useState, type MouseEvent } from "react"
import { useLocation, useNavigate } from "react-router"
import { useConnectionContext } from "../connection"
import { FileLinkProvider } from "../files/FileLinkContext"
import { ActionAnswer, type TicketDetail } from "../generated/server"
import { ticketHref, ticketPath } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { StateHeader } from "../page/StateHeader"
import { useKeyboardOpen } from "../page/useKeyboardOpen"
import { useDevice } from "../useWide"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { useDetailReady } from "../useReady"
import { AnswerForm, AnswerPageFrame, BackButton } from "./AnswerForm"
import { useTicketDetail, type FetchDetail } from "./detail"
import { recordSent } from "./sent"
import { askerLabel, EarlierRounds, Label } from "./TicketReading"

const ELSEWHERE = "This question was answered elsewhere"
const CHANGED = "This question changed. Back to ticket to see the new one."
const ON_TICKET_PAGE = "This question is answered on the ticket page."

// Why the form can no longer send, or null while it can. A latest round that is closed or missing
// has no wording in the spec, so it reads as answered elsewhere.
function staleBanner(
  detail: TicketDetail,
  offered: boolean,
  t: { firstSeq?: number | null; form: TicketDetail | null },
  refused: boolean,
): string | null {
  if (t.form && t.firstSeq !== undefined && detail.latest?.seq !== t.firstSeq) return CHANGED
  // A refusal whose refetch still shows the same open round has no better reason than changed.
  if (offered) return refused ? CHANGED : null
  if (t.form) return ELSEWHERE
  return detail.latest?.outcome === "open" ? ON_TICKET_PAGE : ELSEWHERE
}

// Where a person answers one open question. Opened from the ticket page it sits on top of it, so
// leaving goes back one entry (the ticket page marks that with fromTicket state); opened any other
// way (deep link, reload, push tap) the entry below is not the ticket, so leaving replaces this one.
export function AnswerPage(props: { address: string; fetchDetail?: FetchDetail }) {
  // Keyed by address so a route change in place starts a fresh form, not the old ticket's question.
  return <AnswerPageFor key={props.address} {...props} />
}

function AnswerPageFor({ address, fetchDetail }: { address: string; fetchDetail?: FetchDetail }) {
  const conn = useConnectionContext()
  const navigate = useNavigate()
  const fromTicket = !!(useLocation().state as { fromTicket?: boolean } | null)?.fromTicket
  const { detail, notFound, source, refetch } = useTicketDetail(address, conn, fetchDetail)
  useDetailReady({ detail, notFound, source })

  // The keyboard leaves little height, so the header gives it to the scroll body. The footer stays.
  const keyboardOpen = useKeyboardOpen()
  const device = useDevice()
  const compact = keyboardOpen && device !== "desktop"

  const leave = () => (fromTicket ? navigate(-1) : navigate(ticketPath(address), { replace: true }))

  // The round the form loaded. firstSeq comes from the first network detail only: the cached copy
  // may hold an older question, and a page opened on a replaced question is a fresh form.
  const track = useRef<{ firstSeq?: number | null; form: TicketDetail | null }>({ form: null })
  const [refused, setRefused] = useState(false)
  const t = track.current
  if (detail && source === "network" && t.firstSeq === undefined) {
    t.firstSeq = detail.latest?.seq ?? null
  }
  const offered = !!detail?.actions.includes(ActionAnswer)
  if (detail && offered && (t.firstSeq === undefined || detail.latest?.seq === t.firstSeq)) {
    t.form = detail
  }
  const banner = detail && staleBanner(detail, offered, t, refused)
  const frozen = t.form && t.form.latest?.seq === t.firstSeq ? t.form : null
  const shown = banner ? frozen : offered ? detail : null

  return (
    <DetailPage fill title={{ kind: "Answer question", name: detail?.title.trim() || address }}>
      <FileLinkProvider base={{ project: address.split(":")[0] }}>
        {notFound ? (
          <>
            <StateHeader title={address} />
            <EmptyState title="Ticket not found">{address} does not exist.</EmptyState>
          </>
        ) : detail ? (
          <>
            <Box
              data-mock-part="answer-header"
              display={compact ? "none" : "flex"}
              alignItems="center"
              gap="10px"
              py="12px"
              // The shell's floating Show sidebar button (40px at 16px) covers the start on a phone.
              ps={{ base: "50px", md: "0" }}
              borderBottomWidth="1px"
              borderColor="border"
            >
              <chakra.a
                data-mock-part="back-link"
                href={ticketHref(address)}
                color="text.muted"
                fontSize="15px"
                fontWeight="600"
                whiteSpace="nowrap"
                _hover={{ color: "text" }}
                onClick={(e: MouseEvent) => {
                  e.preventDefault()
                  leave()
                }}
              >
                ← Ticket
              </chakra.a>
              <Heading as="h1" fontSize="17px" fontWeight="600" flex="1" minW="0" truncate>
                Answer question
              </Heading>
            </Box>
            {banner && !shown && (
              <AnswerPageFrame banner={banner} footer={<BackButton onClick={leave} />} />
            )}
            {shown && (
              <AnswerForm
                detail={shown}
                stale={banner ? { banner, onBack: leave } : undefined}
                onSent={(s) => {
                  recordSent(address, s)
                  leave()
                }}
                onStale={() => {
                  setRefused(true)
                  refetch()
                }}
                lead={<Label>{askerLabel(shown.latest)}</Label>}
              >
                <EarlierRounds detail={shown} />
              </AnswerForm>
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
