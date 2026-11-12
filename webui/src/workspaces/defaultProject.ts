import { useEffect, useState } from "react"
import { useLocation } from "react-router"
import { useConnectionContext, useConnectionState } from "../connection"
import { getLastProject } from "../design/drafts"
import type { DraftsClient } from "../design/NewDesignForm"
import { activeProjectFilter, readStoredProjects } from "../epics/storedProjects"
import type { SessionInfo } from "../generated/server"
import { currentAddress } from "../shell/SidebarRows"

/** The project of the open page, or undefined: an epic, Design or ticket address, or a chat's session. */
export function pageProject(
  pathname: string,
  sessions: Pick<SessionInfo, "id" | "project">[],
): string | undefined {
  const address = currentAddress(pathname)
  if (address) return address.includes(":") ? address.slice(0, address.indexOf(":")) : undefined
  const [, kind, id] = pathname.split("/")
  if (kind !== "agents" || !id) return undefined
  return sessions.find((s) => s.id === decodeURIComponent(id))?.project
}

export interface DefaultProjectInput {
  pathname: string
  sessions: Pick<SessionInfo, "id" | "project">[]
  /** The form's project list; a candidate outside it is skipped. */
  projects: string[]
  lastProject?: string
}

/** The project a blank New chat or New design form starts on: page, one-project Epics filter, last used, first. */
export function defaultProject({
  pathname,
  sessions,
  projects,
  lastProject,
}: DefaultProjectInput): string {
  const filter = activeProjectFilter(readStoredProjects(), projects)
  const candidates = [
    pageProject(pathname, sessions),
    filter?.length === 1 ? filter[0] : undefined,
    lastProject,
  ]
  return candidates.find((c): c is string => !!c && projects.includes(c)) ?? projects[0] ?? ""
}

const realLast = { getLastProject }

/** `defaultProject` for the current route and snapshot; the last project arrives once the server answers. */
export function useDefaultProject(
  projects: string[],
  drafts: Pick<DraftsClient, "getLastProject"> = realLast,
): string {
  const { pathname } = useLocation()
  const conn = useConnectionContext()
  const sessions = useConnectionState(conn).snapshot?.sessions ?? []
  const [lastProject, setLastProject] = useState("")
  useEffect(() => {
    let live = true
    drafts.getLastProject().then(
      (p) => live && setLastProject(p),
      () => {},
    )
    return () => {
      live = false
    }
  }, [drafts])
  return defaultProject({ pathname, sessions, projects, lastProject })
}
