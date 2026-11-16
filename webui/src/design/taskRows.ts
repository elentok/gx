import type { SessionTask } from "../generated/server"

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

export function kindWord(kind: string): string {
  return kind === "call" ? "shell" : kind
}

/** Running rows first, then the finished ones; each group keeps the server's launch order. */
export function orderTasks(tasks: SessionTask[]): SessionTask[] {
  return [...tasks.filter((t) => !t.finished), ...tasks.filter((t) => t.finished)]
}

export function taskSummary(tasks: SessionTask[], now: number) {
  const done = tasks.filter((t) => t.finished).length
  const running = tasks.filter((t) => !t.finished)
  const longest = running.length
    ? Math.max(...running.map((t) => now - Date.parse(t.started_at)))
    : undefined
  return {
    done,
    total: tasks.length,
    longest: longest === undefined ? undefined : formatElapsed(longest),
  }
}
