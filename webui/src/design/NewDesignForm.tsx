import { Box, chakra, Text } from "@chakra-ui/react";
import { useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { Button } from "../ui/button/Button";
import { postOpenDesign, type OpenDesign } from "./open";
import { Failure, useWrite } from "./SessionControls";
import { useVisualViewportHeight } from "./useVisualViewportHeight";
import { useWide } from "./useWide";

const SHEET_HEIGHT = 0.88;
const SHEET_TOP_GAP = 8;

export interface NewDesignDraft {
  slug?: string;
  idea?: string;
  project?: string;
}

const DRAFT_KEY = "gx.newDesign.draft";

function readDraft(): NewDesignDraft {
  try {
    return JSON.parse(localStorage.getItem(DRAFT_KEY) ?? "{}") as NewDesignDraft;
  } catch {
    return {};
  }
}

function writeDraft(draft: NewDesignDraft | undefined) {
  try {
    if (draft) localStorage.setItem(DRAFT_KEY, JSON.stringify(draft));
    else localStorage.removeItem(DRAFT_KEY);
  } catch {
    // The draft is a convenience; the open form still holds it.
  }
}

const hasText = (d: NewDesignDraft) => !!(d.slug?.trim() || d.idea?.trim());

/** A draft with text keeps its project, then the page's filter, then the draft's, then the first. */
function startingProject(draft: NewDesignDraft, projects: string[], filterProject?: string): string {
  const known = (p?: string) => (p && projects.includes(p) ? p : undefined);
  const text = hasText(draft);
  return (text && known(draft.project)) || known(filterProject) || known(draft.project) || projects[0] || "";
}

const focusRing = { borderColor: "tone.running.fg", boxShadow: "0 0 0 3px {colors.tone.running.bg}" };
const field = {
  fontSize: "16px",
  color: "text",
  bg: "surface",
  borderWidth: "1px",
  borderColor: "transparent",
  borderRadius: "8px",
  _placeholder: { color: "text.dim" },
  _focusVisible: { outline: "none", ...focusRing },
} as const;

/** The New Design writing sheet: epic name and rough idea for a Design with no map yet. The draft lives in localStorage until Start or Discard. */
export function NewDesignForm({
  projects,
  filterProject,
  open = postOpenDesign,
  onStarted,
  onClose,
  phone,
}: {
  projects: string[];
  /** The Epics page's project filter, when it is on one project. */
  filterProject?: string;
  open?: OpenDesign;
  /** Runs with the new Design's address once its map exists; `startFailure` is why the session did not start. */
  onStarted: (address: string, startFailure?: string) => void;
  /** Closing keeps the draft. */
  onClose: () => void;
  /** Forces the phone bottom sheet (or the centered modal) regardless of the viewport width. */
  phone?: boolean;
}) {
  const wide = useWide();
  const sheetMode = phone ?? !wide;
  const viewportHeight = useVisualViewportHeight();
  // The keyboard shrinks the visual viewport, so the sheet follows it to keep the footer visible.
  const sheetHeight = viewportHeight ? `${Math.round(Math.min(SHEET_HEIGHT * window.innerHeight, viewportHeight - SHEET_TOP_GAP))}px` : "88%";
  const [draft, setDraft] = useState(() => {
    const saved = readDraft();
    return { slug: saved.slug ?? "", idea: saved.idea ?? "", project: startingProject(saved, projects, filterProject) };
  });
  const [confirming, setConfirming] = useState(false);
  const { run, pending, error } = useWrite();
  const sheet = useRef<HTMLDivElement>(null);

  const patch = (p: Partial<typeof draft>) => {
    const next = { ...draft, ...p };
    setDraft(next);
    writeDraft(next);
  };
  const discard = () => {
    writeDraft(undefined);
    onClose();
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const slug = draft.slug.trim();
    let startFailure: string | undefined;
    run(
      async () => {
        const res = await open({ epic: slug, new_design: true, idea: draft.idea, project: draft.project });
        // The map exists though no session started, so the Design page takes over and offers the retry.
        if (!res.epic_created) return res;
        startFailure = res.message ?? "The design session did not start.";
        return { ...res, refused: false };
      },
      () => {
        writeDraft(undefined);
        onStarted(`${draft.project}:${slug}`, startFailure);
      },
    );
  };

  const keyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      e.stopPropagation();
      onClose();
    } else if (e.key === "Tab") {
      const items = [...(sheet.current?.querySelectorAll<HTMLElement>("input, textarea, select, button:not(:disabled)") ?? [])];
      const [first, last] = [items[0], items[items.length - 1]];
      if (e.shiftKey && document.activeElement === first) (e.preventDefault(), last?.focus());
      else if (!e.shiftKey && document.activeElement === last) (e.preventDefault(), first?.focus());
    }
  };

  return (
    <Box
      position="fixed"
      inset="0"
      zIndex="modal"
      bg="overlay"
      display="flex"
      alignItems={sheetMode ? "flex-end" : "center"}
      justifyContent="center"
      p={sheetMode ? "0" : "4"}
      onClick={onClose}
    >
      <Box
        ref={sheet}
        as="form"
        role="dialog"
        aria-modal="true"
        aria-label="New Design"
        onSubmit={submit}
        onKeyDown={keyDown}
        onClick={(e) => e.stopPropagation()}
        bg="surface.raised"
        borderWidth="1px"
        borderColor="border"
        borderRadius={sheetMode ? "xl xl 0 0" : "lg"}
        w={sheetMode ? "full" : "720px"}
        h={sheetMode ? sheetHeight : "520px"}
        maxW="full"
        maxH="full"
        display="flex"
        flexDirection="column"
        overflow="hidden"
      >
        {sheetMode && <Box aria-hidden data-testid="grab-handle" alignSelf="center" w="36px" h="4px" mt="2" borderRadius="full" bg="border" flexShrink="0" />}
        <Box display="flex" alignItems="center" gap="2" p="3" borderBottomWidth="1px" borderColor="border" flexShrink="0">
          {projects.length > 1 && (
            <>
              <chakra.select
                aria-label="Project"
                value={draft.project}
                onChange={(e) => patch({ project: e.target.value })}
                fontSize="16px"
                h="40px"
                px="3"
                borderRadius="8px"
                bg="tone.running.bg"
                color="tone.running.fg"
                fontWeight="600"
              >
                {projects.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </chakra.select>
              <Text color="text.dim" fontSize="lg">
                /
              </Text>
            </>
          )}
          <chakra.input
            aria-label="Epic name"
            autoFocus
            {...field}
            flex="1"
            minW="0"
            h="40px"
            px="3"
            fontWeight="600"
            placeholder="epic-name"
            value={draft.slug}
            onChange={(e) => patch({ slug: e.target.value })}
          />
          <chakra.button
            type="button"
            aria-label="Close"
            title="Close (keeps the draft)"
            onClick={onClose}
            color="text.dim"
            fontSize="xl"
            lineHeight="1"
            w="36px"
            h="36px"
            flexShrink="0"
            borderRadius="8px"
            _hover={{ bg: "surface", color: "text" }}
          >
            ×
          </chakra.button>
        </Box>
        <chakra.textarea
          aria-label="Rough idea"
          {...field}
          flex="1"
          minH="0"
          m="3"
          resize="none"
          px="4"
          py="3"
          lineHeight="1.6"
          placeholder="What is the problem? Who hits it? What does done look like?"
          value={draft.idea}
          onChange={(e) => patch({ idea: e.target.value })}
        />
        <Failure>{error}</Failure>
        <Box display="flex" alignItems="center" gap="2" p="3" borderTopWidth="1px" borderColor="border" bg="surface" flexShrink="0">
          {confirming ? (
            <>
              <Text flex="1" fontSize="sm" color="tone.paused.fg" fontWeight="600">
                Discard this draft?
              </Text>
              <Button type="button" emphasis="ghost" tone="muted" onClick={() => setConfirming(false)}>
                Keep
              </Button>
              <Button type="button" tone="paused" onClick={discard}>
                Discard
              </Button>
            </>
          ) : (
            <>
              <Button type="button" emphasis="ghost" tone="muted" flex={sheetMode ? "1" : undefined} onClick={() => (hasText(draft) ? setConfirming(true) : discard())}>
                Discard
              </Button>
              {!sheetMode && <Box flex="1" />}
              <Button type="submit" tone="running" flex={sheetMode ? "2" : undefined} disabled={pending}>
                Start design
              </Button>
            </>
          )}
        </Box>
      </Box>
    </Box>
  );
}
