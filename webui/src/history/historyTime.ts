const MIN = 60_000
const HOUR = 60 * MIN
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]
const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()
const pad = (n: number) => String(n).padStart(2, "0")

/** "2 min ago", "1 h ago", "yesterday", "Mon 14:02" within the week, then "Nov 12". */
export function historyTime(saved: Date, now: Date): string {
  const age = now.getTime() - saved.getTime()
  if (age < MIN) return "just now"
  if (age < HOUR) return `${Math.floor(age / MIN)} min ago`
  const days = Math.round((startOfDay(now) - startOfDay(saved)) / 86_400_000)
  if (days <= 0) return `${Math.floor(age / HOUR)} h ago`
  if (days === 1) return "yesterday"
  if (days < 7) return `${DAYS[saved.getDay()]} ${pad(saved.getHours())}:${pad(saved.getMinutes())}`
  return `${MONTHS[saved.getMonth()]} ${saved.getDate()}`
}
