import KeyOutlinedIcon from "@mui/icons-material/KeyOutlined";
import { Button, Stack, TextField } from "@mui/material";
import { authClient } from "../../../lib/auth-client";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

export function EmailSection(
  props: SharedSectionProps & {
    newEmail: string;
    setNewEmail: (value: string) => void;
  },
) {
  return (
    <SectionCard
      title="Email"
      description="Request an email address change. Verification is sent to the new address only."
      icon={<KeyOutlinedIcon />}
    >
      <Stack spacing={2}>
        <TextField
          label="New email"
          type="email"
          value={props.newEmail}
          onChange={(event) => props.setNewEmail(event.target.value)}
          slotProps={{ htmlInput: { "data-testid": "settings-email-input" } }}
        />
        <Button
          variant="contained"
          sx={{ alignSelf: "flex-start" }}
          disabled={!props.newEmail.trim() || props.busyAction === "change-email"}
          onClick={() =>
            void props.runAction("change-email", async () => {
              const result = await authClient.changeEmail({
                newEmail: props.newEmail.trim(),
                callbackURL: `${window.location.origin}/app/settings`,
              });
              if (result.error) {
                props.setMessage({
                  severity: "error",
                  text: result.error.message ?? "Unable to request email change.",
                });
                return;
              }
              props.setNewEmail("");
              props.setMessage({ severity: "success", text: "Check the new email address to confirm the change." });
            })
          }
        >
          Send verification
        </Button>
      </Stack>
    </SectionCard>
  );
}
