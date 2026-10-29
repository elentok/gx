import { Text } from "@chakra-ui/react";
import { useMemo } from "react";
import type { SessionInfo } from "../generated/server";
import { agentHref } from "../hrefs";
import { RowList, type ListRowData, type ListSection } from "../nav/JumpMenu";
import { StatusMark } from "./AgentSidebar";
import { isActionable } from "./rules";

// Port of ui/agents/jump.go: attention order, and the search fields plus the session ticket.
const SECTIONS = ["Actionable", "Unread", "Other"];

const bucket = (s: SessionInfo) => (isActionable(s) ? 0 : s.unread.unread ? 1 : 2);

function sessionRow(s: SessionInfo, current: boolean): ListRowData {
  const name = s.name || s.id;
  return {
    key: s.id,
    title: name,
    subtitle: [s.project, s.status.state, s.ticket, s.actionable.reason].filter(Boolean).join(" · "),
    href: agentHref(s.id),
    searchText: [name, s.project, s.dir, s.ticket].join(" "),
    current,
    lead: (
      <>
        <StatusMark session={s} />
        {s.design && (
          <Text as="span" role="img" aria-label="Design" fontSize="xs" color="kind.design.fg">
            ◆
          </Text>
        )}
      </>
    ),
    trailing: (
      <>
        {s.unread.unread && (
          <Text as="span" role="img" aria-label="Unread" color="kind.design.fg">
            ●
          </Text>
        )}
        {isActionable(s) && (
          <Text as="span" role="img" aria-label="Actionable" color="tone.waiting.fg">
            ★
          </Text>
        )}
      </>
    ),
  };
}

/**
 * The Agents page title as a button that opens every session, Design sessions included, even
 * while the sidebar hides them.
 */
export function SessionSwitcher({
  name,
  sessions,
  current,
  onNew,
}: {
  name: string;
  sessions: SessionInfo[];
  current?: string;
  onNew: () => void;
}) {
  const sections = useMemo<ListSection[]>(
    () =>
      SECTIONS.map((title, i) => ({
        key: title.toLowerCase(),
        title,
        rows: sessions.filter((s) => bucket(s) === i).map((s) => sessionRow(s, s.id === current)),
      })),
    [sessions, current],
  );
  const footer = useMemo<ListRowData>(
    () => ({ key: "new", title: "+ New session", subtitle: "", href: "#/agents", searchText: "", onSelect: onNew }),
    [onNew],
  );
  return <RowList name={name} label="Switch session" placeholder="Filter sessions" sections={sections} footer={footer} />;
}
