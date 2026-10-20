import { Box, Button, Text } from "@chakra-ui/react";
import { useState } from "react";
import { ActionUnpark, type TicketDetail } from "../generated/server";
import { ReasonStale } from "./answer";
import { postUnpark, unparkWords, type PostUnpark } from "./unpark";

// The unpark area. The server decides whether it shows (the detail's actions); an
// ambiguous-base park lists the competing blockers and needs one chosen.
export function UnparkForm({
  detail,
  post = postUnpark,
  onDone,
  onStale,
}: {
  detail: TicketDetail;
  post?: PostUnpark;
  /** Called with the words describing what the unpark did. */
  onDone?: (words: string) => void;
  /** The park changed under the person: refetch so the current one shows. */
  onStale?: () => void;
}) {
  const bases = detail.bases ?? [];
  const [base, setBase] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [seenState, setSeenState] = useState(detail.state);

  // A new park clears the pick: it may list different blockers.
  if (seenState !== detail.state) {
    setSeenState(detail.state);
    setBase("");
  }

  if (!detail.actions.includes(ActionUnpark)) return null;

  const ready = bases.length === 0 || base !== "";

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
        onDone?.(unparkWords(res.data));
      }
    } catch {
      setError("Could not unpark. Try again.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Box
      position="sticky"
      bottom="0"
      bg="bg"
      borderTopWidth="1px"
      py="3"
      display="flex"
      flexDirection="column"
      gap="2"
      aria-label="Unpark"
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
        Unpark
      </Button>
    </Box>
  );
}
