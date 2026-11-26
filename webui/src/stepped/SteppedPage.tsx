import { Box, chakra } from "@chakra-ui/react"
import { useEffect, useState, type ReactNode } from "react"
import { currentConfirm, useConfirmRequest } from "../actions/confirmRequest"
import { useConnectionContext, useConnectionState } from "../connection"
import { DesignChat } from "../design/ChatLog"
import { hasChat, roundKeyOf, roundOf, useSessionChat, type FetchChat } from "../design/chat"
import { useFocusReport, type PostFocus } from "../design/focus"
import type { OpenFlow, ResumeSession, StopSession } from "../design/open"
import type { PostAnswer, PostPrompt } from "../design/prompt"
import type { FetchScreen } from "../design/screen"
import { DesignActionsMenu, Failure, StartDesignButton } from "../design/SessionControls"
import type { SessionInfo } from "../generated/server"
import { SubagentOriginContext } from "../subagentOrigin"
import { PageHeader } from "../page/PageHeader"
import { useKeyboardOpen } from "../page/useKeyboardOpen"
import type { MenuEntry } from "../ui/menu/Menu"
import { useDevice } from "../useWide"
import type { SteppedModel } from "./model"
import { Progress, StepLine, StepsSheet, summaryLine, type StepRows } from "./Progress"

/** The injected actions the frame talks to the server through. */
export interface SteppedActions {
  fetchChat?: FetchChat
  post?: PostPrompt
  answer?: PostAnswer
  open?: OpenFlow
  stop?: StopSession
  resume?: ResumeSession
  fetchScreen?: FetchScreen
  focus?: PostFocus
}

/** The Design page's own ⋯ menu (registry actions), given only by the Design page; Triage keeps `DesignActionsMenu`. */
export type DesignMenu = (ctx: {
  shown?: SessionInfo
  live: SessionInfo[]
  onSelect: (id: string) => void
  onOpenChange: (open: boolean) => void
}) => ReactNode

export interface SteppedPageProps {
  /** The flow's key: a Design's map address, a Triage's project. */
  address: string
  /** This page's router path, which its subagent links lead back to. */
  origin?: string
  title: string
  icon: ReactNode
  wide: boolean
  /** Absent until the flow's data has loaded. */
  model?: SteppedModel
  /** The progress ring at a size; absent until the flow's data has loaded. */
  ring?: (size: "header" | "headerSm" | "compact") => ReactNode
  /** Live sessions, newest first, and the one shown unless the reader picks another. */
  live: SessionInfo[]
  current?: SessionInfo
  actions: SteppedActions
  startLabel: string
  startFailure?: string
  /** Replaces the Start button: a flow's primary header button, given the session the page shows. */
  headerButton?: (session?: SessionInfo) => ReactNode
  /** A header control left of the ring. */
  beforeRing?: ReactNode
  /** What the busy row names: the step the agent reported and, for a ticket step, its title. */
  working: (session?: SessionInfo) => { step?: string; title?: string }
  // Optional parts, each rendered only when given.
  subtitleSuffix?: ReactNode
  metaLink?: ReactNode
  chatTop?: (ctx: { questions: boolean }) => ReactNode
  /** The first entry of the Chat view's log (absent in the Questions view). */
  logTop?: ReactNode
  /** After the Chat view's messages (Triage's plan card, under the agent's lead-in). */
  logTail?: ReactNode
  // Design only.
  ticketRows?: ReactNode
  /** Sub-rows under any step (Triage: Questions and Review under Approve). */
  stepRows?: StepRows
  /** The Progress column fills the page height with its own border. */
  progressColumn?: boolean
  // Design only.
  designMenu?: DesignMenu
  /** Extra items for the ⋯ menu, beside Copy ID. */
  menuItems?: MenuEntry[]
  /** Under the Progress column; at the page's foot where Progress sits in a sheet. */
  footer?: ReactNode
}

// The right of the title row, left of the menu: the failed-start message and the first Start button.
function SessionStrip({
  address,
  session,
  actions,
  startFailure,
  startLabel,
  headerButton,
}: Pick<
  SteppedPageProps,
  "address" | "actions" | "startFailure" | "startLabel" | "headerButton"
> & {
  session?: SessionInfo
}) {
  // The reason is for the failed start only: a live session means a retry worked.
  const reason = !session || session.status.state === "exited" ? startFailure : undefined
  return (
    <Box display="flex" alignItems="center" justifyContent="flex-end" gap="3">
      <Failure>{reason ?? ""}</Failure>
      {headerButton
        ? headerButton(session)
        : !session && (
            <StartDesignButton address={address} open={actions.open} label={startLabel} />
          )}
    </Box>
  )
}

/** The title row and the Chat and Progress columns every stepped flow shares. */
export function SteppedPage({
  address,
  origin,
  title,
  icon,
  wide,
  model,
  ring,
  live,
  current,
  actions,
  startLabel,
  startFailure,
  headerButton,
  beforeRing,
  working,
  subtitleSuffix,
  metaLink,
  chatTop,
  logTop,
  logTail,
  ticketRows,
  stepRows,
  progressColumn,
  designMenu,
  menuItems,
  footer,
}: SteppedPageProps) {
  const conn = useConnectionContext()
  const connState = useConnectionState(conn)
  const [selected, setSelected] = useState<string>()
  const [toggle, setToggle] = useState<{ key: string; on: boolean }>()
  const [stepsOpen, setStepsOpen] = useState(false)
  // Below desktop the chat takes the full width: the footer's Add, Note, Auto, countdown and Send need it on one line.
  // The steps then open from the header like on a phone.
  const sideColumn = useDevice() === "desktop"
  const keyboardOpen = useKeyboardOpen()
  // Opening the tabs-row menu moves focus off the field and closes the keyboard; the layout stays until the menu closes.
  const [menuHeld, setMenuHeld] = useState(false)
  // A confirm a menu pick opened is anchored on the tabs-row ⋯, which the layout must keep until it closes.
  const [confirmHeld, setConfirmHeld] = useState(false)
  const confirming = useConfirmRequest()
  useEffect(() => {
    if (!confirming) setConfirmHeld(false)
  }, [confirming])
  const holdMenu = (open: boolean) => {
    setMenuHeld(open)
    if (!open && currentConfirm()) setConfirmHeld(true)
  }
  const compact = !sideColumn && (keyboardOpen || menuHeld || confirmHeld)
  const stepsInline = wide && sideColumn
  // Switching shows an older live session; once it is gone the newest drives the page again.
  const session = live.find((s) => s.id === selected) ?? current
  // Off a wide screen the Start button and failure text would push the ⋯ onto a second row, so they sit in the meta bar.
  const strip = (
    <SessionStrip
      address={address}
      session={session}
      actions={actions}
      startFailure={startFailure}
      startLabel={startLabel}
      headerButton={headerButton}
    />
  )
  const chatty = hasChat(session?.harness)
  const { detail: chat, refetch: refetchChat } = useSessionChat(
    session && chatty ? session.id : undefined,
    conn,
    actions.fetchChat,
  )
  // Derived, not stored, so it cannot go stale: a round opens in Questions, and the reader's toggle holds for
  // that session and round only. The message list and the impl link both read it.
  const viewKey = `${session?.id}|${roundKeyOf(chat)}`
  const hasRound = chatty && roundOf(chat).length > 0
  const questions = hasRound && (toggle?.key === viewKey ? toggle.on : true)
  useFocusReport(conn, session?.id, connState.streams, actions.focus)
  const menu = (onOpenChange: (open: boolean) => void = () => {}) =>
    designMenu ? (
      designMenu({ shown: session, live, onSelect: setSelected, onOpenChange })
    ) : (
      <DesignActionsMenu
        address={address}
        shown={session}
        live={live}
        extraItems={menuItems}
        onSelect={setSelected}
        open={actions.open}
        stop={actions.stop}
        onOpenChange={onOpenChange}
      />
    )
  return (
    <>
      {stepsOpen && model && (
        <StepsSheet
          model={model}
          rows={ticketRows}
          stepRows={stepRows}
          ring={ring?.("compact")}
          onClose={() => setStepsOpen(false)}
        />
      )}
      {/* With the keyboard up the header would take most of the visible height; the question needs it. */}
      <Box mb="3" flex="none" display={compact ? "none" : undefined}>
        <PageHeader
          title={title}
          icon={icon}
          subtitle={
            model && (
              <>
                {stepsInline ? (
                  summaryLine(model)
                ) : (
                  <StepLine model={model} onOpen={() => setStepsOpen(true)} />
                )}
                {subtitleSuffix}
              </>
            )
          }
          meta={
            <Box
              display="flex"
              flexWrap="wrap"
              alignItems="center"
              columnGap="3"
              rowGap="1"
              fontSize="12px"
            >
              {metaLink}
              {!wide && strip}
            </Box>
          }
          actions={
            <>
              {beforeRing}
              {ring &&
                (stepsInline ? (
                  ring("header")
                ) : (
                  <chakra.button
                    type="button"
                    aria-label="Show progress steps"
                    aria-haspopup="dialog"
                    cursor="pointer"
                    onClick={() => setStepsOpen(true)}
                  >
                    {ring("headerSm")}
                  </chakra.button>
                ))}
              {wide && strip}
              {!compact && menu()}
            </>
          }
        />
      </Box>
      <Box
        display="grid"
        gridTemplateColumns={sideColumn ? "minmax(0, 1fr) 300px" : "1fr"}
        gridTemplateRows={"minmax(0, 1fr)"}
        gap={progressColumn ? "0" : "4"}
        flex="1"
        minH="0"
      >
        <Box
          aria-label="Chat"
          role="region"
          minW="0"
          minH="0"
          // The question card keeps a floor height (QuestionSheet); on a very short viewport the region scrolls.
          overflowY="auto"
          display="flex"
          flexDirection="column"
          gap="3"
          gridRow={1}
          gridColumn={1}
          pr={progressColumn && sideColumn ? "4" : undefined}
        >
          {chatTop?.({ questions })}
          <SubagentOriginContext.Provider value={origin}>
            <DesignChat
              session={session}
              chat={chat}
              refetch={refetchChat}
              post={actions.post}
              answer={actions.answer}
              done={model?.done}
              nextTitle={model?.nextTitle}
              working={working(session)}
              logTop={compact ? undefined : logTop}
              logTail={logTail}
              tabsMenu={compact && hasRound ? menu(holdMenu) : undefined}
              questions={questions}
              onQuestions={(on) => setToggle({ key: viewKey, on })}
              actions={{
                address,
                open: actions.open,
                resume: actions.resume,
                fetchScreen: actions.fetchScreen,
              }}
            />
          </SubagentOriginContext.Provider>
        </Box>
        {stepsInline && (
          <Box
            aria-label="Progress"
            role="region"
            gridRow={1}
            gridColumn={2}
            minH="0"
            overflowY="auto"
            // Triage's column is flush: full page height, a hairline on its left, no card. A Design's panel is only as tall as its steps.
            bg={progressColumn ? "sidebar" : undefined}
            borderLeftWidth={progressColumn ? "1px" : undefined}
            borderColor="hairline"
          >
            {model && (
              <Progress
                model={model}
                rows={ticketRows}
                stepRows={stepRows}
                flush={progressColumn}
              />
            )}
            {footer && <Box mt="4">{footer}</Box>}
          </Box>
        )}
      </Box>
      {footer && !stepsInline && (
        <Box flex="none" maxH="30vh" overflowY="auto" mt="3">
          {footer}
        </Box>
      )}
    </>
  )
}
