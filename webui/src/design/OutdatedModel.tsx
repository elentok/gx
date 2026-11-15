import { chakra, Popover, Portal, Text } from "@chakra-ui/react"
import type { SessionInfo } from "../generated/server"
import { SETTINGS_HREF } from "../hrefs"
import { modelName } from "../modelName"
import { openModelPicker, postSwitchSession } from "../shell/modelPickerOpen"
import { Button } from "../ui/button/Button"
import { useWrite } from "../useWrite"
import { blockedNote, TokenCostWarning } from "./modelSwitch"
import { LinkButton } from "./LinkButton"
import { stateToken } from "./open"

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

/** The footer model name as a red button that opens the one-click switch to the latest model. */
export function OutdatedModel({ session, text }: { session: SessionInfo; text: string }) {
  const { run, pending } = useWrite()
  const latest = session.latest_model ?? ""
  const latestLabel = modelName(latest, session.harness)
  const current = modelName(session.model ?? "", session.harness)
  const blocked = blockedNote(session)
  const kept = session.latest_effort ?? ""
  const note =
    blocked ??
    (!kept
      ? "Uses the model's default effort."
      : kept === (session.effort ?? "")
        ? `Keeps effort ${kept}. Only this session changes.`
        : `Changes effort to ${kept}. Only this session changes.`)
  const switchNow = () =>
    void run(() =>
      postSwitchSession({
        id: session.id,
        state: stateToken(session),
        model: latest,
        ...(kept ? { effort: kept } : {}),
      }),
    )
  return (
    <Popover.Root
      positioning={{ placement: "top-start", gutter: 8, fitViewport: true, overflowPadding: 8 }}
      lazyMount
      unmountOnExit
    >
      <Popover.Trigger asChild>
        <chakra.button
          type="button"
          data-testid="outdated-model"
          minW="0"
          truncate
          borderWidth="0"
          bg="transparent"
          p="0"
          fontSize="inherit"
          color="tone.deadlocked.fg"
          fontWeight="700"
          cursor="pointer"
          textDecoration="underline dotted"
          textUnderlineOffset="2px"
        >
          {text}
        </chakra.button>
      </Popover.Trigger>
      <Portal>
        <Popover.Positioner>
          <Popover.Content
            role="dialog"
            aria-label="Newer model"
            bg="surface.raised"
            borderWidth="1px"
            borderColor="border"
            borderRadius="md"
            boxShadow="lg"
            color="text"
            boxSizing="border-box"
            w="80"
            maxW="calc(100vw - 2rem)"
            p="3"
            fontSize="xs"
            display="flex"
            flexDirection="column"
            gap="2"
          >
            <Text fontWeight="700">
              A newer {capitalize(session.latest_family ?? "")} model is available
            </Text>
            <Text color="text.dim">
              This session uses {current}. {latestLabel} replaces it.
            </Text>
            <Button
              tone="running"
              disabled={!!blocked || pending}
              onClick={switchNow}
              _disabled={{
                bg: "surface",
                borderColor: "surface",
                color: "text.dim",
                opacity: 1,
                cursor: "default",
              }}
            >
              Switch to {latestLabel}
            </Button>
            <Text color="text.dim" data-testid="outdated-note">
              {note}
            </Text>
            <TokenCostWarning session={session} />
            {!!session.latest_tiers?.length && (
              <Text color="text.dim" data-testid="tiers-note">
                Settings use {current} for the {session.latest_tiers.join(", ")} tier
                {session.latest_tiers.length > 1 ? "s" : ""}.{" "}
                <chakra.a
                  href={`${SETTINGS_HREF}/s/models`}
                  color="tone.running.fg"
                  textDecoration="underline"
                >
                  Open Models settings
                </chakra.a>
                . This switch does not change Settings.
              </Text>
            )}
            {!session.design && (
              <LinkButton
                alignSelf="flex-start"
                color="text.dim"
                textDecoration="underline"
                onClick={() => openModelPicker({ session })}
              >
                Choose another model
              </LinkButton>
            )}
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  )
}
