import { renderHook, type RenderHookResult } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"
import { registerChatDraftPage, registerDraftPage } from "../shell/draftPageActions"
import { startNewChatDraft, startNewDesign, useNewDesignContext } from "./newDesign"

const h = vi.hoisted(() => ({ showToast: vi.fn(), pathname: "/" }))
const showToast = h.showToast
vi.mock("../ui/toaster", () => ({ showToast: h.showToast }))

const open = vi.fn()
vi.mock("react-router", () => ({
  useLocation: () => ({ pathname: h.pathname }),
  useNavigate: () => (href: string) => open(href),
}))
vi.mock("../connection", () => ({
  useConnectionContext: () => ({}),
  useConnectionState: () => ({
    snapshot: {
      sessions: [{ id: "s1", project: "chatproj" }],
      design_drafts: [{ id: "d1", project: "draftproj" }],
    },
  }),
}))
vi.mock("../epics/list", () => ({
  useEpicList: () => ({
    list: { projects: ["alpha", "chatproj", "last"], epics: [{ project: "draftproj" }] },
  }),
}))
vi.mock("../shell/ShellContext", () => ({ useShell: () => ({ workspace: "w1" }) }))

const create = vi.fn(async () => "d9")
const client = () => ({ create, getLastProject: async () => "last" })

let mounted: RenderHookResult<void, unknown> | undefined
const mountAt = (pathname: string) => {
  h.pathname = pathname
  mounted = renderHook(() => useNewDesignContext())
}

beforeEach(() => {
  open.mockClear()
  create.mockClear()
  showToast.mockClear()
})
afterEach(() => {
  mounted?.unmount()
  mounted = undefined
  localStorage.clear()
})

test("the context follows a navigation and is cleared on unmount", async () => {
  mountAt("/agents/s1")
  h.pathname = "/drafts/d1"
  mounted?.rerender()
  await startNewDesign(client())
  expect(create).toHaveBeenCalledWith({ workspace: "w1", project: "draftproj" })
  mounted?.unmount()
  mounted = undefined
  create.mockClear()
  await startNewDesign(client())
  expect(create).not.toHaveBeenCalled()
})

const page = (empty: boolean) => {
  const focusPrompt = vi.fn()
  const off = registerDraftPage({
    remove: vi.fn(),
    rename: vi.fn(),
    isEmpty: () => empty,
    focusPrompt,
  })
  return { focusPrompt, off }
}

describe("create versus reuse", () => {
  test("on an empty draft's page it creates nothing and focuses the Prompt", async () => {
    mountAt("/drafts/d1")
    const { focusPrompt, off } = page(true)
    await startNewDesign(client())
    off()
    expect(focusPrompt).toHaveBeenCalledTimes(1)
    expect(create).not.toHaveBeenCalled()
    expect(open).not.toHaveBeenCalled()
  })

  test("on a non-empty draft's page it creates a draft in that draft's project", async () => {
    mountAt("/drafts/d1")
    const { focusPrompt, off } = page(false)
    await startNewDesign(client())
    off()
    expect(focusPrompt).not.toHaveBeenCalled()
    expect(create).toHaveBeenCalledTimes(1)
    expect(create).toHaveBeenCalledWith({ workspace: "w1", project: "draftproj" })
    expect(open).toHaveBeenCalledWith("/drafts/d9")
  })

  test("on the Epics page it creates one draft and opens it", async () => {
    mountAt("/epics")
    await startNewDesign(client())
    expect(create).toHaveBeenCalledTimes(1)
    expect(open).toHaveBeenCalledWith("/drafts/d9")
  })
})

describe("startNewChatDraft", () => {
  const chatCreate = vi.fn(async (_init: unknown) => "c9")
  const chatClient = () => ({ create: chatCreate, getLastProject: async () => "last" })
  beforeEach(() => chatCreate.mockClear())

  test("creates one empty draft in the workspace and opens its page", async () => {
    mountAt("/epics")
    await startNewChatDraft(chatClient())
    expect(chatCreate).toHaveBeenCalledTimes(1)
    expect(chatCreate).toHaveBeenCalledWith({
      workspace: "w1",
      project: "last",
      harness: "",
      folder: "source",
    })
    expect(open).toHaveBeenCalledWith("/chat-drafts/c9")
    expect(create).not.toHaveBeenCalled()
  })

  test("on an empty chat draft page it focuses the brief and creates nothing", async () => {
    mountAt("/chat-drafts/c1")
    const focusBrief = vi.fn()
    const off = registerChatDraftPage({ isEmpty: () => true, focusBrief })
    await startNewChatDraft(chatClient())
    off()
    expect(focusBrief).toHaveBeenCalledTimes(1)
    expect(chatCreate).not.toHaveBeenCalled()
    expect(open).not.toHaveBeenCalled()
  })

  test("on a chat draft page with a brief it creates a draft", async () => {
    mountAt("/chat-drafts/c1")
    const off = registerChatDraftPage({ isEmpty: () => false, focusBrief: vi.fn() })
    await startNewChatDraft(chatClient())
    off()
    expect(chatCreate).toHaveBeenCalledTimes(1)
  })

  test("a failed create toasts the reason and opens nothing", async () => {
    mountAt("/epics")
    chatCreate.mockRejectedValueOnce(new Error("no such project"))
    await startNewChatDraft(chatClient())
    expect(open).not.toHaveBeenCalled()
    expect(showToast).toHaveBeenCalledWith({
      type: "error",
      title: "Could not create the draft",
      description: "no such project",
    })
  })
})

describe("the new draft's project", () => {
  test("is the chat's from a chat page", async () => {
    mountAt("/agents/s1")
    await startNewDesign(client())
    expect(create).toHaveBeenCalledWith({ workspace: "w1", project: "chatproj" })
  })

  test("is the one-project Epics filter when the page has none", async () => {
    localStorage.setItem("gx.epics.project", JSON.stringify(["alpha"]))
    mountAt("/epics")
    await startNewDesign(client())
    expect(create).toHaveBeenCalledWith({ workspace: "w1", project: "alpha" })
  })

  test("is the last project otherwise", async () => {
    mountAt("/epics")
    await startNewDesign(client())
    expect(create).toHaveBeenCalledWith({ workspace: "w1", project: "last" })
  })
})

describe("the old draft key", () => {
  const KEY = "gx.newDesign.draft"

  test("with text becomes its own draft before the new one, and is removed", async () => {
    localStorage.setItem(KEY, JSON.stringify({ idea: "old words", slug: "old", project: "alpha" }))
    mountAt("/epics")
    await startNewDesign(client())
    expect(create).toHaveBeenCalledTimes(2)
    expect(create.mock.calls[0]).toEqual([
      { workspace: "w1", project: "alpha", slug: "old", idea: "old words" },
    ])
    expect(create.mock.calls[1]).toEqual([{ workspace: "w1", project: "last" }])
    expect(localStorage.getItem(KEY)).toBeNull()
  })

  test("with no text is removed and creates nothing", async () => {
    localStorage.setItem(KEY, JSON.stringify({ idea: " ", slug: "", project: "alpha" }))
    mountAt("/epics")
    await startNewDesign(client())
    expect(create).toHaveBeenCalledTimes(1)
    expect(create).toHaveBeenCalledWith({ workspace: "w1", project: "last" })
    expect(localStorage.getItem(KEY)).toBeNull()
  })

  test("a failed move keeps the key and the new draft is still created", async () => {
    localStorage.setItem(KEY, JSON.stringify({ idea: "old words" }))
    mountAt("/epics")
    create.mockRejectedValueOnce(new Error("down"))
    await startNewDesign(client())
    expect(localStorage.getItem(KEY)).not.toBeNull()
    expect(create).toHaveBeenCalledTimes(2)
    expect(open).toHaveBeenCalledWith("/drafts/d9")
  })
})

test("a failed create shows an error toast and navigates nowhere", async () => {
  mountAt("/epics")
  create.mockRejectedValueOnce(new Error("disk full"))
  await startNewDesign(client())
  expect(showToast).toHaveBeenCalledWith(expect.objectContaining({ type: "error" }))
  expect(open).not.toHaveBeenCalled()
})
