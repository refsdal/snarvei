import ShieldOutlinedIcon from "@mui/icons-material/ShieldOutlined";
import { Box, Button, Chip, Stack, TextField, Typography } from "@mui/material";
import QRCode from "react-qr-code";
import { twoFactor } from "../../../lib/auth-client";
import { keys } from "../../../lib/data";
import { queryClient } from "../../../lib/query";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

export function TwoFactorSection(
  props: SharedSectionProps & {
    twoFactorPassword: string;
    twoFactorCode: string;
    totpUri: string | null;
    backupCodes: string[];
    setBackupCodes: (codes: string[]) => void;
    setTotpUri: (value: string | null) => void;
    setTwoFactorCode: (value: string) => void;
    setTwoFactorPassword: (value: string) => void;
  },
) {
  const enabled = props.me.user.twoFactorEnabled;

  return (
    <SectionCard
      title="Two-factor authentication"
      description="Enable TOTP, verify enrollment, and manage backup recovery codes."
      icon={<ShieldOutlinedIcon />}
    >
      <Stack spacing={2.5}>
        <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5} sx={{ alignItems: { sm: "center" } }}>
          <Chip label={enabled ? "Enabled" : "Not enabled"} color={enabled ? "success" : "default"} />
          <Typography color="text.secondary">
            {enabled
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
          slotProps={{ htmlInput: { "data-testid": "settings-2fa-password-input" } }}
        />

        {!enabled ? (
          <Button
            variant="contained"
            sx={{ alignSelf: "flex-start" }}
            disabled={!props.twoFactorPassword || props.busyAction === "enable-2fa"}
            onClick={() =>
              void props.runAction("enable-2fa", async () => {
                const { uri } = await twoFactor.initiateSetup(props.twoFactorPassword);
                props.setTotpUri(uri);
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
                  const codes = await twoFactor.getBackupCodes();
                  props.setBackupCodes(codes);
                })
              }
            >
              Show backup codes
            </Button>
            <Button
              variant="outlined"
              disabled={props.busyAction === "generate-backup-codes"}
              onClick={() =>
                void props.runAction("generate-backup-codes", async () => {
                  const codes = await twoFactor.regenerateBackupCodes();
                  props.setBackupCodes(codes);
                  props.setMessage({ severity: "success", text: "Backup codes regenerated." });
                })
              }
            >
              Regenerate backup codes
            </Button>
            <Button
              color="inherit"
              disabled={!props.twoFactorPassword || props.busyAction === "disable-2fa"}
              onClick={() =>
                void props.runAction("disable-2fa", async () => {
                  await twoFactor.disable(props.twoFactorPassword);
                  props.setTotpUri(null);
                  props.setBackupCodes([]);
                  props.setTwoFactorCode("");
                  props.setTwoFactorPassword("");
                  await queryClient.invalidateQueries({ queryKey: keys.me });
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
              <Typography
                variant="body2"
                color="text.secondary"
                data-testid="settings-2fa-uri"
                sx={{ wordBreak: "break-all", fontFamily: "ui-monospace, SFMono-Regular, monospace" }}
              >
                {props.totpUri}
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
                    await twoFactor.finalizeSetup(props.twoFactorCode);
                    const codes = await twoFactor.getBackupCodes();
                    props.setTwoFactorCode("");
                    props.setTwoFactorPassword("");
                    props.setTotpUri(null);
                    props.setBackupCodes(codes);
                    await queryClient.invalidateQueries({ queryKey: keys.me });
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
