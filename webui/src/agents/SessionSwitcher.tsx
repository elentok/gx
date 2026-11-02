import { useMemo } from "react";
import type { SessionInfo } from "../generated/server";
import { agentHref } from "../hrefs";
import { RowList, type ListRowData, type ListSection } from "../nav/JumpMenu";
import { hasAttentionMarks, isActionable, stateLabel } from "./rules";
import { useGroupVisibility } from "./AgentSidebar";
import { SessionBadges, SessionGlyphs } from "./SessionGlyphs";

// Port of ui/agents/jump.go: attention order, and the search fields plus the session ticket.
const SECTIONS = ["Actionable", "Unread", "Other"];

const bucket = (s: SessionInfo) => (isActionable(s) ? 0 : hasAttentionMarks(s) && s.unread.unread ? 1 : 2);

function sessionRow(s: SessionInfo, current: boolean): ListRowData {
  const name = s.name || s.id;
  return {
    key: s.id,
    title: name,
    subtitle: [s.project, stateLabel(s), s.ticket, s.actionable.reason].filter(Boolean).join(" · "),
    href: agentHref(s.id),
    searchText: [name, s.project, s.dir, s.ticket].join(" "),
    current,
    lead: <SessionGlyphs session={s} />,
    trailing: <SessionBadges session={s} />,
  };
}

/** The Agents page title as a button that opens the agents, following the sidebar's Queue/Design toggles. */
export function SessionSwitcher({
  name,
  sessions,
  current,
  onNew,
  fontSize,
}: {
  name: string;
  sessions: SessionInfo[];
  current?: string;
  onNew: () => void;
  fontSize?: string;
}) {
  const { showQueue, showDesign } = useGroupVisibility();
  const sections = useMemo<ListSection[]>(() => {
    const shown = sessions.filter((s) => s.id === current || !((s.queue && !showQueue) || (s.design && !showDesign)));
    return SECTIONS.map((title, i) => ({
      key: title.toLowerCase(),
      title,
      rows: shown.filter((s) => bucket(s) === i).map((s) => sessionRow(s, s.id === current)),
    }));
  }, [sessions, current, showQueue, showDesign]);
  const footer = useMemo<ListRowData>(
    () => ({ key: "new", title: "+ New session", subtitle: "", href: "#/agents", searchText: "", onSelect: onNew }),
    [onNew],
  );
  return <RowList name={name} label="Switch session" placeholder="Filter sessions" sections={sections} footer={footer} fontSize={fontSize} />;
}
