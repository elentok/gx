import { Box, chakra, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useNavigate } from "react-router";
import {
  SessionBlocked,
  SessionExitCrashed,
  SessionExited,
  SessionIdle,
  SessionWorking,
  type SessionInfo,
} from "../generated/server";
import { agentHref } from "../hrefs";
import { Button } from "../ui/button/Button";
import { Sidebar } from "../ui/sidebar/Sidebar";
import type { Tone } from "../ui/tone";
import { groupSessions, isActionable, liveCount } from "./rules";

// Ports of statusIcon in ui/agents/sidebar.go; Working is the animated glyph there.
export function statusMark(s: SessionInfo): { glyph: string; tone: Tone; faded?: boolean } {
  switch (s.status.state) {
    case SessionWorking:
      return { glyph: "●", tone: "running" };
    case SessionIdle:
      return { glyph: "○", tone: "done" };
    case SessionBlocked:
      return { glyph: "?", tone: "waiting" };
    case SessionExited:
      return s.exit === SessionExitCrashed ? { glyph: "✕", tone: "deadlocked" } : { glyph: "○", tone: "muted", faded: true };
    default:
      return { glyph: "◌", tone: "paused" };
  }
}

const actionableReason = (s: SessionInfo) => s.actionable.reason || s.status.message || "Blocked";

function Row({ session: s, current }: { session: SessionInfo; current: boolean }) {
  const navigate = useNavigate();
  const { glyph, tone, faded } = statusMark(s);
  return (
    <chakra.a
      href={agentHref(s.id)}
      onClick={(e: React.MouseEvent) => {
        e.preventDefault();
        navigate(`/agents/${s.id}`);
      }}
      aria-current={current ? "page" : undefined}
      display="flex"
      alignItems="center"
      gap="2"
      py="1.5"
      px="2"
      borderRadius="sm"
      cursor="pointer"
      bg={current ? "surface" : undefined}
      _hover={{ bg: current ? "surface" : "surface.sunken" }}
    >
      <Text as="span" role="img" aria-label={s.exit === SessionExitCrashed ? "crashed" : s.status.state} color={`tone.${tone}.fg`} opacity={faded ? 0.5 : 1}>
        {glyph}
      </Text>
      {s.design && (
        <Text as="span" role="img" aria-label="Design" color="tone.paused.fg">
          ✎
        </Text>
      )}
      <Text as="span" flex="1" truncate fontSize="sm" color={current ? "text" : "text.muted"} fontWeight={s.unread.unread ? "700" : "400"}>
        {s.name || s.id}
      </Text>
      {s.status.state === SessionWorking && s.status.background && (
        <Text as="span" fontSize="xs" color="tone.running.fg">
          +bg
        </Text>
      )}
      {s.ticket && (
        <Text as="span" fontSize="xs" color="text.dim">
          {s.ticket}
        </Text>
      )}
      {s.unread.unread && (
        <Text as="span" role="img" aria-label="Unread" color="tone.running.fg">
          ●
        </Text>
      )}
      {isActionable(s) && (
        <Text as="span" role="img" aria-label="Actionable" title={actionableReason(s)} color="tone.waiting.fg">
          ★
        </Text>
      )}
    </chakra.a>
  );
}

const DESIGN_KEY = "gx.agents.design";

function readShowDesign(): boolean {
  try {
    return localStorage.getItem(DESIGN_KEY) === "1";
  } catch {
    return false;
  }
}

/** `onNew` gets the project of the group whose "+" was pressed, or nothing from the top button. */
export function AgentSidebar({
  sessions,
  selected,
  onNew,
}: {
  sessions: SessionInfo[];
  selected?: string;
  onNew: (project?: string) => void;
}) {
  const [showDesign, setShowDesign] = useState(readShowDesign);
  const change = (on: boolean) => {
    setShowDesign(on);
    try {
      localStorage.setItem(DESIGN_KEY, on ? "1" : "0");
    } catch {
      // Storage is a convenience; the choice still holds for this page.
    }
  };
  const hidden = showDesign ? 0 : sessions.filter((s) => s.design && s.id !== selected).length;
  const visible = showDesign ? sessions : sessions.filter((s) => !s.design || s.id === selected);
  return (
    <Sidebar
      title="Sessions"
      action={
        <Text as="span" fontSize="xs" color="text.dim">
          {liveCount(sessions)} live
        </Text>
      }
    >
      <Box px="2" mb="3">
        <Button type="button" emphasis="secondary" size="sm" onClick={() => onNew()}>
          + New session
        </Button>
      </Box>
      {groupSessions(visible).map(({ project, sessions: group }) => (
        <Box key={project} mb="3">
          <Box display="flex" alignItems="center" justifyContent="space-between" px="2">
            <Text fontSize="xs" textTransform="uppercase" letterSpacing="wide" color="text.dim">
              {project}
            </Text>
            <chakra.button
              type="button"
              aria-label={`New session in ${project}`}
              title={`New session in ${project}`}
              px="1"
              color="text.dim"
              cursor="pointer"
              _hover={{ color: "text" }}
              onClick={() => onNew(project)}
            >
              +
            </chakra.button>
          </Box>
          {group.map((s) => (
            <Row key={s.id} session={s} current={s.id === selected} />
          ))}
        </Box>
      ))}
      <chakra.label display="flex" alignItems="center" gap="2" px="2" fontSize="xs" color="text.dim" cursor="pointer">
        <input type="checkbox" checked={showDesign} onChange={(e) => change(e.target.checked)} />
        Design sessions
        {hidden > 0 && <span>· {hidden} hidden</span>}
      </chakra.label>
    </Sidebar>
  );
}
