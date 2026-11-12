const WEEK = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"]
const WEEKDAYS = WEEK.slice(0, 5)
const WEEKENDS = WEEK.slice(5)

const cap = (d: string) => d.charAt(0).toUpperCase() + d.slice(1)
const same = (a: string[], b: string[]) => a.length === b.length && a.every((d, i) => d === b[i])

/** "Every day", "Weekdays", "Weekends" or a three-letter list in week order. */
export function formatDays(days: string[]): string {
  const sorted = WEEK.filter((d) => days.includes(d))
  if (sorted.length === 7) return "Every day"
  if (same(sorted, WEEKDAYS)) return "Weekdays"
  if (same(sorted, WEEKENDS)) return "Weekends"
  return sorted.map(cap).join(", ")
}

/** "Every day · 07:00, 21:00". */
export const formatWhen = (days: string[], times: string[]) =>
  `${formatDays(days)} · ${times.join(", ")}`

const pad = (n: number) => String(n).padStart(2, "0")

/** "Today 21:00", "Tomorrow 08:30" or "Mon 08:30", in the viewer's local time. */
export function formatNextRun(iso: string, now: Date = new Date()): string {
  const at = new Date(iso)
  const dayStart = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
  const diff = Math.round((dayStart(at) - dayStart(now)) / 86_400_000)
  const time = `${pad(at.getHours())}:${pad(at.getMinutes())}`
  if (diff === 0) return `Today ${time}`
  if (diff === 1) return `Tomorrow ${time}`
  return `${cap(WEEK[(at.getDay() + 6) % 7])} ${time}`
}
