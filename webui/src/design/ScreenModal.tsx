import { Box, Text } from "@chakra-ui/react";
import { useEffect, useState } from "react";
import { Button } from "../ui/button/Button";
import { fetchScreen, type FetchScreen } from "./screen";

/** A screen as plain text rows; the target is whatever the fetcher takes. It reads once on open and again on Refresh, never on its own. */
export function ScreenModal({ target, fetcher = fetchScreen, onClose }: { target: string; fetcher?: FetchScreen; onClose: () => void }) {
  const [rows, setRows] = useState<string[]>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

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

  return (
    <Box position="fixed" inset="0" zIndex="modal" bg="overlay" display="flex" alignItems="center" justifyContent="center" p="4" onClick={onClose}>
      <Box
        role="dialog"
        aria-modal="true"
        aria-label="Screen"
        bg="surface"
        borderWidth="1px"
        borderColor="border"
        borderRadius="md"
        p="3"
        maxW="full"
        maxH="full"
        display="flex"
        flexDirection="column"
        gap="2"
        onClick={(e) => e.stopPropagation()}
      >
        <Box display="flex" gap="2" justifyContent="flex-end">
          <Button emphasis="secondary" disabled={loading} onClick={load}>
            Refresh
          </Button>
          <Button emphasis="ghost" onClick={onClose}>
            Close
          </Button>
        </Box>
        {error && <Text role="alert">{error}</Text>}
        {rows && (
          <Box as="pre" aria-label="Screen rows" fontSize="xs" overflow="auto" m="0">
            {rows.join("\n")}
          </Box>
        )}
      </Box>
    </Box>
  );
}
