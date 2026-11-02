import { Box, chakra, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { refusalText, useWrite } from "../useWrite";
import { postKeys, type PostKeys } from "./keys";
import { fetchScreen, type FetchScreen } from "./screen";

// Label on the button, name the keys route takes.
const KEYS: [label: string, key: string][] = [
  ["Esc", "esc"],
  ["Enter", "enter"],
  ["Tab", "tab"],
  ["↑", "up"],
  ["↓", "down"],
  ["←", "left"],
  ["→", "right"],
  ["Ctrl+C", "ctrl+c"],
];

const keyLabel = (label: string) => ({ "↑": "Up", "↓": "Down", "←": "Left", "→": "Right" })[label] ?? label;

/**
 * A screen as plain text rows; the target is whatever the fetcher takes. It reads once on open and again on Refresh,
 * and after each key sent. `keys` adds the key bar and Type field, only for a session a person may type into.
 */
export function ScreenModal({
  target,
  fetcher = fetchScreen,
  keys = false,
  poster = postKeys,
  onClose,
}: {
  target: string;
  fetcher?: FetchScreen;
  keys?: boolean;
  poster?: PostKeys;
  onClose: () => void;
}) {
  const [rows, setRows] = useState<string[]>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [text, setText] = useState("");
  const { run, pending, refusal, error: sendError } = useWrite({ toastRefusal: false });

  const load = async () => {
    setLoading(true);
    setError("");
    try {
      const res = await fetcher(target);
      if (res.kind === "ok") setRows(res.detail.rows);
      else setError("This session has no screen.");
    } catch {
      setError("Could not load the screen. Try Refresh.");
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    void load();
    // Only a different target reloads; Refresh is the other way in.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target]);

  const send = (body: { key?: string; text?: string }, after?: () => void) =>
    run(() => poster({ id: target, ...body }), {
      onDone: () => {
        after?.();
        void load();
      },
    });

  const problem = refusal ? refusalText(refusal) : sendError;

  return (
    <Modal label="Screen" onClose={onClose} w={{ md: "min(1200px, 92vw)" }} px={{ md: "5" }} py={{ md: "4.5" }} gap={{ md: "3" }}>
      <Box display="flex" gap="2" alignItems="center" justifyContent="space-between">
        <Text as="h3" m="0" fontFamily="condensed" fontSize="22px" textTransform="uppercase" letterSpacing="0.02em">
          Screen
        </Text>
        <Box display="flex" gap="2">
          <Button emphasis="secondary" disabled={loading} onClick={load}>
            Refresh
          </Button>
          <Button emphasis="ghost" tone="muted" onClick={onClose}>
            Close
          </Button>
        </Box>
      </Box>
      {error && <Text role="alert">{error}</Text>}
      {rows && (
        <Box
          as="pre"
          aria-label="Screen rows"
          fontSize="xs"
          overflow="auto"
          maxH={{ md: "70vh" }}
          m="0"
          p="3"
          borderWidth="1px"
          borderRadius="8px"
          bg="surface.raised"
        >
          {rows.join("\n")}
        </Box>
      )}
      {keys && (
        <>
          <Box display="flex" flexDirection="column" gap="1.5" aria-label="Keys" role="group">
            <Text fontSize="xs" color="text.dim" textTransform="uppercase" letterSpacing="0.06em" fontWeight="600">
              Keys
            </Text>
            <Box display="flex" flexWrap="wrap" gap="1.5">
              {KEYS.map(([label, key]) => (
                <Button
                  key={key}
                  emphasis="secondary"
                  aria-label={keyLabel(label)}
                  disabled={pending}
                  h={{ base: "44px", md: "40px" }}
                  minW="44px"
                  flex={{ base: "1 0 20%", md: "0 0 auto" }}
                  fontFamily="mono"
                  fontWeight="400"
                  fontSize="13px"
                  onClick={() => send({ key })}
                >
                  {label}
                </Button>
              ))}
            </Box>
          </Box>
          <Box
            as="form"
            display="flex"
            flexDirection="column"
            gap="1.5"
            onSubmit={(e: React.FormEvent) => {
              e.preventDefault();
              if (text) void send({ text }, () => setText(""));
            }}
          >
            <Text fontSize="xs" color="text.dim" textTransform="uppercase" letterSpacing="0.06em" fontWeight="600">
              Type text
            </Text>
            <Box display="flex" gap="2">
              <chakra.input
                aria-label="Text to type"
                placeholder="Text is typed as is, without Enter"
                value={text}
                onChange={(e: React.ChangeEvent<HTMLInputElement>) => setText(e.target.value)}
                flex="1"
                minW="0"
                h="44px"
                px="3"
                borderWidth="1px"
                borderRadius="8px"
                bg="surface.raised"
              />
              <Button type="submit" emphasis="secondary" disabled={pending} h={{ base: "44px", md: "40px" }} fontWeight="400" fontSize="13px">
                Type
              </Button>
            </Box>
          </Box>
          {problem && <Text role="alert">{problem}</Text>}
        </>
      )}
    </Modal>
  );
}
