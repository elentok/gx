import type { Locator } from "@playwright/test"
import { expect } from "./fixture"

// The message the server sends for a Huh? answer (server/wire/misc.go).
export const HUH_MESSAGE = "I could not follow this question. Could you explain it another way?"

// One option of a question card: named by its label alone, with the letter and id as spans inside the
// button and the consequence as its description.
export const expectOption = async (
  scope: Locator,
  o: { letter: string; label: string; id: string; consequence: string },
) => {
  const button = scope.getByRole("button", { name: o.label, exact: true })
  await expect(button).toBeVisible()
  await expect(button.locator("span", { hasText: new RegExp(`^${o.letter}$`) })).toBeVisible()
  await expect(button.locator("span", { hasText: new RegExp(`^${o.id}$`) })).toBeVisible()
  await expect(button).toHaveAccessibleDescription(o.consequence)
}
