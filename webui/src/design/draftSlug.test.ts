import { expect, test } from "vitest"
import { slugProblem } from "./draftSlug"

test.each(["", "a", "my-epic", "a1-b2-c3"])("%j is a slug", (s) => {
  expect(slugProblem(s)).toBeUndefined()
})

test.each(["My-Epic", "a b", "-a", "a-", "a--b", "a_b"])(
  "%j is refused with the server's text",
  (s) => {
    expect(slugProblem(s)).toBe(`"${s}" is not a slug (want lowercase words joined by -)`)
  },
)
