// Lazy chunk: measures LCP, INP, CLS and FCP and queues them. Loaded by main.tsx after first paint.
import {
  onCLS,
  onFCP,
  onINP,
  onLCP,
  type CLSMetric,
  type FCPMetric,
  type INPMetricWithAttribution,
  type LCPMetricWithAttribution,
} from "web-vitals/attribution"
import { actionName } from "./gxActions"
import { hardLoadDropped, hardLoadNavigationType, toNavigationType } from "./routeSpans"
import { afterQueueEnabled, queueSample, type WebPerfInput } from "./webPerf"

const POOR_INP_MS = 200
const POOR_LCP_MS = 2500

// vite.config.ts defines __GX_CHUNKS__ as the allowlist of chunk names.
const CHUNKS: ReadonlySet<string> = new Set(__GX_CHUNKS__.split(","))

type Metric = INPMetricWithAttribution | LCPMetricWithAttribution | CLSMetric | FCPMetric

export function chunkName(sourceURL: string, chunks: ReadonlySet<string> = CHUNKS): string {
  const name = /\/assets\/(.+)-[\w-]{8}\.js(?:$|\?)/.exec(sourceURL)?.[1]
  return name && chunks.has(name) ? name : "other"
}

type Frame = PerformanceLongAnimationFrameTiming

function worstFrame(
  frames: Frame[] | undefined,
  chunks: ReadonlySet<string>,
): Pick<WebPerfInput, "worst_frame" | "worst_chunk"> {
  if (!frames?.length) return {}
  const frame = frames.reduce((a, b) => (b.duration > a.duration ? b : a))
  const script = frame.scripts?.reduce((a, b) => (b.duration > a.duration ? b : a))
  return {
    worst_frame: frame.duration,
    ...(script && { worst_chunk: chunkName(script.sourceURL, chunks) }),
  }
}

function navigationType(metric: Metric, hard: boolean): WebPerfInput["navigation_type"] {
  if (!hard) return undefined
  if (hardLoadNavigationType() === "back-forward-cache") return "back-forward-cache"
  return toNavigationType(metric.navigationType)
}

/** Builds the sample for a metric, or undefined when it must not be sent. */
export function toSample(
  metric: Metric,
  chunks: ReadonlySet<string> = CHUNKS,
): WebPerfInput | undefined {
  const hard = metric.navigationType !== "soft-navigation"
  if (hard && hardLoadDropped()) return undefined
  const sample: WebPerfInput = {
    metric: metric.name.toLowerCase() as "lcp" | "inp" | "cls" | "fcp",
    value: metric.value,
    tab: hard ? "hard" : "route",
    navigation_type: navigationType(metric, hard),
  }
  if (metric.name === "INP" && metric.value > POOR_INP_MS) {
    const a = metric.attribution
    Object.assign(sample, {
      input_delay: a.inputDelay,
      processing: a.processingDuration,
      presentation_delay: a.presentationDelay,
      interaction_type: a.interactionType,
      // generateTarget (below) already turned the element into an action name.
      action: a.interactionTarget ?? "other",
      ...worstFrame(a.longAnimationFrameEntries, chunks),
    })
  } else if (metric.name === "LCP" && metric.value > POOR_LCP_MS) {
    const a = metric.attribution
    Object.assign(sample, {
      ttfb: a.timeToFirstByte,
      lcp_load_delay: a.resourceLoadDelay,
      lcp_load_duration: a.resourceLoadDuration,
      lcp_render_delay: a.elementRenderDelay,
    })
  }
  return sample
}

function report(metric: Metric): void {
  const sample = toSample(metric)
  if (sample) queueSample(sample)
}

/** Starts the observers once the auth gate has passed. */
export function startWebVitals(): void {
  afterQueueEnabled(() => {
    // generateTarget replaces the library's CSS selector, so no selector is ever built.
    const opts = { reportSoftNavs: true, generateTarget: actionName }
    onLCP(report, opts)
    onINP(report, opts)
    onCLS(report, { reportSoftNavs: true })
    onFCP(report)
  })
}
