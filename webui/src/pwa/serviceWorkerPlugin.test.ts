// @vitest-environment node
import { describe, expect, it } from "vitest"
import { workerSource } from "./serviceWorkerPlugin"

describe("built service worker", () => {
  it.each(["push", "notificationclick", "pushsubscriptionchange"])(
    "registers the %s event",
    (name) => {
      const src = workerSource("b1", ["/"], "var gxWorkerCore = {};")
      expect(src).toContain(`self.addEventListener(\n  "${name}",`)
    },
  )
})
