import { Box, chakra, Heading } from "@chakra-ui/react"
import { useRef, useState, type MouseEvent } from "react"
import { useLocation, useNavigate } from "react-router"
import { useConnectionContext } from "../connection"
import { FileLinkProvider } from "../files/FileLinkContext"
import { ActionAnswer, type TicketDetail } from "../generated/server"
import { ticketHref, ticketPath } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { StateHeader } from "../page/StateHeader"
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
  if (refused) return CHANGED
  if (t.form && t.firstSeq !== undefined && detail.latest?.seq !== t.firstSeq) return CHANGED
  if (offered) return null
  if (t.form) return ELSEWHERE
  return detail.latest?.outcome === "open" ? ON_TICKET_PAGE : ELSEWHERE
}

// Where a person answers one open question. Opened from the ticket page it sits on top of it, so
// leaving goes back one entry; opened cold (deep link, reload, push tap) there is no ticket entry
// below it, so leaving replaces this one.
export function AnswerPage({
  address,
  fetchDetail,
}: {
  address: string
  fetchDetail?: FetchDetail
}) {
  const conn = useConnectionContext()
  const navigate = useNavigate()
  const openedCold = useLocation().key === "default"
  const { detail, notFound, source, refetch } = useTicketDetail(address, conn, fetchDetail)
  useDetailReady({ detail, notFound, source })

  const leave = () => (openedCold ? navigate(ticketPath(address), { replace: true }) : navigate(-1))

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
              display="flex"
              alignItems="center"
              gap="10px"
              py="12px"
              borderBottomWidth="1px"
              borderColor="border"
            >
              <chakra.a
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
