import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Paper,
  Stack,
  TextField,
  Typography,
} from "@mui/material";
import { useState } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { authClient } from "../../lib/auth-client";
import { useWorkspace } from "../../hooks/use-workspace-context";

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
  const [searchParams] = useSearchParams();
  // Destination preserved by RequireSession (e.g. an invitation link); only in-app paths are honoured.
  const nextParam = searchParams.get("next");
  const afterAuthPath = nextParam?.startsWith("/app/") ? nextParam : "/app/select-organization";
  const {
    message,
    refreshOrganizations,
    refreshSessionState,
    session,
    sessionPending,
    setMessage,
    signIn,
    signUp,
    submitting,
  } = useWorkspace();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [twoFactorCode, setTwoFactorCode] = useState("");
  const [twoFactorMethod, setTwoFactorMethod] = useState<"totp" | "backup">("totp");
  const [twoFactorRequired, setTwoFactorRequired] = useState(false);
  const [forgotOpen, setForgotOpen] = useState(searchParams.get("forgot") === "1");
  const [forgotEmail, setForgotEmail] = useState("");
  const [forgotSubmitting, setForgotSubmitting] = useState(false);
  const [forgotSent, setForgotSent] = useState(false);
  const resetDone = searchParams.get("reset") === "done";

  const requestReset = async () => {
    setForgotSubmitting(true);
    setMessage(null);
    const result = await authClient.requestPasswordReset({ email: forgotEmail.trim(), redirectTo: "/reset-password" });
    setForgotSubmitting(false);
    if (result.error) {
      setMessage({ severity: "error", text: result.error.message ?? "Unable to send a reset link right now." });
      return;
    }
    setForgotSent(true);
  };

  if (sessionPending) {
    return (
      <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <CircularProgress />
      </Box>
    );
  }

  if (session) {
    return <Navigate to={afterAuthPath} replace />;
  }

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
            Manage links by organization and team, update destinations safely, and track every click through a single
            Cloudflare-native control plane.
          </Typography>
          <Stack direction={{ xs: "column", sm: "row" }} spacing={2} sx={{ mt: 4 }}>
            <Chip label="Cloudflare Workers" />
            <Chip label="Better Auth organizations + teams" />
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
                  void signIn({ email, password }).then((result) => {
                    setTwoFactorRequired(Boolean(result.requiresTwoFactor));
                    if (result.ok) {
                      void navigate(afterAuthPath);
                    }
                  });
                }}
              >
                <Stack spacing={2}>
                  <TextField
                    label="Name"
                    placeholder="Your name (only needed to create an account)"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    autoComplete="name"
                    fullWidth
                    slotProps={{ htmlInput: { "data-testid": "auth-name-input" } }}
                  />
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
                    <Button
                      type="button"
                      variant="outlined"
                      disabled={submitting === "signup" || !email || !password || !name.trim()}
                      data-testid="create-account-button"
                      onClick={() =>
                        void signUp({ name: name.trim(), email, password }).then(
                          (ok: boolean) => ok && navigate(afterAuthPath),
                        )
                      }
                    >
                      Create account
                    </Button>
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
              <Button
                variant="text"
                onClick={() =>
                  void authClient.signIn.passkey({ autoFill: false }).then(async (result) => {
                    if (result.error) {
                      return;
                    }
                    await refreshSessionState();
                    await refreshOrganizations({ silent: true });
                    void navigate(afterAuthPath);
                  })
                }
              >
                Sign in with passkey
              </Button>
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
                    onClick={() =>
                      void (
                        twoFactorMethod === "totp"
                          ? authClient.twoFactor.verifyTotp({ code: twoFactorCode })
                          : authClient.twoFactor.verifyBackupCode({ code: twoFactorCode })
                      ).then(async (result) => {
                        if (result.error) {
                          return;
                        }
                        setTwoFactorRequired(false);
                        setTwoFactorCode("");
                        await refreshSessionState();
                        await refreshOrganizations({ silent: true });
                        void navigate(afterAuthPath);
                      })
                    }
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
