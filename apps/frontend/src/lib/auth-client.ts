import { createAuthClient } from "limen-auth/react";
import { credentialPasswordPlugin, twoFactorPlugin } from "limen-auth/plugins";
import { resetCache } from "./query";

// Only what the server's allowlist mounts (apps/server/internal/auth/routes.go):
// credential sign-in/sign-up, sign-out, password change/reset, two-factor.
// Sessions, organizations and invitations are Snarvei's own /api routes and
// go through lib/api.ts.
//
// The SDK's session store is deliberately unused for rendering: Limen's
// payload has no user id and no active organization, so GET /api/me (the
// ['me'] query) is the single truth. crossTabSync/refetchOnWindowFocus are
// off so the store never polls /api/auth/me on its own.
let challengePending = false;

// Factory so a shell script (or a test) can point the client at a server
// running on a different origin; the app itself always uses "" (same
// origin — see lib/api.ts).
export function createSnarveiAuthClient(baseURL: string) {
  return createAuthClient({
    baseURL,
    basePath: "/api/auth",
    crossTabSync: false,
    refetchOnWindowFocus: false,
    plugins: [
      credentialPasswordPlugin(),
      twoFactorPlugin({
        onTwoFactorRedirect: () => {
          challengePending = true;
        },
      }),
    ],
  });
}

export const authClient = createSnarveiAuthClient("");

/**
 * Email + password. Resolves { twoFactorRequired: true } when the server
 * answered with a two-factor challenge (the challenge cookie is set; call
 * verifyTwoFactor next). Throws LimenError on bad credentials.
 */
export async function signInWithPassword(email: string, password: string): Promise<{ twoFactorRequired: boolean }> {
  challengePending = false;
  try {
    await authClient.signIn.credential({ credential: email, password });
  } catch (err) {
    // The challenge response has no session; if the SDK trips over that
    // after firing onTwoFactorRedirect, the challenge still stands.
    if (!challengePending) throw err;
  }
  const required = challengePending;
  challengePending = false;
  if (!required) await resetCache();
  return { twoFactorRequired: required };
}

/** TOTP or backup code (the server recognises backup codes by shape). */
export async function verifyTwoFactor(code: string): Promise<void> {
  await authClient.twoFactor.verify({ code: code.trim(), method: "totp" });
  await resetCache();
}

export async function signUpWithPassword(name: string, email: string, password: string): Promise<void> {
  await authClient.signUp.credential({ email, password, additionalFields: { name: name.trim() } });
  await resetCache();
}

export async function signOut(): Promise<void> {
  try {
    await authClient.signout();
  } finally {
    await resetCache();
  }
}

export const password = {
  async requestReset(email: string): Promise<void> {
    await authClient.password.requestReset({ email });
  },
  async reset(token: string, newPassword: string): Promise<void> {
    await authClient.password.reset({ token, newPassword });
  },
  async change(currentPassword: string, newPassword: string): Promise<void> {
    await authClient.password.change({ currentPassword, newPassword, revokeOtherSessions: true });
  },
};

export const twoFactor = {
  async initiateSetup(password: string): Promise<{ uri: string }> {
    return authClient.twoFactor.initiateSetup({ password });
  },
  async finalizeSetup(code: string): Promise<void> {
    await authClient.twoFactor.finalizeSetup({ code: code.trim() });
  },
  async disable(password: string): Promise<void> {
    await authClient.twoFactor.disable({ password });
  },
  async getTotpUri(): Promise<{ uri: string }> {
    return authClient.twoFactor.getTotpUri();
  },
  async getBackupCodes(): Promise<string[]> {
    return authClient.twoFactor.getBackupCodes();
  },
  async regenerateBackupCodes(): Promise<string[]> {
    return authClient.twoFactor.regenerateBackupCodes();
  },
};
