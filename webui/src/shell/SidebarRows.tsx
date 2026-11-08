import { Box, chakra } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { buildSidebar, type SidebarAgent, type SidebarRow, type SidebarStatus } from "../workspaces/sidebarModel";
import type { SessionInfo, Workspace } from "../generated/server";
import { ItemMenu, type ItemRef } from "../workspaces/itemActions";
import { useWorkspaces } from "../workspaces/useActiveWorkspace";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";
import { CHAT_ATTR, NAV_ATTR } from "./SidebarKeys";
import { useAltHeld } from "./useShortcuts";

/** The epic address of the open page, or "". */
export function currentAddress(pathname: string): string {
  const [, kind, ...rest] = pathname.split("/");
  const id = decodeURIComponent(rest.join("/"));
  if (kind === "d" || kind === "e") return id;
  if (kind === "t") return id.replace(/\/[^/]*$/, "");
  return "";
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
  _focusVisible: { outline: "2px solid", outlineColor: "border.focus", outlineOffset: "-2px" },
} as const;

const statusColor = (status: SidebarStatus, dim?: boolean) => (dim || status.tone === "muted" ? "text.faint" : `tone.${status.tone}.fg`);

function Status({ status, dim }: { status: SidebarStatus; dim?: boolean }) {
  return (
    <Box as="span" fontSize="12px" color={statusColor(status, dim)} whiteSpace="nowrap" overflow="hidden" textOverflow="ellipsis">
      {status.words}
    </Box>
  );
}

function ChatIcon() {
  return (
    <svg width="26" height="26" viewBox="0 0 26 26" fill="none" stroke="var(--chakra-colors-chat)" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-label="Chat" role="img">
      <path d="M5 7.5a2.5 2.5 0 0 1 2.5-2.5h11A2.5 2.5 0 0 1 21 7.5v7a2.5 2.5 0 0 1-2.5 2.5H12l-4 3.5V17h-.5A2.5 2.5 0 0 1 5 14.5v-7z" />
    </svg>
  );
}

function WrenchIcon({ status }: { status: SidebarStatus }) {
  return (
    <Box as="span" data-testid="fix-wrench" flex="none" display="grid" color={statusColor(status)}>
      <svg width="18" height="18" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-label="Fix agent" role="img">
        <path d="M13.5 3a4 4 0 0 0-3.6 5.5L3.5 15a1.8 1.8 0 0 0 2.5 2.5l6.5-6.4A4 4 0 0 0 17 6.5l-2.5 2.5-2-.5-.5-2z" />
      </svg>
    </Box>
  );
}

function Chevron({ open, onToggle, label }: { open: boolean; onToggle: () => void; label: string }) {
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
      <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden style={{ transform: open ? "rotate(90deg)" : "none" }}>
        <path d="M4.5 2.5 8 6l-3.5 3.5" />
      </svg>
    </chakra.button>
  );
}

function AgentLink({ agent }: { agent: SidebarAgent }) {
  return (
    <chakra.a href={agent.href} {...{ [NAV_ATTR]: `agent:${agent.id}` }} {...rowStyle} pl="10" minH="36px">
      {agent.fix && <WrenchIcon status={agent.status} />}
      <Box as="span" flex="1" minW="0" overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap" fontSize="13px">
        {agent.label}
      </Box>
      <Status status={agent.status} />
    </chakra.a>
  );
}

/** The menu-button slot: the row's Alt+n number while Alt is held, the "..." menu otherwise. Only this leaf watches Alt. */
function MenuSlot({ number, item, workspaces, onRemoved }: { number?: number; item: ItemRef; workspaces: Workspace[]; onRemoved: () => void }) {
  const altHeld = useAltHeld();
  if (altHeld && number !== undefined) {
    return (
      <Box as="span" data-testid="row-number" display="grid" placeItems="center" boxSize="28px" fontSize="12px" color="text.dim">
        {number}
      </Box>
    );
  }
  return (
    // Shown on hover and focus; always on a touch screen, which has no hover.
    <Box opacity="0" _groupHover={{ opacity: 1 }} _focusWithin={{ opacity: 1 }} css={{ "@media (hover: none)": { opacity: 1 } }}>
      <ItemMenu item={item} workspaces={workspaces} onRemoved={onRemoved} />
    </Box>
  );
}

function Row({ row, current, active, workspaces, sessions }: { row: SidebarRow; current: boolean; active: string; workspaces: Workspace[]; sessions: SessionInfo[] }) {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  const nested = row.agents.length > 0;
  const item: ItemRef = {
    key: row.key,
    name: row.title,
    kind: row.kind === "chat" ? "chat" : row.kind === "design" ? "design" : "epic",
    workspace: active,
    queued: row.queued,
    held: row.held,
    session: row.kind === "chat" ? sessions.find((s) => s.id === row.key) : undefined,
  };
  // The removed chat's page is gone; leave it for the agents list, as the agent page does.
  const onRemoved = () => {
    if (current) navigate("/agents");
  };
  return (
    <li className="group">
      <Box display="flex" alignItems="center" pl="1" bg={current ? "sidebar.current" : undefined} borderLeftWidth="3px" borderLeftColor={current ? "tone.running.fg" : "transparent"} borderRadius="6px">
        <Box w="20px" flex="none">
          {nested && <Chevron open={open} onToggle={() => setOpen(!open)} label={`${open ? "Collapse" : "Expand"} ${row.title}`} />}
        </Box>
        <chakra.a href={row.href} {...{ [NAV_ATTR]: `row:${row.key}`, ...(row.kind === "chat" ? { [CHAT_ATTR]: "" } : {}) }} aria-current={current ? "page" : undefined} {...rowStyle} flex="1" minW="0" pl="2">
          {row.kind === "chat" ? (
            <ChatIcon />
          ) : (
            <Box position="relative" flex="none">
              <ProgressRing progress={row.ring ?? 0} kind={row.kind} size="sidebar" tone={row.status.tone} live={row.status.tone === "running"} />
              <Box data-testid="kind-dot" position="absolute" right="-2px" bottom="-2px" boxSize="8px" borderRadius="full" borderWidth="1.5px" borderColor="sidebar" bg={`kind.${row.kind}.fg`} />
            </Box>
          )}
          <Box as="span" flex="1" minW="0" fontSize="14px" overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap">
            {row.title}
          </Box>
          <Status status={row.status} dim={row.held} />
        </chakra.a>
        <Box flex="none">
          <MenuSlot number={row.number} item={item} workspaces={workspaces} onRemoved={onRemoved} />
        </Box>
      </Box>
      {nested && open && (
        <Box as="ul" listStyleType="none" m="0" p="0">
          {row.agents.map((a) => (
            <li key={a.id}>
              <AgentLink agent={a} />
            </li>
          ))}
        </Box>
      )}
    </li>
  );
}

function Section({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <li>
      <chakra.button type="button" {...{ [NAV_ATTR]: `section:${id}` }} aria-expanded={open} onClick={() => setOpen(!open)} display="flex" alignItems="center" gap="1" w="100%" px="3" minH="36px" color="text.dim" fontSize="12px" cursor="pointer">
        <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden style={{ transform: open ? "rotate(90deg)" : "none" }}>
          <path d="M4.5 2.5 8 6l-3.5 3.5" />
        </svg>
        {title}
      </chakra.button>
      {open && (
        <Box as="ul" listStyleType="none" m="0" p="0">
          {children}
        </Box>
      )}
    </li>
  );
}

// The sidebar body: the active workspace's rows, its collapsed On hold, then the global Other.
export function useSidebarModel(fetcher: FetchEpicList | undefined, active: string) {
  const conn = useConnectionContext();
  const { list } = useEpicList(fetcher, conn);
  const snapshot = useConnectionState(conn).snapshot;
  const pathname = useLocation().pathname;
  if (!list?.epics) return undefined;
  const sort = snapshot?.workspaces?.find((w) => w.name === active)?.sort;
  return buildSidebar({ rows: list.epics, sessions: snapshot?.sessions ?? [], placements: snapshot?.placements ?? [], workspace: active, sort });
}

export function SidebarRows({ fetcher, active }: { fetcher?: FetchEpicList; active: string }) {
  const { workspaces } = useWorkspaces(useConnectionContext());
  const pathname = useLocation().pathname;
  const model = useSidebarModel(fetcher, active);
  const sessions = useConnectionState(useConnectionContext()).snapshot?.sessions ?? [];
  if (!model) return null;
  return (
    <>
      {model.rows.map((r) => (
        <Row key={r.key} row={r} current={r.href === `#${pathname}`} active={active} workspaces={workspaces} sessions={sessions} />
      ))}
      {model.held.length > 0 && (
        <Section id="hold" title={`On hold (${model.held.length})`}>
          {model.held.map((r) => (
            <Row key={r.key} row={r} current={r.href === `#${pathname}`} active={active} workspaces={workspaces} sessions={sessions} />
          ))}
        </Section>
      )}
      {model.other.length > 0 && (
        <Section id="other" title="Other">
          {model.other.map((a) => (
            <li key={a.id}>
              <AgentLink agent={a} />
            </li>
          ))}
        </Section>
      )}
    </>
  );
}
