// @vitest-environment node
import { describe, expect, it } from "vitest"
import { baseCliEnv, cliEnvWith } from "./cliEnv"

describe("baseCliEnv", () => {
  it("drops GX_AGENT_SESSION from the test process env", () => {
    const env = baseCliEnv({ PATH: "/bin", GX_AGENT_SESSION: "missing" }, { HOME: "/tmp/h" })
    expect(env.GX_AGENT_SESSION).toBeUndefined()
    expect(env.PATH).toBe("/bin")
    expect(env.HOME).toBe("/tmp/h")
  })

  it("keeps a variable the fixture itself sets", () => {
    const env = baseCliEnv({}, { GX_AGENT_SESSION: "fixture" })
    expect(env.GX_AGENT_SESSION).toBe("fixture")
  })
})

describe("cliEnvWith", () => {
  it("lets an explicit GX_AGENT_SESSION reach the call", () => {
    const base = baseCliEnv({ GX_AGENT_SESSION: "missing" }, {})
    expect(cliEnvWith(base, { GX_AGENT_SESSION: "abc" }).GX_AGENT_SESSION).toBe("abc")
  })
})
