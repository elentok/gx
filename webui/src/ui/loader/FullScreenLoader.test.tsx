import { expect, it } from "vitest"
import { renderThemed } from "../render"
import { FullScreenLoader } from "./FullScreenLoader"

it("announces its label as a status", () => {
  const { getByRole } = renderThemed(<FullScreenLoader label="Creating chat…" />)
  expect(getByRole("status", { name: "Creating chat…" })).toBeTruthy()
})
