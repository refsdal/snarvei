import VpnKeyOutlinedIcon from "@mui/icons-material/VpnKeyOutlined";
import { Alert, Box, Button, CircularProgress, Stack, TextField, Typography } from "@mui/material";
import { authClient } from "../../../lib/auth-client";
import type { PasskeySummary } from "../../../types";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

const formatDateValue = (value: string | number | null | undefined) => {
  if (!value) {
    return "Unknown";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "Unknown";
  }

  return date.toLocaleString();
};

export function PasskeysSection(
  props: SharedSectionProps & {
    editingPasskeyId: string | null;
    editingPasskeyName: string;
    newPasskeyName: string;
    passkeys: PasskeySummary[];
    passkeysLoading: boolean;
    loadPasskeys: () => Promise<void>;
    setEditingPasskeyId: (value: string | null) => void;
    setEditingPasskeyName: (value: string) => void;
    setNewPasskeyName: (value: string) => void;
  },
) {
  return (
    <SectionCard
      title="Passkeys"
      description="Register biometric or hardware-backed sign-in methods and manage existing credentials."
      icon={<VpnKeyOutlinedIcon />}
    >
      <Stack spacing={2.5}>
        <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
          <TextField
            label="New passkey name"
            value={props.newPasskeyName}
            onChange={(event) => props.setNewPasskeyName(event.target.value)}
            placeholder="MacBook Touch ID"
            sx={{ flex: 1 }}
          />
          <Button
            variant="contained"
            disabled={props.busyAction === "add-passkey"}
            onClick={() =>
              void props.runAction("add-passkey", async () => {
                const result = await authClient.passkey.addPasskey({ name: props.newPasskeyName.trim() || undefined });
                if (result.error) {
                  props.setMessage({ severity: "error", text: result.error.message ?? "Unable to add passkey." });
                  return;
                }
                props.setNewPasskeyName("");
                await props.loadPasskeys();
                props.setMessage({ severity: "success", text: "Passkey added." });
              })
            }
          >
            Add passkey
          </Button>
        </Stack>

        {props.passkeysLoading ? <CircularProgress size={24} /> : null}
        <Stack spacing={1.5}>
          {props.passkeys.map((passkey) => (
            <Box key={passkey.id} sx={{ border: "1px solid rgba(255,255,255,0.08)", borderRadius: 3, p: 2.5 }}>
              <Stack spacing={1.5}>
                <Stack
                  direction={{ xs: "column", md: "row" }}
                  spacing={1.5}
                  sx={{ justifyContent: "space-between", alignItems: { md: "center" } }}
                >
                  <Box>
                    <Typography sx={{ fontWeight: 700 }}>{passkey.name || "Unnamed passkey"}</Typography>
                    <Typography color="text.secondary">
                      {passkey.deviceType || "Unknown device"} · Created {formatDateValue(passkey.createdAt)} ·{" "}
                      {passkey.backedUp ? "Backed up" : "Not backed up"}
                    </Typography>
                  </Box>
                  <Stack direction="row" spacing={1}>
                    <Button
                      variant="outlined"
                      onClick={() => {
                        props.setEditingPasskeyId(passkey.id);
                        props.setEditingPasskeyName(passkey.name ?? "");
                      }}
                    >
                      Rename
                    </Button>
                    <Button
                      color="inherit"
                      disabled={props.busyAction === `delete-passkey-${passkey.id}`}
                      onClick={() =>
                        void props.runAction(`delete-passkey-${passkey.id}`, async () => {
                          const result = await authClient.passkey.deletePasskey({ id: passkey.id });
                          if (result.error) {
                            props.setMessage({
                              severity: "error",
                              text: result.error.message ?? "Unable to delete passkey.",
                            });
                            return;
                          }
                          await props.loadPasskeys();
                          props.setMessage({ severity: "success", text: "Passkey deleted." });
                        })
                      }
                    >
                      Delete
                    </Button>
                  </Stack>
                </Stack>
                {props.editingPasskeyId === passkey.id ? (
                  <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
                    <TextField
                      value={props.editingPasskeyName}
                      onChange={(event) => props.setEditingPasskeyName(event.target.value)}
                      sx={{ flex: 1 }}
                    />
                    <Button
                      variant="contained"
                      disabled={!props.editingPasskeyName.trim() || props.busyAction === `rename-passkey-${passkey.id}`}
                      onClick={() =>
                        void props.runAction(`rename-passkey-${passkey.id}`, async () => {
                          const result = await authClient.passkey.updatePasskey({
                            id: passkey.id,
                            name: props.editingPasskeyName.trim(),
                          });
                          if (result.error) {
                            props.setMessage({
                              severity: "error",
                              text: result.error.message ?? "Unable to rename passkey.",
                            });
                            return;
                          }
                          props.setEditingPasskeyId(null);
                          props.setEditingPasskeyName("");
                          await props.loadPasskeys();
                          props.setMessage({ severity: "success", text: "Passkey renamed." });
                        })
                      }
                    >
                      Save
                    </Button>
                    <Button color="inherit" onClick={() => props.setEditingPasskeyId(null)}>
                      Cancel
                    </Button>
                  </Stack>
                ) : null}
              </Stack>
            </Box>
          ))}
          {!props.passkeysLoading && !props.passkeys.length ? (
            <Alert severity="info">No passkeys registered yet.</Alert>
          ) : null}
        </Stack>
      </Stack>
    </SectionCard>
  );
}
