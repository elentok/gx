import { Box, chakra, Text } from "@chakra-ui/react"
import { useEffect, useRef, useState, type ReactNode } from "react"
import { useNavigate } from "react-router"
import { useConnectionContext } from "../connection"
import type {
  ScheduleInfo,
  ScheduleRun,
  ScheduleRunNowResult,
  ScheduleShowResult,
} from "../generated/server"
import { ticketHref } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { HeaderLink } from "../page/HeaderLink"
import { Button } from "../ui/button/Button"
import { PromptBox } from "../ui/expandable-text/PromptBox"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { useFetched } from "../useFetched"
import { refusalText, useWrite, type Refusable } from "../useWrite"
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
import { LastRuns } from "./LastRuns"
import {
  fetchScheduleShow,
  fetchSchedules,
  postCreate,
  postDelete,
  postEdit,
  postRunNow,
  postSwitch,
  type FetchSchedules,
  type FetchScheduleShow,
  type PostCreate,
  type PostDelete,
  type PostEdit,
  type PostRunNow,
  type PostSwitch,
} from "./schedules"
import { Switch } from "./Switch"

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

interface ExistingTools {
  info: ScheduleInfo
  runs: ScheduleRun[]
  runNow: PostRunNow
  switchTo: PostSwitch
  remove: PostDelete
  refetch: () => void
}

function RunNowRefusal({ refusal, project }: { refusal: Refusable; project: string }) {
  const attention = (refusal as ScheduleRunNowResult).attention_address
  let text = refusalText(refusal)
  if (refusal.reason === "previous-run-started") text = "previous run started"
  else if (refusal.reason === "previous-run-needs-attention") text = "previous run needs attention"
  else if (refusal.reason === "unknown-project") text = `no project ${project} in the ticket store`
  return (
    <Text role="alert" color="tone.error.fg" fontSize="14px" mt="2">
      {text}
      {attention && (
        <>
          {" "}
          <chakra.a href={ticketHref(attention)} textDecoration="underline">
            {attention}
          </chakra.a>
        </>
      )}
    </Text>
  )
}

function ScheduleHeader({
  existing,
  tools,
}: {
  existing: { project: string; slug: string }
  tools: ExistingTools
}) {
  const runWrite = useWrite({ toastRefusal: false })
  const switchWrite = useWrite()
  const [answered, setAnswered] = useState<boolean>()
  useEffect(() => setAnswered(undefined), [tools.info.switch])
  const on = answered ?? tools.info.switch
  const runNow = () =>
    void runWrite.run(() => tools.runNow(existing.project, existing.slug), {
      onDone: tools.refetch,
    })
  const toggle = () =>
    void switchWrite.run(() => tools.switchTo(existing.project, existing.slug, !on), {
      onDone: (res) => {
        setAnswered(res.schedule.switch)
        tools.refetch()
      },
    })
  return (
    <Box data-mock-part="schedule-header" textAlign="right" flexShrink={0}>
      <Box display="flex" alignItems="center" justifyContent="flex-end" gap="3" minH="40px">
        {!tools.info.project_found && (
          <Text data-mock-part="next-run" color="text.muted" fontSize="14px">
            project not found
          </Text>
        )}
        <Button
          data-mock-part="run-now"
          emphasis="secondary"
          onClick={runNow}
          disabled={runWrite.pending}
        >
          Run now
        </Button>
        <Switch on={on} disabled={switchWrite.pending} onToggle={toggle} />
      </Box>
      {!on && (
        <Text color="text.muted" fontSize="13px" mt="1">
          Runs once now. The switch is off on this machine.
        </Text>
      )}
      {runWrite.refusal && <RunNowRefusal refusal={runWrite.refusal} project={existing.project} />}
    </Box>
  )
}

function DeleteSchedule({
  existing,
  remove,
}: {
  existing: { project: string; slug: string }
  remove: PostDelete
}) {
  const navigate = useNavigate()
  const write = useWrite()
  const [asking, setAsking] = useState(false)
  if (!asking)
    return (
      <Box data-mock-part="delete-link" mt="8" pb="4">
        <chakra.button
          type="button"
          color="text.muted"
          fontSize="14px"
          onClick={() => setAsking(true)}
        >
          Delete this schedule
        </chakra.button>
      </Box>
    )
  return (
    <Box data-mock-part="delete-link" mt="8" pb="4">
      <Text fontSize="14px">
        Delete this schedule? Queued runs that never started are cancelled. Live and paused runs
        continue.
      </Text>
      <Box display="flex" gap="3" mt="2">
        <Button
          tone="deadlocked"
          disabled={write.pending}
          onClick={() =>
            void write.run(() => remove(existing.project, existing.slug), {
              onDone: () => navigate("/schedules"),
            })
          }
        >
          Delete
        </Button>
        <Button emphasis="secondary" onClick={() => setAsking(false)}>
          Cancel
        </Button>
      </Box>
    </Box>
  )
}

function Editor({
  base,
  setBase,
  existing,
  tools,
  projects,
  defaultHarness,
  postCreateFn,
  postEditFn,
}: {
  base: ScheduleForm
  setBase: (f: ScheduleForm) => void
  existing?: { project: string; slug: string }
  tools?: ExistingTools
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
      <Box display="flex" alignItems="flex-start" gap="4" mt="4">
        <Box data-mock-part="name" flex="1" minW="0">
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
        {existing && tools && <ScheduleHeader existing={existing} tools={tools} />}
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
                  set({
                    times: form.times.map((x, j) => (j === i ? e.target.value : x)),
                  })
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

      {tools && <LastRuns runs={tools.runs} />}

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
      {existing && tools && <DeleteSchedule existing={existing} remove={tools.remove} />}
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
  runNow = postRunNow,
  switchTo = postSwitch,
  remove = postDelete,
}: {
  address?: string
  fetchList?: FetchSchedules
  fetchShow?: FetchScheduleShow
  create?: PostCreate
  edit?: PostEdit
  runNow?: PostRunNow
  switchTo?: PostSwitch
  remove?: PostDelete
}) {
  return (
    <DetailPage title={{ kind: "Scheduled prompts", name: address ? address : "New" }}>
      {address ? (
        <Existing
          address={address}
          fetchShow={fetchShow}
          edit={edit}
          create={create}
          post={{ runNow, switchTo, remove }}
        />
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
  post,
}: {
  address: string
  fetchShow: FetchScheduleShow
  create: PostCreate
  edit: PostEdit
  post: Pick<ExistingTools, "runNow" | "switchTo" | "remove">
}) {
  const conn = useConnectionContext()
  const i = address.indexOf(":")
  const project = address.slice(0, i)
  const slug = address.slice(i + 1)
  const { data, failed, retry } = useFetched(
    "schedule",
    () => fetchShow(project, slug),
    conn,
    address,
  )
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
      tools={{
        ...post,
        info: data.schedule,
        runs: data.last_runs,
        refetch: retry,
      }}
      projects={[]}
      defaultHarness={data.schedule.default_harness}
      postCreateFn={create}
      postEditFn={edit}
    />
  )
}
