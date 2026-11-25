import { Box, Link, Text } from "@chakra-ui/react"
import { useEffect, useState, type ReactNode } from "react"
import { ChatMessages, WorkingRow } from "../design/ChatCore"
import { formatElapsed, tokenText } from "../design/taskRows"
import { FileLinkProvider } from "../files/FileLinkContext"
import {
  SubagentStatusDone,
  SubagentStatusRunning,
  SubagentStatusStopped,
  type SessionChatResponse,
  type SessionInfo,
} from "../generated/server"
import { agentHref } from "../hrefs"

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
  if (!sub) return null
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
  const result = sub.result ? (
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
          <Link href={agentHref(session.id)} color="tone.running.fg" textDecoration="none">
            ← Back to parent chat
          </Link>
        </Box>
      </Box>
      <FileLinkProvider base={{ session: session.id }} recheckKey={chat.messages.at(-1)?.id}>
        <Box display="flex" flexDirection="column" gap="3" flex="1 1 auto" minH="0">
          <ChatMessages
            chat={chat}
            busy={running}
            sessionId={session.id}
            showTools
            head={prompt}
            tail={result}
            working={<WorkingRow current={chat.current} others={chat.others} />}
          />
        </Box>
      </FileLinkProvider>
    </Box>
  )
}
