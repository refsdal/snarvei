import WarningAmberOutlinedIcon from "@mui/icons-material/WarningAmberOutlined";
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Stack,
  TextField,
} from "@mui/material";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useDeleteMe } from "../../../lib/data";
import { SectionCard } from "./section-card";
import type { SharedSectionProps } from "./types";

export function DangerSection(props: SharedSectionProps) {
  const navigate = useNavigate();
  const deleteMe = useDeleteMe();
  const [password, setPassword] = useState("");
  const [confirmOpen, setConfirmOpen] = useState(false);

  const deleteAccount = () =>
    void props.runAction("delete-account", async () => {
      try {
        await deleteMe.mutateAsync({ password });
        // `useDeleteMe`'s onSuccess already cleared the cache, which unmounts
        // this dialog (SettingsPage falls back to a spinner once `me` is
        // gone) — don't touch this component's own state after that.
        void navigate({ to: "/", replace: true });
      } catch (err) {
        // A failure (e.g. LAST_OWNER) leaves the page mounted: close the
        // dialog so the error alert — rendered in AppShell, outside this
        // MUI Modal — isn't hidden by the modal's aria-hidden on the rest
        // of the page. `runAction` still shows the message.
        setConfirmOpen(false);
        throw err;
      }
    });

  return (
    <SectionCard
      title="Delete account"
      description="Permanently delete your account and every session. This cannot be undone."
      icon={<WarningAmberOutlinedIcon />}
    >
      <Stack spacing={2}>
        <TextField
          label="Current password"
          type="password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          slotProps={{ htmlInput: { "data-testid": "settings-delete-password-input" } }}
        />
        <Button
          color="error"
          variant="outlined"
          sx={{ alignSelf: "flex-start" }}
          disabled={!password || props.busyAction === "delete-account"}
          data-testid="settings-delete-account-button"
          onClick={() => setConfirmOpen(true)}
        >
          Delete my account
        </Button>
      </Stack>

      <Dialog open={confirmOpen} onClose={() => setConfirmOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>Delete your account?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This permanently deletes your account, signs out every session, and cannot be undone. If you are the sole
            owner of an organization, transfer ownership or delete the organization first.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmOpen(false)}>Cancel</Button>
          <Button
            color="error"
            variant="contained"
            disabled={props.busyAction === "delete-account"}
            data-testid="settings-delete-account-confirm-button"
            onClick={deleteAccount}
          >
            Delete my account
          </Button>
        </DialogActions>
      </Dialog>
    </SectionCard>
  );
}
