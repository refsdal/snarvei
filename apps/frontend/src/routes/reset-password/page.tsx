import { Alert, Box, Button, Card, CardContent, Stack, TextField, Typography } from "@mui/material";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { errorMessage } from "../../lib/api";
import { password } from "../../lib/auth-client";
import { resetPasswordRoute } from "../../router";

export const MIN_PASSWORD_LENGTH = 8;

/**
 * Landing page for the password-reset email. Limen verifies the emailed
 * token and redirects here with `?token=…` (valid) or `?error=INVALID_TOKEN`.
 */
export function ResetPasswordPage() {
  const navigate = useNavigate();
  const { token, error: linkError } = resetPasswordRoute.useSearch();
  const [newPassword, setNewPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const mismatch = confirm.length > 0 && confirm !== newPassword;
  const tooShort = newPassword.length > 0 && newPassword.length < MIN_PASSWORD_LENGTH;
  const canSubmit =
    Boolean(token) && newPassword.length >= MIN_PASSWORD_LENGTH && confirm === newPassword && !submitting;

  const submit = async () => {
    if (!token) {
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await password.reset(token, newPassword);
      void navigate({ to: "/", search: { reset: "done" }, replace: true });
    } catch (err) {
      setError(errorMessage(err, "This reset link is invalid or has expired."));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center", p: 3 }}>
      <Card sx={{ width: "100%", maxWidth: 460, border: "1px solid rgba(255,255,255,0.08)" }}>
        <CardContent sx={{ p: 4 }}>
          <Stack spacing={2}>
            <Typography variant="h5" sx={{ fontWeight: 700 }}>
              Choose a new password
            </Typography>
            {linkError || !token ? (
              <>
                <Alert severity="error">This password reset link is invalid or has expired.</Alert>
                <Button variant="contained" onClick={() => void navigate({ to: "/", search: { forgot: "1" } })}>
                  Request a new link
                </Button>
              </>
            ) : (
              <Box
                component="form"
                noValidate
                onSubmit={(event) => {
                  event.preventDefault();
                  void submit();
                }}
              >
                <Stack spacing={2}>
                  {error ? (
                    <Alert severity="error" onClose={() => setError(null)}>
                      {error}
                    </Alert>
                  ) : null}
                  <TextField
                    label="New password"
                    type="password"
                    value={newPassword}
                    onChange={(event) => setNewPassword(event.target.value)}
                    autoComplete="new-password"
                    required
                    fullWidth
                    error={tooShort}
                    helperText={tooShort ? `Use at least ${MIN_PASSWORD_LENGTH} characters.` : " "}
                    slotProps={{ htmlInput: { "data-testid": "reset-password-input" } }}
                  />
                  <TextField
                    label="Confirm new password"
                    type="password"
                    value={confirm}
                    onChange={(event) => setConfirm(event.target.value)}
                    autoComplete="new-password"
                    required
                    fullWidth
                    error={mismatch}
                    helperText={mismatch ? "Passwords do not match." : " "}
                    slotProps={{ htmlInput: { "data-testid": "reset-password-confirm-input" } }}
                  />
                  <Button type="submit" variant="contained" disabled={!canSubmit} data-testid="reset-password-button">
                    Set new password
                  </Button>
                  <Typography variant="body2" color="text.secondary">
                    Saving signs you out everywhere; sign in again with the new password.
                  </Typography>
                </Stack>
              </Box>
            )}
          </Stack>
        </CardContent>
      </Card>
    </Box>
  );
}
