import { useEffect } from "react"
import { useLocation, useNavigate } from "react-router"
import { useConnectionContext, useConnectionState } from "../connection"
import { useEpicList } from "../epics/list"
import type { DesignDraftRow, SessionInfo } from "../generated/server"
import { draftHref } from "../hrefs"
import { FOCUS_PROMPT_STATE } from "../agents/promptFocus"
import { draftPageActions } from "../shell/draftPageActions"
import { useShell } from "../shell/ShellContext"
import { showToast } from "../ui/toaster"
import { defaultProject } from "../workspaces/defaultProject"
import { createDraft, getLastProject, type CreateDraft } from "./drafts"
import { moveLegacyDraft } from "./legacyDraft"

/** What the action needs from the running app; the host keeps it current. */
export interface NewDesignContext {
  pathname: string
  sessions: Pick<SessionInfo, "id" | "project">[]
  drafts: Pick<DesignDraftRow, "id" | "project">[]
  projects: string[]
  workspace?: string
  /** Opens the draft page; the phone sidebar closes after it. */
  open: (href: string) => void
}

export interface NewDesignClient {
  create: CreateDraft
  getLastProject: () => Promise<string>
}

const realClient: NewDesignClient = { create: createDraft, getLastProject }

let context: NewDesignContext | null = null

/** Keeps `startNewDesign` pointed at the current page and snapshot. Mounted once, by the host. */
export function useNewDesignContext() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const shell = useShell()
  const conn = useConnectionContext()
  const snapshot = useConnectionState(conn).snapshot
  const { list } = useEpicList()
  const projects = [
    ...new Set([...(list?.projects ?? []), ...(list?.epics ?? []).map((e) => e.project)]),
  ].sort()
  const closeSliding = shell?.closeSliding
  const next: NewDesignContext = {
    pathname,
    sessions: snapshot?.sessions ?? [],
    drafts: snapshot?.design_drafts ?? [],
    projects,
    workspace: shell?.workspace,
    open: (href) => {
      navigate(href, { state: FOCUS_PROMPT_STATE })
      closeSliding?.()
    },
  }
  // Written during render so a chord pressed right after a navigation sees the new page.
  setNewDesignContext(next)
  useEffect(() => () => dropNewDesignContext(next), [next])
}

export function setNewDesignContext(ctx: NewDesignContext) {
  context = ctx
}

export function dropNewDesignContext(ctx: NewDesignContext) {
  if (context === ctx) context = null
}

/**
 * The one New design action. On an empty draft's page it only focuses that page's Prompt;
 * anywhere else it creates a draft in the rule-order project and opens it.
 */
export async function startNewDesign(client: NewDesignClient = realClient) {
  const ctx = context
  if (!ctx) return
  const page = draftPageActions()
  if (page?.isEmpty()) {
    page.focusPrompt()
    return
  }
  try {
    await moveLegacyDraft(client.create, ctx.workspace)
    const lastProject = await client.getLastProject().catch(() => "")
    const project = defaultProject({ ...ctx, lastProject })
    const id = await client.create({ workspace: ctx.workspace, project })
    ctx.open(draftHref(id).slice(1))
  } catch (e) {
    showToast({
      type: "error",
      title: "Could not create the draft",
      description: e instanceof Error ? e.message : undefined,
    })
  }
}
