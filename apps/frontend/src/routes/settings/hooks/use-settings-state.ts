import { useState } from "react";
import type { AppMessage } from "../../../components/message-context";
import { errorMessage } from "../../../lib/api";
import type { Me } from "../../../lib/data";
import type { ActionRunner } from "../components/types";

export function useSettingsState(options: { me: Me | null; setMessage: (message: AppMessage) => void }) {
  const { me, setMessage } = options;
  // `me` is null for the one render between account deletion clearing the
  // cache and the page unmounting on navigation (see routes/settings/page.tsx).
  const [profileName, setProfileName] = useState(me?.user.name ?? "");
  // Keep the editable name in sync with the loaded user without an effect
  // (React "adjusting state during render" pattern).
  const [syncedUserName, setSyncedUserName] = useState(me?.user.name ?? "");
  if (me && syncedUserName !== me.user.name) {
    setSyncedUserName(me.user.name);
    setProfileName(me.user.name);
  }
  const [newEmail, setNewEmail] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [twoFactorPassword, setTwoFactorPassword] = useState("");
  const [twoFactorCode, setTwoFactorCode] = useState("");
  const [totpUri, setTotpUri] = useState<string | null>(null);
  const [backupCodes, setBackupCodes] = useState<string[]>([]);
  const [busyAction, setBusyAction] = useState<string | null>(null);

  const runAction: ActionRunner = async (action, work) => {
    setBusyAction(action);
    try {
      await work();
    } catch (err) {
      setMessage({ severity: "error", text: errorMessage(err, "Something went wrong.") });
    } finally {
      setBusyAction(null);
    }
  };

  return {
    backupCodes,
    busyAction,
    currentPassword,
    newEmail,
    newPassword,
    profileName,
    runAction,
    setBackupCodes,
    setCurrentPassword,
    setNewEmail,
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
