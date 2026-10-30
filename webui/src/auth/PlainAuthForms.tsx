import { useState, type FormEvent } from "react";
import { postAuth, type AuthResult, type AuthState } from "./authState";

// Unstyled stand-in for the setup page; that ticket replaces this file.
interface FormProps {
  auth: AuthState;
  onDone: () => void;
}

function useSubmit(step: "setup-start" | "setup-finish" | "login", onDone: () => void) {
  const [message, setMessage] = useState("");
  const [result, setResult] = useState<AuthResult | null>(null);
  const submit = async (body: object) => {
    try {
      const res = await postAuth(step, body);
      setResult(res);
      setMessage(res.refused ? (res.reason ?? "refused") : "");
      if (!res.refused && !res.authenticator_needed && step !== "setup-start") onDone();
    } catch (err) {
      setMessage(String(err));
    }
  };
  return { message, result, submit };
}

function field(form: HTMLFormElement, name: string): string {
  return (form.elements.namedItem(name) as HTMLInputElement | null)?.value ?? "";
}

export function PlainSetupForm({ auth, code, onDone }: FormProps & { code: string }) {
  const start = useSubmit("setup-start", onDone);
  const finish = useSubmit("setup-finish", onDone);
  const onSubmit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const f = e.currentTarget;
    void finish.submit({ code: field(f, "code"), password: field(f, "password"), totp_code: field(f, "totp_code") });
  };
  return (
    <form onSubmit={onSubmit}>
      <input name="code" placeholder="Setup code" defaultValue={code} />
      <input name="password" type="password" placeholder="Password" />
      {auth.totp_on && (
        <>
          <button type="button" onClick={(e) => void start.submit({ code: field(e.currentTarget.form!, "code") })}>
            Get secret
          </button>
          {start.result?.secret && <p>{start.result.secret}</p>}
          <input name="totp_code" placeholder="Code" />
        </>
      )}
      <button type="submit">Finish setup</button>
      {(start.message || finish.message) && <p role="alert">{start.message || finish.message}</p>}
    </form>
  );
}
