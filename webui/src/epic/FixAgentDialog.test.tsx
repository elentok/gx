import { cleanup, screen } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test } from "vitest"
import { ConnectionContext, type Connection } from "../connection"
import type { FixAgentRequest } from "../generated/server"
import { focusTopDialog } from "../ui/modal/layers"
import { renderThemed } from "../ui/render"
import { FixAgentDialog } from "./FixAgentDialog"

afterEach(cleanup)

test("focusTopDialog called from outside the dialog moves focus into the FixAgentDialog panel", () => {
  const conn = {
    getState: () => ({}),
    subscribe: () => () => {},
    onEvent: () => () => {},
    refresh: () => {},
  } as unknown as Connection
  const router = createMemoryRouter(
    [
      {
        path: "/",
        element: (
          <>
            <button>outside</button>
            <FixAgentDialog prompt="fix it" request={{} as FixAgentRequest} onClose={() => {}} />
          </>
        ),
      },
    ],
    { initialEntries: ["/"] },
  )
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )

  const outside = screen.getByRole("button", { name: "outside" })
  outside.focus()
  expect(document.activeElement).toBe(outside)

  focusTopDialog(outside)

  const dialog = screen.getByRole("dialog", { name: "Fix it with an agent" })
  expect(dialog.contains(document.activeElement)).toBe(true)
})
