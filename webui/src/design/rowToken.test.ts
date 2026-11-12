import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import { describe, expect, it } from "vitest"
import type { SessionInfo } from "../generated/server"
import { rowToken } from "./open"

// The same fixture server/sessions/web_sessions_test.go checks against SessionRowToken.
const cases: { name: string; id: string; run_started_at: string; state: string; token: string }[] =
  JSON.parse(
    readFileSync(resolve(process.cwd(), "../server/wire/testdata/row_tokens.json"), "utf8"),
  )

describe("rowToken", () => {
  it.each(cases)("matches the server's SessionRowToken: $name", (c) => {
    const s = {
      id: c.id,
      run_started_at: c.run_started_at,
      status: { state: c.state },
    } as SessionInfo
    expect(rowToken(s)).toBe(c.token)
  })
})
