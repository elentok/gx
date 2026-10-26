import { Box, Text } from "@chakra-ui/react";
import { useRef, useState } from "react";
import { copyText } from "../clipboard";
import { Button } from "../ui/button/Button";

export const triageCommand = (project: string) => `/gx-triage-follow-ups --project ${project}`;

/** Stands in for a triage screen: shows the skill command to run in a terminal. */
export function TriageDialog({ project, onClose }: { project: string; onClose: () => void }) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  const commandRef = useRef<HTMLElement>(null);
  const command = triageCommand(project);

  async function copy() {
    setStatus((await copyText(command, commandRef.current)) ? "copied" : "failed");
  }

  return (
    <Box position="fixed" inset="0" zIndex="modal" bg="overlay" display="flex" alignItems="center" justifyContent="center" p="4" onClick={onClose}>
      <Box
        role="dialog"
        aria-modal="true"
        aria-label="Triage"
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
        <Text>Triage UI not yet implemented, for now run:</Text>
        <Box as="code" ref={commandRef} fontFamily="mono" fontSize="sm" bg="surface.raised" borderRadius="md" p="2" overflowWrap="anywhere">
          {command}
        </Box>
        <Box display="flex" gap="2" justifyContent="flex-end">
          <Button emphasis="secondary" onClick={copy}>
            {status === "copied" ? "Copied" : status === "failed" ? "Copy failed" : "Copy"}
          </Button>
          <Button emphasis="ghost" onClick={onClose}>
            Close
          </Button>
        </Box>
      </Box>
    </Box>
  );
}
