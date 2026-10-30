import { Box, chakra, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { useParams } from "react-router";
import { fetchAuthState, type AuthState } from "../auth/authState";
import { SETTINGS_HREF } from "../hrefs";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { Button } from "../ui/button/Button";
import { Chip } from "../ui/chip/Chip";
import { Spinner } from "../ui/spinner/Spinner";
import { useFetched } from "../useFetched";
import { useWrite } from "../useWrite";
import {
  endsInDays,
  fetchLogins,
  lastSeenText,
  renameLogin,
  revokeLogin,
  signOutThisDevice,
  type FetchLogins,
  type LoginWrite,
  type WebLoginEntry,
} from "./logins";

const DAY_MS = 24 * 60 * 60 * 1000;
const LOGINS_HREF =`${SETTINGS_HREF}/logins`;

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
  <chakra.a
    href={SETTINGS_HREF}
    display="inline-flex"
    alignItems="center"
    h="8"
    px="3"
    fontSize="sm"
    fontWeight="600"
    color="text.muted"
    _hover={{ color: "text" }}
  >
    ‹ Settings
  </chakra.a>
);

export function WebLoginsView({ logins, now }: { logins: WebLoginEntry[]; now: number }) {
  return (
    <Frame>
      <BackLink />
      <PageHeader title="Web logins" />
      <Text mt="2" fontSize="sm" color="text.muted">
        A web login ends after 90 days without use.
      </Text>
      <Group>
        {logins.map((login) => {
          const endsIn = endsInDays(login.last_used, now);
          return (
            <chakra.a key={login.id} href={`${LOGINS_HREF}/${login.id}`} {...rowProps} py="1">
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

const Fact = ({ label, value }: { label: string; value: string }) => (
  <Box {...rowProps} cursor="default" _hover={{}}>
    <Box flex="1">{label}</Box>
    <Text fontSize="sm" color="text.muted">
      {value}
    </Text>
  </Box>
);

const signedInText = (created?: string) =>
  created ? new Date(created).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" }) : "Unknown";

// The mock counts in days from one day on; lastSeenText would say "10w".
function lastUsedText(lastUsed: string, now: number): string {
  const days = Math.floor((now - Date.parse(lastUsed)) / DAY_MS);
  return days >= 1 ? `${days} ${days === 1 ? "day" : "days"} ago` : lastSeenText({ last_used: lastUsed, last_ip: "" }, now);
}

export function WebLoginView({
  login,
  now,
  onRename,
  onSignOut,
  pending = false,
  startConfirming = false,
}: {
  login: WebLoginEntry;
  now: number;
  onRename: (name: string) => void;
  onSignOut: () => void;
  pending?: boolean;
  /** Opens with the confirm step showing, for the gallery. */
  startConfirming?: boolean;
}) {
  const [confirming, setConfirming] = useState(startConfirming);
  return (
    <Frame>
      <BackLink />
      <PageHeader title={login.name} />
      <Group>
        <Box data-mock-part="login-facts">
          <Fact label="Signed in" value={signedInText(login.created)} />
          <Fact label="Last used" value={lastUsedText(login.last_used, now)} />
          <Fact label="Last IP" value={login.last_ip || "Unknown"} />
        </Box>
      </Group>
      <Box mt="6">
        <chakra.label data-mock-part="name" display="block">
          <Text fontSize="sm" color="text.muted" mb="2">
            Name
          </Text>
          <chakra.input
            key={login.name}
            type="text"
            defaultValue={login.name}
            w="100%"
            h="40px"
            px="3"
            borderWidth="1px"
            borderColor="border"
            borderRadius="8px"
            bg="surface.raised"
            color="text"
            onBlur={(e) => {
              const name = e.currentTarget.value.trim();
              if (name && name !== login.name) onRename(name);
            }}
          />
          <Text fontSize="xs" color="text.dim" mt="1">
            Saved when you leave the field.
          </Text>
        </chakra.label>
      </Box>
      <Group>
        <chakra.button type="button" data-mock-part="sign-out" onClick={() => setConfirming(true)} {...rowProps} color="tone.deadlocked.fg">
          Sign out this device
        </chakra.button>
      </Group>
      {confirming && (
        <Box data-mock-part="confirm" mt="4" display="flex" flexDirection="column" gap="3">
          <Text data-mock-part="note" fontSize="sm" bg="tone.waiting.bg" color="tone.waiting.fg" px="3" py="2" borderRadius="md">
            It has to sign in again with the password and a code.
          </Text>
          <Box display="flex" gap="2">
            <Button tone="paused" disabled={pending} onClick={onSignOut}>
              Sign out
            </Button>
            <Button emphasis="ghost" tone="muted" onClick={() => setConfirming(false)}>
              Cancel
            </Button>
          </Box>
        </Box>
      )}
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

export function WebLoginPage({
  fetchList = fetchLogins,
  rename = renameLogin,
  revoke = revokeLogin,
  signOut = signOutThisDevice,
  now = Date.now,
}: {
  fetchList?: FetchLogins;
  rename?: LoginWrite;
  revoke?: LoginWrite;
  signOut?: () => void | Promise<void>;
  now?: () => number;
}) {
  const { id = "" } = useParams();
  const logins = useFetched("settings-logins", fetchList);
  const write = useWrite();
  const login = logins.data?.find((l) => l.id === id);
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings", name: login?.name ?? "Web login" }}>
      {!logins.data ? (
        <Spinner label="Loading" />
      ) : !login ? (
        <Frame>
          <BackLink />
          <Text mt="4" color="text.muted">
            This web login no longer exists.
          </Text>
        </Frame>
      ) : (
        <WebLoginView
          login={login}
          now={now()}
          pending={write.pending}
          onRename={(name) => void write.run(() => rename(login.id, name), { onDone: logins.retry })}
          onSignOut={() =>
            login.this_device
              ? void signOut()
              : void write.run(() => revoke(login.id), {
                  onDone: () => {
                    window.location.hash = LOGINS_HREF;
                  },
                })
          }
        />
      )}
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
