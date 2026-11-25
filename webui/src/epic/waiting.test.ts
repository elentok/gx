import { expect, test } from "vitest"
import { EpicListPaused, type EpicBlocker } from "../generated/server"
import { blockerHint, waitingWords } from "./waiting"

// The server puts an invalid stored entry (a ticket ref, no name or state) first.
const stale: EpicBlocker = { invalid: true, ref: "gx:old/03" }
const paused: EpicBlocker = { name: "beta", state: EpicListPaused }

test("a stale entry is named by its stored ref in the waiting words", () => {
  expect(waitingWords([stale, paused])).toBe("Waiting on gx:old/03 and 1 more")
})

test("a valid first entry is named by its name", () => {
  expect(waitingWords([paused])).toBe("Waiting on beta")
})

test("the hint reads the first valid entry behind a stale one", () => {
  expect(blockerHint([stale, paused])).toEqual({
    text: "beta is Paused. This epic stays put until it is merged.",
    stuck: true,
  })
})

test("no valid entry gives no hint", () => {
  expect(blockerHint([stale])).toBeNull()
})
