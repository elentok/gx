import { Box, chakra, Text } from "@chakra-ui/react";
import { useState, type FormEvent, type ReactNode } from "react";
import { AddAuthenticator } from "../auth/AddAuthenticator";
import { ErrorLine, Field, formValues } from "../auth/AuthFields";
import { fetchAuthState, type AuthState } from "../auth/authState";
import { MIN_PASSWORD, PASSWORD_HINT } from "../auth/refusals";
import { SETTINGS_HREF } from "../hrefs";
import { DetailPage } from "../page/DetailPage";
import { PageHeader } from "../page/PageHeader";
import { Button } from "../ui/button/Button";
import { Spinner } from "../ui/spinner/Spinner";
import { useFetched } from "../useFetched";
import { useWrite } from "../useWrite";
import { beginReset, changePassword, finishReset, type AccountCall } from "./account";
import { fetchLogins, type FetchLogins } from "./logins";
import { BackLink, Frame } from "./parts";

function Form({ onSubmit, children }: { onSubmit: (e: FormEvent<HTMLFormElement>) => void; children: ReactNode }) {
  return (
    <chakra.form
      mt="4"
      display="flex"
      flexDirection="column"
      gap="3"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(e);
      }}
    >
      {children}
    </chakra.form>
  );
}

function OtherDevicesNote({ count }: { count: number }) {
  if (count === 0) return null;
  return (
    <Text data-mock-part="note" fontSize="sm" bg="tone.waiting.bg" color="tone.waiting.fg" px="3" py="2" borderRadius="md">
      The {count} other {count === 1 ? "device" : "devices"} will be signed out.
    </Text>
  );
}

function Submit({ pending, children }: { pending: boolean; children: ReactNode }) {
  return (
    <Box data-mock-part="submit">
      <Button type="submit" disabled={pending}>
        {children}
      </Button>
    </Box>
  );
}

export function ChangePasswordView({
  otherDevices,
  totpOn,
  pending = false,
  error,
  onSubmit,
}: {
  otherDevices: number;
  totpOn: boolean;
  pending?: boolean;
  error?: string;
  onSubmit: (body: { password: string; totp_code?: string; new_password: string }) => void;
}) {
  const [mismatch, setMismatch] = useState(false);
  return (
    <Frame>
      <BackLink />
      <PageHeader title="Change password" />
      <Form
        onSubmit={(e) => {
          const v = formValues(e.currentTarget);
          const differ = v.new_password !== v.new_password_again;
          setMismatch(differ);
          if (!differ) onSubmit({ password: v.password, ...(totpOn && { totp_code: v.totp_code }), new_password: v.new_password });
        }}
      >
        {(error || mismatch) && <ErrorLine>{mismatch ? "The new passwords don't match." : error}</ErrorLine>}
        <Field part="current-password" label="Current password" name="password" type="password" autoComplete="current-password" required />
        {totpOn && (
          <Field
            part="code"
            label="Code from your authenticator app"
            name="totp_code"
            placeholder="123456"
            inputMode="numeric"
            autoComplete="one-time-code"
            required
          />
        )}
        <Field
          part="new-password"
          label="New password"
          name="new_password"
          type="password"
          autoComplete="new-password"
          minLength={MIN_PASSWORD}
          hint={PASSWORD_HINT}
          required
        />
        <Field part="new-password-again" label="New password again" name="new_password_again" type="password" autoComplete="new-password" required />
        <OtherDevicesNote count={otherDevices} />
        <Submit pending={pending}>Change password</Submit>
      </Form>
    </Frame>
  );
}

export interface NewAuthenticator {
  secret: string;
  uri: string;
}

/** "reset" asks for a code from the current app; "add" is for a cleared secret, so only the password. */
export type AuthenticatorMode = "reset" | "add";

export function AuthenticatorView({
  mode,
  otherDevices,
  next,
  pending = false,
  error,
  onBegin,
  onFinish,
}: {
  mode: AuthenticatorMode;
  otherDevices: number;
  /** Set once the password and code checked out: the second step shows it. */
  next?: NewAuthenticator;
  pending?: boolean;
  error?: string;
  onBegin: (body: { password: string; totp_code?: string }) => void;
  onFinish: (body: { totp_code: string }) => void;
}) {
  return (
    <Frame>
      <BackLink />
      <PageHeader title={mode === "add" ? "Add authenticator" : "Reset authenticator"} />
      {next ? (
        <Form onSubmit={(e) => onFinish({ totp_code: formValues(e.currentTarget).totp_code })}>
          <AddAuthenticator secret={next.secret} uri={next.uri} error={error} codeLabel="Code from the new app" />
          <Submit pending={pending}>Save new authenticator</Submit>
          {mode === "reset" && (
            <Text data-mock-part="keep-note" fontSize="sm" color="text.dim">
              The old app keeps working until this code checks out.
            </Text>
          )}
        </Form>
      ) : (
        <Form
          onSubmit={(e) => {
            const v = formValues(e.currentTarget);
            onBegin({ password: v.password, ...(mode === "reset" && { totp_code: v.totp_code }) });
          }}
        >
          {error && <ErrorLine>{error}</ErrorLine>}
          <Field part="current-password" label="Current password" name="password" type="password" autoComplete="current-password" required />
          {mode === "reset" && (
            <Field
              part="code"
              label="Code from the current app"
              name="totp_code"
              placeholder="123456"
              inputMode="numeric"
              autoComplete="one-time-code"
              required
            />
          )}
          <OtherDevicesNote count={otherDevices} />
          <Submit pending={pending}>Next</Submit>
        </Form>
      )}
    </Frame>
  );
}

function useAccountPage(fetchAuth: () => Promise<AuthState>, fetchList: FetchLogins) {
  const auth = useFetched("settings-auth", fetchAuth);
  const logins = useFetched("settings-logins", fetchList);
  const write = useWrite({ toastRefusal: false });
  const otherDevices = logins.data?.filter((l) => !l.this_device).length ?? 0;
  const leave = () => {
    window.location.hash = SETTINGS_HREF;
  };
  return { auth: auth.data, loading: !auth.data || !logins.data, otherDevices, write, leave, error: write.refusal?.message ?? write.error };
}

export function ChangePasswordPage({
  fetchAuth = fetchAuthState,
  fetchList = fetchLogins,
  change = changePassword,
}: {
  fetchAuth?: () => Promise<AuthState>;
  fetchList?: FetchLogins;
  change?: AccountCall;
}) {
  const { auth, loading, otherDevices, write, leave, error } = useAccountPage(fetchAuth, fetchList);
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings", name: "Change password" }}>
      {loading || !auth ? (
        <Spinner label="Loading" />
      ) : (
        <ChangePasswordView
          otherDevices={otherDevices}
          totpOn={auth.totp_on}
          pending={write.pending}
          error={error}
          onSubmit={(body) =>
            void write.run(() => change(body), { onDone: leave, success: () => ({ type: "success", title: "Password changed." }) })
          }
        />
      )}
    </DetailPage>
  );
}

export function AuthenticatorPage({
  fetchAuth = fetchAuthState,
  fetchList = fetchLogins,
  begin = beginReset,
  finish = finishReset,
}: {
  fetchAuth?: () => Promise<AuthState>;
  fetchList?: FetchLogins;
  begin?: AccountCall;
  finish?: AccountCall;
}) {
  const { auth, loading, otherDevices, write, leave, error } = useAccountPage(fetchAuth, fetchList);
  const [next, setNext] = useState<NewAuthenticator>();
  const mode: AuthenticatorMode = auth?.setup_code_required ? "add" : "reset";
  return (
    <DetailPage mode="narrow" title={{ kind: "Settings", name: mode === "add" ? "Add authenticator" : "Reset authenticator" }}>
      {loading || !auth ? (
        <Spinner label="Loading" />
      ) : !auth.totp_on ? (
        <Frame>
          <BackLink />
          <Text mt="4" color="text.muted">
            The authenticator is turned off.
          </Text>
        </Frame>
      ) : (
        <AuthenticatorView
          mode={mode}
          otherDevices={otherDevices}
          next={next}
          pending={write.pending}
          error={error}
          onBegin={(body) =>
            void write.run(() => begin(body), {
              onDone: (res) => res.secret && res.uri && setNext({ secret: res.secret, uri: res.uri }),
            })
          }
          onFinish={(body) =>
            void write.run(() => finish(body), { onDone: leave, success: () => ({ type: "success", title: "Authenticator saved." }) })
          }
        />
      )}
    </DetailPage>
  );
}
