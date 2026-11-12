import { Box, chakra, Text } from "@chakra-ui/react"
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

const addressOf = (s: ScheduleInfo) => `${s.project}:${s.slug}`

function Switch({
  on,
  disabled,
  onToggle,
}: {
  on: boolean
  disabled: boolean
  onToggle: () => void
}) {
  return (
    <chakra.button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label="Run on this machine"
      data-mock-part="switch"
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation()
        onToggle()
      }}
      onKeyDown={(e: KeyboardEvent) => e.stopPropagation()}
      flex="none"
      w="40px"
      h="22px"
      borderRadius="full"
      position="relative"
      bg={on ? "tone.running.fg" : "border"}
    >
      <Box
        position="absolute"
        top="2px"
        left={on ? "20px" : "2px"}
        w="18px"
        h="18px"
        borderRadius="full"
        bg="surface"
      />
    </chakra.button>
  )
}

function ScheduleRow({
  item,
  onOpen,
  post,
}: {
  item: ScheduleInfo
  onOpen: () => void
  post: PostSwitch
}) {
  const write = useWrite()
  // The switch's answer shows at once; a fresh list replaces it.
  const [answer, setAnswer] = useState<ScheduleInfo>()
  useEffect(() => setAnswer(undefined), [item.switch, item.next_run])
  const shown = answer ?? item
  const toggle = () =>
    void write.run(() => post(item.project, item.slug, !shown.switch), {
      onDone: (res) => setAnswer(res.schedule),
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
        {shown.switch && shown.next_run ? `Next ${formatNextRun(shown.next_run)}` : "Off here"}
      </Text>
      <Switch on={shown.switch} disabled={write.pending} onToggle={toggle} />
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
      ) : (
        <Box as="ul" m="0" p="0" mt="4" data-mock-part="schedule-rows">
          {(data.schedules ?? []).map((s) => (
            <ScheduleRow
              key={addressOf(s)}
              item={s}
              post={post}
              onOpen={() => navigate(`/schedules/${addressOf(s)}`)}
            />
          ))}
        </Box>
      )}
    </DetailPage>
  )
}
