import { Box, Text } from "@chakra-ui/react";
import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation } from "react-router";
import { isActionable } from "../agents/rules";
import { useConnectionContext, useConnectionState } from "../connection";
import { useEpicList, isCurrent, type FetchEpicList } from "../epics/list";
import {
  CardKindDesign,
  CardKindImplementation,
  PlacementChat,
  PlacementEpic,
  PlacementOther,
  SectionFinished,
  SectionNeedsYou,
  SectionOnHold,
  SectionReadyToDesign,
  type EpicListRow,
  type SessionInfo,
  type SessionPlacement,
} from "../generated/server";
import { agentHref } from "../hrefs";
import { epicRow, Menu, type ListRowData, type ListSection } from "../nav/JumpMenu";
import { hasLayer, useIsTopLayer } from "../ui/modal/layers";
import { useWide } from "../useWide";
import { cardStatus, sessionStatus, type SidebarStatus } from "../workspaces/sidebarModel";
import { currentAddress } from "./SidebarRows";

const isCard = (r: EpicListRow) => r.card_kind === CardKindDesign || r.card_kind === CardKindImplementation;

const dot = (
  <Box aria-hidden boxSize="10px" borderRadius="full" bg="text.dim" flex="none" />
);

function Words({ status }: { status: SidebarStatus }) {
  return (
    <Text as="span" fontSize="13px" flex="none" color={status.tone === "muted" ? "text.faint" : `tone.${status.tone}.fg`}>
      {status.words}
    </Text>
  );
}

function agentRow(s: SessionInfo, subtitle = ""): ListRowData {
  const name = s.name || s.ticket || s.id;
  return {
    key: s.id,
    title: name,
    subtitle,
    href: agentHref(s.id),
    searchText: [name, s.project, s.ticket, subtitle].join(" "),
    lead: dot,
    trailing: <Words status={sessionStatus(s)} />,
  };
}

export interface PaletteInput {
  rows: EpicListRow[];
  sessions: SessionInfo[];
  placements: SessionPlacement[];
  workspace: string;
  current: string;
}

/**
 * The jump palette's sections. Empty filter: the active workspace's Designs, epics and chats, then
 * Needs you for the rest. The sections marked `hiddenUntilSearch` hold everything else.
 */
export function buildPalette({ rows, sessions, placements, workspace, current }: PaletteInput): ListSection[] {
  const byId = new Map(sessions.map((s) => [s.id, s]));
  const placed = placements.flatMap((p) => {
    const s = byId.get(p.session);
    return s ? [{ p, s }] : [];
  });
  const cards = rows.filter(isCard);
  const mine = (r: EpicListRow) => r.workspace === workspace;
  const row = (r: EpicListRow, subtitle?: string): ListRowData => {
    return { ...epicRow(r, isCurrent(r, current)), subtitle: subtitle ?? "", trailing: <Words status={cardStatus(r)} /> };
  };
  const elsewhere = (r: EpicListRow) => (mine(r) ? undefined : r.workspace);
  const held = (r: EpicListRow) => r.section === SectionOnHold;
  const staged = (r: EpicListRow) => r.section === SectionReadyToDesign || r.section === SectionFinished;
  const open = cards.filter((r) => !held(r) && !staged(r));

  const chats = placed.filter(({ p }) => p.kind === PlacementChat);
  const chatRow = ({ p, s }: { p: SessionPlacement; s: SessionInfo }) => agentRow(s, p.workspace === workspace ? "" : (p.workspace ?? ""));
  const epicName = (epic: string) => cards.find((r) => r.address === epic || r.addresses?.includes(epic))?.name ?? epic;

  return [
    { key: "designs", title: "Designs", rows: open.filter((r) => mine(r) && r.card_kind === CardKindDesign).map((r) => row(r)) },
    { key: "epics", title: "Epics", rows: open.filter((r) => mine(r) && r.card_kind !== CardKindDesign).map((r) => row(r)) },
    { key: "chats", title: "Chats", rows: chats.filter(({ p }) => p.workspace === workspace).map(chatRow) },
    {
      key: "needs-you",
      title: "Needs you elsewhere",
      rows: [
        ...open.filter((r) => !mine(r) && r.section === SectionNeedsYou).map((r) => row(r, elsewhere(r))),
        ...chats.filter(({ p, s }) => p.workspace !== workspace && isActionable(s)).map(chatRow),
        ...placed.filter(({ p, s }) => p.kind === PlacementOther && isActionable(s)).map(({ s }) => agentRow(s)),
      ],
    },
    {
      key: "elsewhere",
      title: "Other workspaces",
      hiddenUntilSearch: true,
      rows: [
        ...open.filter((r) => !mine(r) && r.section !== SectionNeedsYou).map((r) => row(r, elsewhere(r))),
        ...chats.filter(({ p, s }) => p.workspace !== workspace && !isActionable(s)).map(chatRow),
      ],
    },
    { key: "on-hold", title: "On hold", hiddenUntilSearch: true, rows: cards.filter(held).map((r) => row(r, "On hold")) },
    {
      key: "ready-to-design",
      title: "Ready to design",
      hiddenUntilSearch: true,
      rows: cards.filter((r) => !held(r) && r.section === SectionReadyToDesign).map((r) => row(r, elsewhere(r))),
    },
    {
      key: "finished",
      title: "Finished",
      hiddenUntilSearch: true,
      rows: cards.filter((r) => !held(r) && r.section === SectionFinished).map((r) => row(r, elsewhere(r))),
    },
    {
      key: "agents",
      title: "Agents",
      hiddenUntilSearch: true,
      rows: [
        ...placed.filter(({ p }) => p.kind === PlacementEpic && !!p.epic).map(({ p, s }) => agentRow(s, epicName(p.epic ?? ""))),
        ...placed.filter(({ p, s }) => p.kind === PlacementOther && !isActionable(s)).map(({ s }) => agentRow(s)),
      ],
    },
  ];
}

/** Cmd/Ctrl+K on every page, in text fields too. A popover on tablet and desktop, a full screen on a phone. */
export function JumpPalette({ fetcher, active }: { fetcher?: FetchEpicList; active: string }) {
  const conn = useConnectionContext();
  const { list } = useEpicList(fetcher, conn);
  const snapshot = useConnectionState(conn).snapshot;
  const pathname = useLocation().pathname;
  const phone = !useWide();
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const isTop = useIsTopLayer(open);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        if (open ? isTop() : !hasLayer()) setOpen((o) => !o);
      } else if (e.key === "Escape" && isTop()) setOpen(false);
    };
    const onDown = (e: MouseEvent) => {
      if (!phone && root.current && !root.current.contains(e.target as Node)) setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onDown);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onDown);
    };
  }, [phone, open, isTop]);

  const rows = list?.epics;
  const sessions = snapshot?.sessions;
  const placements = snapshot?.placements;
  const current = currentAddress(pathname);
  const sections = useMemo(
    () => buildPalette({ rows: rows ?? [], sessions: sessions ?? [], placements: placements ?? [], workspace: active, current }),
    [rows, sessions, placements, active, current],
  );
  if (!open) return null;
  return (
    <Box ref={root} display="contents">
      <Menu sections={sections} label="Jump to" placeholder="Jump to…" phone={phone} floating onClose={() => setOpen(false)} />
    </Box>
  );
}
