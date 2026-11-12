// @vitest-environment node
import { describe, expect, it } from "vitest"
import { isMoveKey, moveInGrid, type Point } from "./gridNav"

// Three columns, the last row short:   0 1 2
//                                      3 4 2+
const grid: Point[] = [
  { x: 0, y: 0 },
  { x: 100, y: 0 },
  { x: 200, y: 0 },
  { x: 0, y: 50 },
  { x: 100, y: 50 },
]

describe("moveInGrid", () => {
  it.each([
    ["ArrowRight", 0, 1],
    ["l", 1, 2],
    ["ArrowLeft", 1, 0],
    ["h", 4, 3],
    ["ArrowDown", 1, 4],
    ["j", 0, 3],
    ["ArrowUp", 4, 1],
    ["k", 3, 0],
  ])("%s from %i goes to %i", (key, from, to) => {
    expect(moveInGrid(grid, from, key)).toBe(to)
  })

  it("wraps Right from the end of a row to the next row", () => {
    expect(moveInGrid(grid, 2, "ArrowRight")).toBe(3)
  })

  it("stays at the edges", () => {
    expect(moveInGrid(grid, 0, "ArrowLeft")).toBe(0)
    expect(moveInGrid(grid, 4, "ArrowRight")).toBe(4)
    expect(moveInGrid(grid, 0, "ArrowUp")).toBe(0)
    expect(moveInGrid(grid, 4, "ArrowDown")).toBe(4)
  })

  it("Down into a short row picks the closest column", () => {
    expect(moveInGrid(grid, 2, "ArrowDown")).toBe(4)
  })

  it("Down crosses into the next section's grid", () => {
    const sections: Point[] = [
      { x: 0, y: 0 },
      { x: 100, y: 0 },
      { x: 0, y: 300 },
    ]
    expect(moveInGrid(sections, 1, "ArrowDown")).toBe(2)
  })

  it("one column behaves like a list", () => {
    const column: Point[] = [0, 1, 2].map((i) => ({ x: 0, y: i * 40 }))
    expect(moveInGrid(column, 0, "j")).toBe(1)
    expect(moveInGrid(column, 2, "k")).toBe(1)
  })

  it("ignores other keys", () => {
    expect(moveInGrid(grid, 1, "x")).toBe(1)
    expect(isMoveKey("x")).toBe(false)
    expect(isMoveKey("j")).toBe(true)
  })
})
