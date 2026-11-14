import { rm } from "node:fs/promises"
import { join } from "node:path"
import { expect, test } from "./fixture"
import { ask, go } from "./agentsKit"

test("204. Hosted files, wide", async ({ page, start }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  const fixture = await start("host-files")
  const hf = fixture.hostFiles!
  const plan = join(hf.repo, "docs/plans/host-files.md")

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
  const sessionId = (await fixture.cli("agents", "list")).split("\n")[0].split("\t")[0]
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

  // A second start in the same test works; the absolute path still links on the other shape.
  // Its relative path should not link (no checked-out trunk), but the fixture repo is not bare,
  // so the project base still resolves: see the ticket's Spec deviations.
  const bare = await start("host-files", { shape: "no-main-worktree" })
  await go(page, `#/t/${bare.hostFiles!.ticket}`)
  await expect(ticketLink(join(bare.hostFiles!.repo, ".scratch/flow.mmd"))).toBeVisible()
})
