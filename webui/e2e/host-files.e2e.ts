import { rm } from "node:fs/promises"
import { join } from "node:path"
import { expect, test, touch, type Fixture } from "./fixture"
import { ask, go } from "./agentsKit"

// The id of the session that runs in dir.
async function sessionIn(fixture: Fixture, dir: string): Promise<string> {
  const list: { id: string; dir: string }[] = JSON.parse(
    await fixture.cli("agents", "list", "--json"),
  )
  const found = list.find((s) => s.dir === dir)
  if (!found) throw new Error(`no session in ${dir}`)
  return found.id
}

test("204. Hosted files, wide", async ({ page, start }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  const fixture = await start("host-files")
  const hf = fixture.hostFiles!
  const plan = join(hf.repo, "docs/plans/host-files.md")

  // The Pele bundle (the `pele/lazy` chunk) is requested only when a diagram shows: a Markdown file without one asks for none.
  const bundleRequests: string[] = []
  page.on("request", (r) => {
    if (/\/assets\/lazy-/.test(new URL(r.url()).pathname)) bundleRequests.push(r.url())
  })
  await go(page, `#/files${join(hf.repo, "docs/plans/other.md")}`)
  await expect(page.getByRole("heading", { name: "Other", level: 1 })).toBeVisible()
  expect(bundleRequests).toHaveLength(0)

  // Steps 1: the CLI prints a stable viewer link; the viewer shows the Markdown.
  const link = (await fixture.cli("files", "url", plan)).trim()
  expect(link).toMatch(/^https:\/\//)
  const hash = new URL(link).hash
  expect(hash).toContain("#/files/")
  await go(page, hash)
  await expect(page.getByRole("heading", { name: "Host files plan", level: 1 })).toBeVisible()

  const raw = new URL(link)
  raw.hash = ""
  raw.pathname = `/v1/files${plan}`
  const res = await page.context().request.get(raw.toString())
  expect(res.status()).toBe(200)
  expect(res.headers()["content-type"]).toContain("text/plain")

  // The stable link keeps working.
  await go(page, "#/")
  await go(page, hash)
  await expect(page.getByRole("heading", { name: "Host files plan", level: 1 })).toBeVisible()

  // Step 2: the outside symlink, .env and a missing path look the same.
  for (const name of ["outside-link", ".env", "nope/missing.md"]) {
    await go(page, `#/files${join(hf.repo, name)}`)
    await expect(page.getByText("File not found")).toBeVisible()
  }
  await expect(fixture.cli("files", "url", join(hf.repo, ".env"))).rejects.toThrow(
    /path-not-allowed/,
  )
  const shot = (await fixture.cli("files", "url", join(hf.temp_dir, "shot-1.png"))).trim()
  expect(shot).toMatch(/^https:\/\//)

  // Step 3: the HTML mock runs in the sandboxed frame.
  const mock = join(hf.repo, "mocks/04-viewer.html")
  await go(page, `#/files${mock}`)
  const frame = page.frameLocator("iframe")
  await expect(frame.locator("body[data-done='1']")).toBeAttached()
  const results = JSON.parse((await frame.locator("#results").textContent()) ?? "{}")
  expect(results).toMatchObject({
    sameOriginFetch: "ok",
    snapshot: "failed",
    otherOriginFetch: "failed",
    otherScript: "loaded",
    image: "loaded",
    style: "rgb(1, 2, 3)",
  })
  const mockFrame = page.frames().find((f) => f.url().includes("/v1/files/"))!
  expect(mockFrame).toBeDefined()
  // The frame's origin is opaque, so the browser blocks the reply and the network log shows only a
  // failed request. The write the frame would send carries `Origin: null`; send it with the cookie.
  const write = await page
    .context()
    .request.post(new URL("/v1/hosted-files/resolve", page.url()).href, {
      headers: { Origin: "null" },
      data: { paths: [] },
    })
  expect(write.status()).toBe(403)
  const rawMock = new URL(page.url())
  rawMock.hash = ""
  rawMock.pathname = `/v1/files${mock}`
  const second = await page.context().newPage()
  await second.goto(rawMock.toString())
  await expect(second).toHaveURL(/\/v1\/files\/t\//)
  await expect(second.locator("#title")).toHaveText("Viewer mock")
  await second.close()

  // Step 4: links in the chat.
  const sessionId = await sessionIn(fixture, hf.repo)
  await go(page, `#/agents/${sessionId}`)
  await expect(page.getByText("Which file next?").first()).toBeVisible()
  const resolves: string[] = []
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().endsWith("/v1/hosted-files/resolve"))
      resolves.push(r.url())
  })
  await go(page, "#/")
  await go(page, `#/agents/${sessionId}`)
  await expect(page.getByText("I wrote the plan to")).toBeVisible()
  // Assistant message, code span, mock and screenshot paths.
  const message = page
    .locator("p, div")
    .filter({ hasText: /^I wrote the plan to/ })
    .last()
  await expect(message.getByRole("link", { name: "docs/plans/host-files.md" })).toBeVisible()
  await expect(message.getByRole("link", { name: "~/dev/gx/.scratch/flow.mmd" })).toBeVisible()
  await expect(message.getByRole("link", { name: "mocks/04-viewer.html" })).toBeVisible()
  await expect(message.getByRole("link", { name: hf.temp_dir + "/shot-1.png" })).toBeVisible()
  // Tool line, the person's prompt and the question card.
  await page.getByText("1 tool call").click()
  await expect(
    page.getByRole("link", { name: "server/files/handler.go", exact: true }),
  ).toBeVisible()
  const prompt = page
    .locator("p, div")
    .filter({ hasText: /^Please read docs/ })
    .last()
  await expect(prompt.getByRole("link", { name: "docs/plans/host-files.md" })).toBeVisible()
  const card = page
    .locator("p, div")
    .filter({ hasText: /^Open docs\/plans\/host-files.md or/ })
    .last()
  await expect(card.getByRole("link", { name: "docs/plans/host-files.md" })).toBeVisible()
  expect(resolves).toHaveLength(1)

  // A new message adds one resolve call; .env and a missing path stay plain text.
  await ask(fixture, sessionId, "Also look at .env and nope/missing.md and docs/plans/other.md")
  await expect(page.getByRole("link", { name: "docs/plans/other.md" })).toBeVisible()
  await expect(page.getByText("Also look at .env and nope/missing.md")).toBeVisible()
  await expect(page.getByRole("link", { name: ".env" })).toHaveCount(0)
  await expect(page.getByRole("link", { name: "nope/missing.md" })).toHaveCount(0)
  expect(resolves).toHaveLength(2)

  // Step 6 (before leaving the chat): a file deleted after the chat loaded shows as missing on hover.
  const gone = message.getByRole("link", { name: "notes/gone.md" })
  await expect(gone).not.toHaveAttribute("data-missing", "")
  await rm(join(hf.repo, "notes/gone.md"))
  await gone.hover()
  await expect(page.getByText("File not found")).toBeVisible()
  await expect(gone).toHaveAttribute("data-missing", "")
  await expect(gone).toHaveCSS("text-decoration-line", "none")
  await expect(gone).toHaveCSS("opacity", "0.55")

  // Step 5: handler.go:42 opens the viewer at that line.
  await message.getByRole("link", { name: "server/files/handler.go:42" }).click()
  await expect(page).toHaveURL(/#\/files\/.*\/server\/files\/handler\.go\?line=42$/)
  // The hover cards of the chat links, still mounted, hold a code view of their own.
  const code = page.locator("[data-testid='code-view']:not([id^='hover-card'] *)")
  await expect(code.locator("[data-line='42']")).toHaveAttribute("aria-current", "true")
  await expect(code.locator("[data-line='41']")).not.toHaveAttribute("aria-current", "true")

  // Step 7: the ticket page links a relative path against the main worktree, and an absolute one.
  await go(page, `#/t/${hf.ticket}`)
  const ticketLink = (name: string) => page.getByRole("link", { name, exact: true })
  await expect(ticketLink("docs/plans/host-files.md")).toBeVisible()
  await expect(ticketLink(join(hf.repo, ".scratch/flow.mmd"))).toBeVisible()

  // Step 8: hover cards, one per kind.
  await go(page, `#/agents/${sessionId}`)
  await ask(
    fixture,
    sessionId,
    "Previews: docs/plans/host-files.md .scratch/flow.mmd mocks/04-viewer.html docs/plans/pic.png server/files/handler.go:42 big.log doc.pdf blob.bin logo.svg",
  )
  const previews = page
    .locator("p, div")
    .filter({ hasText: /^Previews:/ })
    .last()
  // The cards of every chat link stay mounted, closed; only the open one counts.
  const hoverCard = page.locator("[data-file-hover-card][data-state='open']")
  const body = hoverCard.locator("[data-preview-body]")
  const previewLink = (name: string) => previews.getByRole("link", { name, exact: true })
  const hoverOver = async (name: string) => {
    const l = previewLink(name)
    await expect(l).toBeVisible()
    await l.hover()
    await expect(hoverCard).toBeVisible()
    return l
  }
  const leave = async () => {
    await page.mouse.move(0, 0)
    await expect(hoverCard).toHaveCount(0)
  }
  await hoverOver("docs/plans/host-files.md")
  await expect(body).toContainText("Host files plan")
  await leave()
  await hoverOver(".scratch/flow.mmd")
  await expect(body.locator("svg")).toBeVisible()
  await leave()
  await hoverOver("mocks/04-viewer.html")
  await expect(body).toHaveAttribute("data-preview-body", "html")
  await expect(body).toContainText("No live preview")
  await expect(hoverCard.locator("iframe")).toHaveCount(0)
  await leave()
  await hoverOver("docs/plans/pic.png")
  await expect(body.locator("img")).toBeVisible()
  await leave()
  await hoverOver("server/files/handler.go:42")
  await expect(body.locator("[data-line='42']")).toHaveAttribute("aria-current", "true")
  await leave()
  for (const name of ["big.log", "doc.pdf"]) {
    await hoverOver(name)
    await expect(body).toContainText("No preview")
    await leave()
  }
  // Keyboard focus opens the card; Escape closes it.
  await previewLink("big.log").focus()
  await expect(hoverCard).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(hoverCard).toHaveCount(0)

  // Step 9: the viewer, one kind at a time.
  const bar = page.getByTestId("viewer-bar")
  const view = (name: string) => go(page, `#/files${join(hf.repo, name)}`)
  const source = bar.getByRole("button", { name: "Source" })
  const rawLink = bar.getByRole("link", { name: "Raw" })
  const download = bar.getByRole("link", { name: "Download" })
  await view("docs/plans/other.md")
  await expect(page.getByRole("heading", { name: "Other", level: 1 })).toBeVisible()
  await expect(source).toHaveAttribute("aria-pressed", "false")
  await source.click()
  await expect(source).toHaveAttribute("aria-pressed", "true")
  await expect(page.getByTestId("code-view")).toContainText("## Install")
  await view(".scratch/flow.mmd")
  await expect(page.getByTestId("mermaid-view").locator("svg")).toBeVisible()
  expect(bundleRequests.length).toBeGreaterThan(0)
  await view("mocks/04-viewer.html")
  await expect(page.getByText("Sandboxed page, scripts allowed, no access to gx")).toBeVisible()
  await view("server/files/handler.go")
  await expect(page.getByTestId("code-view").locator("[data-line='42']")).toBeVisible()
  await view("logo.svg")
  await expect(page.locator("img[src*='logo.svg']")).toBeVisible()
  expect(await page.evaluate(() => (window as { svgRan?: boolean }).svgRan)).toBeUndefined()
  for (const name of ["big.log", "blob.bin"]) {
    await view(name)
    await expect(rawLink).toBeVisible()
    await expect(download).toBeVisible()
    await expect(source).toHaveCount(0)
  }

  // Step 10: doc.pdf has Raw and Download, no Source; Raw is a PDF with the sandbox header.
  await view("doc.pdf")
  await expect(rawLink).toBeVisible()
  await expect(download).toBeVisible()
  await expect(source).toHaveCount(0)
  const pdfUrl = new URL((await rawLink.getAttribute("href"))!, page.url()).toString()
  const pdf = await page.context().request.get(pdfUrl)
  expect(pdf.status()).toBe(200)
  expect(pdf.headers()["content-type"]).toContain("application/pdf")
  expect(pdf.headers()["content-security-policy"]).toContain("sandbox")
  // Opened as a top-level page, headless Chromium downloads the PDF instead of showing it (see the
  // ticket's Spec deviations), so the flow checks the reply, not what the browser draws.

  // Step 11: hosted Markdown links. A link is a real <a href>; the fragment link scrolls without a refetch.
  const fileFetches: string[] = []
  page.on("request", (r) => {
    if (r.url().includes("/v1/files/") && r.url().endsWith("/docs/plans/host-files.md"))
      fileFetches.push(r.url())
  })
  await view("docs/plans/host-files.md")
  const md = page.getByTestId("markdown-view")
  await expect(md.getByRole("heading", { name: "Host files plan", level: 1 })).toBeVisible()
  await expect(md.locator("svg[aria-roledescription='flowchart']")).toBeVisible()
  const handlerLink = md.getByRole("link", { name: "the handler" })
  await expect(handlerLink).toHaveAttribute("href", /#\/files\/.*\/server\/files\/handler\.go$/)
  const picture = md.getByRole("img", { name: "pic" })
  await expect(picture).toHaveAttribute("src", /\/v1\/files\//)
  await expect
    .poll(() => picture.evaluate((i: HTMLImageElement) => i.naturalWidth))
    .toBeGreaterThan(0)
  const fetchedBefore = fileFetches.length
  const install = md.getByRole("heading", { name: "Install steps" })
  await expect(install).not.toBeInViewport()
  await md.getByRole("link", { name: "the install steps" }).click()
  await expect(page).toHaveURL(/heading=install-steps/)
  await expect(install).toBeInViewport()
  expect(fileFetches).toHaveLength(fetchedBefore)
  await md.getByRole("link", { name: "other.md#install" }).click()
  await expect(page).toHaveURL(/docs\/plans\/other\.md\?heading=install/)
  await expect(page.getByRole("heading", { name: "Install", level: 2 })).toBeInViewport()

  // Step 12: Copy link puts the stable viewer link on the clipboard.
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
  await view("docs/plans/host-files.md")
  await bar.getByRole("button", { name: "Copy link" }).click()
  await expect(page.getByText("Link copied")).toBeVisible()
  const copied = await page.evaluate(() => navigator.clipboard.readText())
  expect(copied).toBe(link)

  // A second start in the same test works. In the bare layout no worktree has trunk checked out:
  // the absolute path still links, the relative one in the ticket does not, and a session in the
  // second worktree links the file only that worktree has.
  const bare = await start("host-files", { shape: "no-main-worktree" })
  const bareFiles = bare.hostFiles!
  await go(page, `#/t/${bareFiles.ticket}`)
  await expect(ticketLink(join(bareFiles.repo, ".scratch/flow.mmd"))).toBeVisible()
  await expect(ticketLink("docs/plans/host-files.md")).toHaveCount(0)
  await go(page, `#/agents/${await sessionIn(bare, bareFiles.second_worktree)}`)
  await expect(page.getByRole("link", { name: bareFiles.second_only, exact: true })).toBeVisible()
})

// The bottom sheet needs the real (hover: none) media query, so this flow gets its own touch context.
test.describe("205. Hosted files, phone", () => {
  test.use({ ...touch, viewport: { width: 390, height: 844 } })

  test("a tap opens the bottom sheet, Copy link copies the viewer link, Open goes to the viewer", async ({
    page,
    start,
  }) => {
    const fixture = await start("host-files")
    const hf = fixture.hostFiles!
    await go(page, "#/")
    expect(await page.evaluate(() => matchMedia("(hover: none)").matches)).toBe(true)

    const sessionId = await sessionIn(fixture, hf.repo)
    await go(page, `#/agents/${sessionId}`)
    await expect(page.getByText("I wrote the plan to")).toBeVisible()
    const message = page
      .locator("p, div")
      .filter({ hasText: /^I wrote the plan to/ })
      .last()
    const sheet = page.locator("[data-file-sheet]")
    const hoverCard = page.locator("[data-file-hover-card][data-state='open']")

    // Story 38: a tap opens the sheet with the preview, Open and Copy link; the page does not navigate.
    const before = page.url()
    await message.getByRole("link", { name: "docs/plans/host-files.md" }).tap()
    await expect(sheet).toBeVisible()
    await expect(sheet).toContainText("Host files plan")
    await expect(sheet.getByRole("link", { name: "Open" })).toBeVisible()
    await expect(sheet.getByRole("button", { name: "Copy link" })).toBeVisible()
    await expect(hoverCard).toHaveCount(0)
    expect(page.url()).toBe(before)

    // Story 39: Copy link puts the stable viewer link on the clipboard.
    await page.context().grantPermissions(["clipboard-read", "clipboard-write"])
    await sheet.getByRole("button", { name: "Copy link" }).tap()
    await expect(page.getByText("Link copied")).toBeVisible()
    const copied = await page.evaluate(() => navigator.clipboard.readText())
    const stable = (
      await fixture.cli("files", "url", join(hf.repo, "docs/plans/host-files.md"))
    ).trim()
    expect(copied).toBe(stable)
    expect(copied).toContain("#/files/")

    // Tap outside closes the sheet; a Mermaid file shows the same sheet with its own preview.
    await page.touchscreen.tap(195, 40)
    await expect(sheet).toHaveCount(0)
    await message.getByRole("link", { name: "~/dev/gx/.scratch/flow.mmd" }).tap()
    await expect(sheet).toBeVisible()
    await expect(sheet.locator("svg")).toBeVisible()
    await expect(sheet.getByRole("link", { name: "Open" })).toBeVisible()
    await page.touchscreen.tap(195, 40)
    await expect(sheet).toHaveCount(0)

    // Open goes to the viewer for that file.
    await message.getByRole("link", { name: "docs/plans/host-files.md" }).tap()
    await expect(sheet).toBeVisible()
    await sheet.getByRole("link", { name: "Open" }).tap()
    await expect(page).toHaveURL(/#\/files\/.*\/docs\/plans\/host-files\.md/)
    await expect(page.getByRole("heading", { name: "Host files plan", level: 1 })).toBeVisible()
  })
})
