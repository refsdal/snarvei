import { Button, Divider, Stack, Typography } from "@mui/material";
import { useNavigate } from "@tanstack/react-router";
import { PageFallback } from "../../components/page-fallback";
import { useMessage } from "../../components/message-context";
import { signOut } from "../../lib/auth-client";
import { useMe } from "../../lib/data";
import { useSettingsState } from "./hooks/use-settings-state";
import {
  DangerSection,
  EmailSection,
  PasswordSection,
  ProfileSection,
  SessionsSection,
  TwoFactorSection,
} from "./components";

export function SettingsPage() {
  const navigate = useNavigate();
  const { setMessage } = useMessage();
  // The route guards on a session at entry, but account deletion clears the
  // whole cache (see useDeleteMe) while this page is still mounted, so `me`
  // can go nullish mid-render. Call every hook unconditionally, then fall
  // back to a spinner instead of asserting non-null.
  const { data: me } = useMe();
  const settingsState = useSettingsState({ me: me ?? null, setMessage });

  if (!me) {
    return <PageFallback fullScreen />;
  }

  const sharedProps = { me, busyAction: settingsState.busyAction, setMessage, runAction: settingsState.runAction };

  return (
    <Stack spacing={3} sx={{ maxWidth: 960 }}>
      <Stack spacing={1}>
        <Typography variant="h3" sx={{ fontWeight: 900 }}>
          Your settings
        </Typography>
        <Typography color="text.secondary">Manage your profile, security settings and active logins.</Typography>
      </Stack>

      <ProfileSection
        {...sharedProps}
        profileName={settingsState.profileName}
        setProfileName={settingsState.setProfileName}
      />

      <EmailSection {...sharedProps} newEmail={settingsState.newEmail} setNewEmail={settingsState.setNewEmail} />

      <PasswordSection
        {...sharedProps}
        currentPassword={settingsState.currentPassword}
        newPassword={settingsState.newPassword}
        setCurrentPassword={settingsState.setCurrentPassword}
        setNewPassword={settingsState.setNewPassword}
      />

      <SessionsSection {...sharedProps} />

      <TwoFactorSection
        {...sharedProps}
        backupCodes={settingsState.backupCodes}
        setBackupCodes={settingsState.setBackupCodes}
        setTotpUri={settingsState.setTotpUri}
        setTwoFactorCode={settingsState.setTwoFactorCode}
        setTwoFactorPassword={settingsState.setTwoFactorPassword}
        totpUri={settingsState.totpUri}
        twoFactorCode={settingsState.twoFactorCode}
        twoFactorPassword={settingsState.twoFactorPassword}
      />

      <DangerSection {...sharedProps} />

      <Divider sx={{ borderColor: "rgba(255,255,255,0.08)" }} />

      <div>
        <Button color="inherit" sx={{ px: 0 }} onClick={() => void signOut().then(() => navigate({ to: "/" }))}>
          Sign out from this device
        </Button>
      </div>
    </Stack>
  );
}
