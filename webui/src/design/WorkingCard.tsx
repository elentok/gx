import { Box, chakra, Text } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import type { SessionChatResponse, SessionTask } from "../generated/server"
import { subagentHref } from "../hrefs"
import { Card } from "../ui/card/Card"
import { withKeys } from "../uniqueKeys"
import { CallLabel } from "./CallLabel"
import { TaskCommand } from "./TaskCommandBlock"
import { elapsedText, orderRows, taskDetails, taskKey, taskMark, type TaskMark } from "./taskRows"
import { WorkingRow, type WorkingStep } from "./WorkingRow"

type FinishedMark = Exclude<TaskMark, "running">

const GLYPH: Record<FinishedMark, string> = {
  done: "✓",
  failed: "✕",
  killed: "✕",
  finished: "–",
}

const GLYPH_COLOR: Record<FinishedMark, string> = {
  done: "tone.done.fg",
  failed: "tone.paused.fg",
  killed: "tone.paused.fg",
  finished: "text.faint",
}

function StatusMark({ mark }: { mark: TaskMark }) {
  if (mark === "running") {
    return (
      <Box
        role="img"
        aria-label="running"
        w="7px"
        h="7px"
        flexShrink="0"
        borderRadius="full"
        bg="tone.running.fg"
        alignSelf="center"
        animation="taskPulse 1.6s ease-in-out infinite"
      />
    )
  }
  return (
    <Text
      as="span"
      role="img"
      aria-label={mark}
      w="7px"
      flexShrink="0"
      fontSize="xs"
      color={GLYPH_COLOR[mark]}
    >
      {GLYPH[mark]}
    </Text>
  )
}

const ELLIPSIS = {
  minW: "0",
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
} as const

function SubagentName({ task, sessionId }: { task: SessionTask; sessionId: string }) {
  if (!task.call_id) {
    return (
      <Text as="span" {...ELLIPSIS} color="tone.running.fg" title={task.label}>
        {task.label}
      </Text>
    )
  }
  return (
    <chakra.a
      href={subagentHref(sessionId, task.call_id)}
      {...ELLIPSIS}
      color="tone.running.fg"
      textDecoration="none"
      _hover={{ textDecoration: "underline" }}
      title={task.label}
    >
      {`${task.label} ↗`}
    </chakra.a>
  )
}

function TaskRow({
  task,
  sessionId,
  now,
  open,
  onToggle,
  mockPart,
}: {
  task: SessionTask
  sessionId: string
  now: number
  open: boolean
  onToggle: () => void
  mockPart?: string
}) {
  const mark = taskMark(task)
  const running = mark === "running"
  const subagent = task.kind === "subagent"
  const details = subagent ? undefined : taskDetails(task, now)
  const blockCommand = task.kind === "shell" || task.kind === "call" ? task.command : undefined
  const expandable = !!details || !!blockCommand
  const line = (
    <>
      <StatusMark mark={mark} />
      <Box as="span" display="flex" flex="1" minW="0">
        {subagent && running ? (
          <SubagentName task={task} sessionId={sessionId} />
        ) : (
          <Text
            as="span"
            {...ELLIPSIS}
            fontFamily={subagent ? undefined : "mono"}
            title={task.label}
          >
            {task.label}
          </Text>
        )}
      </Box>
      <Text
        as="span"
        fontSize="xs"
        color="text.dim"
        whiteSpace="nowrap"
        css={{ fontVariantNumeric: "tabular-nums" }}
      >
        {elapsedText(task, now)}
      </Text>
    </>
  )
  const flex = { display: "flex", gap: "2", alignItems: "baseline", minW: "0" } as const
  return (
    <Box
      role="listitem"
      data-mock-part={mockPart}
      minW="0"
      color={running ? "text.muted" : "text.faint"}
      borderRadius="md"
      {...(open && { bg: "surface.hover", px: "2", mx: "-2" })}
    >
      {expandable ? (
        <chakra.button
          type="button"
          aria-expanded={open}
          onClick={onToggle}
          {...flex}
          w="100%"
          cursor="pointer"
          borderRadius="md"
          _hover={{ bg: "surface.hover" }}
          textAlign="left"
          color="inherit"
        >
          {line}
        </chakra.button>
      ) : (
        <Box {...flex}>{line}</Box>
      )}
      {subagent && running && task.current_call && (
        <Box
          as="p"
          display="flex"
          pl="15px"
          fontSize="xs"
          color="text.dim"
          fontFamily="mono"
          {...ELLIPSIS}
        >
          <CallLabel label={task.current_call} path={task.current_call_path} />
        </Box>
      )}
      {open && expandable && (
        <Box pl="15px" minW="0">
          {details && (
            <Box
              fontSize="xs"
              color="text.faint"
              overflowWrap="anywhere"
              data-testid="task-details"
            >
              {details.command && (
                <Text as="span" fontFamily="mono" color="text.dim">
                  {details.command}
                  <br />
                </Text>
              )}
              {details.text}
            </Box>
          )}
          {blockCommand && <TaskCommand command={blockCommand} />}
        </Box>
      )}
    </Box>
  )
}

/**
 * The block under the chat: the Working row as its header while `working` is set, then the tasks the agent
 * waits on. The open rows and the clock live above `Card`, whose live and still variants are different
 * element trees: state below it would reset when the turn ends.
 */
export function WorkingCard({
  working,
  tasks = [],
  current,
  others,
  sessionId,
}: Pick<SessionChatResponse, "tasks" | "current" | "others"> & {
  working?: WorkingStep
  sessionId: string
}) {
  const anyRunning = tasks.some((t) => !t.finished)
  const [now, setNow] = useState(() => Date.now())
  const [openRows, setOpenRows] = useState<ReadonlySet<string>>(new Set())
  const toggleRow = (key: string) =>
    setOpenRows((s) => {
      const next = new Set(s)
      if (!next.delete(key)) next.add(key)
      return next
    })
  useEffect(() => {
    if (!anyRunning) return
    setNow(Date.now())
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [anyRunning])
  if (!working && tasks.length === 0) return null
  let subagents = 0
  let finished = 0
  const mockPart = (t: SessionTask) => {
    if (t.finished) return `done-row-${++finished}`
    return t.kind === "subagent" ? `subagent-row-${++subagents}` : undefined
  }
  return (
    <Card tone="muted" live={!!working} maxHeight="40dvh" data-mock-part="card">
      {working && <WorkingRow {...working} current={current} others={others} inCard />}
      {tasks.length > 0 && (
        <Box
          display="flex"
          flexDirection="column"
          minH="0"
          {...(working && { mt: "2", pt: "2", borderTopWidth: "1px", borderColor: "border" })}
        >
          {/* The padding and equal negative margin keep an open row's bleed and the focus rings unclipped. */}
          <Box
            role="list"
            aria-label="Tasks"
            display="flex"
            flexDirection="column"
            gap="2"
            fontSize="sm"
            minH="0"
            overflowY="auto"
            p="2"
            m="-2"
          >
            {orderRows(withKeys(tasks, taskKey)).map(({ key, item: t }) => (
              <TaskRow
                key={key}
                task={t}
                sessionId={sessionId}
                now={now}
                open={openRows.has(key)}
                onToggle={() => toggleRow(key)}
                mockPart={mockPart(t)}
              />
            ))}
          </Box>
        </Box>
      )}
    </Card>
  )
}
