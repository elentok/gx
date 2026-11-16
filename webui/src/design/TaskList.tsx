import { Box, chakra, Text } from "@chakra-ui/react"
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { tokenStyles } from "../files/tokenStyles"
import type { SessionTask } from "../generated/server"
import { Button } from "../ui/button/Button"
import { commandBlock } from "./taskCommand"
import {
  elapsedText,
  kindWord,
  orderTasks,
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

const HOVER_BG = "rgba(255,255,255,.03)"

const COMMAND_LINE_LIMIT = 5

/** Read-only command text, lightly highlighted once the lazy highlighter loads. */
function TaskCommand({ command }: { command: string }) {
  const { text, language } = useMemo(() => commandBlock(command), [command])
  const lines = text.split("\n")
  const [expanded, setExpanded] = useState(false)
  const [clipped, setClipped] = useState(false)
  const [html, setHtml] = useState<{ text: string; language: string; lines: string[] }>()
  const body = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let live = true
    void import("../files/highlight").then(
      (m) => {
        const out = m.highlightLines(text, language)
        if (live && out) setHtml({ text, language, lines: out })
      },
      () => {},
    )
    return () => {
      live = false
    }
  }, [text, language])

  const cut = !expanded && lines.length > COMMAND_LINE_LIMIT
  // jsdom has no layout, so this only fires in a browser: a long single line wrapped past five rows.
  useLayoutEffect(() => {
    const el = body.current
    setClipped(!expanded && !!el && el.scrollHeight > el.clientHeight)
  }, [expanded, text, html])

  const shown = html && html.text === text && html.language === language ? html.lines : undefined
  const visible = cut ? lines.slice(0, COMMAND_LINE_LIMIT) : lines
  return (
    <Box
      data-testid="task-command"
      minW="0"
      mt="6px"
      mb="2px"
      bg="surface"
      border="1px solid"
      borderColor="border"
      borderRadius="8px"
      px="10px"
      py="8px"
      fontFamily="mono"
      fontSize="12px"
      lineHeight="1.5"
      color="text.dim"
      whiteSpace="pre-wrap"
      wordBreak="break-all"
      overflowWrap="anywhere"
      css={tokenStyles}
    >
      <Box
        ref={body}
        maxH={expanded ? undefined : "7.5em"}
        overflow={expanded ? undefined : "hidden"}
      >
        {visible.map((src, i) => (
          <Box
            key={i}
            minH="1.5em"
            {...(shown
              ? { dangerouslySetInnerHTML: { __html: shown[i] ?? "" } }
              : { children: src || undefined })}
          />
        ))}
      </Box>
      {cut && (
        <Box data-testid="task-command-more" color="text.faint" fontStyle="italic">
          {`# …${lines.length - COMMAND_LINE_LIMIT} more lines`}
        </Box>
      )}
      {(cut || clipped) && (
        <Button size="sm" emphasis="ghost" tone="running" onClick={() => setExpanded(true)}>
          {`Show all (${lines.length} lines)`}
        </Button>
      )}
    </Box>
  )
}

function TaskRow({
  task,
  now,
  open,
  onToggle,
}: {
  task: SessionTask
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
    w: "100%",
  } as const
  return (
    <Box
      role="listitem"
      py="0.5"
      color={fin ? "text.dim" : "text.muted"}
      borderRadius="md"
      bg={open ? HOVER_BG : undefined}
      px={open ? "2" : undefined}
      mx={open ? "-2" : undefined}
      my={open ? "0.5" : undefined}
    >
      {expandable ? (
        <chakra.button
          type="button"
          aria-expanded={open}
          onClick={onToggle}
          cursor="pointer"
          textAlign="left"
          color="inherit"
          {...grid}
        >
          {header}
        </chakra.button>
      ) : (
        <Box {...grid}>{header}</Box>
      )}
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
export function TaskList({ tasks }: { tasks?: SessionTask[] }) {
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
      {orderTasks(list).map((t, i) => {
        const key = taskKey(t)
        return (
          <TaskRow
            key={`${key}|${i}`}
            task={t}
            now={now}
            open={openRows.has(key)}
            onToggle={() => toggleRow(key)}
          />
        )
      })}
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
