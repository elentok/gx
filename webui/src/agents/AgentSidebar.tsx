import { Box, chakra, Text } from "@chakra-ui/react";
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
import { Sidebar } from "../ui/sidebar/Sidebar";
import type { Tone } from "../ui/tone";
import { groupSessions, isActionable, liveCount } from "./rules";

// Ports of statusIcon in ui/agents/sidebar.go; Working is the animated glyph there.
function statusMark(s: SessionInfo): { glyph: string; tone: Tone; faded?: boolean } {
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

export function AgentSidebar({ sessions, selected }: { sessions: SessionInfo[]; selected?: string }) {
  return (
    <Sidebar
      title="Sessions"
      action={
        <Text as="span" fontSize="xs" color="text.dim">
          {liveCount(sessions)} live
        </Text>
      }
    >
      {groupSessions(sessions).map(({ project, sessions: group }) => (
        <Box key={project} mb="3">
          <Text px="2" fontSize="xs" textTransform="uppercase" letterSpacing="wide" color="text.dim">
            {project}
          </Text>
          {group.map((s) => (
            <Row key={s.id} session={s} current={s.id === selected} />
          ))}
        </Box>
      ))}
    </Sidebar>
  );
}
