import { Box, chakra } from "@chakra-ui/react"
import { Fragment, useState, type ReactNode } from "react"
import { useLocation, useNavigate } from "react-router"
import { useConnectionContext, useConnectionState } from "../connection"
import { useEpicList, type FetchEpicList } from "../epics/list"
import {
  buildSidebar,
  type SidebarAgent,
  type SidebarRow,
  type SidebarStatus,
} from "../workspaces/sidebarModel"
import type { Workspace } from "../generated/server"
import { ItemMenu, type ItemRef } from "../workspaces/itemActions"
import { SessionActions } from "../agents/SessionActions"
import { canManage } from "../agents/sessionWrites"
import { useWorkspaces } from "../workspaces/useActiveWorkspace"
import { ProgressRing } from "../ui/progress-ring/ProgressRing"
import { CHAT_ATTR, NAV_ATTR } from "./SidebarKeys"
import { useMoreActionsSize } from "../ui/menu/MoreActionsButton"
import { GROUP_ON_HOLD, GROUP_OTHER } from "../select/selectModel"
import { SelectBox, useSidebarSelect } from "./SidebarSelect"
import { useAltHeld } from "./useShortcuts"

/** The epic address of the open page, or "". */
export function currentAddress(pathname: string): string {
  const [, kind, ...rest] = pathname.split("/")
  const id = decodeURIComponent(rest.join("/"))
  if (kind === "d" || kind === "e") return id
  if (kind === "t") return id.replace(/\/[^/]*$/, "")
  return ""
}

const rowStyle = {
  display: "flex",
  alignItems: "center",
  gap: "2",
  minH: "34px",
  px: "3",
  textDecoration: "none",
  color: "inherit",
  _hover: { bg: "sidebar.hover" },
  // The ring is drawn by RowFrame so it covers the whole row, not just this link.
  "data-focus-ring": "none",
} as const

const statusColor = (status: SidebarStatus, dim?: boolean) =>
  dim || status.tone === "muted" ? "text.faint" : `tone.${status.tone}.fg`

function Status({ status, dim }: { status: SidebarStatus; dim?: boolean }) {
  return (
    <Box
      as="span"
      fontSize="12px"
      color={statusColor(status, dim)}
      whiteSpace="nowrap"
      overflow="hidden"
      textOverflow="ellipsis"
    >
      {status.words}
    </Box>
  )
}

/** The chat icon. While working, three dots inside the bubble bounce in turn (still under reduced motion). */
function ChatIcon({ working }: { working?: boolean }) {
  return (
    <Box
      as="span"
      display="grid"
      data-working={working ? "" : undefined}
      css={{
        "& circle": {
          transformBox: "fill-box",
          transformOrigin: "center",
          animation: "chatDot 1.2s ease-in-out infinite",
        },
        "& circle:nth-of-type(2)": { animationDelay: "0.15s" },
        "& circle:nth-of-type(3)": { animationDelay: "0.3s" },
        "@media (prefers-reduced-motion: reduce)": { "& circle": { animation: "none" } },
      }}
    >
      <svg
        width="26"
        height="26"
        viewBox="0 0 26 26"
        fill="none"
        stroke="var(--chakra-colors-chat)"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-label="Chat"
        role="img"
      >
        <path d="M5 7.5a2.5 2.5 0 0 1 2.5-2.5h11A2.5 2.5 0 0 1 21 7.5v7a2.5 2.5 0 0 1-2.5 2.5H12l-4 3.5V17h-.5A2.5 2.5 0 0 1 5 14.5v-7z" />
        {working && (
          <g data-testid="chat-dots" fill="var(--chakra-colors-chat)" stroke="none">
            <circle cx="9.5" cy="11" r="1.3" />
            <circle cx="13" cy="11" r="1.3" />
            <circle cx="16.5" cy="11" r="1.3" />
          </g>
        )}
      </svg>
    </Box>
  )
}

function WrenchIcon({ status }: { status: SidebarStatus }) {
  return (
    <Box as="span" data-testid="fix-wrench" flex="none" display="grid" color={statusColor(status)}>
      <svg
        width="18"
        height="18"
        viewBox="0 0 20 20"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-label="Fix agent"
        role="img"
      >
        <path d="M13.5 3a4 4 0 0 0-3.6 5.5L3.5 15a1.8 1.8 0 0 0 2.5 2.5l6.5-6.4A4 4 0 0 0 17 6.5l-2.5 2.5-2-.5-.5-2z" />
      </svg>
    </Box>
  )
}

/** The Triage icon: an inbox tray, coloured by status. */
function InboxIcon({ status }: { status: SidebarStatus }) {
  const working = status.tone === "running"
  return (
    <Box
      as="span"
      data-testid="triage-icon"
      data-working={working ? "" : undefined}
      flex="none"
      display="grid"
      color={statusColor(status)}
      css={{
        "& circle": {
          transformBox: "fill-box",
          transformOrigin: "center",
          animation: "inboxDrop 1.2s ease-in infinite",
        },
        "@media (prefers-reduced-motion: reduce)": { "& circle": { animation: "none" } },
      }}
    >
      <svg
        width="18"
        height="18"
        viewBox="0 0 20 20"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-label="Triage"
        role="img"
      >
        <path d="M2.5 11.5l2.2-6.2A1.5 1.5 0 0 1 6.1 4.3h7.8a1.5 1.5 0 0 1 1.4 1l2.2 6.2" />
        <path d="M2.5 11.5v3.2a1.5 1.5 0 0 0 1.5 1.5h12a1.5 1.5 0 0 0 1.5-1.5v-3.2h-4.2a2.3 2.3 0 0 1-2.3 1.8h-2a2.3 2.3 0 0 1-2.3-1.8z" />
        {working && (
          <circle
            data-testid="triage-drop"
            cx="10"
            cy="6.5"
            r="1.4"
            fill="currentColor"
            stroke="none"
          />
        )}
      </svg>
    </Box>
  )
}

function Chevron({
  open,
  onToggle,
  label,
}: {
  open: boolean
  onToggle: () => void
  label: string
}) {
  return (
    <chakra.button
      type="button"
      data-chevron
      tabIndex={-1}
      aria-label={label}
      aria-expanded={open}
      onClick={onToggle}
      boxSize="16px"
      display="grid"
      placeItems="center"
      color="text.faint"
      cursor="pointer"
      flex="none"
    >
      <svg
        width="12"
        height="12"
        viewBox="0 0 12 12"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden
        style={{ transform: open ? "rotate(90deg)" : "none" }}
      >
        <path d="M4.5 2.5 8 6l-3.5 3.5" />
      </svg>
    </chakra.button>
  )
}

// A row's frame (the mock's `padding 0 6px 0 4px`): 3px edge, 4px left padding, the chevron cell, then the icon slot.
// The nested guide line sits in the icon column, 3px left of its centre so a Fix agent's wrench is not drawn over it.
// `left` on an absolute child is measured from the padding edge, so the 3px border is not counted.
const CHEVRON_W = 20
const GUIDE_X = `calc(4px + ${CHEVRON_W}px + 13px - 3px - 0.75px)`

/** The frame both a top-level row and a nested agent row share: hover group, current background and edge. */
function RowFrame({
  current,
  selected,
  children,
}: {
  current: boolean
  selected?: boolean
  children: ReactNode
}) {
  return (
    <Box
      className="group"
      data-selected={selected ? "" : undefined}
      position="relative"
      display="flex"
      alignItems="center"
      pl="1"
      pr="6px"
      bg={selected ? "sidebar.selected" : current ? "sidebar.current" : undefined}
      borderLeftWidth="3px"
      borderLeftColor={current ? "tone.running.fg" : "transparent"}
      borderRadius="6px"
      css={{
        "&:has([data-nav]:focus-visible)": {
          boxShadow: "inset 0 0 0 2px {colors.tone.running.fg} !important",
        },
      }}
    >
      {children}
    </Box>
  )
}

/** The row's checkbox in select mode, in its own cell so the columns stay aligned. */
function RowCheck({ id, label }: { id: string; label: string }) {
  const sel = useSidebarSelect()
  return (
    <Box flex="none" display="flex" alignItems="center" pr="1">
      <SelectBox
        checked={sel.state.keys.includes(id)}
        label={`Select ${label}`}
        onToggle={() => sel.toggle(id)}
      />
    </Box>
  )
}

function IconSlot({ children }: { children?: ReactNode }) {
  return (
    <Box
      data-testid="icon-slot"
      w="26px"
      flex="none"
      display="flex"
      alignItems="center"
      justifyContent="center"
    >
      {children}
    </Box>
  )
}

/** Reserves the menu button's width (the nested row has no menu yet), so status words line up with the parent's. */
function MenuSpacer() {
  return <Box data-testid="menu-spacer" flex="none" w={useMoreActionsSize()} />
}

/** A nested agent row on the parent row's columns: empty chevron cell, icon slot (wrench for a Fix agent), title, status, menu slot. */
function AgentLink({
  agent,
  last,
  nested = true,
}: {
  agent: SidebarAgent
  last: boolean
  /** False under Other: no parent row, so no guide line. */
  nested?: boolean
}) {
  const current = agent.href === `#${useLocation().pathname}`
  const navigate = useNavigate()
  // A Queue agent is gx-driven: no menu, just the spacer that keeps the status column aligned.
  const managed = canManage(agent.session)
  const sel = useSidebarSelect()
  const selecting = sel.state.on
  return (
    <RowFrame current={current} selected={selecting && sel.state.keys.includes(agent.id)}>
      {nested && (
        <Box
          data-testid="nested-guide"
          aria-hidden
          position="absolute"
          left={GUIDE_X}
          top="0"
          bottom={last ? "50%" : "0"}
          w="1.5px"
          bg="border"
        />
      )}
      <Box w={`${CHEVRON_W}px`} flex="none" />
      {selecting && managed && <RowCheck id={agent.id} label={agent.label} />}
      <chakra.a
        href={agent.href}
        {...{ [NAV_ATTR]: `agent:${agent.id}` }}
        aria-current={current ? "page" : undefined}
        {...rowStyle}
        flex="1"
        minW="0"
        px="0"
        minH="36px"
      >
        <IconSlot>
          {agent.fix ? (
            <WrenchIcon status={agent.status} />
          ) : (
            agent.session.flow === "triage" && <InboxIcon status={agent.status} />
          )}
        </IconSlot>
        <Box
          as="span"
          flex="1"
          minW="0"
          overflow="hidden"
          textOverflow="ellipsis"
          whiteSpace="nowrap"
          fontSize="13px"
        >
          {agent.label}
        </Box>
        <Status status={agent.status} />
      </chakra.a>
      {managed ? (
        <Box flex="none">
          <MenuSlot>
            <SessionActions
              session={agent.session}
              onRemoved={() => current && navigate("/agents")}
            />
          </MenuSlot>
        </Box>
      ) : (
        <MenuSpacer />
      )}
    </RowFrame>
  )
}

/**
 * The menu-button slot: the row's Alt+n number replaces the "..." button while Alt is held. Only this leaf watches Alt.
 * The menu stays mounted either way, so Alt never closes it or the dialogs it opened; only the trigger is hidden.
 * The button shows on its own row's hover (the row frame carries the "group" class), so a nested row and its parent never share one.
 */
function MenuSlot({ number, children }: { number?: number; children: ReactNode }) {
  const showNumber = useAltHeld() && number !== undefined
  return (
    <Box position="relative">
      {showNumber && (
        <Box
          as="span"
          data-testid="row-number"
          position="absolute"
          inset="0"
          display="grid"
          placeItems="center"
          fontSize="12px"
          color="text.dim"
        >
          {number}
        </Box>
      )}
      {/* Shown on hover and focus; always on a touch screen, which has no hover. */}
      <Box
        opacity="0"
        visibility={showNumber ? "hidden" : undefined}
        _groupHover={{ opacity: 1 }}
        _focusWithin={{ opacity: 1 }}
        css={{ "@media (hover: none)": { opacity: 1 } }}
      >
        {children}
      </Box>
    </Box>
  )
}

const itemKind = (kind: SidebarRow["kind"]): ItemRef["kind"] =>
  kind === "chat" || kind === "design" ? kind : "epic"

function Row({
  row,
  current,
  active,
  workspaces,
}: {
  row: SidebarRow
  current: boolean
  active: string
  workspaces: Workspace[]
}) {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const nested = row.agents.length > 0
  const item: ItemRef = {
    key: row.key,
    name: row.title,
    kind: itemKind(row.kind),
    workspace: active,
    queued: row.queued,
    held: row.held,
    session: row.session,
  }
  // The removed chat's page is gone; leave it for the agents list, as the agent page does.
  const onRemoved = () => {
    if (current) navigate("/agents")
  }
  const sel = useSidebarSelect()
  const selecting = sel.state.on
  return (
    <li>
      <RowFrame current={current} selected={selecting && sel.state.keys.includes(row.key)}>
        <Box w={`${CHEVRON_W}px`} flex="none">
          {nested && (
            <Chevron
              open={open}
              onToggle={() => setOpen(!open)}
              label={`${open ? "Collapse" : "Expand"} ${row.title}`}
            />
          )}
        </Box>
        {selecting && <RowCheck id={row.key} label={row.title} />}
        <chakra.a
          href={row.href}
          {...{ [NAV_ATTR]: `row:${row.key}`, ...(row.kind === "chat" ? { [CHAT_ATTR]: "" } : {}) }}
          aria-current={current ? "page" : undefined}
          {...rowStyle}
          flex="1"
          minW="0"
          px="0"
        >
          <IconSlot>
            {row.kind === "chat" ? (
              <ChatIcon working={row.status.tone === "running"} />
            ) : (
              <Box position="relative" flex="none">
                <ProgressRing
                  progress={row.ring ?? 0}
                  kind={row.kind}
                  size="sidebar"
                  tone={row.status.tone}
                  live={row.status.tone === "running"}
                />
                <Box
                  data-testid="kind-dot"
                  position="absolute"
                  right="-2px"
                  bottom="-2px"
                  boxSize="8px"
                  borderRadius="full"
                  borderWidth="1.5px"
                  borderColor="sidebar"
                  bg={`kind.${row.kind}.fg`}
                />
              </Box>
            )}
          </IconSlot>
          <Box
            as="span"
            flex="1"
            minW="0"
            fontSize="14px"
            overflow="hidden"
            textOverflow="ellipsis"
            whiteSpace="nowrap"
          >
            {row.title}
          </Box>
          <Status status={row.status} dim={row.held} />
        </chakra.a>
        <Box flex="none">
          <MenuSlot number={row.number}>
            <ItemMenu item={item} workspaces={workspaces} onRemoved={onRemoved} />
          </MenuSlot>
        </Box>
      </RowFrame>
      {nested && open && (
        <Box as="ul" listStyleType="none" m="0" p="0">
          {row.agents.map((a, i) => (
            <li key={a.id}>
              <AgentLink agent={a} last={i === row.agents.length - 1} />
            </li>
          ))}
        </Box>
      )}
    </li>
  )
}

/** A separator for a By project or By status group. */
function GroupHeader({ title }: { title: string }) {
  return (
    <li role="presentation" data-part="group-header">
      <Box
        px="3"
        pt="3"
        pb="1"
        borderTopWidth="1px"
        borderColor="border"
        mt="1"
        fontSize="11px"
        letterSpacing="0.08em"
        textTransform="uppercase"
        color="text.muted"
      >
        {title}
      </Box>
    </li>
  )
}

function Section({
  id,
  group,
  title,
  children,
}: {
  id: string
  group: string
  title: string
  children: ReactNode
}) {
  const sel = useSidebarSelect()
  const open = !!sel.sectionOpen[group]
  const gs = sel.groupState(group)
  return (
    <li>
      <Box display="flex" alignItems="center" pr="3">
        {sel.state.on && (
          <Box pl="3" flex="none" display="flex">
            <SelectBox
              checked={gs === "all"}
              mixed={gs === "some"}
              label={`Select all in ${title}`}
              onToggle={() => sel.toggleGroup(group)}
            />
          </Box>
        )}
        <chakra.button
          type="button"
          {...{ [NAV_ATTR]: `section:${id}` }}
          aria-expanded={open}
          onClick={() => sel.setSectionOpen(group, !open)}
          display="flex"
          alignItems="center"
          gap="1"
          flex="1"
          px="3"
          minH="36px"
          color="text.dim"
          fontSize="12px"
          cursor="pointer"
        >
          <svg
            width="12"
            height="12"
            viewBox="0 0 12 12"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.6"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden
            style={{ transform: open ? "rotate(90deg)" : "none" }}
          >
            <path d="M4.5 2.5 8 6l-3.5 3.5" />
          </svg>
          {title}
        </chakra.button>
      </Box>
      {open && (
        <Box as="ul" listStyleType="none" m="0" p="0">
          {children}
        </Box>
      )}
    </li>
  )
}

// The sidebar body: the active workspace's rows, its collapsed On hold, then the global Other.
export function useSidebarModel(fetcher: FetchEpicList | undefined, active: string) {
  const conn = useConnectionContext()
  const { list } = useEpicList(fetcher, conn)
  const snapshot = useConnectionState(conn).snapshot
  if (!list?.epics) return undefined
  const sort = snapshot?.workspaces?.find((w) => w.name === active)?.sort
  return buildSidebar({
    rows: list.epics,
    sessions: snapshot?.sessions ?? [],
    placements: snapshot?.placements ?? [],
    workspace: active,
    sort,
  })
}

export function SidebarRows({ fetcher, active }: { fetcher?: FetchEpicList; active: string }) {
  const { workspaces } = useWorkspaces(useConnectionContext())
  const pathname = useLocation().pathname
  const model = useSidebarModel(fetcher, active)
  if (!model) return null
  return (
    <>
      {model.rows.map((r, i) => (
        <Fragment key={r.key}>
          {r.group !== undefined && r.group !== model.rows[i - 1]?.group && (
            <GroupHeader title={r.group} />
          )}
          <Row
            row={r}
            current={r.href === `#${pathname}`}
            active={active}
            workspaces={workspaces}
          />
        </Fragment>
      ))}
      {model.held.length > 0 && (
        <Section id="hold" group={GROUP_ON_HOLD} title={`On hold (${model.held.length})`}>
          {model.held.map((r) => (
            <Row
              key={r.key}
              row={r}
              current={r.href === `#${pathname}`}
              active={active}
              workspaces={workspaces}
            />
          ))}
        </Section>
      )}
      {model.other.length > 0 && (
        <Section id="other" group={GROUP_OTHER} title="Other">
          {model.other.map((a, i) => (
            <li key={a.id}>
              <AgentLink agent={a} last={i === model.other.length - 1} nested={false} />
            </li>
          ))}
        </Section>
      )}
    </>
  )
}
