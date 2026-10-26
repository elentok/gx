import { Box, Spinner, Text } from "@chakra-ui/react";
import { useRef, useState } from "react";
import { copyText } from "../clipboard";
import { useDesignWrite } from "../design/SessionControls";
import { Button } from "../ui/button/Button";
import { postFixAgent, type FixAgent } from "./fixAgent";
import type { EpicDetailRow } from "./detail";

/** Shows the investigate prompt to copy, or starts a gx agent on it. */
export function FixAgentDialog({
  row,
  fixAgent = postFixAgent,
  onClose,
}: {
  row: EpicDetailRow;
  fixAgent?: FixAgent;
  onClose: () => void;
}) {
  const [copy, setCopy] = useState<"idle" | "copied" | "failed">("idle");
  const [started, setStarted] = useState<{ remote: boolean }>();
  const { run, pending } = useDesignWrite();
  const promptRef = useRef<HTMLElement>(null);
  const prompt = row.fix_prompt ?? "";

  return (
    <Box position="fixed" inset="0" zIndex="modal" bg="overlay" display="flex" alignItems="center" justifyContent="center" p="4" onClick={onClose}>
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
        <Box as="code" ref={promptRef} fontFamily="mono" fontSize="sm" bg="surface.raised" borderRadius="md" p="2" overflowWrap="anywhere">
          {prompt}
        </Box>
        {started && (
          <Text>{`Agent started. View it in the gx TUI Agents tab${started.remote ? " or in the Claude app" : ""}`}</Text>
        )}
        <Box display="flex" gap="2" justifyContent="flex-end">
          <Button
            emphasis="secondary"
            onClick={async () => setCopy((await copyText(prompt, promptRef.current)) ? "copied" : "failed")}
          >
            {copy === "copied" ? "Copied" : copy === "failed" ? "Copy failed" : "Copy"}
          </Button>
          {!started && (
            <Button
              tone="running"
              disabled={pending}
              onClick={() => run(() => fixAgent({ address: row.address, state: row.state }), { onDone: (res) => setStarted({ remote: res.remote_control }) })}
            >
              {pending && <Spinner size="sm" />}
              Start gx agent
            </Button>
          )}
          <Button emphasis="ghost" onClick={onClose}>
            Close
          </Button>
        </Box>
      </Box>
    </Box>
  );
}
