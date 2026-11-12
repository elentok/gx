import { apiFetch } from "../apiFetch"
import type {
  ScheduleCreateRequest,
  ScheduleEditRequest,
  ScheduleShowResult,
  SchedulesResult,
  ScheduleSwitchRequest,
} from "../generated/server"
import { WriteError } from "../useWrite"

export type FetchSchedules = () => Promise<SchedulesResult>

export const fetchSchedules: FetchSchedules = async () => {
  const res = await apiFetch("/v1/schedules")
  if (!res.ok) throw new Error(`schedules: HTTP ${res.status}`)
  return res.json()
}

export type FetchScheduleShow = (project: string, slug: string) => Promise<ScheduleShowResult>

export const fetchScheduleShow: FetchScheduleShow = async (project, slug) => {
  const q = new URLSearchParams({ project, slug })
  const res = await apiFetch(`/v1/schedules/show?${q}`)
  if (!res.ok) throw new Error(`schedule: HTTP ${res.status}`)
  return res.json()
}

const postTo =
  <Req>(path: string) =>
  async (body: Req): Promise<ScheduleShowResult> => {
    const res = await apiFetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })
    if (!res.ok)
      throw new WriteError((await res.text().catch(() => "")).trim() || `HTTP ${res.status}`)
    return res.json()
  }

export type PostSwitch = (project: string, slug: string, on: boolean) => Promise<ScheduleShowResult>

export const postSwitch: PostSwitch = (project, slug, on) =>
  postTo<ScheduleSwitchRequest>("/v1/schedules/switch")({ project, slug, on })

export type PostCreate = (req: ScheduleCreateRequest) => Promise<ScheduleShowResult>
export const postCreate: PostCreate = postTo<ScheduleCreateRequest>("/v1/schedules/create")

export type PostEdit = (req: ScheduleEditRequest) => Promise<ScheduleShowResult>
export const postEdit: PostEdit = postTo<ScheduleEditRequest>("/v1/schedules/edit")
