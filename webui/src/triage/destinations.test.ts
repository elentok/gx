// @vitest-environment node
import { expect, test } from "vitest"
import type { TriageDestination, TriageRow } from "../generated/server"
import { destinationChoices } from "./destinations"

const destinations: TriageDestination[] = [
  { kind: "suggested", value: "sidebar", label: "sidebar" },
  { kind: "map-epic", value: "queue", label: "queue" },
  { kind: "new-map-epic", label: "New map epic" },
]
const epicActions = ["ready", "map"]
const rows: TriageRow[] = [
  { address: "gx:f/1", title: "one", action: "ready" },
  { address: "gx:f/2", title: "two", action: "map" },
  { address: "gx:f/3", title: "three", action: "close" },
  { address: "gx:f/4", title: "four", action: "ready", gone: true },
]

test("ready and map list the suggested epics, the open map epics and a typed new one", () => {
  for (const action of ["ready", "map"])
    expect(destinationChoices(epicActions, action, "gx:f/1", destinations, rows)).toEqual([
      { value: "sidebar", label: "sidebar", kind: "epic" },
      { value: "queue", label: "queue", kind: "epic" },
      { label: "New map epic…", kind: "new" },
    ])
})

test("merge lists the other live ready and map rows", () => {
  expect(destinationChoices(epicActions, "merge", "gx:f/1", destinations, rows)).toEqual([
    { value: "gx:f/2", label: "two", kind: "row" },
  ])
})

test("close and keep have no destination", () => {
  expect(destinationChoices(epicActions, "close", "gx:f/1", destinations, rows)).toEqual([])
  expect(destinationChoices(epicActions, "keep", "gx:f/1", destinations, rows)).toEqual([])
})
