import { Box, Button, Text } from "@chakra-ui/react";
import { useState } from "react";
import { ActionResume, type TicketDetail } from "../generated/server";
import { ReasonStale } from "./answer";
import { postResume, resumeWords, type PostResume } from "./resume";

const REASON_MANUAL = "manual";

/** The action's name by pause reason: Resume lifts only a pause the person chose. */
export function resumeLabel(reason: string | undefined): string {
  switch (reason) {
    case "ambiguous-base":
      return "Choose base";
    case "merge-conflict":
      return "Resolve conflict";
    case "stranded-commits":
      return "Merge commits";
    case REASON_MANUAL:
      return "Resume";
    default:
      return "Retry";
  }
}

// The resume area. The server decides whether it shows (the detail's actions); an
// ambiguous-base pause lists the competing blockers and needs one chosen.
export function ResumeForm({
  detail,
  post = postResume,
  onDone,
  onStale,
}: {
  detail: TicketDetail;
  post?: PostResume;
  /** Called with the words describing what the resume did. */
  onDone?: (words: string) => void;
  /** The pause changed under the person: refetch so the current one shows. */
  onStale?: () => void;
}) {
  const bases = detail.bases ?? [];
  const [base, setBase] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [seenState, setSeenState] = useState(detail.state);

  // A new pause clears the pick: it may list different blockers.
  if (seenState !== detail.state) {
    setSeenState(detail.state);
    setBase("");
  }

  if (!detail.actions.includes(ActionResume)) return null;

  const ready = bases.length === 0 || base !== "";
  const label = resumeLabel(detail.pause_reason);

  const send = async () => {
    if (!ready || pending) return;
    setPending(true);
    setError("");
    try {
      const res = await post({ address: detail.address, state: detail.state ?? "", ...(base ? { base } : {}) });
      if ("refused" in res) {
        setError(res.message);
        if (res.reason === ReasonStale) onStale?.();
      } else {
        onDone?.(resumeWords(res.data));
      }
    } catch {
      setError(`Could not ${label.toLowerCase()}. Try again.`);
    } finally {
      setPending(false);
    }
  };

  return (
    <Box
      position="sticky"
      bottom="0"
      bg="surface"
      borderTopWidth="1px"
      py="3"
      display="flex"
      flexDirection="column"
      gap="2"
      aria-label={label}
      role="form"
    >
      {bases.length > 0 && (
        <>
          <Text>Choose the base branch:</Text>
          {bases.map((b) => (
            <Button
              key={b}
              variant={base === b ? "solid" : "outline"}
              aria-pressed={base === b}
              disabled={pending}
              onClick={() => setBase(b)}
              width="full"
              justifyContent="flex-start"
            >
              {b}
            </Button>
          ))}
        </>
      )}
      {error && <Text role="alert">{error}</Text>}
      <Button disabled={!ready || pending} onClick={send}>
        {label}
      </Button>
    </Box>
  );
}
