/**
 * Variables that name the agent session the test process runs in. They must not reach the gx CLI
 * calls, because the fixture server does not know that session. A flow that needs a source session
 * passes the variable per call through `cliEnvWith`.
 */
const AGENT_SESSION_VARS = ["GX_AGENT_SESSION"]

/** The CLI base env: the test process env without agent-session variables, then the fixture's own entries. */
export function baseCliEnv(
  processEnv: NodeJS.ProcessEnv,
  fixtureEnv: Record<string, string>,
): NodeJS.ProcessEnv {
  const env = { ...processEnv }
  for (const name of AGENT_SESSION_VARS) delete env[name]
  return { ...env, ...fixtureEnv }
}

/** `base` plus the per-call `extra` entries, which win. */
export function cliEnvWith(
  base: NodeJS.ProcessEnv,
  extra: Record<string, string>,
): NodeJS.ProcessEnv {
  return { ...base, ...extra }
}
