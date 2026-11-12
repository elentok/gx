import { expect, test } from "vitest"
import {
  harnessOptions,
  isDirty,
  newForm,
  parseLateLimit,
  refusalField,
  SHORTCUTS,
  toCreateRequest,
  toEditRequest,
  toggleDay,
  validate,
} from "./form"

const filled = () => ({ ...newForm(), name: "Morning", prompt: "Summarise" })

test("the three shortcuts set the days", () => {
  const by = Object.fromEntries(SHORTCUTS.map((s) => [s.label, s.days]))
  expect(by["Weekdays"]).toEqual(["mon", "tue", "wed", "thu", "fri"])
  expect(by["Every day"]).toHaveLength(7)
  expect(by["Weekends"]).toEqual(["sat", "sun"])
})

test("a new Schedule starts on weekdays at 07:00 in scratch with 2 hours", () => {
  const f = newForm()
  expect(f.days).toEqual(["mon", "tue", "wed", "thu", "fri"])
  expect(f.times).toEqual(["07:00"])
  expect(f.project).toBe("scratch")
  expect(f.lateLimit).toBe("2")
})

test("the first harness option is the Default and means unset", () => {
  expect(harnessOptions("claude")[0]).toEqual({ value: "", label: "Default (claude)" })
  expect(harnessOptions("").map((o) => o.value)).toEqual(["", "claude", "codex"])
})

test("the late limit takes a decimal and refuses 0, negatives and text", () => {
  expect(parseLateLimit("1.5")).toBe(1.5)
  expect(parseLateLimit("0.25")).toBe(0.25)
  expect(parseLateLimit("0")).toBeUndefined()
  expect(parseLateLimit("-1")).toBeUndefined()
  expect(parseLateLimit("")).toBeUndefined()
  expect(parseLateLimit("abc")).toBeUndefined()
})

test("each missing field gives its own error", () => {
  expect(validate(filled())).toEqual({})
  expect(validate({ ...filled(), name: " " }).name).toBeTruthy()
  expect(validate({ ...filled(), prompt: "" }).prompt).toBeTruthy()
  expect(validate({ ...filled(), days: [] }).days).toBeTruthy()
  expect(validate({ ...filled(), times: [] }).times).toBeTruthy()
  expect(validate({ ...filled(), times: ["25:00"] }).times).toBeTruthy()
  expect(validate({ ...filled(), lateLimit: "0" }).lateLimit).toBeTruthy()
})

test("dirty compares every field", () => {
  const a = filled()
  expect(isDirty(a, { ...a })).toBe(false)
  expect(isDirty(a, { ...a, name: "x" })).toBe(true)
  expect(isDirty(a, { ...a, times: ["07:00", "08:00"] })).toBe(true)
  expect(isDirty(a, { ...a, harness: "codex" })).toBe(true)
  expect(isDirty(a, { ...a, lateLimit: "3" })).toBe(true)
})

test("toggleDay adds and removes", () => {
  expect(toggleDay(["mon"], "tue")).toEqual(["mon", "tue"])
  expect(toggleDay(["mon", "tue"], "mon")).toEqual(["tue"])
})

test("the requests put days in week order and collapse duplicate times", () => {
  const f = {
    ...filled(),
    days: ["fri", "mon"],
    times: ["21:00", "07:00", "21:00"],
    lateLimit: "1.5",
  }
  expect(toCreateRequest(f)).toEqual({
    name: "Morning",
    prompt: "Summarise",
    days: ["mon", "fri"],
    times: ["07:00", "21:00"],
    project: "scratch",
    harness: "",
    late_limit_hours: 1.5,
  })
  const edit = toEditRequest("gx", "morning", f)
  expect(edit).toMatchObject({ project: "gx", slug: "morning", days: ["mon", "fri"] })
  expect(edit).not.toHaveProperty("new_project")
})

test("a refusal maps to its field", () => {
  expect(refusalField("empty-name")).toBe("name")
  expect(refusalField("invalid-late-limit")).toBe("lateLimit")
  expect(refusalField("slug-exists")).toBeUndefined()
})
