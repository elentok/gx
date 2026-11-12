import { Box, chakra, Text } from "@chakra-ui/react"
import { useEffect, useRef, useState, type ReactNode } from "react"
import { useNavigate } from "react-router"
import { useConnectionContext } from "../connection"
import type { ScheduleShowResult } from "../generated/server"
import { DetailPage } from "../page/DetailPage"
import { HeaderLink } from "../page/HeaderLink"
import { Button } from "../ui/button/Button"
import { PromptBox } from "../ui/expandable-text/PromptBox"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { useFetched } from "../useFetched"
import { refusalText, useWrite } from "../useWrite"
import {
  DAYS,
  formOf,
  harnessOptions,
  isDirty,
  newForm,
  refusalField,
  SHORTCUTS,
  toCreateRequest,
  toEditRequest,
  toggleDay,
  validate,
  type FormErrors,
  type ScheduleForm,
} from "./form"
import {
  fetchScheduleShow,
  fetchSchedules,
  postCreate,
  postEdit,
  type FetchSchedules,
  type FetchScheduleShow,
  type PostCreate,
  type PostEdit,
} from "./schedules"

const DAY_LABEL: Record<string, string> = {
  mon: "Mon",
  tue: "Tue",
  wed: "Wed",
  thu: "Thu",
  fri: "Fri",
  sat: "Sat",
  sun: "Sun",
}

const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface.raised",
  borderWidth: "1px",
  borderColor: "hairline",
  borderRadius: "8px",
  px: "3",
  h: "40px",
} as const

function Row({
  part,
  label,
  error,
  hint,
  children,
}: {
  part: string
  label: string
  error?: string
  hint?: string
  children: ReactNode
}) {
  return (
    <Box
      data-mock-part={part}
      display="grid"
      gridTemplateColumns={{ base: "1fr", md: "120px 1fr" }}
      gap="2"
      py="4"
      borderBottomWidth="1px"
      borderColor="border"
    >
      <Text color="text.muted" fontSize="14px" pt="2">
        {label}
      </Text>
      <Box minW="0">
        {children}
        {hint && (
          <Text color="text.muted" fontSize="13px" mt="1">
            {hint}
          </Text>
        )}
        {error && (
          <Text role="alert" color="tone.error.fg" fontSize="13px" mt="1">
            {error}
          </Text>
        )}
      </Box>
    </Box>
  )
}

function Editor({
  base,
  setBase,
  existing,
  projects,
  defaultHarness,
  postCreateFn,
  postEditFn,
}: {
  base: ScheduleForm
  setBase: (f: ScheduleForm) => void
  existing?: { project: string; slug: string }
  projects: string[]
  defaultHarness: string
  postCreateFn: PostCreate
  postEditFn: PostEdit
}) {
  const navigate = useNavigate()
  const write = useWrite()
  const [form, setForm] = useState(base)
  const [tried, setTried] = useState(false)
  const set = (patch: Partial<ScheduleForm>) => {
    setForm((f) => ({ ...f, ...patch }))
  }
  // A fresh answer replaces the form unless the person has edits in it.
  const prevBase = useRef(base)
  useEffect(() => {
    const prev = prevBase.current
    prevBase.current = base
    setForm((f) => (isDirty(f, prev) ? f : base))
  }, [base])
  const dirty = isDirty(form, base)
  const errors: FormErrors = validate(form)
  const shown: FormErrors = tried ? errors : {}
  const refusedAt = refusalField(write.refusal?.reason)
  const inline = (k: keyof FormErrors) =>
    shown[k] ?? (refusedAt === k ? refusalText(write.refusal!) : undefined)
  const save = () => {
    setTried(true)
    if (Object.keys(errors).length > 0) return
    const onDone = (res: ScheduleShowResult) => {
      const saved = formOf(res)
      if (!existing) {
        navigate(`/schedules/${res.schedule.project}:${res.schedule.slug}`)
        return
      }
      setBase(saved)
      setForm(saved)
      setTried(false)
    }
    if (existing)
      void write.run(() => postEditFn(toEditRequest(existing.project, existing.slug, form)), {
        onDone,
      })
    else void write.run(() => postCreateFn(toCreateRequest(form)), { onDone })
  }
  const discard = () => {
    setForm(base)
    setTried(false)
  }
  const choices = projects.includes(form.project) ? projects : [form.project, ...projects]
  return (
    <Box>
      <Box data-mock-part="back-link" pt="4">
        <chakra.a href="#/schedules" color="text.muted" fontSize="14px" _hover={{ color: "text" }}>
          ← Scheduled prompts
        </chakra.a>
      </Box>
      <Box data-mock-part="name" mt="4">
        <chakra.input
          aria-label="Name"
          name="schedule-name"
          autoComplete="off"
          placeholder="Name"
          value={form.name}
          onChange={(e) => set({ name: e.target.value })}
          w="100%"
          bg="transparent"
          fontSize="24px"
          fontWeight="600"
          color="text"
          borderBottomWidth="1px"
          borderColor="hairline"
          py="1"
        />
        {inline("name") && (
          <Text role="alert" color="tone.error.fg" fontSize="13px" mt="1">
            {inline("name")}
          </Text>
        )}
      </Box>

      <Row part="row-days" label="Days" error={inline("days")}>
        <Box data-mock-part="days" display="flex" flexWrap="wrap" gap="2" alignItems="center">
          {DAYS.map((d) => (
            <chakra.button
              key={d}
              type="button"
              aria-pressed={form.days.includes(d)}
              onClick={() => set({ days: toggleDay(form.days, d) })}
              h="32px"
              px="3"
              borderRadius="full"
              borderWidth="1px"
              borderColor={form.days.includes(d) ? "tone.running.fg" : "border"}
              bg={form.days.includes(d) ? "tone.running.bg" : "transparent"}
              color={form.days.includes(d) ? "tone.running.fg" : "text.muted"}
            >
              {DAY_LABEL[d]}
            </chakra.button>
          ))}
          <Box flexBasis="100%" display="flex" gap="3">
            {SHORTCUTS.map((s) => (
              <chakra.button
                key={s.label}
                type="button"
                color="text.muted"
                fontSize="14px"
                _hover={{ color: "text" }}
                onClick={() => set({ days: s.days })}
              >
                {s.label}
              </chakra.button>
            ))}
          </Box>
        </Box>
      </Row>

      <Row
        part="row-times"
        label="Times"
        hint="Local time on this machine."
        error={inline("times")}
      >
        <Box data-mock-part="times" display="flex" flexWrap="wrap" gap="2" alignItems="center">
          {form.times.map((t, i) => (
            <Box key={i} display="flex" alignItems="center" gap="1">
              <chakra.input
                {...field}
                aria-label={`Time ${i + 1}`}
                name={`schedule-time-${i}`}
                autoComplete="off"
                placeholder="HH:MM"
                value={t}
                onChange={(e) =>
                  set({ times: form.times.map((x, j) => (j === i ? e.target.value : x)) })
                }
                w="88px"
              />
              <Button
                emphasis="ghost"
                size="sm"
                tone="muted"
                aria-label={`Remove time ${i + 1}`}
                onClick={() => set({ times: form.times.filter((_, j) => j !== i) })}
              >
                ×
              </Button>
            </Box>
          ))}
          <Button
            emphasis="secondary"
            size="sm"
            onClick={() => set({ times: [...form.times, ""] })}
          >
            Add time
          </Button>
        </Box>
      </Row>

      <Row part="row-project" label="Project">
        {existing ? (
          <Text pt="2">{form.project}</Text>
        ) : (
          <chakra.select
            {...field}
            aria-label="Project"
            value={form.project}
            onChange={(e) => set({ project: e.target.value })}
          >
            {choices.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </chakra.select>
        )}
      </Row>

      <Row part="row-harness" label="Harness">
        <chakra.select
          {...field}
          aria-label="Harness"
          value={form.harness}
          onChange={(e) => set({ harness: e.target.value })}
        >
          {harnessOptions(defaultHarness).map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </chakra.select>
      </Row>

      <Row
        part="row-late-limit"
        label="Late limit"
        hint="hours · a missed run still starts within this"
        error={inline("lateLimit")}
      >
        <chakra.input
          {...field}
          aria-label="Late limit"
          name="schedule-late-limit"
          autoComplete="off"
          inputMode="decimal"
          value={form.lateLimit}
          onChange={(e) => set({ lateLimit: e.target.value })}
          w="88px"
        />
      </Row>

      <Row
        part="row-prompt"
        label="Prompt"
        hint={'The agent can run gx notify "…" to message you.'}
        error={inline("prompt")}
      >
        <Box data-mock-part="prompt-box">
          <PromptBox
            rows={8}
            label="Prompt"
            name="schedule-prompt"
            title="Prompt"
            value={form.prompt}
            onChange={(prompt) => set({ prompt })}
          />
        </Box>
      </Row>

      {write.refusal && !refusedAt && (
        <Text role="alert" color="tone.error.fg" fontSize="14px" mt="2">
          {refusalText(write.refusal)}
        </Text>
      )}

      {(dirty || !existing) && (
        <Box
          data-mock-part="save-bar"
          display="flex"
          alignItems="center"
          gap="3"
          mt="6"
          px="4"
          py="3"
          borderWidth="1px"
          borderColor="border"
          borderRadius="12px"
          bg="surface.raised"
        >
          {existing && (
            <Text color="text.muted" flex="1">
              Unsaved changes
            </Text>
          )}
          {!existing && <Box flex="1" />}
          {dirty && (
            <Button emphasis="secondary" onClick={discard} disabled={write.pending}>
              Discard
            </Button>
          )}
          <Button onClick={save} disabled={write.pending}>
            {existing ? "Save" : "Create"}
          </Button>
        </Box>
      )}
    </Box>
  )
}

/** The Schedule page: `address` is "<project>:<slug>", or nothing for a new Schedule. */
export function SchedulePage({
  address,
  fetchList = fetchSchedules,
  fetchShow = fetchScheduleShow,
  create = postCreate,
  edit = postEdit,
}: {
  address?: string
  fetchList?: FetchSchedules
  fetchShow?: FetchScheduleShow
  create?: PostCreate
  edit?: PostEdit
}) {
  return (
    <DetailPage title={{ kind: "Scheduled prompts", name: address ? address : "New" }}>
      {address ? (
        <Existing address={address} fetchShow={fetchShow} edit={edit} create={create} />
      ) : (
        <Fresh fetchList={fetchList} create={create} edit={edit} />
      )}
    </DetailPage>
  )
}

function Fresh({
  fetchList,
  create,
  edit,
}: {
  fetchList: FetchSchedules
  create: PostCreate
  edit: PostEdit
}) {
  const conn = useConnectionContext()
  const { data, failed } = useFetched("schedules", fetchList, conn)
  const [base, setBase] = useState(newForm)
  if (failed) return <Text mt="6">Could not load the projects</Text>
  if (!data) return <FullScreenLoader label="Loading Schedule" />
  const defaultHarness = data.schedules.find((s) => s.default_harness)?.default_harness ?? ""
  return (
    <Editor
      base={base}
      setBase={setBase}
      projects={data.projects}
      defaultHarness={defaultHarness}
      postCreateFn={create}
      postEditFn={edit}
    />
  )
}

function Existing({
  address,
  fetchShow,
  create,
  edit,
}: {
  address: string
  fetchShow: FetchScheduleShow
  create: PostCreate
  edit: PostEdit
}) {
  const conn = useConnectionContext()
  const i = address.indexOf(":")
  const project = address.slice(0, i)
  const slug = address.slice(i + 1)
  const { data, failed } = useFetched("schedule", () => fetchShow(project, slug), conn, address)
  const [base, setBase] = useState<ScheduleForm>()
  const loaded = data && !data.refused ? formOf(data) : undefined
  const loadedKey = JSON.stringify(loaded)
  useEffect(() => {
    if (loaded) setBase(loaded)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loadedKey])
  if (i < 0 || failed) return <Text mt="6">Could not load the Schedule</Text>
  if (data?.refused)
    return (
      <Box>
        <Box pt="4">
          <HeaderLink href="#/schedules">← Scheduled prompts</HeaderLink>
        </Box>
        <Text mt="6" color="text.muted">
          {refusalText(data)}
        </Text>
      </Box>
    )
  if (!data || !base) return <FullScreenLoader label="Loading Schedule" />
  return (
    <Editor
      key={address}
      base={base}
      setBase={setBase}
      existing={{ project, slug }}
      projects={[]}
      defaultHarness={data.schedule.default_harness}
      postCreateFn={create}
      postEditFn={edit}
    />
  )
}
