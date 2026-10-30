import { chakra, Heading, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Button } from "../ui/button/Button";
import { AuthCard } from "./AuthCard";
import { ErrorLine, Field, formValues } from "./AuthFields";
import type { AuthResult, AuthState } from "./authState";
import { AddAuthenticator } from "./AddAuthenticator";
import { TrustHelp } from "./cert";
import { postJSON } from "./http";
import { RATE_LIMITED, reasonText } from "./refusals";

export interface LoginCardProps {
  totpOn: boolean;
  setupCodeRequired?: boolean;
  error?: string;
  submitDisabled?: boolean;
  /** Echoed back into the fields; only the gallery sets it. */
  password?: string;
  setupCode?: string;
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
}

export interface LoginAuthenticatorCardProps {
  secret: string;
  uri: string;
  error?: string;
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
}

export function LoginAuthenticatorCard({ secret, uri, error, onSubmit }: LoginAuthenticatorCardProps) {
  return (
    <AuthCard below={<TrustHelp />}>
      <chakra.form onSubmit={onSubmit} display="flex" flexDirection="column" gap="4">
        <Heading as="h1" data-mock-part="heading" fontFamily="condensed" fontSize="xl">
          Add gx to your authenticator app
        </Heading>
        <AddAuthenticator secret={secret} uri={uri} error={error} />
        <Button type="submit" data-mock-part="submit" style={{ justifyContent: "center" }}>
          Sign in
        </Button>
        <Text data-mock-part="save-note" fontSize="xs" color="text.dim">
          gx saves nothing until this code checks out.
        </Text>
      </chakra.form>
    </AuthCard>
  );
}

export function LoginCard({ totpOn, setupCodeRequired, error, submitDisabled, password, setupCode, onSubmit }: LoginCardProps) {
  return (
    <AuthCard below={<TrustHelp />}>
      <chakra.form onSubmit={onSubmit} display="flex" flexDirection="column" gap="4">
        <Heading as="h1" data-mock-part="heading" fontFamily="condensed" fontSize="xl">
          Sign in
        </Heading>
        {error && <ErrorLine>{error}</ErrorLine>}
        {setupCodeRequired && (
          <Field part="setup-code" label="Setup code" name="setup_code" placeholder="XXXX-XXXX" defaultValue={setupCode} />
        )}
        <Field part="password" label="Password" name="password" type="password" defaultValue={password} autoFocus />
        {totpOn && (
          <Field
            part="code"
            label="Code from your authenticator app"
            name="totp_code"
            placeholder="123456"
            inputMode="numeric"
            autoComplete="one-time-code"
          />
        )}
        <Button type="submit" data-mock-part="submit" disabled={submitDisabled} style={{ justifyContent: "center" }}>
          Sign in
        </Button>
        <Text data-mock-part="reset-hint" fontSize="xs" color="text.dim">
          Forgot the password or lost the authenticator? Run <code>gx server web-reset</code> on the server.
        </Text>
      </chakra.form>
    </AuthCard>
  );
}

export function LoginPage({ auth, onDone }: { auth: AuthState; onDone: () => void }) {
  const [error, setError] = useState("");
  const [authenticator, setAuthenticator] = useState<{ pending_id: string; secret: string; uri: string } | null>(null);
  const [limited, setLimited] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const v = formValues(e.currentTarget);
    const body: Record<string, string> = { password: v.password };
    if (auth.totp_on) body.totp_code = v.totp_code;
    if (auth.setup_code_required) body.setup_code = v.setup_code;
    try {
      const res = await postJSON<AuthResult>("/v1/web/auth/login", body);
      setError(res.refused ? reasonText(res) : "");
      if (res.authenticator_needed) {
        setAuthenticator({ pending_id: res.pending_id ?? "", secret: res.secret ?? "", uri: res.uri ?? "" });
      }
      if (res.refused && res.reason === RATE_LIMITED && res.wait_seconds) {
        setLimited(true);
        clearTimeout(timer.current);
        timer.current = setTimeout(() => {
          setLimited(false);
          setError("");
        }, res.wait_seconds * 1000);
      }
      if (!res.refused && !res.authenticator_needed) onDone();
    } catch (err) {
      setError(String(err));
    }
  };
  const onAddAuthenticator = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const { totp_code } = formValues(e.currentTarget);
    try {
      const res = await postJSON<AuthResult>("/v1/web/auth/add-authenticator", { pending_id: authenticator?.pending_id, totp_code });
      if (res.refused) return setError(reasonText(res));
      onDone();
    } catch (err) {
      setError(String(err));
    }
  };
  if (authenticator) {
    return (
      <LoginAuthenticatorCard
        secret={authenticator.secret}
        uri={authenticator.uri}
        error={error}
        onSubmit={(e) => void onAddAuthenticator(e)}
      />
    );
  }
  return (
    <LoginCard
      totpOn={auth.totp_on}
      setupCodeRequired={auth.setup_code_required}
      error={error}
      submitDisabled={limited}
      onSubmit={(e) => void onSubmit(e)}
    />
  );
}
