import { screen } from "@testing-library/react"
import type { ReactElement } from "react"
import { expect, test } from "vitest"
import { renderThemed } from "../ui/render"
import { showDiffUrl } from "../actions/entity"
import { diffHref } from "../hrefs"
import { ViewDiffLink } from "./ViewDiffLink"

const range = { epic: "p:epic", base: "main", tip: "feat" }

test("the link and the Show diff action open the same URL for an epic and a done ticket", () => {
  const epic = showDiffUrl({ kind: "epic", key: "p:epic", name: "e", viewDiff: range })
  const done = showDiffUrl({
    kind: "ticket",
    key: "p:epic/01",
    name: "t",
    status: "done",
    viewDiff: range,
  })

  const hrefOf = (ui: ReactElement) => {
    const { unmount } = renderThemed(ui)
    const href = screen.getByRole("link", { name: "View diff" }).getAttribute("href")
    unmount()
    return href
  }
  expect(hrefOf(<ViewDiffLink {...range} />)).toBe(diffHref(epic!))
  expect(hrefOf(<ViewDiffLink {...range} ticket="p:epic/01" />)).toBe(diffHref(done!))
})
