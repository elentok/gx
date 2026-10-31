import { Box, chakra, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import { useParams } from "react-router";
import { fetchAuthState, type AuthState } from "../auth/authState";
import { CER_LABEL, Fingerprint, PROFILE_LABEL, type CertInfo } from "../auth/cert";
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
import { harnessName, fetchAgentSettings, type FetchAgentSettings } from "./agents";
import { BackLink, Frame, Group, rowProps } from "./parts";

const DAY_MS = 24 * 60 * 60 * 1000;
const LOGINS_HREF = `${SETTINGS_HREF}/logins`;
const AGENTS_HREF = `${SETTINGS_HREF}/agents`;

const Chevron = () => (
  <Text aria-hidden color="text.dim">
    ›
  </Text>
);

function LinkRow({ href, label, detail }: { href: string; label: string; detail?: ReactNode }) {
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

export function SettingsHomeView({
  loginCount,
  totpOn,
  authenticatorMissing = false,
  defaultHarness,
  onSignOut,
}: {
  loginCount?: number;
  /** The global Default harness; the Agents row shows no detail until it loads. */
  defaultHarness?: string;
  totpOn: boolean;
  /** TOTP is on but the saved secret was cleared, so the row adds one instead of resetting it. */
  authenticatorMissing?: boolean;
  onSignOut: () => void;
}) {
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
        {totpOn && <LinkRow href={`${SETTINGS_HREF}/authenticator`} label={authenticatorMissing ? "Add authenticator" : "Reset authenticator"} />}
        <LinkRow href={`${SETTINGS_HREF}/certificate`} label="Certificate" />
      </Group>
      <Group>
        <LinkRow
          href={AGENTS_HREF}
          label="Agents"
          detail={
            defaultHarness === undefined ? undefined : (
              <>
                Default: <b>{harnessName(defaultHarness)}</b>
              </>
            )
          }
        />
      </Group>
      <Group>
        <chakra.button type="button" onClick={onSignOut} {...rowProps} color="tone.deadlocked.fg">
          Sign out of this device
        </chakra.button>
      </Group>
    </Frame>
  );
}

// The mock lists this device first, then the other logins newest to oldest.
const sortLogins = (logins: WebLoginEntry[]) =>
  [...logins].sort((a, b) => Number(b.this_device) - Number(a.this_device) || Date.parse(b.last_used) - Date.parse(a.last_used));

export function WebLoginsView({ logins, now }: { logins: WebLoginEntry[]; now: number }) {
  return (
    <Frame>
      <BackLink />
      <PageHeader title="Web logins" />
      <Text mt="2" fontSize="sm" color="text.muted">
        A web login ends after 90 days without use.
      </Text>
      <Group>
        {sortLogins(logins).map((login) => {
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

export function CertificateView({ cert }: { cert: CertInfo }) {
  return (
    <Frame>
      <BackLink />
      <PageHeader title="Certificate" />
      <Box data-mock-part="downloads">
        <Group>
          <LinkRow href={cert.profile_path} label={PROFILE_LABEL} />
          <LinkRow href={cert.cer_path} label={`${CER_LABEL} (.cer)`} />
        </Group>
      </Box>
      <Box data-mock-part="fingerprint" mt="4">
        <Text fontSize="sm" color="text.dim" mb="1">
          SHA-256 fingerprint
        </Text>
        <Fingerprint value={cert.fingerprint} size="sm" />
      </Box>
    </Frame>
  );
}

export function CertificatePage({ fetchAuth = fetchAuthState }: { fetchAuth?: () => Promise<AuthState> }) {
  const auth = useFetched("settings-auth", fetchAuth);
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings", name: "Certificate" }}>
      {auth.data ? <CertificateView cert={auth.data} /> : <Spinner label="Loading" />}
    </DetailPage>
  );
}

export function SettingsPage({
  fetchAuth = fetchAuthState,
  fetchList = fetchLogins,
  fetchAgents = fetchAgentSettings,
  signOut = signOutThisDevice,
}: {
  fetchAuth?: () => Promise<AuthState>;
  fetchList?: FetchLogins;
  fetchAgents?: FetchAgentSettings;
  signOut?: () => void | Promise<void>;
}) {
  const auth = useFetched("settings-auth", fetchAuth);
  const logins = useFetched("settings-logins", fetchList);
  const agents = useFetched("settings-agents", fetchAgents);
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings" }}>
      {auth.data ? (
        <SettingsHomeView
          loginCount={logins.data?.length}
          defaultHarness={agents.data?.default}
          totpOn={auth.data.totp_on}
          authenticatorMissing={auth.data.setup_code_required}
          onSignOut={() => void signOut()}
        />
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

/** The `/settings/*` catch-all: a settings URL nothing handles, so only the way back. */
export function UnknownSettingsPage() {
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings" }}>
      <Frame>
        <BackLink />
      </Frame>
    </DetailPage>
  );
}
