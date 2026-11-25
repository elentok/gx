import { expect, test } from "vitest"
import type { EpicListRow } from "../generated/server"
import { candidates } from "./blockers"

const row = (over: Partial<EpicListRow>): EpicListRow =>
  ({
    address: "gx:x",
    project: "gx",
    name: "x",
    state: "running",
    card_kind: "implementation",
    section: "implementing",
    ...over,
  }) as EpicListRow

test("a joined card stands for its -impl address and a Needs you card shows its pill", () => {
  const list = {
    epics: [
      row({ address: "gx:c", card_kind: "design", addresses: ["gx:c-impl", "gx:c"] }),
      row({ address: "gx:p", section: "needs-you", state_pill: "Paused" }),
      row({ address: "gx:m", card_kind: "design", addresses: ["gx:m"] }),
      row({ address: "gx:f", card_kind: "follow-ups" }),
    ],
    follow_ups: [],
  }
  expect(candidates(list, "gx:me")).toEqual([
    { address: "gx:c-impl", name: "c-impl", state: "Implementing" },
    { address: "gx:p", name: "p", state: "Paused" },
  ])
})

test("no list gives no candidates", () => {
  expect(candidates(null, "gx:me")).toEqual([])
})
