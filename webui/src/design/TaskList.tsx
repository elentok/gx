import { Box, chakra, Text } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import type { SessionTask } from "../generated/server"
import { subagentHref } from "../hrefs"
import { withKeys } from "../uniqueKeys"
import { TaskCommand } from "./TaskCommandBlock"
import {
  elapsedText,
  kindWord,
  orderRows,
  taskDetails,
  taskKey,
  taskMark,
  taskSummary,
  type TaskMark,
} from "./taskRows"

const GLYPH: Record<Exclude<TaskMark, "running">, string> = {
  done: "✓",
  failed: "✕",
  killed: "✕",
  finished: "–",
}

const GLYPH_COLOR: Record<Exclude<TaskMark, "running">, string> = {
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
      fontSize="xs"
      lineHeight="1"
      color={GLYPH_COLOR[mark]}
    >
      {GLYPH[mark]}
    </Text>
  )
}

function TaskRow({
  task,
  sessionId,
  now,
  open,
  onToggle,
}: {
  task: SessionTask
  sessionId: string
  now: number
  open: boolean
  onToggle: () => void
}) {
  const mark = taskMark(task)
  const fin = mark !== "running"
  const details = taskDetails(task, now)
  const blockCommand = task.kind === "shell" || task.kind === "call" ? task.command : undefined
  const expandable = !!details || !!blockCommand
  const header = (
    <>
      <StatusMark mark={mark} />
      <Text as="span" fontSize="xs" color="text.faint">
        {kindWord(task.kind)}
      </Text>
      <Text
        as="span"
        minW="0"
        whiteSpace="nowrap"
        overflow="hidden"
        textOverflow="ellipsis"
        fontFamily={task.kind === "subagent" ? undefined : "mono"}
        color={fin ? "text.faint" : undefined}
        title={task.label}
      >
        {task.label}
      </Text>
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
  const grid = {
    display: "grid",
    gridTemplateColumns: "14px 62px minmax(0, 1fr) auto",
    columnGap: "2.5",
    alignItems: "baseline",
    flex: "1",
    minW: "0",
  } as const
  const link =
    task.kind === "subagent" && task.call_id ? subagentHref(sessionId, task.call_id) : undefined
  return (
    <Box
      role="listitem"
      py="0.5"
      color={fin ? "text.dim" : "text.muted"}
      borderRadius="md"
      bg={open ? "surface.hover" : undefined}
      px={open ? "2" : undefined}
      mx={open ? "-2" : undefined}
      my={open ? "0.5" : undefined}
    >
      <Box display="flex" columnGap="2.5" alignItems="baseline">
        {expandable ? (
          <chakra.button
            type="button"
            aria-expanded={open}
            onClick={onToggle}
            cursor="pointer"
            borderRadius="md"
            _hover={{ bg: "surface.hover" }}
            textAlign="left"
            color="inherit"
            {...grid}
          >
            {header}
          </chakra.button>
        ) : (
          <Box {...grid}>{header}</Box>
        )}
        {link && (
          <chakra.a
            href={link}
            fontSize="xs"
            color="tone.running.fg"
            whiteSpace="nowrap"
            textDecoration="none"
            _hover={{ textDecoration: "underline" }}
          >
            Open ↗
          </chakra.a>
        )}
      </Box>
      {open && (details || blockCommand) && (
        <Box pl="24px" minW="0">
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

/** The tasks the agent waits on: a summary line for two or more, then the rows. Read-only; the clock is the only thing that moves. */
export function TaskList({ tasks, sessionId }: { tasks?: SessionTask[]; sessionId: string }) {
  const list = tasks ?? []
  const anyRunning = list.some((t) => !t.finished)
  const [now, setNow] = useState(() => Date.now())
  const [open, setOpen] = useState(false)
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
  if (list.length === 0) return null
  const rows = (
    <Box role="list" aria-label="Tasks" display="flex" flexDirection="column" fontSize="sm">
      {orderRows(withKeys(list, taskKey)).map(({ key, item: t }) => (
        <TaskRow
          key={key}
          task={t}
          sessionId={sessionId}
          now={now}
          open={openRows.has(key)}
          onToggle={() => toggleRow(key)}
        />
      ))}
    </Box>
  )
  if (list.length === 1) return rows
  const { done, total, longest } = taskSummary(list, now)
  return (
    <Box display="flex" flexDirection="column" gap="1">
      <chakra.button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        display="flex"
        gap="2"
        alignItems="center"
        fontSize="sm"
        color="text.dim"
        cursor="pointer"
        textAlign="left"
      >
        <Text
          as="span"
          fontSize="2xs"
          color="text.faint"
          transition="transform 0.15s"
          transform={open ? "rotate(90deg)" : undefined}
          aria-hidden
        >
          ▶
        </Text>
        <span>{`${done} of ${total} tasks done`}</span>
        <Box w="48px" h="2px" borderRadius="1px" overflow="hidden" bg="border" aria-hidden>
          <Box h="100%" bg="tone.done.fg" style={{ width: `${(done / total) * 100}%` }} />
        </Box>
        {longest && <Text as="span" color="text.faint">{`· longest ${longest}`}</Text>}
      </chakra.button>
      {open && rows}
    </Box>
  )
}
