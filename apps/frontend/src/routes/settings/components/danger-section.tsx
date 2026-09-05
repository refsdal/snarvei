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
      await deleteMe.mutateAsync({ password });
      setConfirmOpen(false);
      void navigate({ to: "/" });
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
