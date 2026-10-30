import { Box, chakra, Heading, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Button } from "../ui/button/Button";
import { AuthCard } from "./AuthCard";
import { ErrorLine, Field } from "./AuthFields";
import { postAuth, SETUP_CODE_WRONG, type AuthResult, type AuthState } from "./authState";
import { AddAuthenticator, TOTP_MISMATCH } from "./AddAuthenticator";
import { CER_LABEL, CertLink, Fingerprint, PROFILE_LABEL, useCert } from "./cert";

const WRONG_CREDENTIALS = "wrong-credentials";
const RATE_LIMITED = "rate-limited";
const SETUP_CODE_WRONG_REASON = "setup-code-wrong";
const TOTP_MISMATCH_REASON = "totp-mismatch";

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

export function TrustHelp() {
  const { cert, startOpen } = useCert();
  return (
    <chakra.details data-mock-part="trust-help" open={startOpen} mt="6" color="text.dim" fontSize="sm">
      <Box as="summary" cursor="pointer" textAlign="center">
        Browser warns about this page?
      </Box>
      {cert && (
        <Box mt="3" display="flex" flexDirection="column" gap="2">
          <Text>
            gx signs this page with its own certificate. Install it once per device:{" "}
            <CertLink href={cert.profile_path}>{PROFILE_LABEL}</CertLink> · <CertLink href={cert.cer_path}>{CER_LABEL}</CertLink>
          </Text>
          <Text>On iPhone, after installing, turn it on in Settings › General › About › Certificate Trust Settings.</Text>
          <Text>
            Check that the fingerprint matches <code>gx server status</code>:
          </Text>
          <Fingerprint value={cert.fingerprint} />
        </Box>
      )}
    </chakra.details>
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

export function rateLimitText(waitSeconds: number): string {
  const minutes = Math.max(1, Math.ceil(waitSeconds / 60));
  return `Too many tries. Try again in ${minutes} ${minutes === 1 ? "minute" : "minutes"}.`;
}

function refusalText(res: AuthResult): string {
  if (res.reason === WRONG_CREDENTIALS) return "Wrong password or code";
  if (res.reason === SETUP_CODE_WRONG_REASON) return SETUP_CODE_WRONG;
  if (res.reason === TOTP_MISMATCH_REASON) return TOTP_MISMATCH;
  if (res.reason === RATE_LIMITED && res.wait_seconds) return rateLimitText(res.wait_seconds);
  return res.reason ?? "Sign in refused";
}

function value(form: HTMLFormElement, name: string): string {
  return (form.elements.namedItem(name) as HTMLInputElement | null)?.value ?? "";
}

export function LoginPage({ auth, onDone }: { auth: AuthState; onDone: () => void }) {
  const [error, setError] = useState("");
  const [authenticator, setAuthenticator] = useState<{ pending_id: string; secret: string; uri: string } | null>(null);
  const [limited, setLimited] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const f = e.currentTarget;
    const body: Record<string, string> = { password: value(f, "password") };
    if (auth.totp_on) body.totp_code = value(f, "totp_code");
    if (auth.setup_code_required) body.setup_code = value(f, "setup_code");
    try {
      const res = await postAuth("login", body);
      setError(res.refused ? refusalText(res) : "");
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
    const totp_code = value(e.currentTarget, "totp_code");
    try {
      const res = await postAuth("add-authenticator", { pending_id: authenticator?.pending_id, totp_code });
      if (res.refused) return setError(refusalText(res));
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
