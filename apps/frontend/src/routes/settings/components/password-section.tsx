import KeyOutlinedIcon from "@mui/icons-material/KeyOutlined";
import { Button, Stack, TextField } from "@mui/material";
import { password } from "../../../lib/auth-client";
import { keys } from "../../../lib/data";
import { queryClient } from "../../../lib/query";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

export function PasswordSection(
  props: SharedSectionProps & {
    currentPassword: string;
    newPassword: string;
    setCurrentPassword: (value: string) => void;
    setNewPassword: (value: string) => void;
  },
) {
  return (
    <SectionCard
      title="Password"
      description="Change your account password and optionally revoke other active sessions."
      icon={<KeyOutlinedIcon />}
    >
      <Stack spacing={2}>
        <TextField
          label="Current password"
          type="password"
          value={props.currentPassword}
          onChange={(event) => props.setCurrentPassword(event.target.value)}
          slotProps={{ htmlInput: { "data-testid": "settings-current-password-input" } }}
        />
        <TextField
          label="New password"
          type="password"
          value={props.newPassword}
          onChange={(event) => props.setNewPassword(event.target.value)}
          slotProps={{ htmlInput: { "data-testid": "settings-new-password-input" } }}
        />
        <Button
          variant="contained"
          sx={{ alignSelf: "flex-start" }}
          disabled={!props.currentPassword || !props.newPassword || props.busyAction === "change-password"}
          onClick={() =>
            void props.runAction("change-password", async () => {
              await password.change(props.currentPassword, props.newPassword);
              props.setCurrentPassword("");
              props.setNewPassword("");
              await queryClient.invalidateQueries({ queryKey: keys.sessions });
              props.setMessage({ severity: "success", text: "Password updated. Other sessions were signed out." });
            })
          }
        >
          Change password
        </Button>
      </Stack>
    </SectionCard>
  );
}
