import { Box, chakra } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { AgentLine } from "../design/AgentLine";
import { DesignChat } from "../design/ChatLog";
import { hasChat, roundKeyOf, useSessionChat, type FetchChat } from "../design/chat";
import { useFocusReport, type PostFocus } from "../design/focus";
import type { DeleteDesign, OpenFlow, ResumeSession, StopSession } from "../design/open";
import type { PostPrompt } from "../design/prompt";
import type { FetchScreen } from "../design/screen";
import { DesignActionsMenu, Failure, StartDesignButton } from "../design/SessionControls";
import type { SessionInfo } from "../generated/server";
import { PageHeader } from "../page/PageHeader";
import { useKeyboardOpen } from "../page/useKeyboardOpen";
import type { MenuEntry } from "../ui/menu/Menu";
import { useDevice } from "../useWide";
import type { SteppedModel } from "./model";
import { Progress, StepLine, StepsSheet, summaryLine, type StepRows } from "./Progress";

/** The injected actions the frame talks to the server through. */
export interface SteppedActions {
  fetchChat?: FetchChat;
  post?: PostPrompt;
  open?: OpenFlow;
  stop?: StopSession;
  resume?: ResumeSession;
  fetchScreen?: FetchScreen;
  focus?: PostFocus;
}

/** The Design-only menu items (Delete, Hold, workspace Move), given only by the Design page. */
export interface DesignMenu {
  /** The Move, Remove and Hold sections; the page renders the dialogs they open. */
  itemGroups?: MenuEntry[][];
  /** The map has no Destination and no tickets yet, so it can still be deleted. */
  deletable?: boolean;
  remove?: DeleteDesign;
  onDeleted: () => void;
  onOpenEpic: () => void;
}

export interface SteppedPageProps {
  /** The flow's key: a Design's map address, a Triage's project. */
  address: string;
  title: string;
  icon: ReactNode;
  wide: boolean;
  /** Absent until the flow's data has loaded. */
  model?: SteppedModel;
  /** The progress ring at a size; absent until the flow's data has loaded. */
  ring?: (size: "header" | "headerSm" | "compact") => ReactNode;
  /** Live sessions, newest first, and the one shown unless the reader picks another. */
  live: SessionInfo[];
  current?: SessionInfo;
  actions: SteppedActions;
  startLabel: string;
  startFailure?: string;
  /** Replaces the Start button: a flow's primary header button, given the session the page shows. */
  headerButton?: (session?: SessionInfo) => ReactNode;
  /** A header control left of the ring. */
  beforeRing?: ReactNode;
  /** What the busy row names: the step the agent reported and, for a ticket step, its title. */
  working: (session?: SessionInfo) => { step?: string; title?: string };
  // Optional parts, each rendered only when given.
  subtitleSuffix?: ReactNode;
  metaLink?: ReactNode;
  chatTop?: (ctx: { questions: boolean }) => ReactNode;
  /** The first entry of the Chat view's log (absent in the Questions view). */
  logTop?: ReactNode;
  // Design only.
  ticketRows?: ReactNode;
  /** Sub-rows under any step (Triage: Questions and Review under Approve). */
  stepRows?: StepRows;
  /** The step offers no Continue: the person acts on the page itself. */
  noContinue?: boolean;
  // Design only.
  designMenu?: DesignMenu;
  /** Extra items for the ⋯ menu, beside Copy ID. */
  menuItems?: MenuEntry[];
  /** Shown below the columns. */
  footer?: ReactNode;
}

// The right of the title row: the failed-start message, the first Start button, and the menu.
function SessionStrip({
  address,
  session,
  live,
  actions,
  onSelect,
  startFailure,
  startLabel,
  headerButton,
  designMenu,
  menuItems,
}: Pick<SteppedPageProps, "address" | "live" | "actions" | "startFailure" | "startLabel" | "headerButton" | "designMenu" | "menuItems"> & {
  session?: SessionInfo;
  onSelect: (id: string) => void;
}) {
  // The reason is for the failed start only: a live session means a retry worked.
  const reason = !session || session.status.state === "exited" ? startFailure : undefined;
  return (
    <Box display="flex" alignItems="center" justifyContent="flex-end" gap="3">
      <Failure>{reason ?? ""}</Failure>
      {headerButton ? (
        headerButton(session)
      ) : (
        !session && <StartDesignButton address={address} open={actions.open} label={startLabel} />
      )}
      <DesignActionsMenu
        address={address}
        shown={session}
        live={live}
        designItems={!!designMenu}
        extraItems={menuItems}
        deletable={designMenu?.deletable}
        itemGroups={designMenu?.itemGroups}
        onOpenEpic={designMenu?.onOpenEpic}
        onSelect={onSelect}
        onDeleted={designMenu?.onDeleted ?? (() => {})}
        open={actions.open}
        stop={actions.stop}
        remove={designMenu?.remove}
      />
    </Box>
  );
}

/** The title row and the Chat and Progress columns every stepped flow shares. */
export function SteppedPage({
  address,
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
  ticketRows,
  stepRows,
  noContinue,
  designMenu,
  menuItems,
  footer,
}: SteppedPageProps) {
  const conn = useConnectionContext();
  const connState = useConnectionState(conn);
  const [selected, setSelected] = useState<string>();
  const [toggle, setToggle] = useState<{ key: string; on: boolean }>();
  const [stepsOpen, setStepsOpen] = useState(false);
  // Below desktop the chat takes the full width: the footer's Add, Note, Auto, countdown and Send need it on one line.
  // The steps then open from the header like on a phone.
  const sideColumn = useDevice() === "desktop";
  const keyboardOpen = useKeyboardOpen();
  const stepsInline = wide && sideColumn;
  // Switching shows an older live session; once it is gone the newest drives the page again.
  const session = live.find((s) => s.id === selected) ?? current;
  const chatty = hasChat(session?.harness);
  const { detail: chat, refetch: refetchChat } = useSessionChat(session && chatty ? session.id : undefined, conn, actions.fetchChat);
  // Derived, not stored, so it cannot go stale: a round opens in Questions, and the reader's toggle holds for
  // that session and round only. The message list and the impl link both read it.
  const viewKey = `${session?.id}|${roundKeyOf(chat)}`;
  const questions = chatty && (chat?.round?.length ?? 0) > 0 && (toggle?.key === viewKey ? toggle.on : true);
  useFocusReport(conn, session?.id, connState.streams, actions.focus);
  return (
    <>
      {stepsOpen && model && (
        <StepsSheet model={model} rows={ticketRows} stepRows={stepRows} ring={ring?.("compact")} onClose={() => setStepsOpen(false)} />
      )}
      {/* With the keyboard up the header would take most of the visible height; the question needs it. */}
      <Box mt="4" mb="3" flex="none" display={keyboardOpen && !sideColumn ? "none" : undefined}>
        <PageHeader
          title={title}
          icon={icon}
          caret
          subtitle={
            model && (
              <>
                {stepsInline ? summaryLine(model) : <StepLine model={model} onOpen={() => setStepsOpen(true)} />}
                {subtitleSuffix}
              </>
            )
          }
          meta={
            <Box display="flex" flexWrap="wrap" alignItems="center" columnGap="3" rowGap="1" fontSize="12px">
              {session && !designMenu && <AgentLine session={session} wide={wide} />}
              {metaLink}
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
              <SessionStrip
                address={address}
                session={session}
                live={live}
                actions={actions}
                onSelect={setSelected}
                startFailure={startFailure}
                startLabel={startLabel}
                headerButton={headerButton}
                designMenu={designMenu}
                menuItems={menuItems}
              />
            </>
          }
        />
      </Box>
      <Box
        display="grid"
        gridTemplateColumns={sideColumn ? "minmax(0, 1fr) 300px" : "1fr"}
        gridTemplateRows={"minmax(0, 1fr)"}
        gap="4"
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
        >
          {chatTop?.({ questions })}
          <DesignChat
            session={session}
            chat={noContinue && chat ? { ...chat, next: undefined } : chat}
            refetch={refetchChat}
            post={actions.post}
            done={model?.done}
            nextTitle={model?.nextTitle}
            working={working(session)}
            logTop={logTop}
            questions={questions}
            onQuestions={(on) => setToggle({ key: viewKey, on })}
            actions={{ address, open: actions.open, resume: actions.resume, fetchScreen: actions.fetchScreen }}
          />
        </Box>
        {stepsInline && (
          <Box aria-label="Progress" role="region" gridRow={1} gridColumn={2} minH="0" overflowY="auto">
            {model && <Progress model={model} rows={ticketRows} stepRows={stepRows} />}
          </Box>
        )}
      </Box>
      {footer && (
        <Box flex="none" maxH="30vh" overflowY="auto" mt="3">
          {footer}
        </Box>
      )}
    </>
  );
}
