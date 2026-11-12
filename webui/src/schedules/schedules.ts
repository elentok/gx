import { apiFetch } from "../apiFetch"
import type { ScheduleShowResult, SchedulesResult } from "../generated/server"
import { WriteError } from "../useWrite"

export type FetchSchedules = () => Promise<SchedulesResult>

export const fetchSchedules: FetchSchedules = async () => {
  const res = await apiFetch("/v1/schedules")
  if (!res.ok) throw new Error(`schedules: HTTP ${res.status}`)
  return res.json()
}

export type PostSwitch = (project: string, slug: string, on: boolean) => Promise<ScheduleShowResult>

export const postSwitch: PostSwitch = async (project, slug, on) => {
  const res = await apiFetch("/v1/schedules/switch", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ project, slug, on }),
  })
  if (!res.ok)
    throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`)
  return res.json()
}
