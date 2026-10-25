import { Box, Text } from "@chakra-ui/react";
import { useState } from "react";
import { Button } from "../ui/button/Button";

export const triageCommand = (project: string) => `/gx-triage-follow-ups --project ${project}`;

/** Stands in for a triage screen: shows the skill command to run in a terminal. */
export function TriageDialog({ project, onClose }: { project: string; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  const command = triageCommand(project);

  async function copy() {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
    } catch {
      setCopied(false);
    }
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
        <Box as="code" fontFamily="mono" fontSize="sm" bg="surface.raised" borderRadius="md" p="2" overflowWrap="anywhere">
          {command}
        </Box>
        <Box display="flex" gap="2" justifyContent="flex-end">
          <Button emphasis="secondary" onClick={copy}>
            {copied ? "Copied" : "Copy"}
          </Button>
          <Button emphasis="ghost" onClick={onClose}>
            Close
          </Button>
        </Box>
      </Box>
    </Box>
  );
}
