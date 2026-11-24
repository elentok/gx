import { Box } from "@chakra-ui/react"
import { useRef } from "react"
import { openConfirm, type ConfirmRequest } from "../actions/confirmRequest"
import { Button } from "../ui/button/Button"
import type { GalleryEntry } from "./registry"

const base = {
  dismissLabel: "Cancel",
  onConfirm: () => {},
} satisfies Partial<ConfirmRequest>

/** One button per confirm state; the host is a singleton, so one opens at a time. */
function ConfirmCases() {
  const ref = useRef<HTMLButtonElement>(null)
  const open = (over: (anchor: HTMLElement) => Partial<ConfirmRequest>) => () =>
    openConfirm({
      ...base,
      actionId: "remove",
      entityName: "gx bugs",
      group: "danger",
      confirmLabel: "Remove",
      title: "Remove gx bugs",
      body: { lines: ["The epic and its tickets are deleted."] },
      surface: { mode: "anchor", anchor: ref.current! },
      ...over(ref.current!),
    })
  return (
    <Box display="flex" gap="2" flexWrap="wrap">
      <Button ref={ref} emphasis="secondary" onClick={open(() => ({}))}>
        Popover
      </Button>
      <Button
        emphasis="secondary"
        onClick={open(() => ({
          actionId: "start-over",
          title: "Start over gx bugs 03",
          confirmLabel: "Start over",
          body: {
            lines: ["The ticket runs again from the start."],
            input: { label: "Reason", required: true },
          },
        }))}
      >
        Popover with input
      </Button>
      <Button
        emphasis="secondary"
        onClick={open(() => ({
          actionId: "stop",
          title: "Stop 3 sessions",
          confirmLabel: "Stop 3",
          body: { lines: [], rows: ["talk", "gx bugs 03", "gx bugs 04"] },
        }))}
      >
        Popover bulk
      </Button>
      <Button
        emphasis="secondary"
        onClick={open(() => ({
          title: "Remove gx bugs",
          prepare: () => new Promise(() => {}),
        }))}
      >
        Popover, held prepare
      </Button>
      <Button emphasis="secondary" onClick={open(() => ({ surface: { mode: "modal" } }))}>
        Modal
      </Button>
    </Box>
  )
}

export const confirmEntries: GalleryEntry[] = [
  {
    name: "Confirm",
    cases: [
      { label: "popover, input, bulk and modal", node: <ConfirmCases /> },
      // Open the gallery at 390px: the confirm flips above this button and a tall one becomes the bottom dialog.
      {
        label: "phone, button at the bottom of the page",
        node: (
          <Box minH="80vh" display="flex" alignItems="flex-end">
            <ConfirmCases />
          </Box>
        ),
      },
    ],
  },
]
