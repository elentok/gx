import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { VerdictConcurrencyCap, type EpicListRow, type SessionInfo } from "../generated/server"
import { ConfirmHost } from "../shell/ConfirmHost"
import { renderThemed } from "../ui/render"
import { toaster } from "../ui/toaster"
import { EpicCard } from "./EpicCard"
import { followUpsTone, ringArc, ringTone } from "./ring"

afterEach(() => {
  cleanup()
  toaster.remove()
  vi.useRealTimers()
})

/** Every button but the card's own ⋯. */
const footerButtons = () =>
  screen.queryAllByRole("button").filter((b) => b.getAttribute("aria-label") !== "More actions")

const row = (over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: "gx:blog",
  project: "gx",
  name: "blog",
  kind: "loop",
  state: "running",
  done: 2,
  total: 5,
  queued: false,
  queueable: false,
  addresses: ["gx:blog"],
  card_kind: "implementation",
  section: "implementing",
  status_words: "Implementing...",
  working: true,
  working_word: "Implementing",
  work_kind: "implementation",
  ring_progress: 40,
  actions: [],
  ...over,
})

const needsYou = (over: Partial<EpicListRow> = {}) =>
  row({
    state: "paused",
    section: "needs-you",
    state_pill: "Paused",
    reason: "06 Retry the launch: zero commits",
    actions: [{ kind: "open-ticket", label: "Open ticket", target: "gx:blog/06" }],
    ...over,
  })

const text = (el: HTMLElement) => el.textContent ?? ""

test("the title is the only link and points at the epic page", () => {
  renderThemed(<EpicCard row={needsYou()} />)
  const links = screen.getAllByRole("link")
  expect(links).toHaveLength(1)
  expect(links[0].getAttribute("href")).toBe("#/e/gx:blog")
})

test.each(["implementation", "design"])(
  "a %s card with back-link data shows Milestone N of the Initiative, linked to its page",
  (kind) => {
    renderThemed(
      <EpicCard
        row={needsYou({
          card_kind: kind as EpicListRow["card_kind"],
          milestone: 2,
          initiative: "Launch",
          initiative_address: "gx:launch",
        })}
      />,
    )
    const line = screen.getByTestId("back-link")
    expect(text(line)).toBe("Milestone 2 of Launch")
    const links = screen.getAllByRole("link")
    expect(links).toHaveLength(2)
    expect(within(line).getByRole("link", { name: "Launch" }).getAttribute("href")).toBe(
      "#/initiatives/gx:launch",
    )
  },
)

test("a card whose link does not resolve shows no back-link line", () => {
  renderThemed(<EpicCard row={needsYou({ milestone: 2 })} />)
  expect(screen.queryByTestId("back-link")).toBeNull()
})

test("a dirty-trunk row renders both buttons and the reason, with no file list", () => {
  renderThemed(
    <EpicCard
      row={needsYou({
        reason: "Trunk has local changes",
        actions: [
          { kind: "stash-merge", label: "Stash, merge and restore", target: "gx:blog" },
          { kind: "merge", label: "Merge again", target: "gx:blog" },
        ],
      })}
    />,
  )
  expect(screen.getByRole("button", { name: "Stash, merge and restore" })).toBeTruthy()
  expect(screen.getByRole("button", { name: "Merge again" })).toBeTruthy()
  expect(text(screen.getByTestId("reason"))).toBe("Trunk has local changes")
  expect(screen.queryByTestId("merge-block-files")).toBeNull()
})

test("the stash button posts through the stash write with the state token", async () => {
  const stashMerge = vi.fn().mockResolvedValue({})
  const onChanged = vi.fn()
  const r = needsYou({
    actions: [{ kind: "stash-merge", label: "Stash, merge and restore", target: "gx:blog" }],
  })
  renderThemed(<EpicCard row={r} writes={{ stashMerge }} onChanged={onChanged} />)
  fireEvent.click(screen.getByRole("button", { name: "Stash, merge and restore" }))
  await waitFor(() => expect(onChanged).toHaveBeenCalled())
  expect(stashMerge).toHaveBeenCalledWith({ address: "gx:blog", state: "paused" })
})

test("title targets: one-off ticket, follow-ups epic, joined card's active part, pending Design", () => {
  const href = (r: EpicListRow) => {
    renderThemed(<EpicCard row={r} />)
    const h = screen.getByRole("link").getAttribute("href")
    cleanup()
    return h
  }
  expect(
    href(row({ address: "gx:one-offs/03", card_kind: "one-off", addresses: ["gx:one-offs/03"] })),
  ).toBe("#/t/gx:one-offs/03")
  expect(
    href(
      row({
        address: "gx:follow-ups",
        project: "gx",
        card_kind: "follow-ups",
        addresses: ["gx:follow-ups"],
      }),
    ),
  ).toBe("#/triage/gx")
  expect(href(row({ address: "gx:blog", addresses: ["gx:blog-impl", "gx:blog"] }))).toBe(
    "#/e/gx:blog-impl",
  )
})

test("the pill and reason show on a Needs-you card; a quiet card shows neither", () => {
  renderThemed(<EpicCard row={needsYou()} />)
  expect(text(screen.getByTestId("pill"))).toContain("Paused")
  expect(text(screen.getByTestId("reason"))).toContain("zero commits")
  cleanup()
  renderThemed(<EpicCard row={row()} />)
  expect(screen.queryByTestId("pill")).toBeNull()
  expect(screen.queryByTestId("reason")).toBeNull()
})

test("a non-Needs-you card shows a reason but no pill", () => {
  renderThemed(
    <EpicCard row={row({ state: "queued", reason: "Starts after agent-runner-phase2" })} />,
  )
  expect(screen.queryByTestId("pill")).toBeNull()
  expect(text(screen.getByTestId("reason"))).toContain("Starts after")
})

test("a launching card shows the launch step, not the cap line", () => {
  renderThemed(
    <EpicCard
      row={row({
        verdict: VerdictConcurrencyCap,
        reason: "global cap 1/1",
        launch_step: { label: "installing dependencies", position: 2, total: 4 },
      })}
    />,
  )
  expect(text(screen.getByTestId("reason"))).toBe("Launching: installing dependencies (2/4)")
})

test("a cap-held card shows Waiting for a slot with the caps", () => {
  renderThemed(
    <EpicCard
      row={row({ state: "queued", verdict: VerdictConcurrencyCap, reason: "global cap 1/1" })}
    />,
  )
  expect(text(screen.getByTestId("reason"))).toBe("Waiting for a slot: global cap 1/1")
})

test.each([
  ["queued", { state: "queued", status_words: "Queued #1" }],
  ["running", {}],
  ["finished", { state: "finished", section: "finished", status_words: "Merged" }],
  ["draft", { state: "draft", section: "ready-to-design", status_words: "Draft" }],
])("%s card has no footer", (_, over) => {
  renderThemed(<EpicCard row={row(over)} />)
  expect(footerButtons()).toEqual([])
  expect(screen.queryByTestId("footer")).toBeNull()
})

test("ring labels: percent, n/m, ellipsis, empty", () => {
  renderThemed(<EpicCard row={row({ card_kind: "design", ring_progress: 57 })} />)
  expect(screen.getByText("57%")).toBeTruthy()
  cleanup()
  renderThemed(<EpicCard row={row()} />)
  expect(screen.getByText("2/5")).toBeTruthy()
  cleanup()
  renderThemed(
    <EpicCard
      row={row({
        card_kind: "design",
        work_kind: "design",
        working_word: "Creating tickets",
        status_words: "Creating tickets (3 so far)...",
      })}
    />,
  )
  expect(screen.getByText("…")).toBeTruthy()
  expect(screen.getByTestId("comet")).toBeTruthy()
  cleanup()
  renderThemed(
    <EpicCard
      row={row({ card_kind: "one-off", state: "queued", working: false, work_kind: undefined })}
    />,
  )
  expect(screen.queryByText(/%|\d\/\d/)).toBeNull()
})

const arcOf = () => document.querySelector("[data-arc]")?.getAttribute("data-arc") ?? null
const toneOf = () => document.querySelector("[data-tone]")?.getAttribute("data-tone")

test("a working Design draws the purple arc with a comet", () => {
  renderThemed(
    <EpicCard
      row={row({ card_kind: "design", work_kind: "design", status_words: "Charting..." })}
    />,
  )
  expect(arcOf()).toBe("kind.design.fg")
  expect(screen.getByTestId("comet")).toBeTruthy()
})

test("the joined card is purple while the Design works and blue once the impl epic runs", () => {
  renderThemed(<EpicCard row={row({ work_kind: "design", status_words: "Branching..." })} />)
  expect(arcOf()).toBe("kind.design.fg")
  cleanup()
  renderThemed(<EpicCard row={row()} />)
  expect(arcOf()).toBeNull()
  expect(toneOf()).toBe("running")
})

test("a row without the working flag is grey and does not spin, whatever its words say", () => {
  renderThemed(<EpicCard row={row({ state: "ready", working: false, status_words: "Working" })} />)
  expect(toneOf()).toBe("muted")
  expect(screen.queryByTestId("comet")).toBeNull()
  expect(arcOf()).toBeNull()
})

test("a working row spins from the flag whatever its state", () => {
  renderThemed(<EpicCard row={row({ state: "queued", status_words: "Working" })} />)
  expect(toneOf()).toBe("running")
  expect(screen.getByTestId("comet")).toBeTruthy()
})

test("Queue posts in place, then reports the change", async () => {
  const queue = vi.fn().mockResolvedValue({})
  const onChanged = vi.fn()
  const r = row({
    state: "ready",
    section: "ready-to-queue",
    actions: [{ kind: "queue", label: "Queue" }],
  })
  renderThemed(<EpicCard row={r} writes={{ queue }} onChanged={onChanged} />)
  fireEvent.click(screen.getByRole("button", { name: "Queue" }))
  await waitFor(() => expect(onChanged).toHaveBeenCalled())
  expect(queue).toHaveBeenCalledWith({ address: "gx:blog", state: "ready" })
})

test("a refused Queue shows its reason in a toast", async () => {
  const queue = vi
    .fn()
    .mockResolvedValue({ refused: true, reason: "epic-draft", message: "gx:blog is a draft" })
  const onChanged = vi.fn()
  const r = row({
    state: "ready",
    section: "ready-to-queue",
    actions: [{ kind: "queue", label: "Queue" }],
  })
  renderThemed(<EpicCard row={r} writes={{ queue }} onChanged={onChanged} />)
  fireEvent.click(screen.getByRole("button", { name: "Queue" }))
  expect(await screen.findByText("gx:blog is a draft")).toBeTruthy()
  expect(onChanged).not.toHaveBeenCalled()
})

test("a refused Merge shows its reason in a toast", async () => {
  const merge = vi
    .fn()
    .mockResolvedValue({ refused: true, reason: "x", message: "Conflicts with main" })
  const r = row({
    state: "merge-blocked",
    section: "needs-you",
    state_pill: "Merge blocked",
    actions: [{ kind: "merge", label: "Merge" }],
  })
  renderThemed(<EpicCard row={r} writes={{ merge }} />)
  fireEvent.click(screen.getByRole("button", { name: "Merge" }))
  expect(await screen.findByText("Conflicts with main")).toBeTruthy()
})

const ciFailing = (over: Partial<EpicListRow> = {}) =>
  needsYou({
    state: "ci-failing",
    status_words: "CI failing",
    state_pill: "Paused",
    reason:
      "CI failing: go-test failed 3 times (limit 3 per check). Fix it by hand and push, or press Try again to give the agent 3 more attempts.",
    pr: { number: 12, url: "https://github.com/o/r/pull/12", checks: "1 of 5 checks failed" },
    actions: [
      { kind: "try-again", label: "Try again", target: "gx:blog" },
      { kind: "open-epic", label: "Open epic", target: "gx:blog" },
      { kind: "mark-merged", label: "Mark as merged", target: "gx:blog" },
      {
        kind: "open-pr",
        label: "Open on GitHub",
        target: "gx:blog",
        link: "https://github.com/o/r/pull/12",
      },
    ],
    ...over,
  } as Partial<EpicListRow>)

test("a CI failing card shows the Paused pill, the reason, Try again and Open PR", () => {
  renderThemed(<EpicCard row={ciFailing()} />)
  expect(text(screen.getByTestId("status"))).toBe("CI failing")
  expect(text(screen.getByTestId("pill"))).toContain("Paused")
  expect(text(screen.getByTestId("reason"))).toContain("go-test failed 3 times")
  expect(footerButtons().map(text)).toEqual(["Open PR", "Try again"])
  const open = vi.spyOn(window, "open").mockReturnValue(null)
  fireEvent.click(screen.getByRole("button", { name: "Open PR" }))
  expect(open).toHaveBeenCalledWith(
    "https://github.com/o/r/pull/12",
    "_blank",
    "noopener,noreferrer",
  )
  open.mockRestore()
})

test("Try again posts the epic address and state; a refusal shows its reason", async () => {
  const tryAgain = vi
    .fn()
    .mockResolvedValue({ refused: true, reason: "x", message: "The pause already changed" })
  renderThemed(<EpicCard row={ciFailing()} writes={{ tryAgain }} />)
  fireEvent.click(screen.getByRole("button", { name: "Try again" }))
  expect(await screen.findByText("The pause already changed")).toBeTruthy()
  expect(tryAgain).toHaveBeenCalledWith({ address: "gx:blog", state: "ci-failing" })
})

test("a merge-blocked PR card shows both merge buttons", () => {
  renderThemed(
    <EpicCard
      row={needsYou({
        state: "merge-blocked",
        status_words: "PR merged",
        state_pill: "Merge blocked",
        reason: "Trunk has local changes",
        actions: [
          { kind: "stash-merge", label: "Stash, merge and restore", target: "gx:blog" },
          { kind: "merge", label: "Merge", target: "gx:blog" },
        ],
      })}
    />,
  )
  expect(footerButtons().map(text)).toEqual(["Stash, merge and restore", "Merge"])
  expect(text(screen.getByTestId("status"))).toBe("PR merged")
})

test("Ready for review has no buttons and shows the waiting reason", () => {
  renderThemed(
    <EpicCard
      row={row({
        state: "pr-open",
        status_words: "Ready for review",
        pr: { number: 12, url: "https://github.com/o/r/pull/12", checks: "5 of 5 checks passed" },
        actions: [
          {
            kind: "open-pr",
            label: "Open on GitHub",
            target: "gx:blog",
            link: "https://github.com/o/r/pull/12",
          },
        ],
      } as Partial<EpicListRow>)}
    />,
  )
  expect(footerButtons()).toEqual([])
  expect(text(screen.getByTestId("reason"))).toContain("Waiting for you to merge")
  expect(text(screen.getByTestId("pr-checks"))).toBe("5 of 5 checks passed")
})

test("the Merge button is disabled while the merge is pending", async () => {
  let release!: (v: unknown) => void
  const merge = vi.fn().mockReturnValue(new Promise((r) => (release = r)))
  const r = row({
    state: "merge-blocked",
    section: "needs-you",
    state_pill: "Merge blocked",
    actions: [{ kind: "merge", label: "Merge" }],
  })
  renderThemed(<EpicCard row={r} writes={{ merge }} />)
  fireEvent.click(screen.getByRole("button", { name: "Merge" }))
  await waitFor(() => expect((footerButtons()[0] as HTMLButtonElement).disabled).toBe(true))
  release({ refused: true, reason: "x", message: "Conflicts with main" })
  expect(await screen.findByText("Conflicts with main")).toBeTruthy()
})

test("a paused Merge shows a warning toast with the message and a link to the epic", async () => {
  const merge = vi.fn().mockResolvedValue({ parked: true, message: "Rebase onto main failed" })
  const r = row({
    state: "awaiting-merge",
    section: "needs-you",
    actions: [{ kind: "merge", label: "Merge" }],
  })
  renderThemed(<EpicCard row={r} writes={{ merge }} />)
  fireEvent.click(screen.getByRole("button", { name: "Merge" }))
  expect(await screen.findByText("Rebase onto main failed")).toBeTruthy()
  expect(screen.getByRole("link", { name: "Open the paused epic" }).getAttribute("href")).toBe(
    "#/e/gx:blog",
  )
})

test("a forked Merge shows 'Merge ticket <name> created' with a link", async () => {
  const merge = vi.fn().mockResolvedValue({ merge_ticket: "gx:blog/09" })
  const r = row({
    state: "awaiting-merge",
    section: "needs-you",
    actions: [{ kind: "merge", label: "Merge" }],
  })
  renderThemed(<EpicCard row={r} writes={{ merge }} />)
  fireEvent.click(screen.getByRole("button", { name: "Merge" }))
  expect(await screen.findByText("Merge ticket gx:blog/09 created")).toBeTruthy()
  expect(screen.getByRole("link", { name: "Open the merge ticket" }).getAttribute("href")).toBe(
    "#/t/gx:blog/09",
  )
})

test("a Queue shows no success toast", async () => {
  const queue = vi.fn().mockResolvedValue({ queue: [] })
  const onChanged = vi.fn()
  const r = row({
    state: "ready",
    section: "ready-to-queue",
    actions: [{ kind: "queue", label: "Queue" }],
  })
  renderThemed(<EpicCard row={r} writes={{ queue }} onChanged={onChanged} />)
  fireEvent.click(screen.getByRole("button", { name: "Queue" }))
  await waitFor(() => expect(onChanged).toHaveBeenCalled())
  expect(document.querySelector("[data-scope='toast'][data-part='root']")).toBeNull()
})

test("Design posts open-design, then navigates to the Design page", async () => {
  const openDesign = vi.fn().mockResolvedValue({})
  const navigate = vi.fn()
  const r = row({
    card_kind: "design",
    state: "waiting-to-design",
    actions: [{ kind: "design", label: "Design", target: "gx:blog" }],
  })
  renderThemed(<EpicCard row={r} writes={{ openDesign, navigate }} />)
  fireEvent.click(screen.getByRole("button", { name: "Design" }))
  await waitFor(() => expect(navigate).toHaveBeenCalledWith("#/d/gx:blog"))
  expect(openDesign).toHaveBeenCalledWith({ epic: "gx:blog" })
})

test("a refused Design stays on the page and says why", async () => {
  const openDesign = vi.fn().mockResolvedValue({ refused: true, message: "Already running" })
  const navigate = vi.fn()
  const r = row({
    card_kind: "design",
    actions: [{ kind: "continue", label: "Continue", target: "gx:blog" }],
  })
  renderThemed(<EpicCard row={r} writes={{ openDesign, navigate }} />)
  fireEvent.click(screen.getByRole("button", { name: "Continue" }))
  expect(await screen.findByText(/Already running/)).toBeTruthy()
  expect(navigate).not.toHaveBeenCalled()
})

test.each([
  ["answer", "gx:blog/06", "#/t/gx:blog/06"],
  ["open-ticket", "gx:blog/06", "#/t/gx:blog/06"],
  ["open-epic", "gx:blog", "#/e/gx:blog"],
])("%s navigates", (kind, target, href) => {
  const navigate = vi.fn()
  renderThemed(
    <EpicCard row={needsYou({ actions: [{ kind, label: "Go", target }] })} writes={{ navigate }} />,
  )
  fireEvent.click(screen.getByRole("button", { name: "Go" }))
  expect(navigate).toHaveBeenCalledWith(href)
})

test("Answer opens the link the server sent, even on a joined implementation card", () => {
  const navigate = vi.fn()
  const answer = { kind: "answer", label: "Answer", target: "gx:blog", link: "#/d/gx:blog" }
  renderThemed(
    <EpicCard
      row={needsYou({ card_kind: "implementation", actions: [answer] })}
      writes={{ navigate }}
    />,
  )
  fireEvent.click(screen.getByRole("button", { name: "Answer" }))
  expect(navigate).toHaveBeenCalledWith("#/d/gx:blog")
})

const followUps = (over: Partial<EpicListRow> = {}) =>
  row({ card_kind: "follow-ups", project: "gx", ...over })

test.each([
  ["triage", "Triage"],
  ["continue", "Continue"],
])("%s on a Follow-ups card starts the Triage and opens its page", async (kind, label) => {
  const openTriage = vi.fn().mockResolvedValue({})
  const openDesign = vi.fn()
  const navigate = vi.fn()
  renderThemed(
    <EpicCard
      row={followUps({ actions: [{ kind, label, target: "gx" }] })}
      writes={{ openTriage, openDesign, navigate }}
    />,
  )
  fireEvent.click(screen.getByRole("button", { name: label }))
  await waitFor(() => expect(navigate).toHaveBeenCalledWith("#/triage/gx"))
  expect(openTriage).toHaveBeenCalledWith({ epic: "gx" })
  expect(openDesign).not.toHaveBeenCalled()
})

test("a refused Triage stays on the page and says why", async () => {
  const openTriage = vi
    .fn()
    .mockResolvedValue({ refused: true, message: "Triage already starting" })
  const navigate = vi.fn()
  renderThemed(
    <EpicCard
      row={followUps({ actions: [{ kind: "triage", label: "Triage", target: "gx" }] })}
      writes={{ openTriage, navigate }}
    />,
  )
  fireEvent.click(screen.getByRole("button", { name: "Triage" }))
  expect(await screen.findByText(/Triage already starting/)).toBeTruthy()
  expect(navigate).not.toHaveBeenCalled()
})

test.each([undefined, "#/triage/gx"])(
  "Answer (link %s) on a Needs you Follow-ups card opens the Triage page",
  (link) => {
    const openTriage = vi.fn()
    const navigate = vi.fn()
    const row = followUps({
      section: "needs-you",
      state_pill: "Needs you",
      reason: "Round 1 · 2 questions left",
      actions: [{ kind: "answer", label: "Answer", target: "gx", link }],
    })
    renderThemed(<EpicCard row={row} writes={{ openTriage, navigate }} />)
    expect(screen.queryByRole("button", { name: "Continue" })).toBeNull()
    expect(text(screen.getByTestId("pill"))).toBe("Needs you")
    expect(screen.getByText("Round 1 · 2 questions left")).toBeTruthy()
    fireEvent.click(screen.getByRole("button", { name: "Answer" }))
    expect(navigate).toHaveBeenCalledWith("#/triage/gx")
    expect(openTriage).not.toHaveBeenCalled()
  },
)

test.each([
  ["Round 1 · 2 questions left", "Answer"],
  ["Write this plan?", "Review plan"],
])(
  "at a phone width the %s card shows the pill, the line and one wrapping button",
  (reason, label) => {
    window.innerWidth = 390
    renderThemed(
      <EpicCard
        row={followUps({
          section: "needs-you",
          state_pill: "Needs you",
          reason,
          actions: [{ kind: "answer", label, target: "gx", link: "#/triage/gx" }],
        })}
      />,
    )
    expect(text(screen.getByTestId("pill"))).toBe("Needs you")
    expect(screen.getByTestId("reason").textContent).toBe(reason)
    expect(within(screen.getByTestId("footer")).getAllByRole("button")).toHaveLength(1)
    expect(screen.getByRole("button", { name: label })).toBeTruthy()
    expect(getComputedStyle(screen.getByTestId("footer")).flexWrap).toBe("wrap")
  },
)

const writeCard = (actions: EpicListRow["actions"]) =>
  followUps({
    section: "designing",
    state_pill: "Write",
    reason: "52 of 84 written",
    ring_progress: 85,
    actions,
  })

test("a Write card shows Retry failed small and secondary left of the main Continue", () => {
  const retry = { kind: "open", label: "Retry failed (2)", target: "gx", link: "#/triage/gx" }
  const cont = { kind: "continue", label: "Continue", target: "gx" }
  const navigate = vi.fn()
  const openTriage = vi.fn()
  renderThemed(<EpicCard row={writeCard([retry, cont])} writes={{ navigate, openTriage }} />)
  expect(text(screen.getByTestId("pill"))).toBe("Write")
  expect(screen.getByText("52 of 84 written")).toBeTruthy()
  expect(screen.getByText("85%")).toBeTruthy()
  const buttons = screen.getAllByRole("button").filter((b) => /Retry|Continue/.test(text(b)))
  expect(buttons.map(text)).toEqual(["Retry failed (2)", "Continue"])
  expect(buttons[0].getAttribute("data-emphasis")).toBe("secondary")
  expect(buttons[1].getAttribute("data-emphasis")).toBe("main")
  fireEvent.click(buttons[0])
  expect(navigate).toHaveBeenCalledWith("#/triage/gx")
  expect(openTriage).not.toHaveBeenCalled()
})

test("a Write card without failed rows shows only Continue", () => {
  renderThemed(
    <EpicCard row={writeCard([{ kind: "continue", label: "Continue", target: "gx" }])} />,
  )
  expect(screen.queryByRole("button", { name: /Retry failed/ })).toBeNull()
  expect(screen.getByRole("button", { name: "Continue" })).toBeTruthy()
})

test("Queue on a Done Follow-ups card queues the ready epic once and refetches", async () => {
  const queue = vi.fn().mockResolvedValue({})
  const onChanged = vi.fn()
  const done = followUps({
    state_pill: "Done",
    reason: "39 ready to queue",
    ring_progress: 100,
    actions: [{ kind: "queue", label: "Queue", target: "gx:ready" }],
  })
  renderThemed(<EpicCard row={done} writes={{ queue }} onChanged={onChanged} />)
  expect(text(screen.getByTestId("pill"))).toBe("Done")
  expect(screen.getByText("100%")).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: "Queue" }))
  await waitFor(() => expect(onChanged).toHaveBeenCalledTimes(1))
  expect(queue).toHaveBeenCalledTimes(1)
  expect(queue.mock.calls[0][0].address).toBe("gx:ready")
  expect(followUpsTone(done)).toBe("running")
})

test("the ring of a Follow-ups Needs you card stays running while the pill is amber", () => {
  const row = followUps({ section: "needs-you", state_pill: "Needs you" })
  expect(ringTone(row)).toBe("running")
  expect(followUpsTone(row)).toBe("waiting")
})

test.each(["Review plan", "Review failures", "Open"])(
  "%s opens the Triage page without posting",
  (label) => {
    const openTriage = vi.fn()
    const navigate = vi.fn()
    const open = { kind: "open", label, target: "gx", link: "#/triage/gx" }
    renderThemed(
      <EpicCard
        row={followUps({ section: "needs-you", state_pill: "Waiting for you", actions: [open] })}
        writes={{ openTriage, navigate }}
      />,
    )
    fireEvent.click(screen.getByRole("button", { name: label }))
    expect(navigate).toHaveBeenCalledWith("#/triage/gx")
    expect(openTriage).not.toHaveBeenCalled()
  },
)

test.each([
  ["Waiting for you", "needs-you"],
  ["Stopped", "designing"],
])("a Follow-ups card shows the %s pill", (pill, section) => {
  renderThemed(<EpicCard row={followUps({ section, state_pill: pill })} />)
  expect(text(screen.getByTestId("pill"))).toBe(pill)
})

test.each(["Verify", "Route"])(
  "a Follow-ups %s pill is running, with its reason on one row",
  (pill) => {
    renderThemed(
      <EpicCard
        row={followUps({
          section: "designing",
          state_pill: pill,
          step_line: "Verifying 51 of 84",
          ring_progress: 20,
        })}
      />,
    )
    const pillEl = screen.getByTestId("pill")
    expect(text(pillEl)).toBe(pill)
    const reason = screen.getByTestId("reason")
    expect(reason.parentElement).toBe(pillEl.parentElement)
    expect(getComputedStyle(pillEl.parentElement as HTMLElement).flexDirection).toBe("row")
    expect(screen.getByText("20%")).toBeTruthy()
  },
)

test("a Follow-ups Needs you pill stays amber but a step pill does not", () => {
  expect(followUpsTone(followUps({ state_pill: "Needs you" }))).toBe("waiting")
  expect(followUpsTone(followUps({ state_pill: "Verify" }))).toBe("running")
  expect(followUpsTone(followUps({ state_pill: "Stopped" }))).toBe("paused")
})

test("the ring of a Follow-ups card is running with a pill and muted without", () => {
  expect(ringTone(followUps({ state_pill: "Route" }))).toBe("running")
  expect(ringTone(followUps({ state: "idle", state_pill: undefined, working: false }))).toBe(
    "muted",
  )
})

test("a Follow-ups card draws a teal arc at Write and Done, other cards keep their tone", () => {
  const teal = "kind.implementation.fg"
  expect(ringArc(followUps({ state_pill: "Write" }))).toBe(teal)
  expect(ringArc(followUps({ state: "finished", state_pill: undefined }))).toBe(teal)
  expect(
    ringArc(followUps({ state: "idle", state_pill: undefined, working: false })),
  ).toBeUndefined()
  expect(ringArc(row({ state: "finished", working: false }))).toBeUndefined()
})

test("the Follow-ups title links to the Triage page and the ring shows the percent", () => {
  renderThemed(<EpicCard row={followUps({ name: "Follow-ups", ring_progress: 40 })} />)
  expect(screen.getByRole("link", { name: "Follow-ups" }).getAttribute("href")).toBe("#/triage/gx")
  expect(screen.getByText("40%")).toBeTruthy()
})

test("a countdown reason ticks from the deadline", () => {
  vi.useFakeTimers()
  const ends = new Date(Date.now() + 12_000).toISOString()
  renderThemed(<EpicCard row={row({ card_kind: "design", countdown_ends_at: ends })} />)
  expect(text(screen.getByTestId("reason"))).toContain("Next run starts in 12s")
  act(() => {
    vi.advanceTimersByTime(3000)
  })
  expect(text(screen.getByTestId("reason"))).toContain("Next run starts in 9s")
})

describe("time label", () => {
  const start = "2026-01-01T10:00:00Z"
  const NOW = Date.parse(start) + 125 * 60_000
  const timed = (time_kind: string) => row({ time_kind, time_start: start })
  const line = () => text(screen.getByTestId("status").parentElement as HTMLElement)

  test.each([
    ["elapsed", "2h 5m"],
    ["waiting", "waiting 2h 5m"],
    ["created", "created 2h 5m ago"],
    ["finished", "2h 5m ago"],
  ])("%s kind reads %s", (kind, words) => {
    renderThemed(<EpicCard row={timed(kind)} now={NOW} />)
    expect(text(screen.getByTestId("time"))).toBe(words)
    expect(line()).toBe(`gx · ${words} · Implementing...`)
  })

  test("the span does not wrap and its title is the local start", () => {
    renderThemed(<EpicCard row={timed("elapsed")} now={NOW} />)
    const el = screen.getByTestId("time")
    expect(getComputedStyle(el).whiteSpace).toBe("nowrap")
    expect(el.getAttribute("title")).toBe(new Date(start).toLocaleString())
  })

  test("no time means no span and no extra dot", () => {
    renderThemed(<EpicCard row={row()} now={NOW} />)
    expect(screen.queryByTestId("time")).toBeNull()
    expect(line()).toBe("gx · Implementing...")
  })

  test("a later now moves the label", () => {
    renderThemed(<EpicCard row={timed("elapsed")} now={NOW + 60 * 60_000} />)
    expect(text(screen.getByTestId("time"))).toBe("3h 5m")
  })
})

test("a card with a workspace shows its name and a menu; one without shows the menu only", () => {
  const { unmount } = renderThemed(
    <EpicCard
      row={row({ workspace: "Side" })}
      workspaces={[{ name: "Default", default: true }, { name: "Side" }] as never}
    />,
  )
  expect(screen.getByTestId("workspace").textContent).toBe("Side")
  expect(screen.getByRole("button", { name: "More actions" })).toBeTruthy()
  unmount()
  renderThemed(<EpicCard row={row()} />)
  expect(screen.queryByTestId("workspace")).toBeNull()
  expect(screen.getByRole("button", { name: "More actions" })).toBeTruthy()
})

describe("the menu on a card outside every workspace", () => {
  const menuItems = async () => {
    fireEvent.click(screen.getByRole("button", { name: "More actions" }))
    return (await screen.findAllByRole("menuitem")).map(text)
  }

  test("a One-off card holds Copy ID only; it copies the ticket address and toasts it", async () => {
    const writeText = vi.fn(async () => {})
    vi.stubGlobal("navigator", { clipboard: { writeText } })
    try {
      renderThemed(
        <EpicCard
          row={row({ card_kind: "one-off", address: "demo:fix-typo/01", workspace: "" })}
        />,
      )
      expect(await menuItems()).toEqual(["Copy ID"])
      fireEvent.click(screen.getByRole("menuitem", { name: "Copy ID" }))
      await waitFor(() => expect(writeText).toHaveBeenCalledWith("demo:fix-typo/01"))
      expect(await screen.findByText("Copied demo:fix-typo/01")).toBeTruthy()
    } finally {
      vi.unstubAllGlobals()
    }
  })

  test("a Follow-ups card holds Copy ID and no Move to workspace, Hold or Archive", async () => {
    renderThemed(<EpicCard row={row({ card_kind: "follow-ups", workspace: "" })} />)
    expect(await menuItems()).toEqual(["Copy ID"])
  })

  test("an implementation card with no workspace has a menu with Copy ID", async () => {
    renderThemed(<EpicCard row={row({ workspace: "" })} />)
    expect(await menuItems()).toEqual(["Copy ID"])
  })

  test("Open PR leads the menu when the row has a PR URL, and opens it in a new tab", async () => {
    const open = vi.spyOn(window, "open").mockReturnValue(null)
    const pr = { number: 12, url: "https://github.com/o/r/pull/12", checks: "" }
    renderThemed(<EpicCard row={row({ pr } as Partial<EpicListRow>)} />)
    expect(await menuItems()).toEqual(["Open PR", "Copy ID"])
    fireEvent.click(screen.getByRole("menuitem", { name: "Open PR" }))
    expect(open).toHaveBeenCalledWith(pr.url, "_blank", "noopener")
    open.mockRestore()
  })
})

test("a held card is dimmed and Take off hold unholds it", async () => {
  const unhold = vi.fn(async () => ({}) as never)
  renderThemed(
    <EpicCard
      row={row({
        section: "on-hold",
        workspace: "Default",
        actions: [{ kind: "unhold", label: "Take off hold" }],
      })}
      writes={{ unhold }}
    />,
  )
  const card = screen.getByRole("link", { name: "blog" }).closest("[data-section]")!
  expect(getComputedStyle(card).opacity).not.toBe("1")
  fireEvent.click(screen.getByRole("button", { name: "Take off hold" }))
  await waitFor(() => expect(unhold).toHaveBeenCalledWith("gx:blog"))
})

describe("PR line", () => {
  const pr = {
    number: 412,
    url: "https://github.com/o/r/pull/412",
    chip: "#412",
    checks: "2 of 5 checks running",
  }

  test("a PR row shows the number chip, the check summary, the status words and the GitHub link", () => {
    renderThemed(
      <EpicCard
        row={row({
          pr,
          status_words: "PR draft · CI running",
          actions: [{ kind: "open-pr", label: "Open on GitHub", target: "gx:blog", link: pr.url }],
        })}
      />,
    )
    expect(screen.getByTestId("pr-number").textContent).toBe("#412")
    expect(screen.getByTestId("pr-checks").textContent).toBe("2 of 5 checks running")
    expect(screen.getByTestId("pr-link").getAttribute("href")).toBe(pr.url)
    expect(screen.getByTestId("status").textContent).toBe("PR draft · CI running")
    expect(screen.queryByRole("button", { name: "Open on GitHub" })).toBeNull()
  })

  test("a fixing row links the fix ticket", () => {
    renderThemed(
      <EpicCard
        row={row({
          state: "fixing-ci",
          pr: { ...pr, checks: "go-test failed" },
          actions: [{ kind: "open-ticket", label: "Open ticket", target: "gx:blog/10" }],
        })}
      />,
    )
    const link = screen.getByTestId("pr-link")
    expect(link.textContent).toBe("Fix ticket 10")
    expect(link.getAttribute("href")).toContain("gx:blog/10")
  })

  test("a row without a PR has no PR line", () => {
    renderThemed(<EpicCard row={row()} />)
    expect(screen.queryByTestId("pr-line")).toBeNull()
  })

  test("an unreachable GitHub adds a line and keeps the chip", () => {
    renderThemed(<EpicCard row={row({ pr: { number: 412, chip: "#412", unreachable: true } })} />)
    expect(screen.getByTestId("pr-unreachable").textContent).toBe("GitHub unreachable")
    expect(screen.getByTestId("pr-number").textContent).toBe("#412")
  })
})

describe("Create PR and Mark as merged", () => {
  const create = { kind: "create-pr", label: "Create PR", target: "gx:blog" } as const
  const markMerged = { kind: "mark-merged", label: "Mark as merged", target: "gx:blog" } as const
  const awaitingPR = (over: Partial<EpicListRow> = {}) =>
    needsYou({
      state: "awaiting-pr",
      status_words: "Implemented",
      state_pill: "Awaiting PR",
      reason: undefined,
      actions: [{ kind: "open-epic", label: "Open epic", target: "gx:blog" }, create],
      ...over,
    })

  test("an awaiting-PR card has Create PR as its one main button", () => {
    renderThemed(<EpicCard row={awaitingPR()} />)
    expect(text(screen.getByTestId("pill"))).toContain("Awaiting PR")
    expect(footerButtons().map(text)).toEqual(["Create PR"])
  })

  test("a merge-project card shows Create PR as the secondary button left of Merge", () => {
    const r = needsYou({
      state: "awaiting-merge",
      state_pill: "Awaiting merge",
      actions: [
        { kind: "open-epic", label: "Open epic", target: "gx:blog" },
        { kind: "merge", label: "Merge", target: "gx:blog" },
        create,
      ],
    })
    renderThemed(<EpicCard row={r} />)
    const buttons = footerButtons()
    expect(buttons.map(text)).toEqual(["Create PR", "Merge"])
    expect(buttons[0].className).not.toBe(buttons[1].className)
  })

  test("Create PR posts the address and shown state, with a spinner-free pending label", async () => {
    let finish!: (v: never) => void
    const createPR = vi.fn(() => new Promise<never>((res) => (finish = res)))
    const { container } = renderThemed(<EpicCard row={awaitingPR()} writes={{ createPR }} />)
    fireEvent.click(screen.getByRole("button", { name: "Create PR" }))
    expect(createPR).toHaveBeenCalledWith({ address: "gx:blog", state: "awaiting-pr" })
    expect(await screen.findByRole("button", { name: "Creating PR…" })).toBeTruthy()
    expect(container.querySelector(".chakra-spinner")).toBeNull()
    await act(async () => finish({} as never))
  })

  test("a failed gh check shows its reason", async () => {
    const createPR = vi
      .fn()
      .mockResolvedValue({ refused: true, reason: "gh", message: "gh is not signed in" })
    renderThemed(<EpicCard row={awaitingPR()} writes={{ createPR }} />)
    fireEvent.click(screen.getByRole("button", { name: "Create PR" }))
    expect(await screen.findByText("gh is not signed in")).toBeTruthy()
  })

  test("Mark as merged is on the menu when offered, asks first, then posts", async () => {
    const markMergedWrite = vi.fn().mockResolvedValue({})
    const onChanged = vi.fn()
    renderThemed(
      <>
        <EpicCard
          row={awaitingPR({ workspace: "Default", actions: [create, markMerged] })}
          writes={{ markMerged: markMergedWrite }}
          onChanged={onChanged}
        />
        <ConfirmHost />
      </>,
    )
    expect(footerButtons().map(text)).not.toContain("Mark as merged")
    fireEvent.click(screen.getByRole("button", { name: "More actions" }))
    fireEvent.click(await screen.findByText("Mark as merged"))
    expect(markMergedWrite).not.toHaveBeenCalled()
    // The shared confirm is a popover on the ⋯, not a Modal.
    const dialog = await screen.findByRole("alertdialog")
    expect(dialog.textContent).toContain("Do this only when its PR was merged outside gx.")
    await waitFor(() =>
      expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: /^Cancel/ })),
    )
    fireEvent.click(within(dialog).getByRole("button", { name: /^Mark as merged/ }))
    await waitFor(() =>
      expect(markMergedWrite).toHaveBeenCalledWith({ address: "gx:blog", state: "awaiting-pr" }),
    )
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
  })

  test("Esc closes the Mark as merged confirm without posting", async () => {
    const markMergedWrite = vi.fn().mockResolvedValue({})
    renderThemed(
      <>
        <EpicCard
          row={awaitingPR({ workspace: "Default", actions: [create, markMerged] })}
          writes={{ markMerged: markMergedWrite }}
        />
        <ConfirmHost />
      </>,
    )
    fireEvent.click(screen.getByRole("button", { name: "More actions" }))
    fireEvent.click(await screen.findByText("Mark as merged"))
    const dialog = await screen.findByRole("alertdialog")
    fireEvent.keyDown(dialog, { key: "Escape" })
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
    expect(markMergedWrite).not.toHaveBeenCalled()
  })

  test("a card without the actions shows neither", async () => {
    renderThemed(<EpicCard row={row({ workspace: "Default" })} />)
    expect(screen.queryByRole("button", { name: "Create PR" })).toBeNull()
    fireEvent.click(screen.getByRole("button", { name: "More actions" }))
    await screen.findByText("Hold")
    expect(screen.queryByText("Mark as merged")).toBeNull()
  })
})

describe("Stop on a Design card", () => {
  const live = (id: string, over: Partial<SessionInfo> = {}) =>
    session(id, "gx", { design: true, ticket: "gx:blog/00", ...over })
  const designCard = () => row({ card_kind: "design", workspace: "Default" })
  const menuItems = async () => {
    fireEvent.click(screen.getByRole("button", { name: "More actions" }))
    return (await screen.findAllByRole("menuitem")).map(text)
  }

  test("one live session shows Stop, and the confirm ends it", async () => {
    const fetchMock = vi.fn(
      async (_url: RequestInfo | URL, _init?: RequestInit) =>
        new Response(JSON.stringify({ results: [{ target: "s1", status: "done" }] }), {
          status: 200,
        }),
    )
    vi.stubGlobal("fetch", fetchMock)
    renderThemed(
      <>
        <EpicCard
          row={designCard()}
          sessions={[live("s1"), live("s2", { status: { state: "exited" } })]}
        />
        <ConfirmHost />
      </>,
    )
    expect(await menuItems()).toContain("Stop")
    fireEvent.click(screen.getByRole("menuitem", { name: "Stop" }))
    const dialog = await screen.findByRole("alertdialog")
    fireEvent.click(within(dialog).getByRole("button", { name: /^Stop/ }))
    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/sessions/stop"))
      expect(JSON.parse((call![1] as RequestInit).body as string).targets[0].id).toBe("s1")
    })
  })

  test.each([[[]], [[live("s1"), live("s2")]]])("%j live sessions hide Stop", async (sessions) => {
    renderThemed(<EpicCard row={designCard()} sessions={sessions} />)
    expect(await menuItems()).not.toContain("Stop")
  })
})
