import { Box, Text } from "@chakra-ui/react"
import { useEffect, useState, type KeyboardEvent } from "react"
import { useNavigate } from "react-router"
import { useConnectionContext } from "../connection"
import type { ScheduleInfo } from "../generated/server"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { Button } from "../ui/button/Button"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { useFetched } from "../useFetched"
import { useWrite } from "../useWrite"
import { formatNextRun, formatWhen } from "./format"
import { fetchSchedules, postSwitch, type FetchSchedules, type PostSwitch } from "./schedules"
import { Switch } from "./Switch"

const addressOf = (s: ScheduleInfo) => `${s.project}:${s.slug}`

function ScheduleRow({
  item,
  onOpen,
  post,
  onAnswer,
}: {
  item: ScheduleInfo
  onOpen: () => void
  post: PostSwitch
  onAnswer: (s: ScheduleInfo) => void
}) {
  const write = useWrite()
  const toggle = () =>
    void write.run(() => post(item.project, item.slug, !item.switch), {
      onDone: (res) => onAnswer(res.schedule),
    })
  return (
    <Box
      as="li"
      listStyleType="none"
      data-mock-part="schedule-row"
      data-focus-ring="inset"
      tabIndex={0}
      aria-label={item.name}
      onClick={onOpen}
      onKeyDown={(e: KeyboardEvent) => {
        if (e.key === "Enter") onOpen()
      }}
      display="flex"
      alignItems="center"
      gap="4"
      py="3"
      cursor="pointer"
      borderBottomWidth="1px"
      borderColor="border"
    >
      <Box minW="0" flex="1">
        <Text data-mock-part="schedule-name" fontWeight="600" truncate>
          {item.name}
        </Text>
        <Text data-mock-part="schedule-when" color="text.muted" fontSize="14px">
          {formatWhen(item.days, item.times)}
        </Text>
      </Box>
      <Text data-mock-part="next-run" color="text.muted" fontSize="14px" flex="none">
        {!item.project_found
          ? "project not found"
          : item.switch && item.next_run
            ? `Next ${formatNextRun(item.next_run)}`
            : "Off here"}
      </Text>
      <Switch on={item.switch} disabled={write.pending} onToggle={toggle} />
    </Box>
  )
}

/** The Schedules, one plain row each. */
export function SchedulesPage({
  fetchList = fetchSchedules,
  post = postSwitch,
}: {
  fetchList?: FetchSchedules
  post?: PostSwitch
}) {
  const conn = useConnectionContext()
  const navigate = useNavigate()
  const { data, failed } = useFetched("schedules", fetchList, conn)
  // A switch's answer shows at once; a fresh list replaces the answers.
  const [answers, setAnswers] = useState<Record<string, ScheduleInfo>>({})
  useEffect(() => setAnswers({}), [data])
  const schedules = (data?.schedules ?? []).map((s) => answers[addressOf(s)] ?? s)
  const allOff = schedules.length > 0 && schedules.every((s) => !s.switch)
  return (
    <DetailPage title={{ kind: "Scheduled prompts" }}>
      <PageHeader
        title="Scheduled prompts"
        actions={
          <Button
            data-mock-part="new-button"
            flexShrink="0"
            whiteSpace="nowrap"
            onClick={() => navigate("/schedules/new")}
          >
            New
          </Button>
        }
      />
      {failed ? (
        <Text mt="6" color="text.muted">
          Could not load the Scheduled prompts
        </Text>
      ) : !data ? (
        <FullScreenLoader label="Loading Scheduled prompts" />
      ) : schedules.length === 0 ? (
        <Text data-mock-part="empty-text" mt="60px" textAlign="center" color="text.muted">
          No scheduled prompts yet.
          <br />
          Run a prompt at set times, for example every day at 07:00 and 21:00.
        </Text>
      ) : (
        <>
          {allOff && (
            <Box
              data-mock-part="off-banner"
              mt="4"
              color="tone.waiting.fg"
              bg="tone.waiting.bg"
              borderRadius="8px"
              px="3"
              py="2"
              fontSize="14px"
            >
              Every schedule is off on this machine. Each one runs only where its switch is on.
            </Box>
          )}
          <Box as="ul" m="0" p="0" mt="4" data-mock-part="schedule-rows">
            {schedules.map((s) => (
              <ScheduleRow
                key={addressOf(s)}
                item={s}
                post={post}
                onAnswer={(a) => setAnswers((prev) => ({ ...prev, [addressOf(a)]: a }))}
                onOpen={() => navigate(`/schedules/${addressOf(s)}`)}
              />
            ))}
          </Box>
        </>
      )}
    </DetailPage>
  )
}
