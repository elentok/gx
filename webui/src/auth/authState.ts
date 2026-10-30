import { apiFetch } from "../apiFetch";
import type { CertInfo } from "./cert";

export interface AuthState extends CertInfo {
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
