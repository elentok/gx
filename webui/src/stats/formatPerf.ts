import { sizeText } from "../health/formatBytes"

export type PerfUnit = "ms" | "cls" | "bytes"

export function perfUnit(metric: string): PerfUnit {
  if (metric === "cls") return "cls"
  return metric.endsWith("_bytes") ? "bytes" : "ms"
}

export function formatPerf(v: number, unit: PerfUnit): string {
  if (unit === "cls") return v.toFixed(2)
  if (unit === "bytes") return sizeText(v, "decimal")
  return v >= 1000 ? `${+(v / 1000).toFixed(2)} s` : `${Math.round(v)} ms`
}
