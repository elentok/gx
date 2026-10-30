import { Box, chakra, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { fetchAuthState, type AuthState } from "../auth/authState";
import { SETTINGS_HREF } from "../hrefs";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { Chip } from "../ui/chip/Chip";
import { Spinner } from "../ui/spinner/Spinner";
import { useFetched } from "../useFetched";
import { endsInDays, fetchLogins, lastSeenText, signOutThisDevice, type FetchLogins, type WebLoginEntry } from "./logins";

const LOGINS_HREF = `${SETTINGS_HREF}/logins`;

const Frame = ({ children }: { children: ReactNode }) => <Box maxW="560px">{children}</Box>;

const Group = ({ label, children }: { label?: string; children: ReactNode }) => (
  <Box mt="6">
    {label && (
      <Text fontSize="xs" letterSpacing="0.08em" textTransform="uppercase" color="text.muted" mb="2" px="3">
        {label}
      </Text>
    )}
    <Box borderWidth="1px" borderColor="border" borderRadius="lg" bg="surface.raised" overflow="hidden">
      {children}
    </Box>
  </Box>
);

const rowProps = {
  display: "flex",
  alignItems: "center",
  gap: "3",
  px: "4",
  py: "3",
  color: "text",
  textAlign: "left",
  width: "100%",
  cursor: "pointer",
  bg: "transparent",
  _notFirst: { borderTopWidth: "1px", borderTopColor: "border" },
  _hover: { bg: "surface" },
} as const;

const Chevron = () => (
  <Text aria-hidden color="text.dim">
    ›
  </Text>
);

function LinkRow({ href, label, detail }: { href: string; label: string; detail?: string }) {
  return (
    <chakra.a href={href} {...rowProps}>
      <Box flex="1">{label}</Box>
      {detail && (
        <Text fontSize="sm" color="text.muted">
          {detail}
        </Text>
      )}
      <Chevron />
    </chakra.a>
  );
}

export function SettingsHomeView({ loginCount, totpOn, onSignOut }: { loginCount?: number; totpOn: boolean; onSignOut: () => void }) {
  return (
    <Frame>
      <PageHeader title="Settings" />
      <Group label="Security">
        <LinkRow
          href={LOGINS_HREF}
          label="Web logins"
          detail={loginCount === undefined ? undefined : `${loginCount} ${loginCount === 1 ? "device" : "devices"}`}
        />
        <LinkRow href={`${SETTINGS_HREF}/password`} label="Change password" />
        {totpOn && <LinkRow href={`${SETTINGS_HREF}/authenticator`} label="Reset authenticator" />}
        <LinkRow href={`${SETTINGS_HREF}/certificate`} label="Certificate" />
      </Group>
      <Group>
        <chakra.button type="button" onClick={onSignOut} {...rowProps} color="tone.deadlocked.fg">
          Sign out of this device
        </chakra.button>
      </Group>
    </Frame>
  );
}

const BackLink = () => (
  <chakra.a href={SETTINGS_HREF} display="inline-block" fontSize="sm" color="text.muted" mb="2" _hover={{ color: "text" }}>
    ‹ Settings
  </chakra.a>
);

export function WebLoginsView({ logins, now }: { logins: WebLoginEntry[]; now: number }) {
  return (
    <Frame>
      <BackLink />
      <PageHeader title="Web logins" />
      <Text mt="2" color="text.muted">
        A web login ends after 90 days without use.
      </Text>
      <Group>
        {logins.map((login) => {
          const endsIn = endsInDays(login.last_used, now);
          return (
            <chakra.a key={login.id} href={`${LOGINS_HREF}/${login.id}`} {...rowProps}>
              <Box flex="1" minW="0">
                <Text fontWeight="600">{login.name}</Text>
                <Text fontSize="sm" color={endsIn === undefined ? "text.muted" : "tone.waiting.fg"}>
                  {endsIn === undefined ? lastSeenText(login, now) : `Ends in ${endsIn} ${endsIn === 1 ? "day" : "days"} unless used`}
                </Text>
              </Box>
              {login.this_device && <Chip tone="done">This device</Chip>}
              <Chevron />
            </chakra.a>
          );
        })}
      </Group>
    </Frame>
  );
}

export function SettingsStubView() {
  return (
    <Frame>
      <BackLink />
    </Frame>
  );
}

export function SettingsPage({
  fetchAuth = fetchAuthState,
  fetchList = fetchLogins,
  signOut = signOutThisDevice,
}: {
  fetchAuth?: () => Promise<AuthState>;
  fetchList?: FetchLogins;
  signOut?: () => void | Promise<void>;
}) {
  const auth = useFetched("settings-auth", fetchAuth);
  const logins = useFetched("settings-logins", fetchList);
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings" }}>
      {auth.data ? (
        <SettingsHomeView loginCount={logins.data?.length} totpOn={auth.data.totp_on} onSignOut={() => void signOut()} />
      ) : (
        <Spinner label="Loading" />
      )}
    </DetailPage>
  );
}

export function WebLoginsPage({ fetchList = fetchLogins, now = Date.now }: { fetchList?: FetchLogins; now?: () => number }) {
  const logins = useFetched("settings-logins", fetchList);
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings", name: "Web logins" }}>
      {logins.data ? <WebLoginsView logins={logins.data} now={now()} /> : <Spinner label="Loading" />}
    </DetailPage>
  );
}

export function SettingsStubPage() {
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings" }}>
      <SettingsStubView />
    </DetailPage>
  );
}
