import { Box, chakra, Heading, Text } from "@chakra-ui/react";
import { useEffect, useState, type FormEvent } from "react";
import { Button } from "../ui/button/Button";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { AuthCard } from "./AuthCard";
import { ErrorLine, Field, formValues } from "./AuthFields";
import type { AuthResult, AuthState } from "./authState";
import { AddAuthenticator } from "./AddAuthenticator";
import { TrustHelp } from "./cert";
import { postJSON } from "./http";
import { MIN_PASSWORD, PASSWORD_HINT, PASSWORD_TOO_SHORT, reasonText, SETUP_CODE_WRONG } from "./refusals";

const SETUP_HASH = "#/setup";
const MISMATCH = "The two passwords don't match.";

export interface SetupCodeCardProps {
  totpOn: boolean;
  error?: string;
  /** Echoed back into the field; the page passes the code from the QR link. */
  code?: string;
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
}

export function SetupCodeCard({ totpOn, error, code, onSubmit }: SetupCodeCardProps) {
  const steps = totpOn ? 3 : 2;
  return (
    <AuthCard below={<TrustHelp />}>
      <chakra.form onSubmit={onSubmit} display="flex" flexDirection="column" gap="4">
        <Box data-mock-part="step-header">
          <Text fontSize="xs" color="text.dim" mb="2" textTransform="uppercase" letterSpacing="0.06em">
            Set up the web UI · step 1 of {steps}
          </Text>
          <ProgressBar value={1} max={steps} />
        </Box>
        <Heading as="h1" data-mock-part="heading" fontFamily="condensed" fontSize="xl">
          Enter the setup code
        </Heading>
        <Text data-mock-part="intro" fontSize="sm" color="text.dim">
          Run <code>gx server status</code> or <code>gx server web-url --qr</code> on the server to see it.
        </Text>
        {error && <ErrorLine>{error}</ErrorLine>}
        <Field
          part="setup-code"
          label="Setup code"
          name="code"
          placeholder="XXXX-XXXX"
          defaultValue={code}
          key={code}
          autoFocus
        />
        <Button type="submit" data-mock-part="submit">
          Continue
        </Button>
      </chakra.form>
    </AuthCard>
  );
}

export interface SetupPasswordCardProps {
  totpOn: boolean;
  error?: string;
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
}

export function SetupPasswordCard({ totpOn, error, onSubmit }: SetupPasswordCardProps) {
  const steps = totpOn ? 3 : 2;
  return (
    <AuthCard below={<TrustHelp />}>
      <chakra.form onSubmit={onSubmit} display="flex" flexDirection="column" gap="4">
        <Box data-mock-part="step-header">
          <Text fontSize="xs" color="text.dim" mb="2" textTransform="uppercase" letterSpacing="0.06em">
            Set up the web UI · step 2 of {steps}
          </Text>
          <ProgressBar value={2} max={steps} />
        </Box>
        <Heading as="h1" data-mock-part="heading" fontFamily="condensed" fontSize="xl">
          Choose a password
        </Heading>
        {error && <ErrorLine>{error}</ErrorLine>}
        <Field
          part="password"
          label="Password"
          name="password"
          type="password"
          autoComplete="new-password"
          hint={PASSWORD_HINT}
          autoFocus
        />
        <Field
          part="password-again"
          label="Password again"
          name="again"
          type="password"
          autoComplete="new-password"
        />
        <Button type="submit" data-mock-part="submit">
          {totpOn ? "Continue" : "Finish setup"}
        </Button>
      </chakra.form>
    </AuthCard>
  );
}

export interface SetupTotpCardProps {
  secret: string;
  uri: string;
  error?: string;
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
}

export function SetupTotpCard({ secret, uri, error, onSubmit }: SetupTotpCardProps) {
  return (
    <AuthCard below={<TrustHelp />}>
      <chakra.form onSubmit={onSubmit} display="flex" flexDirection="column" gap="4">
        <Box data-mock-part="step-header">
          <Text fontSize="xs" color="text.dim" mb="2" textTransform="uppercase" letterSpacing="0.06em">
            Set up the web UI · step 3 of 3
          </Text>
          <ProgressBar value={3} max={3} />
        </Box>
        <Heading as="h1" data-mock-part="heading" fontFamily="condensed" fontSize="xl">
          Add gx to your authenticator app
        </Heading>
        <AddAuthenticator secret={secret} uri={uri} error={error} />
        <Button type="submit" data-mock-part="submit">
          Finish setup
        </Button>
        <Text data-mock-part="save-note" fontSize="xs" color="text.dim">
          gx saves nothing until this code checks out.
        </Text>
      </chakra.form>
    </AuthCard>
  );
}

// Reads the code from `#/setup?code=…` once, then drops it from the URL.
function useCodeFromHash(): string {
  const [code] = useState(() => new URLSearchParams(window.location.hash.split("?")[1] ?? "").get("code") ?? "");
  useEffect(() => {
    if (window.location.hash.includes("?")) history.replaceState(null, "", SETUP_HASH);
  }, []);
  return code;
}

const SAVED_KEY = "gx.setup";
interface Saved {
  code: string;
  authenticator: { secret: string; uri: string };
}

// A phone may discard this page while the person is in the authenticator app. The code and key are
// kept for the tab (never the password) so the reload lands on the password step, not a new key.
function loadSaved(): Saved | undefined {
  try {
    return JSON.parse(sessionStorage.getItem(SAVED_KEY) ?? "null") ?? undefined;
  } catch {
    return undefined;
  }
}

function saveSetup(saved?: Saved) {
  try {
    if (saved) sessionStorage.setItem(SAVED_KEY, JSON.stringify(saved));
    else sessionStorage.removeItem(SAVED_KEY);
  } catch {
    // Storage can be blocked; the page then works without the restore.
  }
}

export function SetupPage({ auth, onDone }: { auth: AuthState; onDone: () => void }) {
  const initial = useCodeFromHash();
  const [saved] = useState(() => (auth.totp_on && !initial ? loadSaved() : undefined));
  const [code, setCode] = useState(initial || saved?.code || "");
  const [error, setError] = useState("");
  const [password, setPassword] = useState("");
  const [authenticator, setAuthenticator] = useState(saved?.authenticator ?? { secret: "", uri: "" });
  const [step, setStep] = useState<1 | 2 | 3>(saved ? 2 : 1);

  const onCode = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const typed = formValues(e.currentTarget).code;
    setCode(typed);
    try {
      const res = await postJSON<AuthResult>("/v1/web/auth/setup-start", { code: typed });
      setError(res.refused ? reasonText(res) : "");
      if (res.refused) return;
      const next = { secret: res.secret ?? "", uri: res.uri ?? "" };
      setAuthenticator(next);
      if (auth.totp_on) saveSetup({ code: typed, authenticator: next });
      setStep(2);
    } catch (err) {
      setError(String(err));
    }
  };

  const finish = async (body: object) => {
    try {
      const res = await postJSON<AuthResult>("/v1/web/auth/setup-finish", { code, password, ...body });
      if (res.refused) {
        if (res.reason === SETUP_CODE_WRONG) saveSetup();
        return setError(reasonText(res));
      }
      saveSetup();
      window.location.hash = "#/";
      onDone();
    } catch (err) {
      setError(String(err));
    }
  };

  const onPassword = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const v = formValues(e.currentTarget);
    const typed = v.password;
    if (typed.length < MIN_PASSWORD) return setError(reasonText({ reason: PASSWORD_TOO_SHORT }));
    if (typed !== v.again) return setError(MISMATCH);
    setPassword(typed);
    if (auth.totp_on) {
      setError("");
      return setStep(3);
    }
    await finish({ password: typed });
  };

  const onTotp = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    void finish({ totp_code: formValues(e.currentTarget).totp_code });
  };

  if (step === 3) {
    return <SetupTotpCard {...authenticator} error={error} onSubmit={onTotp} />;
  }
  if (step === 2) {
    return <SetupPasswordCard totpOn={auth.totp_on} error={error} onSubmit={(e) => void onPassword(e)} />;
  }
  return <SetupCodeCard totpOn={auth.totp_on} code={code} error={error} onSubmit={(e) => void onCode(e)} />;
}
