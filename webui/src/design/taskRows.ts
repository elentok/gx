import type { SessionTask } from "../generated/server"
import { sizeText } from "../health/formatBytes"

export type TaskMark = "running" | "done" | "failed" | "killed" | "finished"

export function taskMark(t: SessionTask): TaskMark {
  if (!t.finished) return "running"
  if (t.outcome === "success") return "done"
  if (t.outcome === "failure") return "failed"
  if (t.outcome === "killed") return "killed"
  return "finished"
}

/** "41s", "5m 12s", "1h 05m": the seconds drop out once an hour has passed. */
export function formatElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h > 0) return `${h}h ${String(m).padStart(2, "0")}m`
  if (m > 0) return `${m}m ${String(s).padStart(2, "0")}s`
  return `${s}s`
}

/** A finished task's fixed duration, or undefined when the entry has none. */
export function finishedDuration(t: SessionTask): string | undefined {
  if (t.no_duration || !t.finished_at) return undefined
  return formatElapsed(Date.parse(t.finished_at) - Date.parse(t.started_at))
}

/** The time cell of a row: a running task counts from `now`; a finished one reads its outcome and duration. */
export function elapsedText(t: SessionTask, now: number): string {
  const mark = taskMark(t)
  if (mark === "running") return formatElapsed(now - Date.parse(t.started_at))
  const d = finishedDuration(t)
  const lead =
    mark === "failed"
      ? t.exit_code === undefined
        ? "failed"
        : `exit ${t.exit_code}`
      : mark === "killed"
        ? "killed"
        : undefined
  return [lead, d].filter(Boolean).join(" · ")
}

/** Running rows first, then the finished ones; each group keeps the server's launch order. */
export function orderRows<R extends { item: SessionTask }>(rows: R[]): R[] {
  return [...rows.filter((r) => !r.item.finished), ...rows.filter((r) => r.item.finished)]
}

/** "412", "4.2k", "41k", "1.2M". */
export function tokenText(n: number): string {
  if (n < 1000) return String(n)
  if (n < 10_000) return `${(n / 1000).toFixed(1)}k`
  if (n < 1_000_000) return `${Math.round(n / 1000)}k`
  return `${(n / 1_000_000).toFixed(1)}M`
}

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`

const LAST_LINE_MAX = 80

function lastLineText(line: string): string {
  const one = line.replace(/\s+/g, " ").trim()
  return `last: ${one.length > LAST_LINE_MAX ? `${one.slice(0, LAST_LINE_MAX - 1)}…` : one}`
}

/** What an open row shows under its label: an optional monospace command (Monitor) and a text line. Undefined when the entry has nothing to show. */
export function taskDetails(
  t: SessionTask,
  now: number,
): { command?: string; text: string } | undefined {
  const running = !t.finished
  const parts: string[] = []
  let command: string | undefined
  const exit = t.exit_code === undefined ? undefined : `exit ${t.exit_code}`
  const size =
    t.output_bytes === undefined ? undefined : `${sizeText(t.output_bytes, "decimal")} output`
  if (t.kind === "shell") {
    if (!running && exit) parts.push(exit)
    if (size) parts.push(size)
    if (running && t.last_line) parts.push(lastLineText(t.last_line))
  } else if (t.kind === "call") {
    if (!running && exit) parts.push(exit)
  } else if (t.kind === "monitor") {
    const n = t.event_count ?? 0
    parts.push(plural(n, "event"))
    if (n > 0 && t.last_event_at) {
      parts.push(`last ${formatElapsed(now - Date.parse(t.last_event_at))} ago`)
    }
    if (t.command && t.command !== t.label) command = t.command
  }
  if (parts.length === 0) return undefined
  return { command, text: parts.join(" · ") }
}

export function taskKey(t: SessionTask): string {
  return `${t.started_at}|${t.kind}|${t.label}`
}
