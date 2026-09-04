import ShieldOutlinedIcon from "@mui/icons-material/ShieldOutlined";
import { Box, Button, Chip, Stack, TextField, Typography } from "@mui/material";
import QRCode from "react-qr-code";
import { authClient } from "../../../lib/auth-client";
import type { SessionData } from "../../../types";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

export function TwoFactorSection(
  props: SharedSectionProps & {
    session: SessionData;
    twoFactorPassword: string;
    twoFactorCode: string;
    totpUri: string | null;
    backupCodes: string[];
    loadSessions: () => Promise<void>;
    setBackupCodes: (codes: string[]) => void;
    setTotpUri: (value: string | null) => void;
    setTwoFactorCode: (value: string) => void;
    setTwoFactorPassword: (value: string) => void;
  },
) {
  return (
    <SectionCard
      title="Two-factor authentication"
      description="Enable TOTP, verify enrollment, and manage backup recovery codes."
      icon={<ShieldOutlinedIcon />}
    >
      <Stack spacing={2.5}>
        <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} sx={{ alignItems: { sm: "center" } }}>
          <Chip
            label={props.session.user.twoFactorEnabled ? "Enabled" : "Not enabled"}
            color={props.session.user.twoFactorEnabled ? "success" : "default"}
          />
          <Typography color="text.secondary">
            {props.session.user.twoFactorEnabled
              ? "Use an authenticator app or backup codes to protect sign-in."
              : "Enable TOTP to require a second factor after password sign-in."}
          </Typography>
        </Stack>

        <TextField
          label="Current password"
          type="password"
          value={props.twoFactorPassword}
          onChange={(event) => props.setTwoFactorPassword(event.target.value)}
          helperText="Required for credential accounts."
        />

        {!props.session.user.twoFactorEnabled ? (
          <Button
            variant="contained"
            sx={{ alignSelf: "flex-start" }}
            disabled={props.busyAction === "enable-2fa"}
            onClick={() =>
              void props.runAction("enable-2fa", async () => {
                const result = await authClient.twoFactor.enable({ password: props.twoFactorPassword || undefined });
                if (result.error) {
                  props.setMessage({
                    severity: "error",
                    text: result.error.message ?? "Unable to enable two-factor authentication.",
                  });
                  return;
                }
                props.setTotpUri(result.data?.totpURI ?? null);
                props.setBackupCodes(result.data?.backupCodes ?? []);
                props.setMessage({
                  severity: "info",
                  text: "Scan the QR code and verify one authenticator code to finish enabling 2FA.",
                });
              })
            }
          >
            Start 2FA setup
          </Button>
        ) : (
          <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
            <Button
              variant="outlined"
              disabled={props.busyAction === "view-backup-codes"}
              onClick={() =>
                void props.runAction("view-backup-codes", async () => {
                  const result = await authClient.twoFactor.viewBackupCodes({});
                  if (result.error) {
                    props.setMessage({
                      severity: "error",
                      text: result.error.message ?? "Unable to view backup codes.",
                    });
                    return;
                  }
                  props.setBackupCodes(result.data?.backupCodes ?? []);
                })
              }
            >
              View backup codes
            </Button>
            <Button
              variant="outlined"
              disabled={props.busyAction === "generate-backup-codes"}
              onClick={() =>
                void props.runAction("generate-backup-codes", async () => {
                  const result = await authClient.twoFactor.generateBackupCodes({
                    password: props.twoFactorPassword || undefined,
                  });
                  if (result.error) {
                    props.setMessage({
                      severity: "error",
                      text: result.error.message ?? "Unable to regenerate backup codes.",
                    });
                    return;
                  }
                  props.setBackupCodes(result.data?.backupCodes ?? []);
                  props.setMessage({ severity: "success", text: "Backup codes regenerated." });
                })
              }
            >
              Regenerate backup codes
            </Button>
            <Button
              color="inherit"
              disabled={props.busyAction === "disable-2fa"}
              onClick={() =>
                void props.runAction("disable-2fa", async () => {
                  const result = await authClient.twoFactor.disable({ password: props.twoFactorPassword || undefined });
                  if (result.error) {
                    props.setMessage({
                      severity: "error",
                      text: result.error.message ?? "Unable to disable two-factor authentication.",
                    });
                    return;
                  }
                  props.setTotpUri(null);
                  props.setBackupCodes([]);
                  props.setTwoFactorCode("");
                  await props.refreshSessionState();
                  props.setMessage({ severity: "success", text: "Two-factor authentication disabled." });
                })
              }
            >
              Disable 2FA
            </Button>
          </Stack>
        )}

        {props.totpUri ? (
          <Stack direction={{ xs: "column", md: "row" }} spacing={3} sx={{ alignItems: { md: "center" } }}>
            <Box sx={{ p: 2, borderRadius: 3, background: "white", width: "fit-content" }}>
              <QRCode value={props.totpUri} size={160} />
            </Box>
            <Stack spacing={2} sx={{ flex: 1 }}>
              <Typography color="text.secondary">
                Scan this QR code with your authenticator app, then verify the current code below.
              </Typography>
              <TextField
                label="Authenticator code"
                value={props.twoFactorCode}
                onChange={(event) => props.setTwoFactorCode(event.target.value)}
                slotProps={{ htmlInput: { "data-testid": "settings-2fa-code-input" } }}
              />
              <Button
                variant="contained"
                sx={{ alignSelf: "flex-start" }}
                disabled={!props.twoFactorCode || props.busyAction === "verify-2fa"}
                onClick={() =>
                  void props.runAction("verify-2fa", async () => {
                    const result = await authClient.twoFactor.verifyTotp({ code: props.twoFactorCode });
                    if (result.error) {
                      props.setMessage({
                        severity: "error",
                        text: result.error.message ?? "Unable to verify authenticator code.",
                      });
                      return;
                    }
                    props.setTwoFactorCode("");
                    await props.refreshSessionState();
                    await props.loadSessions();
                    props.setMessage({ severity: "success", text: "Two-factor authentication enabled." });
                  })
                }
              >
                Verify and enable
              </Button>
            </Stack>
          </Stack>
        ) : null}

        {props.backupCodes.length ? (
          <Box sx={{ border: "1px solid rgba(255,255,255,0.08)", borderRadius: 3, p: 2.5 }}>
            <Typography sx={{ fontWeight: 700, mb: 1.5 }}>Backup codes</Typography>
            <Typography color="text.secondary" sx={{ mb: 2 }}>
              Store these offline. Each code can only be used once.
            </Typography>
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", sm: "repeat(2, minmax(0, 1fr))" },
                gap: 1,
                fontFamily: "ui-monospace, SFMono-Regular, monospace",
              }}
            >
              {props.backupCodes.map((code) => (
                <Box key={code} sx={{ px: 1.5, py: 1, borderRadius: 2, background: "rgba(255,255,255,0.04)" }}>
                  {code}
                </Box>
              ))}
            </Box>
          </Box>
        ) : null}
      </Stack>
    </SectionCard>
  );
}
