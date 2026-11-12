import { apiFetch } from "../apiFetch"
import type {
  ScheduleCreateRequest,
  ScheduleDeleteRequest,
  ScheduleDeleteResult,
  ScheduleEditRequest,
  ScheduleRunNowRequest,
  ScheduleRunNowResult,
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
  <Req, Res = ScheduleShowResult>(path: string) =>
  async (body: Req): Promise<Res> => {
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

export type PostRunNow = (project: string, slug: string) => Promise<ScheduleRunNowResult>

export const postRunNow: PostRunNow = (project, slug) =>
  postTo<ScheduleRunNowRequest, ScheduleRunNowResult>("/v1/schedules/run-now")({
    project,
    slug,
  })

export type PostDelete = (project: string, slug: string) => Promise<ScheduleDeleteResult>

export const postDelete: PostDelete = (project, slug) =>
  postTo<ScheduleDeleteRequest, ScheduleDeleteResult>("/v1/schedules/delete")({
    project,
    slug,
  })

export type PostCreate = (req: ScheduleCreateRequest) => Promise<ScheduleShowResult>
export const postCreate: PostCreate = postTo<ScheduleCreateRequest>("/v1/schedules/create")

export type PostEdit = (req: ScheduleEditRequest) => Promise<ScheduleShowResult>
export const postEdit: PostEdit = postTo<ScheduleEditRequest>("/v1/schedules/edit")
