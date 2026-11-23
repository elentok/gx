import { useLayoutEffect } from "react"
import { useLocation, useNavigate } from "react-router"
import { useConnectionContext, useConnectionState } from "../connection"
import { fetchEpicList, useEpicList } from "../epics/list"
import { ChatDraftFolderSource, type DesignDraftRow, type SessionInfo } from "../generated/server"
import { chatDraftHref, draftHref } from "../hrefs"
import { FOCUS_PROMPT_STATE } from "../agents/promptFocus"
import { chatDraftPageActions, draftPageActions } from "../shell/draftPageActions"
import { useShell } from "../shell/ShellContext"
import { showToast } from "../ui/toaster"
import { defaultProject } from "../workspaces/defaultProject"
import { realChatDrafts, type ChatDraftsClient } from "./chatDrafts"
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

/** Project names from the server, for a click that lands before the epic list has loaded. */
const fetchProjects = async (): Promise<string[]> => {
  const list = await fetchEpicList()
  return [
    ...new Set([...(list.projects ?? []), ...(list.epics ?? []).map((e) => e.project)]),
  ].sort()
}

export interface NewDesignClient {
  create: CreateDraft
  getLastProject: () => Promise<string>
  listProjects?: () => Promise<string[]>
}

const realClient: NewDesignClient = {
  create: createDraft,
  getLastProject,
  listProjects: fetchProjects,
}

/** The context's projects, or the server's when the context has none yet. */
async function knownProjects(
  ctx: NewDesignContext,
  listProjects: (() => Promise<string[]>) | undefined,
): Promise<string[]> {
  if (ctx.projects.length > 0 || !listProjects) return ctx.projects
  return listProjects()
}

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
  // A layout effect runs before the browser handles the next key, so a chord pressed right
  // after a navigation sees the new page without writing module state during render.
  useLayoutEffect(() => {
    context = next
  })
  useLayoutEffect(
    () => () => {
      context = null
    },
    [],
  )
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
    const projects = await knownProjects(ctx, client.listProjects)
    const project = defaultProject({ ...ctx, projects, lastProject })
    const id = await client.create({ workspace: ctx.workspace, project })
    ctx.open(draftHref(id).slice(1))
  } catch (e) {
    toastCreateFailed(e)
  }
}

function toastCreateFailed(e: unknown) {
  showToast({
    type: "error",
    title: "Could not create the draft",
    description: e instanceof Error ? e.message : undefined,
  })
}

export interface NewChatDraftClient {
  create: ChatDraftsClient["create"]
  getLastProject: () => Promise<string>
  listProjects?: () => Promise<string[]>
}

const realChatClient: NewChatDraftClient = {
  create: realChatDrafts.create,
  getLastProject,
  listProjects: fetchProjects,
}

/**
 * The New chat draft action. On an empty chat draft page it only focuses the brief; anywhere
 * else it makes an empty draft in the rule-order project and opens it.
 */
export async function startNewChatDraft(client: NewChatDraftClient = realChatClient) {
  const ctx = context
  if (!ctx) return
  const page = chatDraftPageActions()
  if (page?.isEmpty()) {
    page.focusBrief()
    return
  }
  try {
    const lastProject = await client.getLastProject().catch(() => "")
    const projects = await knownProjects(ctx, client.listProjects)
    const project = defaultProject({ ...ctx, projects, lastProject })
    const id = await client.create({
      workspace: ctx.workspace,
      project,
      harness: "",
      folder: ChatDraftFolderSource,
    })
    ctx.open(chatDraftHref(id).slice(1))
  } catch (e) {
    toastCreateFailed(e)
  }
}
