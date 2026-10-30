import { apiFetch } from "../apiFetch";

export interface AuthState {
  state: "setup" | "signed-out" | "signed-in";
  login_id?: string;
  totp_on: boolean;
  setup_code_required: boolean;
}

export interface AuthResult {
  refused?: boolean;
  reason?: string;
  wait_seconds?: number;
  authenticator_needed?: boolean;
  pending_id?: string;
  secret?: string;
  uri?: string;
}

export async function fetchAuthState(): Promise<AuthState> {
  const res = await apiFetch("/v1/web/auth");
  if (!res.ok) throw new Error(`auth state: ${res.status}`);
  return res.json();
}

export const SETUP_CODE_WRONG = "That code is wrong or expired. Run gx server status for a new one.";

export async function postAuth(
  step: "setup-start" | "setup-finish" | "login" | "add-authenticator",
  body: object,
): Promise<AuthResult> {
  const res = await apiFetch(`/v1/web/auth/${step}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${step}: ${res.status}`);
  return res.json();
}
