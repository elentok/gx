import {
  ActionStartOver,
  ReasonNeedsYes,
  type SessionInfo,
  type SessionPromptRequest,
  type Workspace,
} from "../generated/server"
import { forgetLast } from "../agents/lastAgent"
import { postRemoveSession, removeSession, type RemoveSession } from "../agents/sessionWrites"
import { copyText } from "../clipboard"
import { isDraftEmpty } from "./draftEmpty"
import { realChatDrafts, type ChatDraftsClient } from "../design/chatDrafts"
import {
  deleteDraft as postDeleteDraft,
  readDraft as fetchDraft,
  type DeleteDraft,
  type ReadDraft,
} from "../design/drafts"
import {
  postDeleteDesign,
  postOpenDesign,
  postStopSession,
  refusalOf,
  rowToken,
  stateToken,
  type DeleteDesign,
  type OpenFlow,
} from "../design/open"
import { postPrompt, type PostPrompt } from "../design/prompt"
import { switchRunning } from "../design/modelSwitch"
import { promptable } from "../design/view"
import {
  postEpicCreatePR,
  postEpicMarkMerged,
  postEpicMerge,
  postQueueAdd,
  type PostEpicWrite,
} from "../epics/queue"
import { diffHref, epicHref } from "../hrefs"
import { chatDraftPageActions, draftPageActions } from "../shell/draftPageActions"
import { openModelPicker } from "../shell/modelPickerOpen"
import { openRename, type DraftIdentity } from "../shell/renameOpen"
import { bulkUnarchive } from "../select/bulkWrites"
import { ReasonStale } from "../ticket/answer"
import {
  ActionCancel,
  CANCEL_NOTICE,
  DISCARD_NOTICE,
  postRecover,
  trackRecover,
  useRecoverPending,
  type PostRecover,
} from "../ticket/recover"
import { showToast, toaster } from "../ui/toaster"
import { useWrite, type Refusable } from "../useWrite"
import {
  defaultWriters,
  postArchive,
  type ItemWriteResult,
  type ItemWriters,
} from "../workspaces/itemWriters"
import {
  openConfirm,
  type ConfirmBody,
  type ConfirmRequest,
  type SurfaceContext,
} from "./confirmRequest"
import { isDraftEntity, isPlainChat, showDiffUrl, type Entity } from "./entity"
import { actionById, menuActions, type ActionDecl, type Group } from "./registry"

export interface ActionWriters extends ItemWriters {
  unarchive: (address: string) => Promise<ItemWriteResult>
  remove: RemoveSession
  deleteDraft: DeleteDraft
  readDraft: ReadDraft
  deleteChatDraft: ChatDraftsClient["remove"]
  readChatDraft: ChatDraftsClient["read"]
  markMerged: PostEpicWrite
  queue: PostEpicWrite
  merge: PostEpicWrite
  createPr: PostEpicWrite
  openDesign: OpenFlow
  deleteDesign: DeleteDesign
  recover: PostRecover
  copy: (text: string) => Promise<boolean>
  openUrl: (url: string) => void
  navigate: (href: string) => void
  post: PostPrompt
}

export const unarchiveOne = async (address: string): Promise<ItemWriteResult> => {
  const [r] = await bulkUnarchive([address])
  return r ? { ...refusalOf(r), new_address: r.new_address } : {}
}

const defaults: ActionWriters = {
  ...defaultWriters,
  stop: postStopSession,
  unarchive: unarchiveOne,
  remove: postRemoveSession,
  deleteDraft: postDeleteDraft,
  readDraft: fetchDraft,
  deleteChatDraft: realChatDrafts.remove,
  readChatDraft: realChatDrafts.read,
  markMerged: postEpicMarkMerged,
  queue: postQueueAdd,
  merge: postEpicMerge,
  createPr: postEpicCreatePR,
  openDesign: postOpenDesign,
  deleteDesign: postDeleteDesign,
  recover: postRecover,
  copy: (text) => copyText(text, null),
  openUrl: (url) => void window.open(url, "_blank", "noopener"),
  navigate: (href) => {
    window.location.hash = href.replace(/^#/, "")
  },
  post: postPrompt,
}

/** What a surface hands the hook besides the entity. */
export interface ActionContext {
  /** A function resolves at the moment the action runs, for a menu whose anchor is known only then. */
  surface: SurfaceContext | (() => SurfaceContext)
  workspaces?: Workspace[]
  /** Fakes for tests; the defaults call the endpoints each action calls today. */
  writers?: Partial<ActionWriters>
  /** Runs after a chat's Remove lands. */
  onRemoved?: () => void
  /** Runs after Start over or Cancel run landed. */
  onRecovered?: () => void
  /** A draft's identity, for Rename. */
  draft?: DraftIdentity
  /** The state token the page or list holds: the epic's list state, the ticket's page state. */
  state?: string
  /** The server's tooltip text for Start over. */
  tooltip?: string
  /** Promote to Design and the other composer writes need what the chat page knows. */
  promote?: { lastMessage?: string; connected: boolean; onStale?: () => void }
}

export interface ReadyItem {
  label: string
  current?: boolean
  run: () => void
}

/** One action for one entity, ready to show: what to call it, whether it can run, and the call. */
export interface ReadyAction {
  id: string
  label: string
  group?: Group
  order?: number
  danger: boolean
  chord?: string
  disabled: boolean
  /** Why a disabled action cannot run. */
  reason?: string
  /** Move to workspace lists every workspace here. */
  items?: ReadyItem[]
  /** The button the pick came from. Absent uses the context's surface; null forces a modal (a chord). */
  run: (anchor?: HTMLElement | null) => void
}

const ARCHIVE_BUSY_TOAST = "item-archive-busy"

/** The actions that post a recovery write; they wait for any recovery write in flight. */
const RECOVERY_IDS = ["start-over", "cancel-run"]

/** The name a confirm and a toast use for the entity. */
const nameOf = (e: Entity) => e.session?.name || e.name || e.key

export function useActions(entity: Entity | undefined, ctx: ActionContext) {
  const { run, pending } = useWrite()
  const recoverPending = useRecoverPending()
  const w: ActionWriters = { ...defaults, ...ctx.writers }

  const surfaceOf = () => (typeof ctx.surface === "function" ? ctx.surface() : ctx.surface)

  // An explicit anchor (the button or menu the pick came from) wins over the context's surface.
  const askAt =
    (anchor?: HTMLElement | null) =>
    (
      decl: ActionDecl,
      e: Entity,
      req: Partial<ConfirmRequest> &
        Pick<ConfirmRequest, "confirmLabel" | "title" | "onConfirm"> & { body: ConfirmBody },
    ) => {
      const surface = surfaceOf()
      openConfirm({
        actionId: decl.id,
        entityName: nameOf(e),
        group: decl.group,
        danger: decl.danger,
        dismissLabel: "Cancel",
        surface:
          anchor === undefined
            ? surface
            : { ...(anchor ? { mode: "anchor", anchor } : { mode: "modal" }), page: surface.page },
        ...req,
      })
    }
  type Ask = ReturnType<typeof askAt>

  function runHold(decl: ActionDecl, e: Entity, ask: Ask) {
    void run(async () => {
      const res = await w.hold(e.key)
      // A refusal that names live sessions is a question, not an error: ask, then re-post with yes.
      if (res.refused && res.reason === ReasonNeedsYes) {
        const sessions = res.sessions ?? []
        ask(decl, e, {
          confirmLabel: "Hold",
          title: `Hold ${e.name}`,
          body: {
            lines: [`Holding ${e.name} ends its live sessions: ${sessions.join(", ")}.`],
            prepared: res,
          },
          onConfirm: () => void run(() => w.hold(e.key, true)),
        })
        return { ...res, refused: false }
      }
      return res
    })
  }

  function runArchive(decl: ActionDecl, e: Entity, ask: Ask) {
    const archive = w.archive ?? postArchive
    const intro = `Archive ${e.name}? Live agents stop and the epic leaves the sidebar. Unarchive brings it back.`
    // The confirm opens at once with a spinner; the dry run fills it in, or a refusal closes it.
    ask(decl, e, {
      confirmLabel: "Archive",
      title: `Archive ${e.name}`,
      body: { lines: [intro] },
      prepare: async () => {
        const dry = await archive(e.key, { dryRun: true })
        if (dry.refused) return { ...dry, refused: true as const }
        const sessions = dry.sessions ?? []
        return {
          lines: [
            intro,
            ...(sessions.length > 0 ? [`Live sessions will stop: ${sessions.join(", ")}.`] : []),
          ],
          warnings:
            dry.on_trunk === false
              ? [
                  `${e.name} is not merged to trunk. Archiving keeps its branch, but the work is not on trunk.`,
                ]
              : [],
          prepared: dry,
        }
      },
      onConfirm: () => {
        showToast({
          type: "info",
          title: `Archiving ${e.name}…`,
          id: ARCHIVE_BUSY_TOAST,
          persistent: true,
        })
        void run(
          () => archive(e.key, { yes: true }).finally(() => toaster.remove(ARCHIVE_BUSY_TOAST)),
          {
            success: (res) => ({
              type: "success",
              title: `Archived ${res.new_address ? res.new_address.split(":").pop() : e.name}`,
            }),
          },
        )
      },
    })
  }

  function runRemove(decl: ActionDecl, e: Entity, ask: Ask) {
    if (isDraftEntity(e)) {
      const chatDraft = e.kind === "chat-draft"
      const id = ctx.draft?.id ?? e.key.replace(/^(chat-)?draft\//, "")
      const del = () =>
        void run(
          async () => {
            await (chatDraft ? w.deleteChatDraft(id) : w.deleteDraft(id))
            return {}
          },
          { onDone: ctx.onRemoved },
        )
      const askDraft = () =>
        ask(decl, e, {
          confirmLabel: "Remove",
          title: `Remove ${e.name}`,
          body: { lines: [`Remove ${e.name}? Its prompt and images are deleted.`] },
          onConfirm: del,
        })
      const decide = async () => {
        const empty =
          e.draftEmpty ?? (await isDraftEmpty(chatDraft ? "chat-draft" : "design-draft", id, w))
        if (empty) del()
        else askDraft()
      }
      void decide()
      return
    }
    const s = e.session
    if (!s) return
    const name = s.name || s.id
    const exited = s.status.state === "exited"
    const gone = isPlainChat(e) ? "chat" : "agent"
    ask(decl, e, {
      confirmLabel: "Remove",
      title: `Remove ${name}`,
      body: {
        lines: [
          exited
            ? `Remove ${name}? Its ${gone} is deleted.`
            : `Stop and remove ${name}? Its ${gone} is deleted.`,
        ],
      },
      onConfirm: () =>
        void run(() => removeSession(s, { remove: w.remove }), {
          onDone: () => {
            forgetLast(s.id)
            ctx.onRemoved?.()
          },
        }),
    })
  }

  function runStop(decl: ActionDecl, e: Entity, ask: Ask) {
    const s = e.session
    if (!s) return
    ask(decl, e, {
      confirmLabel: "Stop",
      title: `Stop ${nameOf(e)}`,
      body: { lines: [`Stop ${nameOf(e)}? Its agent ends.`] },
      onConfirm: () =>
        void run(() => (w.stop ?? postStopSession)({ id: s.id, state: rowToken(s) })),
    })
  }

  function runRename(e: Entity) {
    if (e.kind === "chat-draft") {
      const page = chatDraftPageActions()
      if (page && surfaceOf().page === e.key) page.rename()
      else
        openRename({
          chatDraft: { id: ctx.draft?.id ?? e.key.replace(/^chat-draft\//, ""), name: e.name },
        })
      return
    }
    if (isDraftEntity(e)) {
      const page = draftPageActions()
      if (page && surfaceOf().page === e.key) page.rename()
      else if (ctx.draft) openRename({ draft: ctx.draft })
      return
    }
    if (e.session) openRename({ session: e.session })
  }

  // Counts the write as pending and tells the page once the server accepted it.
  const recovered = (call: () => Promise<Refusable>) =>
    trackRecover(call).then((res) => {
      if (!res.refused) ctx.onRecovered?.()
      return res
    })

  function runStartOver(decl: ActionDecl, e: Entity, ask: Ask) {
    ask(decl, e, {
      confirmLabel: decl.label,
      title: decl.label,
      body: {
        lines: [...(ctx.tooltip ? [ctx.tooltip] : []), DISCARD_NOTICE],
        input: {
          label: "Reason",
          required: true,
          name: "recovery-reason",
          placeholder: "Why start over?",
        },
      },
      onConfirm: (input) => {
        const reason = input?.trim()
        if (!reason) return
        return recovered(() =>
          w.recover(ActionStartOver, { address: e.key, state: ctx.state ?? "", reason }),
        )
      },
    })
  }

  function runCancelRun(decl: ActionDecl, e: Entity, ask: Ask) {
    ask(decl, e, {
      confirmLabel: "Cancel run",
      dismissLabel: "Cancel",
      title: `Cancel run of ${e.name}`,
      body: { lines: [CANCEL_NOTICE] },
      onConfirm: () =>
        recovered(() => w.recover(ActionCancel, { address: e.key, state: ctx.state ?? "" })),
    })
  }

  function runNewSession(decl: ActionDecl, e: Entity, ask: Ask) {
    const s = e.session
    const isLive = !!s && s.status.state !== "exited"
    const open = () =>
      void run(() =>
        w.openDesign({ epic: e.key, new: true, ...(isLive && s ? { state: stateToken(s) } : {}) }),
      )
    if (!isLive) return open()
    ask(decl, e, {
      confirmLabel: "New session",
      title: `New session of ${e.name}`,
      body: { lines: ["A new session stops the current session."] },
      onConfirm: open,
    })
  }

  function runPromote(e: Entity) {
    const s = e.session
    if (!s) return
    void run(async () => {
      const body: SessionPromptRequest = {
        id: s.id,
        text: "",
        last_message: ctx.promote?.lastMessage,
        promote: true,
      }
      const res = await w.post(body)
      if (res.refused && res.reason === ReasonStale) ctx.promote?.onStale?.()
      return res
    })
  }

  const epicWrite = (post: PostEpicWrite, e: Entity) =>
    void run(() => post({ address: e.key, state: ctx.state ?? "" }))

  function exec(decl: ActionDecl, e: Entity, anchor?: HTMLElement | null) {
    const ask = askAt(anchor)
    switch (decl.id) {
      case "open-design-epic":
        return w.navigate(epicHref(e.key))
      case "show-diff": {
        const url = showDiffUrl(e)
        return url ? w.navigate(diffHref(url)) : undefined
      }
      case "open-pr":
        return e.prUrl ? w.openUrl(e.prUrl) : undefined
      case "rename":
        return runRename(e)
      case "switch-model":
        return e.session ? openModelPicker({ session: e.session }) : undefined
      case "promote":
        return runPromote(e)
      case "new-session":
        return runNewSession(decl, e, ask)
      case "hold":
        return runHold(decl, e, ask)
      case "unhold":
        return void run(() => w.unhold(e.key))
      case "stop":
        return runStop(decl, e, ask)
      case "mark-merged":
        return ask(decl, e, {
          confirmLabel: "Mark as merged",
          title: `Mark ${e.name} as merged`,
          body: {
            lines: [`Mark ${e.name} as merged? Do this only when its PR was merged outside gx.`],
          },
          onConfirm: () => epicWrite(w.markMerged, e),
        })
      case "archive":
        return runArchive(decl, e, ask)
      case "unarchive":
        return void run(() => w.unarchive(e.key), {
          success: (res) =>
            res.new_address
              ? { type: "info", title: `Unarchived as ${res.new_address.split(":").pop()}` }
              : undefined,
        })
      case "copy-id":
        return void w
          .copy(e.key)
          .then((ok) =>
            showToast(
              ok
                ? { type: "success", title: `Copied ${e.key}` }
                : { type: "error", title: "Could not copy the ID." },
            ),
          )
      case "start-over":
        return runStartOver(decl, e, ask)
      case "cancel-run":
        return runCancelRun(decl, e, ask)
      case "remove":
        return runRemove(decl, e, ask)
      case "delete-design":
        return ask(decl, e, {
          confirmLabel: "Delete design",
          title: `Delete ${e.name}`,
          body: { lines: [`Delete ${e.name}? Its map and sessions are removed.`] },
          // The confirm stays open while the delete runs, and a refusal shows inside it.
          removesOpenPage: e.surface === "page",
          onConfirm: () => w.deleteDesign({ epic: e.key }),
        })
      case "queue":
        return epicWrite(w.queue, e)
      case "merge":
        return epicWrite(w.merge, e)
      case "create-pr":
        return epicWrite(w.createPr, e)
      case "start-design-session":
        return void run(() => w.openDesign({ epic: e.key }))
    }
  }

  const disabledReason = (decl: ActionDecl, e: Entity) => {
    const reason = decl.disabled?.(e)
    if (reason) return reason
    if (decl.id === "promote" && e.session && ctx.promote) {
      if (!ctx.promote.connected || !promptable(e.session) || switchRunning(e.session))
        return "The agent cannot take a prompt now."
    }
    return undefined
  }

  const ready = (decl: ActionDecl, e: Entity): ReadyAction => {
    const reason = disabledReason(decl, e)
    return {
      id: decl.id,
      label: decl.label,
      group: decl.group,
      order: decl.order,
      danger: !!decl.danger,
      chord: decl.chord,
      disabled:
        pending || (RECOVERY_IDS.includes(decl.id) && recoverPending) || reason !== undefined,
      reason,
      items:
        decl.id === "move"
          ? (ctx.workspaces ?? []).map((ws) => ({
              label: ws.name,
              current: ws.name === e.workspace,
              run: () => void run(() => w.move(e.key, ws.name)),
            }))
          : undefined,
      run: (anchor) => exec(decl, e, anchor),
    }
  }

  /** Opens the Stop confirm for another session of the entity (a Design's live session list). */
  const stopSession = (session: SessionInfo, anchor?: HTMLElement | null) => {
    const decl = actionById("stop")
    if (decl && entity) runStop(decl, { ...entity, session }, askAt(anchor))
  }
  if (!entity) return { actions: [] as ReadyAction[], pending, get: () => undefined, stopSession }
  const actions = menuActions(entity).map((d) => ready(d, entity))
  const get = (id: string) => {
    const decl = actionById(id)
    return decl ? ready(decl, entity) : undefined
  }
  return { actions, pending, get, stopSession }
}
