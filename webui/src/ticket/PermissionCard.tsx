import { Box, Code, Text, Textarea } from "@chakra-ui/react";
import { useState } from "react";
import type { TicketDetail } from "../generated/server";
import { Badge } from "../ui/badge/Badge";
import { Button } from "../ui/button/Button";
import { postDecision, type Decision, type PostDecision } from "./permission";

// The pending permission prompt of a blocked agent. Only Allow once and Deny: a standing
// permission is never granted from here. A hint means the prompt can only be approved in the TUI.
export function PermissionCard({
  detail,
  post = postDecision,
  onDone,
  onStale,
}: {
  detail: TicketDetail;
  post?: PostDecision;
  /** The decision reached the agent; refetch so the card goes away. */
  onDone?: () => void;
  /** The refusal means the prompt moved on: refetch so the current card shows. */
  onStale?: () => void;
}) {
  const card = detail.permission;
  const [note, setNote] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [seenId, setSeenId] = useState(card?.request_id);

  // A new prompt is a new card: the old note does not belong to it. The error stays, since it
  // is usually why the card changed.
  if (seenId !== card?.request_id) {
    setSeenId(card?.request_id);
    setNote("");
  }

  if (!card) return null;

  const decide = async (decision: Decision) => {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      const res = await post({
        address: detail.address,
        decision,
        request_id: card.request_id,
        ...(decision === "deny" && note.trim() ? { text: note } : {}),
      });
      if (res.refused) {
        setError(res.message);
        onStale?.();
      } else {
        onDone?.();
      }
    } catch {
      setError("Could not send the decision. Try again.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Box
      role="group"
      aria-label="Permission request"
      mt="4"
      p="3"
      borderWidth="1px"
      rounded="md"
      display="flex"
      flexDirection="column"
      gap="2"
    >
      <Box display="flex" gap="2" alignItems="center">
        <Badge tone="paused">Needs approval</Badge>
        {card.tool && <Text fontWeight="600">{card.tool}</Text>}
      </Box>
      {card.description && <Text>{card.description}</Text>}
      {card.input && (
        <Code display="block" whiteSpace="pre-wrap" p="2" overflowX="auto" aria-label="Tool input">
          {card.input}
        </Code>
      )}
      {error && <Text role="alert">{error}</Text>}
      {card.hint ? (
        <Text color="fg.muted">{card.hint}</Text>
      ) : (
        <>
          <Textarea
            aria-label="Note for Deny (optional)"
            placeholder="Note for Deny (optional)"
            value={note}
            disabled={pending}
            onChange={(e) => setNote(e.target.value)}
          />
          <Box display="flex" gap="2">
            <Button disabled={pending} onClick={() => decide("allow")}>
              Allow once
            </Button>
            <Button emphasis="secondary" disabled={pending} onClick={() => decide("deny")}>
              Deny
            </Button>
          </Box>
        </>
      )}
    </Box>
  );
}
