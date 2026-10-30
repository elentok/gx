import { Box, chakra, Heading, Text } from "@chakra-ui/react";
import { useEffect, useState, type FormEvent } from "react";
import { Button } from "../ui/button/Button";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { AuthCard } from "./AuthCard";
import { ErrorLine, Field } from "./AuthFields";
import { postAuth, type AuthState } from "./authState";
import { AddAuthenticator, TOTP_MISMATCH } from "./AddAuthenticator";
import { TrustHelp } from "./LoginPage";

const SETUP_HASH = "#/setup";
const WRONG_CODE = "That code is wrong or expired. Run gx server status for a new one.";
const MIN_PASSWORD = 12;
const SHORT_PASSWORD = `The password needs at least ${MIN_PASSWORD} characters.`;
const MISMATCH = "The two passwords don't match.";
const REFUSALS: Record<string, string> = {
  "setup-code-wrong": WRONG_CODE,
  "password-too-short": SHORT_PASSWORD,
  "totp-mismatch": TOTP_MISMATCH,
};

function field(form: HTMLFormElement, name: string): string {
  return (form.elements.namedItem(name) as HTMLInputElement).value;
}

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
          hint="At least 12 characters."
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

export function SetupPage({ auth, onDone }: { auth: AuthState; onDone: () => void }) {
  const initial = useCodeFromHash();
  const [code, setCode] = useState(initial);
  const [error, setError] = useState("");
  const [password, setPassword] = useState("");
  const [authenticator, setAuthenticator] = useState({ secret: "", uri: "" });
  const [step, setStep] = useState<1 | 2 | 3>(1);

  const onCode = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const typed = field(e.currentTarget, "code");
    setCode(typed);
    try {
      const res = await postAuth("setup-start", { code: typed });
      setError(res.refused ? WRONG_CODE : "");
      if (res.refused) return;
      setAuthenticator({ secret: res.secret ?? "", uri: res.uri ?? "" });
      setStep(2);
    } catch (err) {
      setError(String(err));
    }
  };

  const finish = async (body: object) => {
    try {
      const res = await postAuth("setup-finish", { code, password, ...body });
      if (res.refused) return setError(REFUSALS[res.reason ?? ""] ?? res.reason ?? "refused");
      window.location.hash = "#/";
      onDone();
    } catch (err) {
      setError(String(err));
    }
  };

  const onPassword = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const typed = field(e.currentTarget, "password");
    if (typed.length < MIN_PASSWORD) return setError(SHORT_PASSWORD);
    if (typed !== field(e.currentTarget, "again")) return setError(MISMATCH);
    setPassword(typed);
    if (auth.totp_on) {
      setError("");
      return setStep(3);
    }
    await finish({ password: typed });
  };

  const onTotp = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    void finish({ totp_code: field(e.currentTarget, "totp_code") });
  };

  if (step === 3) {
    return <SetupTotpCard {...authenticator} error={error} onSubmit={onTotp} />;
  }
  if (step === 2) {
    return <SetupPasswordCard totpOn={auth.totp_on} error={error} onSubmit={(e) => void onPassword(e)} />;
  }
  return <SetupCodeCard totpOn={auth.totp_on} code={code} error={error} onSubmit={(e) => void onCode(e)} />;
}
