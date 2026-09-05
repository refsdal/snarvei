import KeyOutlinedIcon from "@mui/icons-material/KeyOutlined";
import { Button, Stack, TextField } from "@mui/material";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { errorMessage } from "../../../lib/api";
import { useConfirmEmailChange, useRequestEmailChange } from "../../../lib/data";
import { settingsRoute } from "../../../router";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

export function EmailSection(
  props: SharedSectionProps & {
    newEmail: string;
    setNewEmail: (value: string) => void;
  },
) {
  const navigate = useNavigate();
  const { emailToken } = settingsRoute.useSearch();
  const [password, setPassword] = useState("");
  const requestEmailChange = useRequestEmailChange();
  const confirmEmailChange = useConfirmEmailChange();
  const confirmedTokenRef = useRef<string | null>(null);
  const { setMessage } = props;
  const { mutateAsync: confirmEmailChangeAsync } = confirmEmailChange;

  useEffect(() => {
    if (!emailToken || confirmedTokenRef.current === emailToken) {
      return;
    }
    confirmedTokenRef.current = emailToken;
    void confirmEmailChangeAsync({ token: emailToken })
      .then(() => {
        setMessage({ severity: "success", text: "Email address updated." });
      })
      .catch((err) => {
        setMessage({ severity: "error", text: errorMessage(err, "Unable to confirm the email change.") });
      })
      .finally(() => {
        void navigate({ to: "/app/settings", search: {}, replace: true });
      });
  }, [confirmEmailChangeAsync, emailToken, navigate, setMessage]);

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
        <TextField
          label="Current password"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          slotProps={{ htmlInput: { "data-testid": "settings-email-password-input" } }}
        />
        <Button
          variant="contained"
          sx={{ alignSelf: "flex-start" }}
          disabled={!props.newEmail.trim() || !password || props.busyAction === "change-email"}
          onClick={() =>
            void props.runAction("change-email", async () => {
              await requestEmailChange.mutateAsync({ newEmail: props.newEmail.trim(), password });
              props.setNewEmail("");
              setPassword("");
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
