import { Box, chakra, Text } from "@chakra-ui/react";
import { useState } from "react";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { Button } from "../ui/button/Button";
import { Chip } from "../ui/chip/Chip";
import { Spinner } from "../ui/spinner/Spinner";
import { useFetched } from "../useFetched";
import { refusalText, useWrite, type Refusable } from "../useWrite";
import {
  fetchAgentSettings,
  harnessName,
  otherHarness,
  postAgentSettings,
  runnerNote,
  usesCodex,
  type DefaultHarnessStatus,
  type FetchAgentSettings,
  type Harness,
  type PostAgentSettings,
} from "./agents";
import { BackLink, Frame, Group, rowProps } from "./parts";

const HARNESSES: Harness[] = ["claude", "codex"];

function HarnessControl({ value, disabled, onPick }: { value: string; disabled: boolean; onPick: (h: Harness) => void }) {
  return (
    <Box display="inline-flex" borderWidth="1px" borderColor="border" borderRadius="md" overflow="hidden" role="group" aria-label="Default harness">
      {HARNESSES.map((h) => {
        const on = h === value;
        return (
          <chakra.button
            key={h}
            type="button"
            aria-pressed={on}
            disabled={disabled}
            onClick={() => !on && onPick(h)}
            px="3.5"
            py="1.5"
            fontSize="sm"
            cursor={on ? "default" : "pointer"}
            bg={on ? "tone.done.fg" : "transparent"}
            color={on ? "surface" : "text"}
            fontWeight={on ? "600" : "400"}
            _notFirst={{ borderLeftWidth: "1px", borderLeftColor: "border" }}
          >
            {harnessName(h)}
          </chakra.button>
        );
      })}
    </Box>
  );
}

export function AgentsSettingsView({
  settings,
  notice,
  refusal,
  pending = false,
  onSetGlobal,
  onSetProject,
}: {
  settings: DefaultHarnessStatus;
  /** The server's text for the last switch, shown as returned. */
  notice?: string;
  refusal?: Refusable;
  pending?: boolean;
  onSetGlobal: (harness: Harness) => void;
  onSetProject: (project: string, harness: Harness | "") => void;
}) {
  const note = runnerNote(settings.runner);
  return (
    <Frame>
      <BackLink />
      <PageHeader title="Agents" />
      {note && (
        <Text mt="1" fontSize="sm" color="text.dim">
          {note}
        </Text>
      )}
      <Group>
        <Box data-mock-part="default-harness" {...rowProps} cursor="default" _hover={{}}>
          <Box flex="1" minW="0">
            <Text fontWeight="600">Default harness</Text>
            <Text fontSize="sm" color="text.muted">
              New agents everywhere, unless a project overrides it.
            </Text>
          </Box>
          <HarnessControl value={settings.default} disabled={pending} onPick={onSetGlobal} />
        </Box>
      </Group>
      {notice && (
        <Text data-mock-part="switch-notice" mt="3" fontSize="sm" bg="tone.waiting.bg" color="tone.waiting.fg" px="3" py="2" borderRadius="md">
          {notice}
        </Text>
      )}
      {refusal && (
        <Text role="alert" mt="3" fontSize="sm" borderWidth="1px" borderColor="tone.deadlocked.fg" color="tone.deadlocked.fg" px="3" py="2" borderRadius="md">
          {refusalText(refusal)}
        </Text>
      )}
      {usesCodex(settings) && (
        <Text mt="3" fontSize="sm" color="text.dim">
          Codex needs approval rules: run <code>gx codex setup</code>
        </Text>
      )}
      {settings.projects.length > 0 && (
        <>
          <Text mt="6" fontSize="xs" letterSpacing="0.08em" textTransform="uppercase" color="text.muted" px="3">
            Projects
          </Text>
          <Box data-mock-part="project-list" mt="2" borderWidth="1px" borderColor="border" borderRadius="lg" bg="surface.raised" overflow="hidden">
            {settings.projects.map((p) => (
              <Box key={p.project} {...rowProps} cursor="default" _hover={{}}>
                <Box flex="1" minW="0" fontFamily="mono">
                  {p.project}
                </Box>
                {p.harness ? (
                  <>
                    <Chip tone={p.harness === "codex" ? "waiting" : "muted"}>{harnessName(p.harness)}</Chip>
                    <Button emphasis="secondary" tone="muted" size="sm" disabled={pending} onClick={() => onSetProject(p.project, "")}>
                      Remove override
                    </Button>
                  </>
                ) : (
                  <>
                    <Text fontSize="sm" color="text.muted">
                      Inherits global ({harnessName(settings.default)})
                    </Text>
                    <Button emphasis="secondary" tone="muted" size="sm" disabled={pending} onClick={() => onSetProject(p.project, otherHarness(settings.default))}>
                      Override
                    </Button>
                  </>
                )}
              </Box>
            ))}
          </Box>
        </>
      )}
    </Frame>
  );
}

export function AgentsSettingsPage({
  fetchSettings = fetchAgentSettings,
  post = postAgentSettings,
}: {
  fetchSettings?: FetchAgentSettings;
  post?: PostAgentSettings;
}) {
  const settings = useFetched("settings-agents", fetchSettings);
  const write = useWrite({ toastRefusal: false });
  const [notice, setNotice] = useState<string>();
  const send = (req: Parameters<PostAgentSettings>[0]) => {
    setNotice(undefined);
    void write.run(() => post(req), {
      onDone: (res) => {
        setNotice(res.notice || undefined);
        settings.retry();
      },
    });
  };
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings", name: "Agents" }}>
      {settings.data ? (
        <AgentsSettingsView
          settings={settings.data}
          notice={notice}
          refusal={write.refusal}
          pending={write.pending}
          onSetGlobal={(harness) => send({ scope: "global", harness })}
          onSetProject={(project, harness) => send({ scope: "project", project, harness })}
        />
      ) : (
        <Spinner label="Loading" />
      )}
    </DetailPage>
  );
}
