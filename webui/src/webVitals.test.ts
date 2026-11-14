import { beforeEach, expect, test, vi } from "vitest"
import { actionName } from "./gxActions"
import { resetRouteSpans } from "./routeSpans"
import { chunkName, startWebVitals, toSample } from "./webVitals"

const mocks = vi.hoisted(() => ({
  queueSample: vi.fn(),
  afterQueueEnabled: vi.fn((fn: () => void) => fn()),
  onLCP: vi.fn(),
  onINP: vi.fn(),
  onCLS: vi.fn(),
  onFCP: vi.fn(),
}))

vi.mock("./webPerf", () => ({
  queueSample: mocks.queueSample,
  afterQueueEnabled: mocks.afterQueueEnabled,
}))
vi.mock("web-vitals/attribution", () => ({
  onLCP: mocks.onLCP,
  onINP: mocks.onINP,
  onCLS: mocks.onCLS,
  onFCP: mocks.onFCP,
}))

beforeEach(() => {
  resetRouteSpans()
  vi.clearAllMocks()
})

const inpAttribution = (over: Record<string, unknown> = {}) => ({
  interactionTarget: "sidebar-row",
  interactionType: "pointer",
  inputDelay: 10,
  processingDuration: 20,
  presentationDelay: 30,
  longAnimationFrameEntries: [],
  // Fields the library returns that must never reach a sample.
  url: "https://x.test/secret",
  loadState: "complete",
  navigationEntry: {},
  ...over,
})

const inp = (value: number, attribution = inpAttribution(), navigationType = "navigate") =>
  ({ name: "INP", value, navigationType, attribution }) as never

const lcp = (value: number, navigationType = "navigate") =>
  ({
    name: "LCP",
    value,
    navigationType,
    attribution: {
      timeToFirstByte: 1,
      resourceLoadDelay: 2,
      resourceLoadDuration: 3,
      elementRenderDelay: 4,
      url: "https://x.test/img.png",
      lcpEntry: { url: "https://x.test/img.png" },
      element: "img#hero",
    },
  }) as never

test("good INP and LCP send the value only", () => {
  expect(toSample(inp(200))).toEqual({
    metric: "inp",
    value: 200,
    tab: "hard",
    navigation_type: "navigate",
  })
  expect(toSample(lcp(2500))).toEqual({
    metric: "lcp",
    value: 2500,
    tab: "hard",
    navigation_type: "navigate",
  })
})

test("poor INP keeps attribution and nothing else", () => {
  const frames = [
    { duration: 60, scripts: [{ duration: 50, sourceURL: "https://x.test/assets/a-AAAAAAAA.js" }] },
    {
      duration: 120,
      scripts: [
        { duration: 5, sourceURL: "https://x.test/assets/a-AAAAAAAA.js" },
        { duration: 90, sourceURL: "https://x.test/assets/stats-page-BBBBBBBB.js" },
      ],
    },
  ]
  const sample = toSample(
    inp(201, inpAttribution({ longAnimationFrameEntries: frames })),
    new Set(["stats-page"]),
  )
  expect(sample).toEqual({
    metric: "inp",
    value: 201,
    tab: "hard",
    navigation_type: "navigate",
    input_delay: 10,
    processing: 20,
    presentation_delay: 30,
    interaction_type: "pointer",
    action: "sidebar-row",
    worst_frame: 120,
    worst_chunk: "stats-page",
  })
  expect(JSON.stringify(sample)).not.toMatch(/x\.test|secret|complete/)
})

test("a poor INP without Long Animation Frames omits the frame fields", () => {
  const sample = toSample(inp(300, inpAttribution({ longAnimationFrameEntries: undefined })))
  expect(sample).not.toHaveProperty("worst_frame")
  expect(sample).not.toHaveProperty("worst_chunk")
  expect(sample).toHaveProperty("input_delay", 10)
})

test("poor LCP keeps only the four timings", () => {
  const sample = toSample(lcp(2501))
  expect(sample).toEqual({
    metric: "lcp",
    value: 2501,
    tab: "hard",
    navigation_type: "navigate",
    ttfb: 1,
    lcp_load_delay: 2,
    lcp_load_duration: 3,
    lcp_render_delay: 4,
  })
  expect(JSON.stringify(sample)).not.toMatch(/x\.test|hero/)
})

test("a soft-navigation metric counts under route changes", () => {
  expect(toSample(lcp(100, "soft-navigation"))).toEqual({
    metric: "lcp",
    value: 100,
    tab: "route",
    navigation_type: undefined,
  })
})

test("chunk names outside the allowlist report other", () => {
  const chunks = new Set(["stats-page"])
  expect(chunkName("https://x.test/assets/stats-page-AbCd1234.js", chunks)).toBe("stats-page")
  expect(chunkName("https://x.test/assets/secret-AbCd1234.js", chunks)).toBe("other")
  expect(chunkName("https://x.test/src/stats.tsx", chunks)).toBe("other")
  expect(chunkName("https://x.test/assets/stats-page-AbCd1234.js?x=1", chunks)).toBe("stats-page")
})

test("action names outside the allowlist, or missing, report other", () => {
  document.body.innerHTML = `
    <div data-gx-action="palette"><span id="in">x</span></div>
    <div data-gx-action="not-listed"><span id="bad">x</span></div>
    <span id="none">x</span>`
  expect(actionName(document.getElementById("in"))).toBe("palette")
  expect(actionName(document.getElementById("bad"))).toBe("other")
  expect(actionName(document.getElementById("none"))).toBe("other")
  expect(actionName(null)).toBe("other")
})

test("startWebVitals turns soft navigation reporting on for LCP, INP and CLS only", () => {
  startWebVitals()
  expect(mocks.onLCP).toHaveBeenCalledWith(
    expect.any(Function),
    expect.objectContaining({ reportSoftNavs: true }),
  )
  expect(mocks.onINP).toHaveBeenCalledWith(
    expect.any(Function),
    expect.objectContaining({ reportSoftNavs: true }),
  )
  expect(mocks.onCLS).toHaveBeenCalledWith(expect.any(Function), { reportSoftNavs: true })
  expect(mocks.onFCP).toHaveBeenCalledWith(expect.any(Function))
})

test("a metric callback queues the sample", () => {
  startWebVitals()
  const report = mocks.onFCP.mock.calls[0][0]
  report({ name: "FCP", value: 900, navigationType: "reload" })
  expect(mocks.queueSample).toHaveBeenCalledWith({
    metric: "fcp",
    value: 900,
    tab: "hard",
    navigation_type: "reload",
  })
})
