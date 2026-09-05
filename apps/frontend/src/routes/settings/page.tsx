import { Button, Divider, Stack, Typography } from "@mui/material";
import { useWorkspace } from "../../hooks/use-workspace-context";
import { useSettingsState } from "./hooks/use-settings-state";
import {
  EmailSection,
  PasskeysSection,
  PasswordSection,
  ProfileSection,
  SessionsSection,
  TwoFactorSection,
} from "./components";

export function SettingsPage() {
  const { refreshSessionState, session, setMessage, signOut } = useWorkspace();
  const settingsState = useSettingsState({ session, setMessage });

  if (!session) {
    return null;
  }

  return (
    <Stack spacing={3} sx={{ maxWidth: 960 }}>
      <Stack spacing={1}>
        <Typography variant="h3" sx={{ fontWeight: 900 }}>
          Your settings
        </Typography>
        <Typography color="text.secondary">
          Manage your profile, security settings, active logins, and passkeys.
        </Typography>
      </Stack>

      <ProfileSection
        busyAction={settingsState.busyAction}
        profileName={settingsState.profileName}
        refreshSessionState={refreshSessionState}
        runAction={settingsState.runAction}
        session={session}
        setMessage={setMessage}
        setProfileName={settingsState.setProfileName}
      />

      <EmailSection
        busyAction={settingsState.busyAction}
        newEmail={settingsState.newEmail}
        refreshSessionState={refreshSessionState}
        runAction={settingsState.runAction}
        setMessage={setMessage}
        setNewEmail={settingsState.setNewEmail}
      />

      <PasswordSection
        busyAction={settingsState.busyAction}
        currentPassword={settingsState.currentPassword}
        loadSessions={settingsState.loadSessions}
        newPassword={settingsState.newPassword}
        refreshSessionState={refreshSessionState}
        runAction={settingsState.runAction}
        setCurrentPassword={settingsState.setCurrentPassword}
        setMessage={setMessage}
        setNewPassword={settingsState.setNewPassword}
      />

      <SessionsSection
        busyAction={settingsState.busyAction}
        currentSessionId={session.session.id}
        loadSessions={settingsState.loadSessions}
        refreshSessionState={refreshSessionState}
        runAction={settingsState.runAction}
        sessions={settingsState.sessions}
        sessionsLoading={settingsState.sessionsLoading}
        setMessage={setMessage}
      />

      <TwoFactorSection
        backupCodes={settingsState.backupCodes}
        busyAction={settingsState.busyAction}
        loadSessions={settingsState.loadSessions}
        refreshSessionState={refreshSessionState}
        runAction={settingsState.runAction}
        session={session}
        setBackupCodes={settingsState.setBackupCodes}
        setMessage={setMessage}
        setTotpUri={settingsState.setTotpUri}
        setTwoFactorCode={settingsState.setTwoFactorCode}
        setTwoFactorPassword={settingsState.setTwoFactorPassword}
        totpUri={settingsState.totpUri}
        twoFactorCode={settingsState.twoFactorCode}
        twoFactorPassword={settingsState.twoFactorPassword}
      />

      <PasskeysSection
        busyAction={settingsState.busyAction}
        editingPasskeyId={settingsState.editingPasskeyId}
        editingPasskeyName={settingsState.editingPasskeyName}
        loadPasskeys={settingsState.loadPasskeys}
        newPasskeyName={settingsState.newPasskeyName}
        passkeys={settingsState.passkeys}
        passkeysLoading={settingsState.passkeysLoading}
        refreshSessionState={refreshSessionState}
        runAction={settingsState.runAction}
        setEditingPasskeyId={settingsState.setEditingPasskeyId}
        setEditingPasskeyName={settingsState.setEditingPasskeyName}
        setMessage={setMessage}
        setNewPasskeyName={settingsState.setNewPasskeyName}
      />

      <Divider sx={{ borderColor: "rgba(255,255,255,0.08)" }} />

      <div>
        <Button color="inherit" sx={{ px: 0 }} onClick={() => void signOut()}>
          Sign out from this device
        </Button>
      </div>
    </Stack>
  );
}
