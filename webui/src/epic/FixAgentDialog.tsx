import { Box, Spinner } from "@chakra-ui/react"
import { useRef, useState } from "react"
import { useNavigate } from "react-router"
import { copyText } from "../clipboard"
import { useConnectionContext } from "../connection"
import { Failure } from "../design/SessionControls"
import { agentPath } from "../hrefs"
import { refusalText, useWrite } from "../useWrite"
import { Button } from "../ui/button/Button"
import { postFixAgent, type FixAgent } from "./fixAgent"
import type { FixAgentRequest } from "../generated/server"

/** Shows a fix prompt to copy, or starts a gx agent with the request. */
export function FixAgentDialog({
  prompt,
  request,
  fixAgent = postFixAgent,
  onClose,
}: {
  prompt: string
  request: FixAgentRequest
  fixAgent?: FixAgent
  onClose: () => void
}) {
  const [copy, setCopy] = useState<"idle" | "copied" | "failed">("idle")
  const conn = useConnectionContext()
  const navigate = useNavigate()
  // The refusal shows in the dialog, which stays open, so it is not also toasted.
  const { run, pending, refusal } = useWrite({ toastRefusal: false })
  const promptRef = useRef<HTMLElement>(null)

  return (
    <Box
      position="fixed"
      inset="0"
      zIndex="modal"
      bg="overlay"
      display="flex"
      alignItems="center"
      justifyContent="center"
      p="4"
      onClick={onClose}
    >
      <Box
        role="dialog"
        aria-modal="true"
        aria-label="Fix it with an agent"
        bg="surface"
        borderWidth="1px"
        borderColor="border"
        borderRadius="md"
        p="4"
        maxW="full"
        display="flex"
        flexDirection="column"
        gap="3"
        onClick={(e) => e.stopPropagation()}
      >
        <Box
          as="code"
          ref={promptRef}
          fontFamily="mono"
          fontSize="sm"
          bg="surface.raised"
          borderRadius="md"
          p="2"
          overflowWrap="anywhere"
        >
          {prompt}
        </Box>
        {refusal && <Failure>{refusalText(refusal)}</Failure>}
        <Box display="flex" gap="2" justifyContent="flex-end">
          <Button
            emphasis="secondary"
            onClick={async () =>
              setCopy((await copyText(prompt, promptRef.current)) ? "copied" : "failed")
            }
          >
            {copy === "copied" ? "Copied" : copy === "failed" ? "Copy failed" : "Copy"}
          </Button>
          <Button
            tone="running"
            disabled={pending}
            onClick={() =>
              run(() => fixAgent(request), {
                onDone: (res) => {
                  conn.refresh()
                  if (res.session) navigate(agentPath(res.session.id))
                  onClose()
                },
              })
            }
          >
            {pending && <Spinner size="sm" />}
            Start gx agent
          </Button>
          <Button emphasis="ghost" onClick={onClose}>
            Close
          </Button>
        </Box>
      </Box>
    </Box>
  )
}
