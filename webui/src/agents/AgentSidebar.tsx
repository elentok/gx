import { Box, chakra, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { SessionWorking, type SessionInfo } from "../generated/server";
import { agentHref } from "../hrefs";
import { useWide } from "../useWide";
import { groupSessions, liveCount } from "./rules";
import { SessionBadges, SessionGlyphs } from "./SessionGlyphs";

function Row({ session: s, current }: { session: SessionInfo; current: boolean }) {
  const navigate = useNavigate();
  const wide = useWide();
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
      py={wide ? "1.5" : "2.5"}
      px="2"
      borderRadius="sm"
      borderBottomWidth={wide ? undefined : "1px"}
      borderBottomColor="border"
      borderLeftWidth="2px"
      borderLeftColor={current ? "tone.running.fg" : "transparent"}
      cursor="pointer"
      bg={current ? "surface" : undefined}
      _hover={{ bg: current ? "surface" : "surface.sunken" }}
    >
      <SessionGlyphs session={s} />
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
      <SessionBadges session={s} tooltip />
    </chakra.a>
  );
}

const DESIGN_KEY = "gx.agents.design";
const QUEUE_KEY = "gx.agents.queue";

function readFlag(key: string): boolean {
  try {
    return localStorage.getItem(key) === "1";
  } catch {
    return false;
  }
}

/** A switch that starts off, remembers its choice under `storageKey`, and counts the rows it hides. */
function useGroupSwitch(storageKey: string) {
  const [on, setOn] = useState(() => readFlag(storageKey));
  const change = (next: boolean) => {
    setOn(next);
    try {
      localStorage.setItem(storageKey, next ? "1" : "0");
    } catch {
      // Storage is a convenience; the choice still holds for this page.
    }
  };
  return [on, change] as const;
}

function GroupSwitch({ label, on, hidden, tone, onChange, bordered }: { label: string; on: boolean; hidden: number; tone: string; onChange: (on: boolean) => void; bordered: boolean }) {
  return (
    <chakra.label display="flex" alignItems="center" gap="2" px="3.5" py="2.5" borderTopWidth={bordered ? "1px" : undefined} borderColor="border" fontSize="sm" color="text.muted" cursor="pointer" position="relative">
      <chakra.input type="checkbox" checked={on} onChange={(e) => onChange(e.target.checked)} position="absolute" opacity={0} w="0" h="0" />
      <Box aria-hidden w="7" h="4" borderRadius="full" bg={on ? tone : "border"} position="relative" flex="none">
        <Box position="absolute" top="2px" left={on ? "14px" : "2px"} w="3" h="3" borderRadius="full" bg={on ? "surface" : "text.muted"} />
      </Box>
      {label}
      {hidden > 0 && <Text as="span" color="text.dim">· {hidden} hidden</Text>}
    </chakra.label>
  );
}

function NewButton({ onNew, full }: { onNew: () => void; full?: boolean }) {
  return (
    <chakra.button
      type="button"
      onClick={() => onNew()}
      w={full ? "100%" : undefined}
      py="1.5"
      px="3"
      borderWidth="1px"
      borderColor="border"
      borderRadius="md"
      fontSize="sm"
      color="text"
      cursor="pointer"
      _hover={{ bg: "surface.sunken" }}
    >
      + New session
    </chakra.button>
  );
}

export const NO_SESSIONS_TITLE = "No sessions yet";
export const NO_SESSIONS_HINT = "Sessions you start here or in the gx Agents tab show up in this list.";

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
  const wide = useWide();
  const [showQueue, setShowQueue] = useGroupSwitch(QUEUE_KEY);
  const [showDesign, setShowDesign] = useGroupSwitch(DESIGN_KEY);
  const isHidden = (s: SessionInfo) => s.id !== selected && ((s.queue && !showQueue) || (s.design && !showDesign));
  const hiddenIn = (group: (s: SessionInfo) => boolean | undefined) => sessions.filter((s) => group(s) && isHidden(s)).length;
  const visible = sessions.filter((s) => !isHidden(s));
  return (
    <Box as="aside" bg="surface.raised" h="100%" minH="0" display="flex" flexDirection="column" borderRightWidth={wide ? "1px" : undefined} borderColor="border">
      <Box display="flex" alignItems="center" gap="2" px="3.5" pt="3" pb="2">
        <Text fontFamily="condensed" fontWeight="600" fontSize="lg" textTransform="uppercase" color="text">
          Sessions
        </Text>
        <Text as="span" flex="1" fontSize="sm" color="text.dim" textAlign={wide ? "right" : undefined}>
          {liveCount(sessions)} live
        </Text>
        {!wide && <NewButton onNew={onNew} />}
      </Box>
      {wide && (
        <Box px="3" pb="3">
          <NewButton onNew={onNew} full />
        </Box>
      )}
      {!wide && sessions.length === 0 && (
        <Box px="3.5" py="4">
          <Text fontWeight="600">{NO_SESSIONS_TITLE}</Text>
          <Text color="text.muted" fontSize="sm">
            {NO_SESSIONS_HINT}
          </Text>
        </Box>
      )}
      <Box flex={wide ? "1" : undefined} minH="0" overflowY="auto" px="1.5">
        {groupSessions(visible).map(({ project, sessions: group }) => (
          <Box key={project} mb="3">
            <Box display="flex" alignItems="center" justifyContent="space-between" px="2">
              <Text fontSize="xs" fontWeight="600" textTransform="uppercase" letterSpacing="widest" color="text.dim">
                {project}
              </Text>
              {wide && (
                <chakra.button
                  type="button"
                  aria-label={`New session in ${project}`}
                  title={`New session in ${project}`}
                  px="1.5"
                  color="text.dim"
                  cursor="pointer"
                  _hover={{ color: "text" }}
                  onClick={() => onNew(project)}
                >
                  +
                </chakra.button>
              )}
            </Box>
            {group.map((s) => (
              <Row key={s.id} session={s} current={s.id === selected} />
            ))}
          </Box>
        ))}
      </Box>
      {sessions.length > 0 && (
        <>
          <GroupSwitch label="Queue agents" on={showQueue} hidden={hiddenIn((s) => s.queue)} tone="tone.running.fg" onChange={setShowQueue} bordered={wide} />
          <GroupSwitch label="Design agents" on={showDesign} hidden={hiddenIn((s) => s.design)} tone="kind.design.fg" onChange={setShowDesign} bordered={false} />
        </>
      )}
    </Box>
  );
}
