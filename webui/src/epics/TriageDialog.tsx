import { Box, Text } from "@chakra-ui/react";
import { useRef, useState } from "react";
import { Button } from "../ui/button/Button";

export const triageCommand = (project: string) => `/gx-triage-follow-ups --project ${project}`;

function copyViaTextarea(text: string): boolean {
  const area = document.createElement("textarea");
  area.value = text;
  area.style.position = "fixed";
  area.style.opacity = "0";
  document.body.appendChild(area);
  area.select();
  try {
    return document.execCommand("copy");
  } finally {
    area.remove();
  }
}

function selectNode(node: Node | null) {
  if (!node) return;
  const range = document.createRange();
  range.selectNodeContents(node);
  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
}

/** Stands in for a triage screen: shows the skill command to run in a terminal. */
export function TriageDialog({ project, onClose }: { project: string; onClose: () => void }) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  const commandRef = useRef<HTMLElement>(null);
  const command = triageCommand(project);

  async function copy() {
    try {
      // navigator.clipboard only exists in a secure context; plain-HTTP LAN addresses lack it.
      if (navigator.clipboard) await navigator.clipboard.writeText(command);
      else if (!copyViaTextarea(command)) throw new Error("copy command rejected");
      setStatus("copied");
    } catch {
      selectNode(commandRef.current);
      setStatus("failed");
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
