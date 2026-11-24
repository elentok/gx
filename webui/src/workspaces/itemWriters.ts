import type { HoldRequest, WorkspaceRequest } from "../generated/server"
import { postTargetsTo, refusalOf, type StopSession } from "../design/open"
import { bulkArchive } from "../select/bulkWrites"
import { WriteError, type Refusable } from "../useWrite"

/** The one result of a one-target write, flat as `useWrite` wants it. */
export interface ItemWriteResult extends Refusable {
  sessions?: string[]
  /** An archive dry run's answer: whether gx sees the epic on trunk. */
  on_trunk?: boolean
  /** The name an archive took when it clashed with one already archived. */
  new_address?: string
}

export interface ItemWriters {
  move: (key: string, to: string) => Promise<ItemWriteResult>
  hold: (epic: string, yes?: boolean) => Promise<ItemWriteResult>
  unhold: (epic: string) => Promise<ItemWriteResult>
  /** One epic's archive; `dryRun` only asks what it would stop. */
  archive?: (epic: string, opts: { dryRun?: boolean; yes?: boolean }) => Promise<ItemWriteResult>
  /** A chat's Stop; the agent page's default when absent. */
  stop?: StopSession
}

/** Posts one target and unwraps its result: a refusal is `refused`, as `useWrite` reads it. */
async function postOne(
  route: string,
  body: WorkspaceRequest | HoldRequest,
): Promise<ItemWriteResult> {
  const [r] = await postTargetsTo(`/v1/workspaces/${route}`, body)
  if (!r) throw new WriteError("The server answered with no result.")
  return { ...refusalOf(r), sessions: r.sessions }
}

export const postMove = (key: string, to: string) => postOne("move", { name: to, targets: [key] })
export const postHold = (epic: string, yes?: boolean) =>
  postOne("hold", { targets: [epic], ...(yes ? { yes } : {}) })
export const postUnhold = (epic: string) => postOne("unhold", { targets: [epic] })

export async function postArchive(
  epic: string,
  opts: { dryRun?: boolean; yes?: boolean },
): Promise<ItemWriteResult> {
  const r = (await bulkArchive([epic], opts))[0]
  if (!r) throw new WriteError("The server answered with no result.")
  return {
    ...refusalOf(r),
    sessions: r.sessions,
    on_trunk: r.on_trunk,
    new_address: r.new_address,
  }
}

export const defaultWriters: ItemWriters = {
  move: postMove,
  hold: postHold,
  unhold: postUnhold,
  archive: postArchive,
}
