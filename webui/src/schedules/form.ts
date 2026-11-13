import type {
  ScheduleCreateRequest,
  ScheduleEditRequest,
  ScheduleShowResult,
} from "../generated/server"
import { TIERS } from "../tiers"

export const DAYS = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"] as const

export const SHORTCUTS: { label: string; days: string[] }[] = [
  { label: "Weekdays", days: ["mon", "tue", "wed", "thu", "fri"] },
  { label: "Every day", days: [...DAYS] },
  { label: "Weekends", days: ["sat", "sun"] },
]

/** One editable Schedule. The late limit stays text so "1." and "" survive typing. */
export interface ScheduleForm {
  name: string
  prompt: string
  days: string[]
  times: string[]
  project: string
  /** Empty means the Default. */
  harness: string
  /** Empty means the worker tier. */
  tier: string
  lateLimit: string
}

export type FormErrors = Partial<Record<"name" | "prompt" | "days" | "times" | "lateLimit", string>>

export const newForm = (): ScheduleForm => ({
  name: "",
  prompt: "",
  days: ["mon", "tue", "wed", "thu", "fri"],
  times: ["07:00"],
  project: "scratch",
  harness: "",
  tier: "",
  lateLimit: "2",
})

export const formOf = (res: ScheduleShowResult): ScheduleForm => ({
  name: res.schedule.name,
  prompt: res.prompt,
  days: [...res.schedule.days],
  times: [...res.schedule.times],
  project: res.schedule.project,
  harness: res.schedule.harness,
  tier: res.schedule.tier,
  lateLimit: String(res.schedule.late_limit_hours),
})

export const harnessOptions = (defaultName: string) => [
  { value: "", label: defaultName ? `Default (${defaultName})` : "Default" },
  { value: "claude", label: "claude" },
  { value: "codex", label: "codex" },
]

export const tierOptions = [
  { value: "", label: "Default (Worker)" },
  ...TIERS.map((t) => ({ value: t.id, label: t.label })),
]

const TIME = /^([01]\d|2[0-3]):[0-5]\d$/

/** The late limit in hours, or undefined when it is not a positive number. */
export function parseLateLimit(text: string): number | undefined {
  const t = text.trim()
  if (!/^\d*\.?\d+$|^\d+\.$/.test(t)) return undefined
  const n = Number(t)
  return n > 0 ? n : undefined
}

export function validate(f: ScheduleForm): FormErrors {
  const e: FormErrors = {}
  if (!f.name.trim()) e.name = "Enter a name."
  if (!f.prompt.trim()) e.prompt = "Enter a prompt."
  if (f.days.length === 0) e.days = "Pick at least one day."
  if (f.times.length === 0) e.times = "Add at least one time."
  else if (f.times.some((t) => !TIME.test(t))) e.times = "Use HH:MM for every time."
  if (parseLateLimit(f.lateLimit) === undefined) e.lateLimit = "Enter a number of hours above 0."
  return e
}

const sameList = (a: string[], b: string[]) =>
  a.length === b.length && a.every((x, i) => x === b[i])

export const isDirty = (a: ScheduleForm, b: ScheduleForm) =>
  a.name !== b.name ||
  a.prompt !== b.prompt ||
  a.project !== b.project ||
  a.harness !== b.harness ||
  a.tier !== b.tier ||
  a.lateLimit !== b.lateLimit ||
  !sameList(a.days, b.days) ||
  !sameList(a.times, b.times)

/** Days in week order, times sorted and without duplicates. */
const days = (f: ScheduleForm) => DAYS.filter((d) => f.days.includes(d))
const times = (f: ScheduleForm) => [...new Set(f.times)].sort()

export const toggleDay = (current: string[], day: string) =>
  current.includes(day) ? current.filter((d) => d !== day) : [...current, day]

export const toCreateRequest = (f: ScheduleForm): ScheduleCreateRequest => ({
  name: f.name.trim(),
  prompt: f.prompt,
  days: days(f),
  times: times(f),
  project: f.project,
  harness: f.harness,
  tier: f.tier,
  late_limit_hours: parseLateLimit(f.lateLimit),
})

export const toEditRequest = (
  project: string,
  slug: string,
  f: ScheduleForm,
): ScheduleEditRequest => ({
  project,
  slug,
  name: f.name.trim(),
  prompt: f.prompt,
  days: days(f),
  times: times(f),
  harness: f.harness,
  tier: f.tier,
  late_limit_hours: parseLateLimit(f.lateLimit),
})

/** Which field a server refusal is about, so its reason shows inline there. */
const REASON_FIELD: Record<string, keyof FormErrors> = {
  "empty-name": "name",
  "empty-prompt": "prompt",
  "no-days": "days",
  "no-times": "times",
  "invalid-time": "times",
  "invalid-late-limit": "lateLimit",
}

export const refusalField = (reason: string | undefined) => REASON_FIELD[reason ?? ""]
