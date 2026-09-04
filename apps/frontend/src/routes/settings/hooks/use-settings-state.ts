import { useCallback, useEffect, useState } from "react";
import { authClient } from "../../../lib/auth-client";
import type { AppMessage, AuthSession, PasskeySummary, SessionData } from "../../../types";

type LoadResult<T> = { items: T[]; error: string | null };

const fetchSessions = async (): Promise<LoadResult<AuthSession>> => {
  const result = await authClient.listSessions();
  if (result.error) {
    return { items: [], error: result.error.message ?? "Unable to load active sessions." };
  }
  return { items: result.data ?? [], error: null };
};

const fetchPasskeys = async (): Promise<LoadResult<PasskeySummary>> => {
  const result = await authClient.passkey.listUserPasskeys({});
  if (result.error) {
    return { items: [], error: result.error.message ?? "Unable to load passkeys." };
  }
  return { items: result.data ?? [], error: null };
};

export function useSettingsState(options: { session: SessionData | null; setMessage: (message: AppMessage) => void }) {
  const sessionUserName = options.session?.user.name ?? "";
  const userId = options.session?.user.id ?? null;
  const { setMessage } = options;
  const [profileName, setProfileName] = useState(sessionUserName);
  // Keep the editable name in sync with the session without an effect
  // (React "adjusting state during render" pattern).
  const [syncedUserName, setSyncedUserName] = useState(sessionUserName);
  if (syncedUserName !== sessionUserName) {
    setSyncedUserName(sessionUserName);
    setProfileName(sessionUserName);
  }
  const [newEmail, setNewEmail] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [twoFactorPassword, setTwoFactorPassword] = useState("");
  const [twoFactorCode, setTwoFactorCode] = useState("");
  const [totpUri, setTotpUri] = useState<string | null>(null);
  const [backupCodes, setBackupCodes] = useState<string[]>([]);
  // Loaded collections are tagged with the user they were loaded for so that
  // "loading" can be derived (no setState in effects) and data never leaks
  // across a user switch.
  const [sessionsState, setSessionsState] = useState<{ userId: string; items: AuthSession[] } | null>(null);
  const [passkeysState, setPasskeysState] = useState<{ userId: string; items: PasskeySummary[] } | null>(null);
  const [newPasskeyName, setNewPasskeyName] = useState("");
  const [editingPasskeyId, setEditingPasskeyId] = useState<string | null>(null);
  const [editingPasskeyName, setEditingPasskeyName] = useState("");
  const [busyAction, setBusyAction] = useState<string | null>(null);

  const applySessions = useCallback(
    (forUserId: string, result: Awaited<ReturnType<typeof fetchSessions>>) => {
      if (result.error) {
        setMessage({ severity: "error", text: result.error });
      }
      setSessionsState({ userId: forUserId, items: result.items });
    },
    [setMessage],
  );

  const applyPasskeys = useCallback(
    (forUserId: string, result: Awaited<ReturnType<typeof fetchPasskeys>>) => {
      if (result.error) {
        setMessage({ severity: "error", text: result.error });
      }
      setPasskeysState({ userId: forUserId, items: result.items });
    },
    [setMessage],
  );

  const loadSessions = useCallback(async () => {
    if (!userId) {
      return;
    }
    applySessions(userId, await fetchSessions());
  }, [applySessions, userId]);

  const loadPasskeys = useCallback(async () => {
    if (!userId) {
      return;
    }
    applyPasskeys(userId, await fetchPasskeys());
  }, [applyPasskeys, userId]);

  useEffect(() => {
    if (!userId) {
      return;
    }

    let cancelled = false;
    void Promise.all([fetchSessions(), fetchPasskeys()]).then(([sessionsResult, passkeysResult]) => {
      if (cancelled) {
        return;
      }
      applySessions(userId, sessionsResult);
      applyPasskeys(userId, passkeysResult);
    });

    return () => {
      cancelled = true;
    };
  }, [applyPasskeys, applySessions, userId]);

  const sessions = sessionsState?.userId === userId ? sessionsState.items : [];
  const sessionsLoading = userId !== null && sessionsState?.userId !== userId;
  const passkeys = passkeysState?.userId === userId ? passkeysState.items : [];
  const passkeysLoading = userId !== null && passkeysState?.userId !== userId;

  const runAction = async (action: string, work: () => Promise<void>) => {
    setBusyAction(action);
    try {
      await work();
    } finally {
      setBusyAction(null);
    }
  };

  return {
    backupCodes,
    busyAction,
    currentPassword,
    editingPasskeyId,
    editingPasskeyName,
    loadPasskeys,
    loadSessions,
    newEmail,
    newPassword,
    newPasskeyName,
    passkeys,
    passkeysLoading,
    profileName,
    runAction,
    sessions,
    sessionsLoading,
    setBackupCodes,
    setCurrentPassword,
    setEditingPasskeyId,
    setEditingPasskeyName,
    setNewEmail,
    setNewPasskeyName,
    setNewPassword,
    setProfileName,
    setTotpUri,
    setTwoFactorCode,
    setTwoFactorPassword,
    totpUri,
    twoFactorCode,
    twoFactorPassword,
  };
}
