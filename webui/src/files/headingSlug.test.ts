// @vitest-environment node
import { expect, it } from "vitest"
import { headingSlugs, slugOf } from "./headingSlug"

it("lowercases, drops punctuation and turns spaces into hyphens", () => {
  expect(slugOf("Install & Run: v2.0!")).toBe("install--run-v20")
  expect(slugOf("snake_case - dash")).toBe("snake_case---dash")
  expect(slugOf("Café Über")).toBe("café-über")
})

it("suffixes repeats in document order", () => {
  expect(headingSlugs(["Intro", "Usage", "Intro", "Intro"])).toEqual([
    "intro",
    "usage",
    "intro-1",
    "intro-2",
  ])
})

it("gives an empty heading an empty slug", () => {
  expect(headingSlugs(["", "!!"])).toEqual(["", "-1"])
})
