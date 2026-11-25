import { Box, chakra, Heading } from "@chakra-ui/react"
import type { MouseEvent } from "react"
import { useLocation, useNavigate } from "react-router"
import { useConnectionContext } from "../connection"
import { FileLinkProvider } from "../files/FileLinkContext"
import { ActionAnswer } from "../generated/server"
import { ticketHref, ticketPath } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { StateHeader } from "../page/StateHeader"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { useDetailReady } from "../useReady"
import { AnswerForm } from "./AnswerForm"
import { useTicketDetail, type FetchDetail } from "./detail"
import { recordSent } from "./sent"
import { askerLabel, EarlierRounds, Label } from "./TicketReading"

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
            {detail.actions.includes(ActionAnswer) && (
              <AnswerForm
                layout="page"
                detail={detail}
                onSent={(s) => {
                  recordSent(address, s)
                  leave()
                }}
                onStale={refetch}
                lead={<Label>{askerLabel(detail.latest)}</Label>}
              >
                <EarlierRounds detail={detail} />
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
