import { Box, chakra } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { useLocation } from "react-router";
import { useConnectionContext, useConnectionState } from "../connection";
import { useEpicList, type FetchEpicList } from "../epics/list";
import { buildSidebar, type SidebarAgent, type SidebarRow, type SidebarStatus } from "../workspaces/sidebarModel";
import type { Workspace } from "../generated/server";
import { ItemMenu, type ItemRef } from "../workspaces/itemActions";
import { useWorkspaces } from "../workspaces/useActiveWorkspace";
import { ProgressRing } from "../ui/progress-ring/ProgressRing";

const CHAT_PINK = "#ec6fa8";

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
  _hover: { bg: "surface.raised" },
  _focusVisible: { outline: "2px solid", outlineColor: "border.focus", outlineOffset: "-2px" },
} as const;

function Status({ status, dim }: { status: SidebarStatus; dim?: boolean }) {
  const color = dim || status.tone === "muted" ? "text.faint" : `tone.${status.tone}.fg`;
  return (
    <Box as="span" fontSize="12px" color={color} whiteSpace="nowrap" overflow="hidden" textOverflow="ellipsis">
      {status.words}
    </Box>
  );
}

function ChatIcon() {
  return (
    <svg width="26" height="26" viewBox="0 0 26 26" fill="none" stroke={CHAT_PINK} strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-label="Chat" role="img">
      <path d="M5 7.5a2.5 2.5 0 0 1 2.5-2.5h11A2.5 2.5 0 0 1 21 7.5v7a2.5 2.5 0 0 1-2.5 2.5H12l-4 3.5V17h-.5A2.5 2.5 0 0 1 5 14.5v-7z" />
    </svg>
  );
}

function Chevron({ open, onToggle, label }: { open: boolean; onToggle: () => void; label: string }) {
  return (
    <chakra.button
      type="button"
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
    <chakra.a href={agent.href} {...rowStyle} pl="10" minH="36px">
      <Box as="span" flex="1" minW="0" overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap" fontSize="13px">
        {agent.label}
      </Box>
      <Status status={agent.status} />
    </chakra.a>
  );
}

function Row({ row, current, active, workspaces }: { row: SidebarRow; current: boolean; active: string; workspaces: Workspace[] }) {
  const [open, setOpen] = useState(!!row.expanded);
  const nested = row.agents.length > 0;
  const item: ItemRef = { key: row.key, name: row.title, kind: row.kind === "chat" ? "chat" : row.kind === "design" ? "design" : "epic", workspace: active, queued: row.queued, held: row.held };
  return (
    <li className="group">
      <Box display="flex" alignItems="center" pl="1" bg={current ? "surface.raised" : undefined} borderLeftWidth="3px" borderLeftColor={current ? "tone.running.fg" : "transparent"} borderRadius="6px">
        <Box w="20px" flex="none">
          {nested && <Chevron open={open} onToggle={() => setOpen(!open)} label={`${open ? "Collapse" : "Expand"} ${row.title}`} />}
        </Box>
        <chakra.a href={row.href} aria-current={current ? "page" : undefined} {...rowStyle} flex="1" minW="0" pl="2">
          {row.kind === "chat" ? (
            <ChatIcon />
          ) : (
            <Box position="relative" flex="none">
              <ProgressRing progress={row.ring ?? 0} kind={row.kind} size="sidebar" tone={row.status.tone} live={row.status.tone === "running"} />
              <Box data-testid="kind-dot" position="absolute" right="-2px" bottom="-2px" boxSize="8px" borderRadius="full" borderWidth="1.5px" borderColor="surface" bg={`kind.${row.kind}.fg`} />
            </Box>
          )}
          <Box as="span" flex="1" minW="0" fontSize="14px" overflow="hidden" textOverflow="ellipsis" whiteSpace="nowrap">
            {row.title}
          </Box>
          <Status status={row.status} dim={row.held} />
        </chakra.a>
        {/* Shown on hover and focus; always on a touch screen, which has no hover. */}
        <Box flex="none" opacity="0" _groupHover={{ opacity: 1 }} _focusWithin={{ opacity: 1 }} css={{ "@media (hover: none)": { opacity: 1 } }}>
          <ItemMenu item={item} workspaces={workspaces} />
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

function Section({ title, children }: { title: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <li>
      <chakra.button type="button" aria-expanded={open} onClick={() => setOpen(!open)} display="flex" alignItems="center" gap="1" w="100%" px="3" minH="36px" color="text.dim" fontSize="12px" cursor="pointer">
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
export function SidebarRows({ fetcher, active }: { fetcher?: FetchEpicList; active: string }) {
  const conn = useConnectionContext();
  const { list } = useEpicList(fetcher, conn);
  const snapshot = useConnectionState(conn).snapshot;
  const { workspaces } = useWorkspaces(conn);
  const pathname = useLocation().pathname;
  if (!list?.epics) return null;
  const current = currentAddress(pathname);
  const model = buildSidebar({ rows: list.epics, sessions: snapshot?.sessions ?? [], placements: snapshot?.placements ?? [], workspace: active, current });
  return (
    <>
      {model.rows.map((r) => (
        <Row key={r.key} row={r} current={r.href === `#${pathname}`} active={active} workspaces={workspaces} />
      ))}
      {model.held.length > 0 && (
        <Section title={`On hold (${model.held.length})`}>
          {model.held.map((r) => (
            <Row key={r.key} row={r} current={r.href === `#${pathname}`} active={active} workspaces={workspaces} />
          ))}
        </Section>
      )}
      {model.other.length > 0 && (
        <Section title="Other">
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
