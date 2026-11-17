import { describe, expect, it } from "vitest"
import {
  CardKindDesign,
  CardKindImplementation,
  WorkKindDesign,
  WorkKindImplementation,
  type EpicListRow,
} from "../generated/server"
import { ringArc } from "./ring"

const row = (over: Partial<EpicListRow>): EpicListRow =>
  ({ address: "p:x", card_kind: CardKindImplementation, ...over }) as EpicListRow

describe("ringArc", () => {
  it("is Design purple for a working Design", () => {
    const r = row({ card_kind: CardKindDesign, working: true, work_kind: WorkKindDesign })
    expect(ringArc(r)).toBe("kind.design.fg")
  })

  it("is Design purple for the joined card at Branch, keyed on work_kind not card_kind", () => {
    const r = row({ card_kind: CardKindImplementation, working: true, work_kind: WorkKindDesign })
    expect(ringArc(r)).toBe("kind.design.fg")
  })

  it("has no arc for a working Implementation or an idle Design", () => {
    expect(ringArc(row({ working: true, work_kind: WorkKindImplementation }))).toBeUndefined()
    expect(ringArc(row({ card_kind: CardKindDesign, work_kind: WorkKindDesign }))).toBeUndefined()
  })
})
