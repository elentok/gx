import { Box, Link, Text } from "@chakra-ui/react"
import { Fragment, useContext, useEffect, useState, type ReactNode } from "react"
import { ChatMessages } from "../design/ChatCore"
import { WorkingRow } from "../design/WorkingRow"
import { formatElapsed, tokenText } from "../design/taskRows"
import { FileLinkProvider } from "../files/FileLinkContext"
import {
  SubagentStatusDone,
  SubagentStatusNotAvailable,
  SubagentStatusRunning,
  SubagentStatusStopped,
  SubagentUnavailableCompressed,
  SubagentUnavailableStarting,
  type SessionChatResponse,
  type SessionInfo,
} from "../generated/server"
import { agentHref, backHref, subagentHref } from "../hrefs"
import { SubagentOriginContext } from "../subagentOrigin"

type PillTone = "muted" | "running" | "done"

function Pill({ tone, dot, children }: { tone: PillTone; dot?: boolean; children: ReactNode }) {
  return (
    <Box
      as="span"
      data-tone={tone}
      display="inline-flex"
      alignItems="center"
      gap="1.5"
      px="2"
      lineHeight="20px"
      borderRadius="full"
      fontSize="xs"
      whiteSpace="nowrap"
      color={`tone.${tone}.fg`}
      bg={`tone.${tone}.bg`}
    >
      {dot && <Box as="span" w="7px" h="7px" borderRadius="full" bg={`tone.${tone}.fg`} />}
      {children}
    </Box>
  )
}

const STATUS_LABEL: Record<string, string> = {
  [SubagentStatusRunning]: "Running",
  [SubagentStatusDone]: "Done",
  [SubagentStatusStopped]: "Stopped",
  [SubagentStatusNotAvailable]: "Not available",
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Text as="span">
      {label}{" "}
      <Text as="b" fontWeight="500" color="text.muted">
        {value}
      </Text>
    </Text>
  )
}

/** Time since the start, ticking each second only while `running`; a finished one is fixed by its end time. */
function useNow(running: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!running) return
    setNow(Date.now())
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [running])
  return now
}

function GoneNote({ children }: { children?: ReactNode }) {
  return (
    <Box
      data-part="missing-note"
      borderWidth="1px"
      borderStyle="dashed"
      borderColor="border"
      borderRadius="lg"
      p="3"
      color="text.dim"
      fontSize="sm"
    >
      <Text>Transcript no longer available</Text>
      {children}
    </Box>
  )
}

/** To the parent subagent when there is one, else to the page the first link was followed from. */
function BackLink({ sessionId, parentCallId }: { sessionId: string; parentCallId?: string }) {
  const origin = useContext(SubagentOriginContext)
  const href = parentCallId
    ? subagentHref(sessionId, parentCallId, origin)
    : backHref(sessionId, origin)
  return (
    <Link href={href} color="tone.running.fg" textDecoration="none">
      ← Back to parent chat
    </Link>
  )
}

/** The parent transcript is gone or archived: nothing of the subagent is known, so no header at all. */
function ParentGone({ sessionId, archived }: { sessionId: string; archived: boolean }) {
  return (
    <Box display="flex" flexDirection="column" gap="3" px="4" py="3">
      <GoneNote>
        {archived && (
          <Text>
            The parent session is archived. Decode it from the{" "}
            <Link href={agentHref(sessionId)} color="inherit">
              chat page
            </Link>
            .
          </Text>
        )}
      </GoneNote>
      <BackLink sessionId={sessionId} />
    </Box>
  )
}

/** A 404 of the subagent read: the server's message, never reworded, and the way back. */
export function SubagentMissing({ sessionId, message }: { sessionId: string; message: string }) {
  return (
    <Box display="flex" flexDirection="column" gap="3" px="4" py="3">
      <Text color="text.dim">{message}</Text>
      <BackLink sessionId={sessionId} />
    </Box>
  )
}

/** One subagent's page: header, the prompt, its messages and tool calls, and the result. Read-only. */
export function SubagentPage({
  session,
  chat,
}: {
  session: SessionInfo
  chat: SessionChatResponse
}) {
  const sub = chat.subagent
  const running = sub?.status === SubagentStatusRunning
  const now = useNow(running)
  const origin = useContext(SubagentOriginContext)
  if (!sub) return null
  // An archived parent shares the reason "compressed" with a compressed child; the server's
  // archived flag tells them apart. A plain gone parent has no title, a child's file holds one.
  if (
    sub.unavailable &&
    sub.unavailable !== SubagentUnavailableStarting &&
    (chat.archived || (!sub.title && !sub.type))
  ) {
    return <ParentGone sessionId={session.id} archived={!!chat.archived} />
  }
  const unavailable = sub.unavailable
  const title = sub.title ?? ""
  const status = STATUS_LABEL[sub.status]
  const started = sub.started_at ? Date.parse(sub.started_at) : undefined
  const ended = sub.ended_at ? Date.parse(sub.ended_at) : undefined
  let time: string | undefined
  if (started !== undefined) {
    if (running) time = formatElapsed(now - started)
    else if (ended !== undefined) time = formatElapsed(ended - started)
  }
  const done = sub.status === SubagentStatusDone

  const prompt = (
    <Box
      alignSelf="stretch"
      borderWidth="1px"
      borderColor="border"
      bg="surface.raised"
      borderRadius="lg"
      px="3"
      py="2"
      whiteSpace="pre-wrap"
      data-part="subagent-prompt"
    >
      <Text fontSize="xs" color="text.faint" mb="0.5">
        Prompt from the parent agent
      </Text>
      {sub.prompt ? sub.prompt : <Text color="text.dim">Prompt not readable</Text>}
    </Box>
  )
  const result =
    sub.result && (done || sub.status === SubagentStatusStopped) ? (
      done ? (
        <Box
          data-part="subagent-result"
          borderWidth="1px"
          borderColor="tone.done.fg"
          borderRadius="lg"
          p="3"
          bg="tone.done.bg"
          whiteSpace="pre-wrap"
        >
          <Text fontSize="xs" color="tone.done.fg">
            Result handed back to the parent
          </Text>
          {sub.result}
        </Box>
      ) : (
        <Box
          data-part="subagent-result"
          borderWidth="1px"
          borderColor="border"
          borderRadius="lg"
          p="3"
          color="text.muted"
          whiteSpace="pre-wrap"
        >
          <Text fontSize="xs" color="text.faint">
            Last message of the stopped subagent
          </Text>
          {sub.result}
        </Box>
      )
    ) : null

  return (
    <Box display="flex" flexDirection="column" flex="1 1 auto" minH="0" gap="3">
      <Text fontSize="sm" color="text.dim" px="4" pt="2">
        <Link href={agentHref(session.id)} color="inherit">
          {session.name || session.id}
        </Link>
        {sub.chain?.map((level) => (
          <Fragment key={level.call_id}>
            {" › "}
            <Link href={subagentHref(session.id, level.call_id, origin)} color="inherit">
              {level.title || level.call_id}
            </Link>
          </Fragment>
        ))}
        {" › "}
        {title}
      </Text>
      <Box
        data-part="subagent-header"
        display="flex"
        flexDirection="column"
        gap="1.5"
        px="4"
        py="3"
        borderBottomWidth="1px"
        borderColor="hairline"
      >
        <Box display="flex" alignItems="center" gap="2" flexWrap="wrap">
          <Text as="h1" fontFamily="condensed" fontWeight="600" fontSize="lg">
            {title}
          </Text>
          {sub.type && <Pill tone="muted">{sub.type}</Pill>}
          {status && (
            <Pill tone={running ? "running" : done ? "done" : "muted"} dot={running}>
              {status}
            </Pill>
          )}
        </Box>
        <Box display="flex" gap="3.5" flexWrap="wrap" fontSize="xs" color="text.dim">
          {sub.model && <Stat label="Model" value={sub.model} />}
          {time && <Stat label="Time" value={time} />}
          {sub.tokens !== undefined && <Stat label="Tokens" value={tokenText(sub.tokens)} />}
          {sub.tool_calls !== undefined && (
            <Stat label="Tool calls" value={String(sub.tool_calls)} />
          )}
          <BackLink sessionId={session.id} parentCallId={sub.parent_call_id} />
        </Box>
      </Box>
      <FileLinkProvider base={{ session: session.id }} recheckKey={chat.messages.at(-1)?.id}>
        <Box display="flex" flexDirection="column" gap="3" flex="1 1 auto" minH="0">
          {unavailable ? (
            <Box display="flex" flexDirection="column" gap="3" px="4">
              {prompt}
              {unavailable === SubagentUnavailableStarting ? (
                <Text color="text.dim">Starting…</Text>
              ) : (
                <GoneNote>
                  {unavailable === SubagentUnavailableCompressed && (
                    <Text>
                      The transcript file is compressed. Reading compressed children is not
                      supported yet.
                    </Text>
                  )}
                </GoneNote>
              )}
            </Box>
          ) : (
            <ChatMessages
              chat={chat}
              sessionId={session.id}
              showTools
              linkAllCalls
              topAnchor
              head={prompt}
              tail={result}
              bottom={running && <WorkingRow current={chat.current} others={chat.others} />}
            />
          )}
        </Box>
      </FileLinkProvider>
    </Box>
  )
}
