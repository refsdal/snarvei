import { Alert, Box, Button, Card, CardContent, Chip, Paper, Stack, TextField, Typography } from "@mui/material";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { errorMessage } from "../../lib/api";
import {
  password as passwordApi,
  signInWithPassword,
  signUpWithPassword,
  verifyTwoFactor,
} from "../../lib/auth-client";
import { useConfig } from "../../lib/data";
import { afterAuthPath } from "../../lib/routes";
import { useMessage } from "../../components/message-context";
import { indexRoute } from "../../router";

const inputStyle = {
  width: "100%",
  padding: "12px 14px",
  borderRadius: 12,
  border: "1px solid rgba(255,255,255,0.16)",
  background: "rgba(255,255,255,0.02)",
  color: "white",
  font: "inherit",
  outline: "none",
} as const;

export function LandingPage() {
  const navigate = useNavigate();
  const { next, forgot, reset } = indexRoute.useSearch();
  const { data: config } = useConfig();
  const signupOpen = config?.openSignup === true;
  const { message, setMessage } = useMessage();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [twoFactorCode, setTwoFactorCode] = useState("");
  const [twoFactorMethod, setTwoFactorMethod] = useState<"totp" | "backup">("totp");
  const [twoFactorRequired, setTwoFactorRequired] = useState(false);
  const [submitting, setSubmitting] = useState<"signin" | "signup" | "verify" | null>(null);
  const [forgotOpen, setForgotOpen] = useState(forgot === "1");
  const [forgotEmail, setForgotEmail] = useState("");
  const [forgotSubmitting, setForgotSubmitting] = useState(false);
  const [forgotSent, setForgotSent] = useState(false);
  const resetDone = reset === "done";

  // `href`, not `to`: `afterAuthPath` may carry a query string (e.g. from an
  // emailed settings link), and `to` does not split on "?".
  const goToApp = () => navigate({ href: afterAuthPath(next), replace: true });

  const handleSignIn = async () => {
    setSubmitting("signin");
    setMessage(null);
    try {
      const { twoFactorRequired: required } = await signInWithPassword(email, password);
      setTwoFactorRequired(required);
      if (!required) goToApp();
    } catch (err) {
      setMessage({ severity: "error", text: errorMessage(err, "Sign-in failed.") });
    } finally {
      setSubmitting(null);
    }
  };

  const handleSignUp = async () => {
    setSubmitting("signup");
    setMessage(null);
    try {
      await signUpWithPassword(name.trim(), email, password);
      goToApp();
    } catch (err) {
      setMessage({ severity: "error", text: errorMessage(err, "Unable to sign up.") });
    } finally {
      setSubmitting(null);
    }
  };

  const handleVerifyTwoFactor = async () => {
    setSubmitting("verify");
    setMessage(null);
    try {
      await verifyTwoFactor(twoFactorCode);
      setTwoFactorRequired(false);
      setTwoFactorCode("");
      goToApp();
    } catch (err) {
      setMessage({ severity: "error", text: errorMessage(err, "Verification failed.") });
    } finally {
      setSubmitting(null);
    }
  };

  const requestReset = async () => {
    setForgotSubmitting(true);
    setMessage(null);
    try {
      await passwordApi.requestReset(forgotEmail.trim());
      setForgotSent(true);
    } catch (err) {
      setMessage({ severity: "error", text: errorMessage(err, "Unable to send a reset link right now.") });
    } finally {
      setForgotSubmitting(false);
    }
  };

  return (
    <Box
      sx={{
        minHeight: "100vh",
        background: "radial-gradient(circle at top, rgba(139,92,246,0.24), transparent 35%), #070b16",
        display: "grid",
        placeItems: "center",
        p: 3,
      }}
    >
      <Stack direction={{ xs: "column", md: "row" }} spacing={4} sx={{ width: "100%", maxWidth: 1200 }}>
        <Paper
          sx={{
            flex: 1.3,
            p: 5,
            minHeight: 420,
            background: "linear-gradient(180deg, rgba(15,23,42,0.98), rgba(15,23,42,0.84))",
            border: "1px solid rgba(255,255,255,0.08)",
          }}
        >
          <Chip label="V1 foundation" color="secondary" sx={{ mb: 2 }} />
          <Typography variant="h2" sx={{ fontWeight: 800, lineHeight: 1.05, maxWidth: 720 }}>
            Short links you can trust long after they are shared.
          </Typography>
          <Typography variant="h6" color="text.secondary" sx={{ mt: 2, maxWidth: 680 }}>
            Manage links by organization and team, update destinations safely, and track every click from one small
            self-hosted service.
          </Typography>
          <Stack direction={{ xs: "column", sm: "row" }} spacing={2} sx={{ mt: 4 }}>
            <Chip label="Self-hosted, one container" />
            <Chip label="Organizations + teams" />
            <Chip label="OpenAPI + Scalar" />
          </Stack>
        </Paper>
        <Card sx={{ flex: 1, border: "1px solid rgba(255,255,255,0.08)" }}>
          <CardContent sx={{ p: 4 }}>
            <Stack spacing={2}>
              <Typography variant="h5" sx={{ fontWeight: 700 }}>
                Sign in or create your first workspace
              </Typography>
              {resetDone && !message ? (
                <Alert severity="success">Password updated. Sign in with your new password.</Alert>
              ) : null}
              {message ? (
                <Alert severity={message.severity} onClose={() => setMessage(null)}>
                  {message.text}
                </Alert>
              ) : null}
              <Box
                component="form"
                noValidate
                onSubmit={(event) => {
                  event.preventDefault();
                  void handleSignIn();
                }}
              >
                <Stack spacing={2}>
                  {signupOpen ? (
                    <TextField
                      label="Name"
                      placeholder="Your name (only needed to create an account)"
                      value={name}
                      onChange={(event) => setName(event.target.value)}
                      autoComplete="name"
                      fullWidth
                      slotProps={{ htmlInput: { "data-testid": "auth-name-input" } }}
                    />
                  ) : null}
                  <TextField
                    label="Email"
                    type="email"
                    placeholder="you@example.com"
                    value={email}
                    onChange={(event) => setEmail(event.target.value)}
                    autoComplete="email"
                    required
                    fullWidth
                    slotProps={{ htmlInput: { "data-testid": "auth-email-input" } }}
                  />
                  <TextField
                    label="Password"
                    type="password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    autoComplete="current-password"
                    required
                    fullWidth
                    slotProps={{ htmlInput: { "data-testid": "auth-password-input" } }}
                  />
                  <Stack direction="row" spacing={2}>
                    <Button
                      type="submit"
                      variant="contained"
                      disabled={submitting === "signin" || !email || !password}
                      data-testid="sign-in-button"
                    >
                      Sign in
                    </Button>
                    {signupOpen ? (
                      <Button
                        type="button"
                        variant="outlined"
                        disabled={submitting === "signup" || !email || !password || !name.trim()}
                        data-testid="create-account-button"
                        onClick={() => void handleSignUp()}
                      >
                        Create account
                      </Button>
                    ) : null}
                  </Stack>
                </Stack>
              </Box>
              <Stack direction="row" spacing={1} sx={{ flexWrap: "wrap" }}>
                <Button
                  variant="text"
                  onClick={() => {
                    setForgotOpen((open) => !open);
                    setForgotSent(false);
                    if (!forgotEmail) {
                      setForgotEmail(email);
                    }
                  }}
                >
                  Forgot password?
                </Button>
              </Stack>
              {forgotOpen ? (
                <Box
                  component="form"
                  noValidate
                  onSubmit={(event) => {
                    event.preventDefault();
                    void requestReset();
                  }}
                  sx={{ p: 2, borderRadius: 2, border: "1px solid rgba(255,255,255,0.12)" }}
                >
                  <Stack spacing={1.5}>
                    <Typography variant="subtitle2">Reset your password</Typography>
                    {forgotSent ? (
                      <Alert severity="success">
                        If an account exists for that email, we have sent a link to reset the password.
                      </Alert>
                    ) : (
                      <>
                        <TextField
                          label="Email"
                          type="email"
                          value={forgotEmail}
                          onChange={(event) => setForgotEmail(event.target.value)}
                          autoComplete="email"
                          required
                          fullWidth
                          size="small"
                          slotProps={{ htmlInput: { "data-testid": "forgot-password-email-input" } }}
                        />
                        <Button
                          type="submit"
                          variant="outlined"
                          disabled={forgotSubmitting || !forgotEmail.trim()}
                          data-testid="forgot-password-button"
                        >
                          Send reset link
                        </Button>
                      </>
                    )}
                  </Stack>
                </Box>
              ) : null}
              {twoFactorRequired ? (
                <Stack spacing={1.5} sx={{ pt: 1 }}>
                  <Typography variant="subtitle2">Two-factor verification</Typography>
                  <Stack direction="row" spacing={1}>
                    <Button
                      variant={twoFactorMethod === "totp" ? "contained" : "outlined"}
                      onClick={() => setTwoFactorMethod("totp")}
                    >
                      Authenticator code
                    </Button>
                    <Button
                      variant={twoFactorMethod === "backup" ? "contained" : "outlined"}
                      onClick={() => setTwoFactorMethod("backup")}
                    >
                      Backup code
                    </Button>
                  </Stack>
                  <input
                    data-testid="two-factor-code-input"
                    value={twoFactorCode}
                    onChange={(event) => setTwoFactorCode(event.target.value)}
                    placeholder={twoFactorMethod === "totp" ? "123456" : "Backup code"}
                    style={inputStyle}
                  />
                  <Button
                    variant="outlined"
                    disabled={submitting === "verify"}
                    onClick={() => void handleVerifyTwoFactor()}
                  >
                    Verify code
                  </Button>
                </Stack>
              ) : null}
            </Stack>
          </CardContent>
        </Card>
      </Stack>
    </Box>
  );
}
