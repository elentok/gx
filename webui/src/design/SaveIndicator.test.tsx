import { ChakraProvider } from "@chakra-ui/react"
import { act, cleanup, render, screen } from "@testing-library/react"
import type { ReactElement } from "react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { system } from "../ui/theme"
import { SaveIndicator } from "./SaveIndicator"

type Status = "none" | "saving" | "saved" | "unsaved"

beforeEach(() => vi.useFakeTimers())
afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

const wrap = (status: Status): ReactElement => (
  <ChakraProvider value={system}>
    <SaveIndicator status={status} />
  </ChakraProvider>
)
const mount = (status: Status) => {
  const r = render(wrap(status))
  return (next: Status) => r.rerender(wrap(next))
}
const wait = (ms: number) => act(async () => void vi.advanceTimersByTime(ms))
const slot = () => screen.getByRole("status")

test("the slot is a polite live region with a reserved size and no text when empty", () => {
  mount("saved")
  expect(slot().getAttribute("aria-live")).toBe("polite")
  expect(slot().textContent).toBe("")
  expect(getComputedStyle(slot()).width).toBe("20px")
  expect(getComputedStyle(slot()).height).toBe("20px")
})

test("a save that ends before 300 ms shows nothing and no check", async () => {
  const set = mount("saved")
  set("saving")
  await wait(299)
  set("saved")
  await wait(2000)
  expect(slot().textContent).toBe("")
})

test("a save past 300 ms shows the icon, holds it 600 ms, then a check for 1 s and an empty slot", async () => {
  const set = mount("saved")
  set("saving")
  await wait(299)
  expect(slot().textContent).toBe("")
  await wait(1)
  expect(slot().textContent).toBe("Saving the draft")
  await wait(100)
  set("saved")
  await wait(499)
  expect(slot().textContent).toBe("Saving the draft")
  await wait(1)
  expect(slot().textContent).toBe("Draft saved")
  await wait(999)
  expect(slot().textContent).toBe("Draft saved")
  await wait(1)
  expect(slot().textContent).toBe("")
})

test("a seen save that ends after the minimum shows the check at once", async () => {
  const set = mount("saved")
  set("saving")
  await wait(1000)
  set("saved")
  await wait(0)
  expect(slot().textContent).toBe("Draft saved")
})

test("a failed save shows the steady error icon at once, no delay", async () => {
  const set = mount("saved")
  set("saving")
  await wait(100)
  set("unsaved")
  expect(slot().textContent).toBe("Not saved, retrying")
  expect(slot().getAttribute("data-save-shown")).toBe("unsaved")
  await wait(20000)
  expect(slot().textContent).toBe("Not saved, retrying")
})

test("a retry that works shows the check for 1 s", async () => {
  const set = mount("unsaved")
  expect(slot().textContent).toBe("Not saved, retrying")
  set("saved")
  expect(slot().textContent).toBe("Draft saved")
  await wait(1000)
  expect(slot().textContent).toBe("")
})

test("none empties the slot and clears a pending timer", async () => {
  const set = mount("saved")
  set("saving")
  await wait(200)
  set("none")
  await wait(2000)
  expect(slot().textContent).toBe("")
})

test("saved with no saving before it shows nothing", async () => {
  const set = mount("none")
  set("saved")
  await wait(0)
  expect(slot().textContent).toBe("")
})

test("under reduced motion the fade is switched off in CSS", async () => {
  const set = mount("saved")
  set("saving")
  await wait(300)
  const css = Array.from(document.querySelectorAll("style"))
    .map(
      (s) =>
        s.textContent +
        Array.from((s as HTMLStyleElement).sheet?.cssRules ?? [])
          .map((r) => r.cssText)
          .join("\n"),
    )
    .join("\n")
  expect(css).toMatch(/prefers-reduced-motion:\s*reduce/)
})
