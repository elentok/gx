import { expect, test } from "vitest"
import { formatDays, formatNextRun, formatWhen } from "./format"

test("days collapse to Every day, Weekdays, Weekends or a list", () => {
  expect(formatDays(["mon", "tue", "wed", "thu", "fri", "sat", "sun"])).toBe("Every day")
  expect(formatDays(["mon", "tue", "wed", "thu", "fri"])).toBe("Weekdays")
  expect(formatDays(["sat", "sun"])).toBe("Weekends")
  expect(formatDays(["sun"])).toBe("Sun")
  expect(formatDays(["mon", "wed"])).toBe("Mon, Wed")
})

test("when joins days and times", () => {
  expect(formatWhen(["sun"], ["10:00"])).toBe("Sun · 10:00")
  expect(formatWhen(["mon", "tue", "wed", "thu", "fri", "sat", "sun"], ["07:00", "21:00"])).toBe(
    "Every day · 07:00, 21:00",
  )
})

test("next run reads Today, Tomorrow or the weekday in local time", () => {
  const now = new Date(2026, 10, 12, 12, 0) // Thu
  expect(formatNextRun(new Date(2026, 10, 12, 21, 0).toISOString(), now)).toBe("Today 21:00")
  expect(formatNextRun(new Date(2026, 10, 13, 8, 30).toISOString(), now)).toBe("Tomorrow 08:30")
  expect(formatNextRun(new Date(2026, 10, 16, 8, 30).toISOString(), now)).toBe("Mon 08:30")
})
