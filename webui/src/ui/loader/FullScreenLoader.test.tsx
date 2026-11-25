import { expect, it } from "vitest"
import { renderThemed } from "../render"
import { FullScreenLoader } from "./FullScreenLoader"

it("announces its label as a status", () => {
  const { getByRole } = renderThemed(<FullScreenLoader label="Starting chat…" />)
  expect(getByRole("status", { name: "Starting chat…" })).toBeTruthy()
})
